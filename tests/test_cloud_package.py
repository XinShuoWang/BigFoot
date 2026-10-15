import io
import pathlib
import sys
import unittest
import zipfile
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]/'scripts'))
import fc_deploy

class PackageTests(unittest.TestCase):
    def test_archive_contains_only_executable(self):
        blob=fc_deploy.package_binary(b'ELF-test')
        with zipfile.ZipFile(io.BytesIO(blob)) as z:
            self.assertEqual(z.namelist(),['tunnel'])
            self.assertEqual(z.read('tunnel'),b'ELF-test')
            self.assertEqual((z.getinfo('tunnel').external_attr>>16)&0o777,0o755)

    def test_gateway_url_validation(self):
        self.assertEqual(fc_deploy.endpoint_host('https://abc.cn-shanghai.fcapp.run'),'abc.cn-shanghai.fcapp.run')
        for bad in ('http://example.com','https://example.com/path','https://user@example.com'):
            with self.assertRaises(ValueError):fc_deploy.endpoint_host(bad)

class ReadbackTests(unittest.TestCase):
    def test_stale_code_or_configuration_is_not_ready(self):
        from types import SimpleNamespace
        submitted=SimpleNamespace(code_checksum='new')
        expected={'runtime':'custom.debian11','environment_variables':{'mode':'voyager'}}
        stale=SimpleNamespace(code_checksum='old',**expected)
        self.assertFalse(fc_deploy.matches_submission(stale,submitted,expected))
        stale.code_checksum='new'
        stale.environment_variables={'mode':'gate'}
        self.assertFalse(fc_deploy.matches_submission(stale,submitted,expected))
        stale.environment_variables={'mode':'voyager'}
        self.assertTrue(fc_deploy.matches_submission(stale,submitted,expected))

class PlatformDefaultsTests(unittest.TestCase):
    def test_readback_accepts_only_unspecified_platform_health_check_default(self):
        from types import SimpleNamespace
        class Runtime:
            def __init__(self,values):self.values=values
            def to_map(self):return self.values
        submitted=SimpleNamespace(code_checksum='new')
        expected={'custom_runtime_config':Runtime({'command':['/code/tunnel'],'port':9000})}
        actual=SimpleNamespace(code_checksum='new',custom_runtime_config=Runtime({'command':['/code/tunnel'],'port':9000,'healthCheckConfig':{'initialDelaySeconds':0}}))
        self.assertTrue(fc_deploy.matches_submission(actual,submitted,expected))
        actual.custom_runtime_config.values['command']=['/code/stale']
        self.assertFalse(fc_deploy.matches_submission(actual,submitted,expected))
        actual.custom_runtime_config.values['command']=['/code/tunnel']
        actual.custom_runtime_config.values['args']=['-mode','local']
        self.assertFalse(fc_deploy.matches_submission(actual,submitted,expected))
        actual.custom_runtime_config.values.pop('args')
        expected['custom_runtime_config'].values['healthCheckConfig']={'initialDelaySeconds':5}
        self.assertFalse(fc_deploy.matches_submission(actual,submitted,expected))
