import json
import pathlib
import sys
import tempfile
import unittest
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]/'scripts'))
from deployment.config import initialize, read_bundle
from deployment.workflow import deploy_package, deployment_lock

class WorkflowTests(unittest.TestCase):
    def test_partial_failure_and_reconcile_on_retry(self):
        for failure in ('function','readback','trigger','health'):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as d:
                path=pathlib.Path(d)/'config.json';initialize(path)
                data=read_bundle(path);creds=data['credentials'].copy();calls=[]
                class Provider:
                    def apply(self,role,env,artifact,stage):
                        calls.append(role)
                        for phase in ('function','readback','trigger'):
                            stage(phase)
                            if role=='gate' and failure==phase:raise RuntimeError('SECRET-SDK-BODY')
                        return role+'.example.com'
                def health(host):
                    if host.startswith('gate.') and failure=='health':raise RuntimeError('SECRET-SDK-BODY')
                with self.assertRaises(ValueError) as error:
                    deploy_package(path,data,'all',build=lambda *_:b'artifact',provider_factory=lambda _:Provider(),health=health)
                self.assertNotIn('SECRET',str(error.exception))
                journal=json.loads((path.parent/'generated/deployment.json').read_text())
                self.assertEqual(journal['status'],'failed')
                self.assertEqual(journal['role'],'gate')
                self.assertEqual(read_bundle(path)['deployment']['voyager']['domain'],'voyager.example.com')
                self.assertNotIn('SECRET',(path.parent/'generated/deployment.json').read_text())
                failure=None
                deploy_package(path,read_bundle(path),'all',build=lambda *_:b'artifact',provider_factory=lambda _:Provider(),health=health)
                self.assertEqual(calls,['voyager','gate','voyager','gate'])
                self.assertEqual(read_bundle(path)['credentials'],creds)
                journal=json.loads((path.parent/'generated/deployment.json').read_text())
                self.assertEqual(journal['status'],'healthy')
                self.assertFalse(journal['link_verified'])

    def test_voyager_failure_stops_gate_and_link_failure_is_not_success(self):
        with tempfile.TemporaryDirectory() as d:
            path=pathlib.Path(d)/'config.json';initialize(path)
            data=read_bundle(path);calls=[]
            class Provider:
                def apply(self,role,env,artifact,stage):
                    calls.append(role);return role+'.example.com'
            def fail(*_):raise RuntimeError('private failure detail')
            with self.assertRaises(ValueError):
                deploy_package(path,data,'all',build=lambda *_:b'zip',provider_factory=lambda _:Provider(),health=fail)
            self.assertEqual(calls,['voyager'])
            with self.assertRaises(ValueError):
                deploy_package(path,read_bundle(path),'all',build=lambda *_:b'zip',provider_factory=lambda _:Provider(),health=lambda _:None,mihomo='fake',link=fail)
            journal=json.loads((path.parent/'generated/deployment.json').read_text())
            self.assertEqual(journal['stage'],'link')
            self.assertEqual(journal['status'],'failed')
            self.assertFalse(journal['link_verified'])

    def test_local_diagnostic_preserved_without_logging_sdk_body(self):
        from deployment.errors import UserError
        with tempfile.TemporaryDirectory() as d:
            path=pathlib.Path(d)/'config.json';initialize(path)
            def missing_go(*_):raise UserError('Install Go 1.25+ in PATH')
            with self.assertRaisesRegex(ValueError,'Install Go 1.25'):
                deploy_package(path,read_bundle(path),'all',build=missing_go,provider_factory=lambda _:None)

    def test_lock_rejects_parallel_deployment(self):
        with tempfile.TemporaryDirectory() as d:
            path=pathlib.Path(d)/'config.json'
            with deployment_lock(path):
                with self.assertRaises(ValueError):
                    with deployment_lock(path):pass
            with deployment_lock(path):pass
