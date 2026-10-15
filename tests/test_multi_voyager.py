"""Catch lost migration identity, incorrect rule packaging, and node credential mixing."""
import copy
import io
import json
import pathlib
import sys
import tempfile
import unittest
import zipfile

sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]/'scripts'))
from deployment.config import initialize, read_bundle, replace_private, environment
from deployment import config as config_module
from deployment.artifact import package_binary

class MultiVoyagerTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.path=pathlib.Path(self.tmp.name)/'config.json';initialize(self.path)

    def test_migration_preserves_identity_and_backup_and_add_is_idempotent(self):
        before=self.path.read_bytes();old=read_bundle(self.path)
        self.assertTrue(hasattr(config_module,'migrate'), 'explicit migration is missing')
        config_module.migrate(self.path)
        data=read_bundle(self.path)
        self.assertEqual(data['version'],3)
        self.assertEqual(data['credentials']['client_password'],old['credentials']['client_password'])
        self.assertEqual(data['credentials']['voyagers']['hongkong']['tunnel_token'],old['credentials']['tunnel_token'])
        self.assertEqual(data['deployment']['voyagers']['hongkong']['function_name'],'bigfoot-voyager')
        self.assertEqual((self.path.parent/'config.json.v2.bak').read_bytes(),before)
        config_module.add_voyager(self.path,'tokyo','ap-northeast-1')
        first=self.path.read_bytes();config_module.add_voyager(self.path,'tokyo','ap-northeast-1')
        self.assertEqual(self.path.read_bytes(),first)
        data=read_bundle(self.path)
        self.assertNotEqual(data['credentials']['voyagers']['tokyo'],data['credentials']['voyagers']['hongkong'])

    def test_same_identity_with_different_pem_whitespace_is_rejected(self):
        config_module.migrate(self.path);config_module.add_voyager(self.path,'tokyo','ap-northeast-1')
        data=read_bundle(self.path)
        for field in ('voyager_identity_ca_pem','voyager_identity_key_pem'):
            data['credentials']['voyagers']['tokyo'][field]=data['credentials']['voyagers']['hongkong'][field]+'\n'
        replace_private(self.path,data)
        with self.assertRaisesRegex(ValueError,'independent'):read_bundle(self.path)

    def test_migration_refuses_edit_between_read_and_snapshot(self):
        from unittest.mock import patch
        original_read=config_module.read_bundle
        def concurrent_edit(path):
            data=original_read(path)
            changed=copy.deepcopy(data);changed['runtime']['idle_timeout']='3m'
            replace_private(path,changed)
            return data
        with patch.object(config_module,'read_bundle',side_effect=concurrent_edit):
            with self.assertRaisesRegex(ValueError,'changed'):config_module.migrate(self.path)
        self.assertEqual(original_read(self.path)['runtime']['idle_timeout'],'3m')
        self.assertEqual(original_read(self.path)['version'],2)

    def test_rule_files_are_packaged_without_credentials_and_gate_has_no_private_keys(self):
        self.assertTrue(hasattr(config_module,'migrate'), 'multi-node configuration is missing')
        config_module.migrate(self.path);config_module.add_voyager(self.path,'tokyo','ap-northeast-1')
        data=read_bundle(self.path)
        for name,node in data['deployment']['voyagers'].items(): node['domain']=name+'.example.com'
        rules=self.path.parent/'rules';rules.mkdir();(rules/'jp.txt').write_text('# list\nDOMAIN-SUFFIX,example.jp\n')
        data['routing']['rules']=[{'rule_set':'rules/jp.txt','voyager':'tokyo'}]
        replace_private(self.path,data)
        routing=config_module.load_routing(self.path,data)
        self.assertEqual(routing['rules'][0]['lines'],['# list','DOMAIN-SUFFIX,example.jp'])
        gate=environment(data,'gate');jp=environment(data,'voyager','tokyo')
        for secret in data['credentials']['voyagers'].values():
            self.assertNotIn(secret['voyager_identity_key_pem'],json.dumps(gate))
            self.assertNotIn(secret['tunnel_token'],json.dumps(routing))
            self.assertNotIn(secret['voyager_identity_key_pem'],json.dumps(routing))
        self.assertEqual(jp['BIGFOOT_TOKEN'],data['credentials']['voyagers']['tokyo']['tunnel_token'])
        blob=package_binary(b'ELF',routing)
        with zipfile.ZipFile(io.BytesIO(blob)) as archive:
            self.assertEqual(json.loads(archive.read('routing.json')),routing)
            self.assertNotIn('PRIVATE KEY',archive.read('routing.json').decode())

    def test_invalid_rule_path_or_syntax_is_rejected_before_deploy(self):
        self.assertTrue(hasattr(config_module,'migrate'), 'rule loading is missing')
        config_module.migrate(self.path);data=read_bundle(self.path)
        data['deployment']['voyagers']['hongkong']['domain']='hk.example.com'
        for filename,content in [('bad.txt','DOMAIN-SUFFIX,bad..com'),('bad.txt','UNKNOWN,example.com'),('../escape','DOMAIN,x.com')]:
            (self.path.parent/'bad.txt').write_text(content)
            data['routing']['rules']=[{'rule_set':filename,'voyager':'hongkong'}]
            with self.assertRaises(ValueError):config_module.load_routing(self.path,data)
        (self.path.parent/'link.txt').symlink_to(self.path.parent/'bad.txt')
        data['routing']['rules']=[{'rule_set':'link.txt','voyager':'hongkong'}]
        with self.assertRaises(ValueError):config_module.load_routing(self.path,data)


