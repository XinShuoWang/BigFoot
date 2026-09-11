import json
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT=pathlib.Path(__file__).resolve().parents[1]

class BundleTests(unittest.TestCase):
    def invoke(self, config, *args):
        return subprocess.run([sys.executable,str(ROOT/'scripts/bigfoot.py'),'--config',str(config),*args],capture_output=True,text=True)

    def test_initialize_once_and_render_separated_credentials(self):
        with tempfile.TemporaryDirectory() as d:
            config=pathlib.Path(d)/'config.json'
            result=self.invoke(config,'init')
            self.assertEqual(result.returncode,0,result.stderr)
            before=config.read_bytes()
            result=self.invoke(config,'init')
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertEqual(config.read_bytes(),before)
            data=json.loads(before)
            self.assertNotEqual(data['credentials']['tunnel_token'],data['credentials']['client_password'])
            self.assertEqual(config.stat().st_mode&0o777,0o600)
            for role in ('gate','voyager'):
                data['deployment'][role].update(domain=f'{role}.example.com')
            config.write_text(json.dumps(data))
            result=self.invoke(config,'render')
            self.assertEqual(result.returncode,0,result.stderr)
            sys.path.insert(0,str(ROOT/'scripts'))
            from deployment.config import environment
            for role in ('gate','voyager'):
                env=environment(data,role)
                self.assertEqual('BIGFOOT_INNER_IDENTITY_KEY_PEM' in env,role=='voyager')
                self.assertEqual('BIGFOOT_CLIENT_PASSWORD' in env,role=='gate')
            target=config.parent/'generated/client.yaml'
            self.assertEqual(target.stat().st_mode&0o777,0o600)
            rendered=json.loads(target.read_text())
            self.assertEqual(rendered['proxies'][0]['password'],data['credentials']['client_password'])
            self.assertNotIn(data['credentials']['tunnel_token'],target.read_text())
            # Regenerating derived files must not rotate saved credentials.
            self.assertEqual(self.invoke(config,'render').returncode,0)
            self.assertEqual(json.loads(config.read_text())['credentials'],data['credentials'])

    def test_default_code_package_render_needs_no_image(self):
        with tempfile.TemporaryDirectory() as d:
            config=pathlib.Path(d)/'config.json'
            self.assertEqual(self.invoke(config,'init').returncode,0)
            data=json.loads(config.read_text())
            self.assertNotIn('backend',data['deployment'])
            for role in ('gate','voyager'):
                self.assertNotIn('image',data['deployment'][role])
            data['deployment']['gate']['domain']='gate.example.com'
            config.write_text(json.dumps(data))
            result=self.invoke(config,'render')
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertTrue((config.parent/'generated/client.yaml').exists())
            result=self.invoke(config,'deploy','--backend','container')
            self.assertNotEqual(result.returncode,0)

    def test_invalid_duration_rejected_before_render_or_deploy(self):
        with tempfile.TemporaryDirectory() as d:
            config=pathlib.Path(d)/'config.json'
            self.assertEqual(self.invoke(config,'init').returncode,0)
            data=json.loads(config.read_text())
            for value in ('0s','typo','-1s'):
                data['runtime']['idle_timeout']=value
                config.write_text(json.dumps(data))
                result=self.invoke(config,'deploy')
                self.assertNotEqual(result.returncode,0)
                self.assertIn('idle_timeout',result.stderr)
                self.assertFalse((config.parent/'generated').exists())

    def test_corrupt_config_not_replaced(self):
        with tempfile.TemporaryDirectory() as d:
            config=pathlib.Path(d)/'config.json';config.write_text('{broken');config.chmod(0o600)
            result=self.invoke(config,'init')
            self.assertNotEqual(result.returncode,0)
            self.assertEqual(config.read_text(),'{broken')

if __name__=='__main__':unittest.main()
