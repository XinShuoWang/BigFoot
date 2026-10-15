"""HTTPS readiness and optional end-to-end client verification."""
from .errors import UserError
import pathlib
import io
import json
import zipfile
import subprocess
import urllib.request
from .config import ROOT, domain, export_client, voyagers, deployment_resources

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
    if data.get('version')==3:
        print('Current routing path verified; other Voyager routes NOT verified by this read-only check.')


def verify(path, data, mihomo=None, roles=('voyager', 'gate')):
    selected=('voyager','gate') if mihomo else roles
    hosts=[]
    if 'voyager' in selected:hosts.extend(('voyager:'+name,node['domain']) for name,node in voyagers(data).items())
    if 'gate' in selected:hosts.append(('gate',data['deployment']['gate']['domain']))
    for role,host in hosts:
        if not host:raise UserError(f'{role} endpoint is missing')
        check_health(host)
        print(f'{role}: HTTPS health/readiness passed')
    if mihomo:
        check_link(path, data, mihomo)
    else:
        print('Health checks passed; full proxy link NOT verified (use --mihomo PATH).')


def check_all_routes(path,data,mihomo,provider,artifact,env,*,health=check_health,probe=check_link,stage=lambda _:None):
    """Temporarily deploy one possible upstream at a time, then restore policy.

    Only used within a locked deployment, never by read-only verify.
    """
    from .artifact import package_binary
    with zipfile.ZipFile(io.BytesIO(artifact)) as archive:
        binary=archive.read('tunnel');routing=json.loads(archive.read('routing.json'))
    def apply(blob):
        resource_options={'resources':deployment_resources(data)} if 'resources' in data['deployment'] else {}
        host=provider.apply('gate',env,blob,lambda _:None,function_name='bigfoot-gate',**resource_options)
        if domain(host)!=domain(data['deployment']['gate']['domain']):raise UserError('Gate endpoint changed during route verification')
        health(host)
    try:
        for name,node in routing['nodes'].items():
            stage('probe:'+name)
            # A single trusted upstream is the physical path evidence: no other
            # node exists in this probe config, regardless of user rule ordering.
            directed={'default':name,'nodes':{name:node},'rules':[]}
            apply(package_binary(binary,directed))
            probe(path,data,mihomo)
            print(f'PASS: Gate -> voyager:{name} -> HTTPS and wrong-password rejection')
    finally:
        try:
            stage('restore-policy')
        finally:
            # Disk/journal failure must not skip restoring the live Gate policy.
            try:apply(artifact)
            except BaseException:
                raise UserError('Gate policy restoration failed; rerun deploy --only gate immediately') from None
    stage('verify-policy')
    probe(path,data,mihomo)
