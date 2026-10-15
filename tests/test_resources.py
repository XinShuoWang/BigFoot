"""Catch mismatched FC and application concurrency or silently ignored sizing."""
import copy
import pathlib
import sys
import tempfile
import unittest

sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]/'scripts'))
from deployment import config
from deployment.aliyun import AliyunFC
from test_aliyun_adapter import FakeFC,Models

class ResourceTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.path=pathlib.Path(self.tmp.name)/'config.json'
        config.initialize(self.path,version=3)
        self.data=config.read_bundle(self.path)

    def test_requested_resources_reach_create_and_update(self):
        client=FakeFC();adapter=AliyunFC(client,Models())
        resources={'cpu':0.05,'memory_mb':128,'instance_concurrency':200}
        adapter.apply('voyager',{},b'zip',lambda _:None,resources=resources)
        self.assertEqual(client.function.cpu,0.05)
        self.assertEqual(client.function.memory_size,128)
        self.assertEqual(client.function.instance_concurrency,200)
        adapter.apply('gate',{},b'zip',lambda _:None,resources=resources)
        self.assertEqual(client.function.cpu,0.05)
        self.assertEqual(client.function.memory_size,128)
        self.assertEqual(client.function.instance_concurrency,200)

    def test_new_config_aligns_application_with_cloud_limit(self):
        self.assertEqual(self.data['deployment'].get('resources'),{'cpu':0.05,'memory_mb':128,'instance_concurrency':200})
        self.assertEqual(self.data['runtime']['max_connections'],200)
        for role in ('gate','voyager'):
            self.assertEqual(config.environment(self.data,role)['BIGFOOT_MAX_CONNECTIONS'],'200')
        self.data['runtime']['max_connections']=201
        config.replace_private(self.path,self.data)
        with self.assertRaisesRegex(ValueError,'max_connections'):config.read_bundle(self.path)

    def test_invalid_specs_are_rejected_before_cloud_access(self):
        baseline={'cpu':0.05,'memory_mb':128,'instance_concurrency':200}
        for field,value in [('cpu',0),('cpu',0.06),('cpu',True),('cpu',float('nan')),('memory_mb',127),('memory_mb',129),('memory_mb',True),('instance_concurrency',201),('instance_concurrency',True),('cpu',1)]:
            data=copy.deepcopy(self.data);data['deployment']['resources']=dict(baseline,**{field:value})
            config.replace_private(self.path,data)
            with self.subTest(field=field,value=value), self.assertRaises(ValueError):config.read_bundle(self.path)

    def test_legacy_missing_resources_preserves_previous_specs(self):
        self.assertTrue(hasattr(config,'deployment_resources'),'resource configuration is missing')
        self.data['deployment'].pop('resources',None)
        self.data['runtime']['max_connections']=128
        self.assertEqual(config.deployment_resources(self.data),{'cpu':1,'memory_mb':1024,'instance_concurrency':128})


    def test_deployment_and_route_probes_preserve_resources(self):
        from deployment.workflow import deploy_package
        from deployment.verification import check_all_routes
        from deployment.artifact import package_binary
        calls=[]
        data=self.data
        class Provider:
            def apply(self,role,env,artifact,stage,**options):
                client=FakeFC()
                host=AliyunFC(client,Models()).apply(role,env,artifact,stage,**options)
                calls.append((role,client.function.cpu,client.function.memory_size,client.function.instance_concurrency,env['BIGFOOT_MAX_CONNECTIONS']))
                return host
        artifacts=[]
        def build(data,roles,routing):
            blob=package_binary(b'ELF',routing);artifacts.append(blob);return blob
        deploy_package(self.path,data,'all',build=build,provider_factory=lambda _:Provider(),health=lambda _:None)
        self.assertEqual(calls,[('voyager',0.05,128,200,'200'),('gate',0.05,128,200,'200')])
        calls.clear()
        check_all_routes(self.path,data,'mihomo',Provider(),artifacts[0],config.environment(data,'gate'),health=lambda _:None,probe=lambda *_:None)
        self.assertEqual(calls,[('gate',0.05,128,200,'200'),('gate',0.05,128,200,'200')])