class MultiWorkflowTests(unittest.TestCase):
    setUp=MultiVoyagerTests.setUp
    def test_deploy_each_node_and_gate_routing_uses_discovered_addresses(self):
        from deployment.workflow import deploy_package
        config_module.migrate(self.path)
        config_module.add_voyager(self.path,'tokyo','ap-northeast-1')
        data=read_bundle(self.path);submitted=[]
        class Provider:
            def apply(self,role,env,artifact,stage,*,function_name=None):
                with zipfile.ZipFile(io.BytesIO(artifact)) as archive:
                    if role=='gate':
                        route=json.loads(archive.read('routing.json'))
                        if route['nodes']['tokyo']['url']!='wss://bigfoot-voyager-tokyo.example.com/tunnel':
                            raise AssertionError('Gate received placeholder or stale address')
                submitted.append((role,function_name))
                return function_name+'.example.com'
        build=lambda data,roles,routing:package_binary(b'ELF',routing)
        deploy_package(self.path,data,'all',build=build,provider_factory=lambda _:Provider(),health=lambda _:None)
        self.assertEqual(submitted,[('voyager','bigfoot-voyager'),('voyager','bigfoot-voyager-tokyo'),('gate','bigfoot-gate')])
        saved=read_bundle(self.path)
        self.assertEqual(saved['deployment']['voyagers']['tokyo']['domain'],'bigfoot-voyager-tokyo.example.com')
        journal=json.loads((self.path.parent/'generated/deployment.json').read_text())
        self.assertEqual(journal['completed'],['voyager:hongkong','voyager:tokyo','gate'])
        self.assertFalse(journal['link_verified'])
        submitted.clear()
        deploy_package(self.path,saved,'voyager',voyager='tokyo',build=build,provider_factory=lambda _:Provider(),health=lambda _:None)
        self.assertEqual(submitted,[('voyager','bigfoot-voyager-tokyo')])

class RouteVerificationTests(unittest.TestCase):
    def test_probe_failure_restores_intended_gate_package(self):
        from deployment.verification import check_all_routes
        route={'default':'hongkong','nodes':{'hongkong':{'url':'wss://hk.example/tunnel'},'tokyo':{'url':'wss://jp.example/tunnel'}},'rules':[{'source':'jp.txt','voyager':'tokyo','lines':['DOMAIN-SUFFIX,example.jp']}]}
        data={'version':3,'deployment':{'gate':{'domain':'gate.example'}},'routing':{'default':'hongkong'}}
        blob=package_binary(b'ELF',route);updates=[];probes=[]
        class Provider:
            def apply(self,role,env,artifact,stage,**kwargs):
                with zipfile.ZipFile(io.BytesIO(artifact)) as archive:updates.append(json.loads(archive.read('routing.json')))
                return 'gate.example'
        def probe(*_):
            probes.append(True)
            if len(probes)==2:raise ValueError('probe failed')
        with self.assertRaisesRegex(ValueError,'probe failed'):
            check_all_routes(pathlib.Path('unused'),data,'mihomo',Provider(),blob,{},health=lambda _:None,probe=probe)
        self.assertEqual(updates[-1],route)
        self.assertEqual(updates[0]['nodes'],{'hongkong':route['nodes']['hongkong']})
        self.assertEqual(updates[1]['default'],'tokyo')
        self.assertEqual(updates[1]['rules'],[])

    def test_restore_attempted_even_if_journal_write_fails(self):
        from deployment.verification import check_all_routes
        route={'default':'hongkong','nodes':{'hongkong':{'url':'wss://hk.example/tunnel'}},'rules':[]}
        blob=package_binary(b'ELF',route);updates=[]
        class Provider:
            def apply(self,role,env,artifact,stage,**kwargs):
                updates.append(artifact);return 'gate.example'
        def stage(phase):
            if phase=='restore-policy':raise OSError('journal unavailable')
        data={'deployment':{'gate':{'domain':'gate.example'}}}
        with self.assertRaisesRegex(OSError,'journal unavailable'):
            check_all_routes(pathlib.Path('unused'),data,'mihomo',Provider(),blob,{},health=lambda _:None,probe=lambda *_:None,stage=stage)
        self.assertEqual(len(updates),2)
        self.assertEqual(updates[-1],blob)


if __name__=='__main__':unittest.main()
