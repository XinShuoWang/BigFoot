import pathlib
import sys
import unittest
from unittest.mock import patch
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]/'scripts'))
from deployment.verification import verify

class VerificationTests(unittest.TestCase):
    def test_voyager_only_health_does_not_require_gate(self):
        data={'deployment':{'voyager':{'domain':'voyager.example.com'},'gate':{'domain':''}}}
        with patch('deployment.verification.check_health') as health:
            verify(pathlib.Path('unused'),data,roles=('voyager',))
            health.assert_called_once_with('voyager.example.com')
        with patch('deployment.verification.check_health'):
            with self.assertRaises(ValueError):verify(pathlib.Path('unused'),data)
