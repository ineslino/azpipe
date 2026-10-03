#!/usr/bin/env python3
"""Confined Azure QA preflight and optional batch execution. Never provisions resources."""
import argparse, json, os, pathlib, re, signal, subprocess, tempfile, urllib.parse


def run_product(command, env, timeout=360):
    options = {'start_new_session': True} if os.name != 'nt' else {'creationflags': subprocess.CREATE_NEW_PROCESS_GROUP}
    process = subprocess.Popen(command, env=env, **options)
    try:
        return process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        try:
            if os.name == 'nt':
                stopped = subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                                         capture_output=True, timeout=10)
                if stopped.returncode and process.poll() is None:
                    raise RuntimeError('QA process tree cleanup failed; inspect saved IDs before any retry')
            else:
                try: os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError: pass
        finally:
            if process.poll() is None: process.kill()
            process.wait()
        print('QA timeout: local processes stopped; inspect saved/uncertain run IDs remotely before retrying', flush=True)
        return 124


def azure(args, *command):
    result = subprocess.run([args.adapter, args.profile, *command], capture_output=True, text=True, timeout=45)
    if result.returncode:
        raise RuntimeError('Azure QA request refused; check the approved session and permissions')
    return json.loads(result.stdout)


def read(args, manifest, area, resource, route=(), query=()):
    command = ['devops', 'invoke', '--organization', manifest['organization'], '--detect', 'false', '--area', area,
               '--resource', resource, '--api-version', '7.1', '--http-method', 'GET', '--output', 'json', '--only-show-errors',
               '--route-parameters', 'project='+manifest['projectId'], *route]
    if query: command += ['--query-parameters', *query]
    return azure(args, *command)


def preflight(args, manifest):
    url = urllib.parse.urlparse(manifest['organization'])
    if (url.scheme != 'https' or url.hostname != 'dev.azure.com' or url.username or url.password or url.port
            or url.query or url.fragment or not re.fullmatch(r'/[A-Za-z0-9_-]+', url.path)):
        raise ValueError('QA organization must be https://dev.azure.com/<organization>')
    if manifest['project'] != 'AZPIPE-QA' or manifest['repository'] != 'azpipe-qa':
        raise ValueError('Only AZPIPE-QA / azpipe-qa is permitted')
    if not re.fullmatch(r'azpipe-qa/[A-Za-z0-9_-]+', manifest['branch']) or not re.fullmatch(r'[a-fA-F0-9]{40}', manifest['commit']):
        raise ValueError('A disposable azpipe-qa/ branch and a full commit are required')
    identity = azure(args, 'whoami')['authenticatedUser']['properties']['Account']['$value']
    if identity.casefold() != args.expected_identity.casefold():
        raise ValueError('Azure identity differs from the authorised identity')
    project = azure(args, 'devops', 'project', 'show', '--project', manifest['projectId'], '--organization', manifest['organization'],
                    '--detect', 'false', '--output', 'json', '--only-show-errors')
    if project['name'] != 'AZPIPE-QA' or project['visibility'].casefold() != 'private':
        raise ValueError('QA project identity or visibility differs from the manifest')
    repo = read(args, manifest, 'git', 'repositories', ['repositoryId='+manifest['repositoryId']])
    if repo['name'] != 'azpipe-qa' or repo['project']['id'].casefold() != manifest['projectId'].casefold():
        raise ValueError('Repository is outside the QA project')
    refs = read(args, manifest, 'git', 'refs', ['repositoryId='+manifest['repositoryId']], ['filter=heads/'+manifest['branch']])['value']
    matches = [r for r in refs if r['name'] == 'refs/heads/'+manifest['branch']]
    if len(matches) != 1 or matches[0]['objectId'].casefold() != manifest['commit'].casefold():
        raise ValueError('QA branch changed; rebuild and review the manifest')
    if len(manifest['pipelines']) != 2 or {p['fixture'] for p in manifest['pipelines']} != {'parameters.yml', 'plan.yml'}:
        raise ValueError('Exactly the two committed QA fixtures are required')
    seen = set()
    for pipeline in manifest['pipelines']:
        if type(pipeline['id']) is not int or pipeline['id'] <= 0 or pipeline['id'] in seen:
            raise ValueError('Invalid or duplicate QA pipeline ID')
        seen.add(pipeline['id'])
        definition = read(args, manifest, 'build', 'definitions', ['definitionId='+str(pipeline['id'])])
        if (definition['repository']['id'].casefold() != manifest['repositoryId'].casefold()
                or definition['revision'] != pipeline['revision']
                or definition['process']['yamlFilename'].lstrip('/') != 'qa/'+pipeline['fixture']):
            raise ValueError('Pipeline definition changed or is outside the QA repository')
        item = read(args, manifest, 'git', 'items', ['repositoryId='+manifest['repositoryId']],
                    ['path=/qa/'+pipeline['fixture'], 'includeContent=true', 'versionDescriptor.versionType=commit',
                     'versionDescriptor.version='+manifest['commit']])
        expected = pathlib.Path(__file__).with_name('fixtures').joinpath(pipeline['fixture']).read_text()
        if item['content'].replace('\r\n', '\n') != expected:
            raise ValueError('Remote YAML differs from the reviewed QA fixture')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', required=True)
    parser.add_argument('--adapter', required=True)
    parser.add_argument('--profile', required=True)
    parser.add_argument('--expected-identity', required=True)
    parser.add_argument('--binary', required=True)
    parser.add_argument('--execute', action='store_true', help='Queue only the two confined QA fixtures')
    args = parser.parse_args()
    manifest = json.loads(pathlib.Path(args.manifest).read_text())
    preflight(args, manifest)
    plan = next(p for p in manifest['pipelines'] if p['fixture'] == 'plan.yml')
    # Both RUN and PLAN contracts pin the reviewed SHA/revision. If a definition
    # or branch changes after preflight, the product refuses the stale contract.
    contracts = [dict(organization=manifest['organization'], project='AZPIPE-QA', pipelineId=p['id'],
                    definitionVersion=p['revision'], commit=manifest['commit'], parameter='planOnly', type='boolean',
                    planValue='true', runValue='false', evidence='Reviewed scripts/qa/fixtures/'+p['fixture']+'; QA jobs only')
                 for p in manifest['pipelines']]
    selection = [dict(id=p['id'], mode='PLAN' if p is plan else 'RUN', branch=manifest['branch'],
                      parameters={} if p is plan else {'outcome':'success'}) for p in manifest['pipelines']]
    # The directory is retained for accepted IDs and uncertain submissions. Never retry automatically.
    directory = pathlib.Path(tempfile.mkdtemp(prefix='azpipe-live-qa-'))
    for name, data in (('contracts.json', contracts), ('selection.json', selection)):
        with os.fdopen(os.open(directory/name, os.O_WRONLY|os.O_CREAT|os.O_EXCL, 0o600), 'w') as output:
            json.dump(data, output)
    env = dict(os.environ, AZDO_PAT='', AZPIPE_AZDO_AS=args.adapter, AZPIPE_AUTH_PROFILE=args.profile,
               AZPIPE_EXPECTED_IDENTITY=args.expected_identity, AZPIPE_CONTRACTS=str(directory/'contracts.json'))
    command = [str(pathlib.Path(args.binary).resolve()), 'batch', '--org', manifest['organization'], '--project', 'AZPIPE-QA',
               '--file', str(directory/'selection.json')]
    if args.execute: command += ['--execute', '--journal', str(directory/'runs.json')]
    print('QA preflight PASS; evidence directory:', directory, flush=True)
    return run_product(command, env)


if __name__ == '__main__':
    raise SystemExit(main())
