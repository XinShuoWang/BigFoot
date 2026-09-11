"""HTTPS readiness and optional end-to-end client verification."""
from .errors import UserError
import pathlib
import subprocess
import urllib.request
from .config import ROOT, domain, export_client

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def check_health(host):
    host = domain(host)
    opener = urllib.request.build_opener(NoRedirect)
    for route in ('/healthz', '/readyz'):
        try:
            with opener.open('https://'+host+route, timeout=20) as response:
                if response.status != 200:
                    raise UserError('unhealthy')
        except Exception:
            raise UserError(f'HTTPS check failed: {host}{route}') from None


def check_link(path, data, mihomo):
    binary = pathlib.Path(mihomo).resolve()
    if not binary.is_file():
        raise UserError('Mihomo executable does not exist')
    target = export_client(path, data)
    import sys
    result = subprocess.run([sys.executable, str(ROOT/'scripts/smoke_cloud.py'), str(binary), '--config', str(target)], capture_output=True, timeout=120)
    if result.returncode:
        raise UserError('End-to-end Mihomo verification failed; deployment is not link-verified')
    print('PASS: end-to-end HTTPS and wrong-password rejection')


def verify(path, data, mihomo=None, roles=('voyager', 'gate')):
    for role in (('voyager', 'gate') if mihomo else roles):
        host = data['deployment'][role]['domain']
        if not host:
            raise UserError(f'{role.title()} endpoint is missing')
        check_health(host)
        print(f'{role.title()}: HTTPS health/readiness passed')
    if mihomo:
        check_link(path, data, mihomo)
    else:
        print('Health checks passed; full proxy link NOT verified (use --mihomo PATH).')
