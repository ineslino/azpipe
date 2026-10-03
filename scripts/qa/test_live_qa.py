import argparse, copy, json, os, pathlib, sys, tempfile, time, unittest
from unittest.mock import patch
import live_qa

ROOT = pathlib.Path(__file__).with_name('fixtures')

class QAConfinement(unittest.TestCase):
    def setUp(self):
        self.manifest = json.loads((ROOT/'manifest.example.json').read_text())
        self.args = argparse.Namespace(expected_identity='fixture@example.test', adapter='fixture', profile='fixture')
    def azure(self, args, *command):
        if command == ('whoami',):
            return {'authenticatedUser': {'properties': {'Account': {'$value':'fixture@example.test'}}}}
        return {'name':'AZPIPE-QA', 'visibility':'private'}
    def read(self, args, manifest, area, resource, route=(), query=()):
        if resource == 'repositories':
            return {'name':'azpipe-qa', 'project':{'id':manifest['projectId']}}
        if resource == 'refs':
            return {'value':[{'name':'refs/heads/'+manifest['branch'], 'objectId':manifest['commit']}]}
        if resource == 'definitions':
            row = next(p for p in manifest['pipelines'] if str(p['id']) == route[0].split('=')[1])
            return {'repository':{'id':manifest['repositoryId']}, 'revision':row['revision'],
                    'process':{'yamlFilename':'qa/'+row['fixture']}}
        fixture = next(q.split('=')[1].split('/')[-1] for q in query if q.startswith('path='))
        return {'content':(ROOT/fixture).read_text()}
    def test_reviewed_fixtures_pass_preflight(self):
        with patch.object(live_qa,'azure',self.azure), patch.object(live_qa,'read',self.read):
            live_qa.preflight(self.args,self.manifest)
    def test_targets_and_protected_branches_are_rejected_before_reads(self):
        for key,value in [('project','production'),('repository','production'),('branch','main'),
                          ('organization','https://dev.azure.com/example-org?redirect=other'),
                          ('organization','https://other.example/example-org')]:
            manifest = copy.deepcopy(self.manifest); manifest[key] = value
            with patch.object(live_qa,'azure') as azure, self.assertRaises(ValueError):
                live_qa.preflight(self.args,manifest)
            azure.assert_not_called()
    def test_changed_branch_is_rejected(self):
        def changed(*args, **kwargs):
            data = self.read(*args, **kwargs)
            if args[3] == 'refs': data['value'][0]['objectId'] = 'a'*40
            return data
        with patch.object(live_qa,'azure',self.azure), patch.object(live_qa,'read',changed), self.assertRaises(ValueError):
            live_qa.preflight(self.args,self.manifest)
    def test_changed_yaml_and_definition_are_rejected(self):
        for resource in ('items','definitions'):
            def changed(*args, **kwargs):
                data = self.read(*args, **kwargs)
                if args[3] == resource:
                    if resource == 'items': data['content'] += '\n# changed\n'
                    else: data['revision'] += 1
                return data
            with patch.object(live_qa,'azure',self.azure), patch.object(live_qa,'read',changed), self.assertRaises(ValueError):
                live_qa.preflight(self.args,self.manifest)

    def test_identity_and_public_project_are_rejected(self):
        for bad in ('identity','visibility'):
            def changed(args,*command):
                data = self.azure(args,*command)
                if bad == 'identity' and command == ('whoami',):
                    data['authenticatedUser']['properties']['Account']['$value'] = 'wrong@example.test'
                if bad == 'visibility' and command != ('whoami',): data['visibility'] = 'public'
                return data
            with patch.object(live_qa,'azure',changed), self.assertRaises(ValueError):
                live_qa.preflight(self.args,self.manifest)


class QAProcessCleanup(unittest.TestCase):
    def test_timeout_stops_descendant(self):
        with tempfile.TemporaryDirectory() as directory:
            pidfile = pathlib.Path(directory)/'child.pid'
            child = 'import os,time,pathlib; pathlib.Path('+repr(str(pidfile))+').write_text(str(os.getpid())); time.sleep(30)'
            parent = 'import subprocess,sys,time; subprocess.Popen([sys.executable,"-c",'+repr(child)+']); time.sleep(30)'
            self.assertEqual(live_qa.run_product([sys.executable,'-c',parent],dict(os.environ),timeout=2),124)
            self.assertTrue(pidfile.exists(),'synthetic child did not start')
            pid = int(pidfile.read_text())
            def alive():
                if os.name == 'nt':
                    import ctypes
                    api = ctypes.WinDLL('kernel32',use_last_error=True)
                    api.OpenProcess.argtypes = [ctypes.c_ulong,ctypes.c_int,ctypes.c_ulong]
                    api.OpenProcess.restype = ctypes.c_void_p
                    api.GetExitCodeProcess.argtypes = [ctypes.c_void_p,ctypes.POINTER(ctypes.c_ulong)]
                    api.CloseHandle.argtypes = [ctypes.c_void_p]
                    handle = api.OpenProcess(0x1000,False,pid)
                    if not handle: return False
                    try:
                        code = ctypes.c_ulong()
                        return bool(api.GetExitCodeProcess(handle,ctypes.byref(code)) and code.value == 259)
                    finally: api.CloseHandle(handle)
                try: os.kill(pid,0); return True
                except ProcessLookupError: return False
            deadline = time.monotonic()+5
            while alive() and time.monotonic()<deadline: time.sleep(.05)
            self.assertFalse(alive(),'QA descendant survived timeout')

if __name__ == '__main__': unittest.main()
