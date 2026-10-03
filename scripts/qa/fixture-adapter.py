#!/usr/bin/env python3
import json, os, sys, time

args = sys.argv[1:]
mode = os.environ.get("AZPIPE_FIXTURE_MODE")
schema_modes = ("long-options", "continuity", "slow-schema")
if args[1:] == ["whoami"]:
    if os.environ.get("AZPIPE_FIXTURE_MODE") == "identity-error":
        sys.exit(1)
    if os.environ.get("AZPIPE_FIXTURE_MODE") == "slow-identity":
        time.sleep(1.5)
    print(json.dumps({"authenticatedUser": {"properties": {"Account": {"$value": "fixture@example.test"}}}}))
    sys.exit(0)
def flag(name):
    return args[args.index(name) + 1]
if flag("--http-method") != "GET":
    sys.exit("fixture refuses mutations")
area, resource = flag("--area"), flag("--resource")
with open(os.environ["AZPIPE_QA_CALL_LOG"], "a") as output:
    output.write(area + "/" + resource + "\n")
if (area, resource) == ("core", "projects"):
    if os.environ.get("AZPIPE_FIXTURE_MODE") == "slow-projects":
        time.sleep(2)
    print(json.dumps({"value": [{"id": "id-%02d" % i, "name": "project-%02d" % i} for i in range(35)]}))
elif (area, resource) == ("build", "definitions"):
    if os.environ.get("AZPIPE_FIXTURE_MODE") == "slow-pipelines":
        time.sleep(2)
    if mode in schema_modes and any(a.startswith("definitionId=") for a in args):
        if mode == "slow-schema":
            time.sleep(2)
        print(json.dumps({"revision": 7, "repository": {"id": "fixture-repo", "type": "TfsGit"}, "process": {"yamlFilename": "/fixture.yml"}}))
    else:
        print(json.dumps({"value": [{"id": 202, "name": "fixture pipeline", "path": "\\fixture", "repository": {"name": "fixture-repo"}}]}))
elif mode in schema_modes and (area, resource) == ("git", "refs"):
    branch = next((a[len("filter="):] for a in args if a.startswith("filter=")), "heads/main")
    print(json.dumps({"value": [{"name": "refs/"+branch, "objectId": "a"*40}]}))
elif mode in schema_modes and (area, resource) == ("git", "items"):
    if mode == "long-options":
        prefix = "destino-"*40
        yaml = "parameters:\n- name: environment\n  displayName: Ambiente\n  type: string\n  default: "+prefix+"test\n  values: ["+prefix+"test, "+prefix+"prod]\n"
    else:
        yaml = "parameters:\n- name: label\n  displayName: Etiqueta\n  type: string\n  default: alpha beta\n"
    print(json.dumps({"content": yaml}))
else:
    sys.exit("unsupported fixture read")
