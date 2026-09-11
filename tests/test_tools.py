import json
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class ClientConfigTests(unittest.TestCase):
    def test_secure_client_config_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as temp:
            temp = pathlib.Path(temp)
            secret = temp / 'password.txt'
            secret.write_text('0123456789abcdef0123456789abcdef\n')
            output = temp / 'client.yaml'
            args = [sys.executable, str(ROOT/'scripts/client_config.py'), '--server', 'gate.example.com', '--password-file', str(secret), '--output', str(output)]
            subprocess.run(args, check=True, capture_output=True)
            config = json.loads(output.read_text())
            proxy = config['proxies'][0]
            self.assertEqual(proxy['server'], 'gate.example.com')
            self.assertEqual(proxy['network'], 'ws')
            self.assertFalse(proxy['skip-cert-verify'])
            self.assertFalse(proxy['udp'])
            self.assertEqual(config['rules'], ['MATCH,BigFoot'])
            before = output.read_bytes()
            self.assertNotEqual(subprocess.run(args, capture_output=True).returncode, 0)
            self.assertEqual(before, output.read_bytes())
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)

if __name__ == '__main__': unittest.main()
