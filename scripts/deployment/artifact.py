"""Fresh native preflight and Linux deployment artifact."""
from .errors import UserError
import copy
import io
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import zipfile
from .config import ROOT, environment

def package_binary(binary, routing=None):
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, 'w', zipfile.ZIP_DEFLATED) as archive:
        item = zipfile.ZipInfo('tunnel')
        item.create_system = 3
        item.external_attr = 0o100755 << 16
        item.compress_type = zipfile.ZIP_DEFLATED
        archive.writestr(item, binary)
        if routing is not None:
            archive.writestr('routing.json',json.dumps(routing,ensure_ascii=False))
    return stream.getvalue()


def build_package(data, roles, routing=None):
    go = shutil.which("go")
    if not go:
        raise UserError("Go 1.25+ is required in PATH")
    checked = copy.deepcopy(data)
    if checked.get('version')==3:
        for node in checked['deployment']['voyagers'].values():
            if not node['domain']:node['domain']='preflight.invalid'
    elif not checked['deployment']['voyager']['domain']:
        checked['deployment']['voyager']['domain']='preflight.invalid'
    envs=[]
    preflight_roles=list(roles)
    if checked.get('version')==3 and 'gate' not in preflight_roles:preflight_roles.append('gate')
    for job in preflight_roles:
        role,_,node=job.partition(':')
        envs.append(environment(checked,role,node or None))
    with tempfile.TemporaryDirectory(prefix='bigfoot-build-') as temp:
        native = pathlib.Path(temp)/'native'
        binary = pathlib.Path(temp)/'tunnel'
        build_env = dict(os.environ, CGO_ENABLED='0')
        build_env.pop('GOOS', None); build_env.pop('GOARCH', None)
        subprocess.run([go, 'build', '-trimpath', '-o', str(native), './cmd/tunnel'], cwd=ROOT, env=build_env, check=True)
        for env in envs:
            if 'BIGFOOT_ROUTING_CONFIG' in env:
                route_path=pathlib.Path(temp)/'routing.json'
                route_path.write_text(json.dumps(routing,ensure_ascii=False))
                env['BIGFOOT_ROUTING_CONFIG']=str(route_path)
            clean = {k:v for k,v in os.environ.items() if not k.startswith('BIGFOOT_') and k != 'PORT'}
            subprocess.run([str(native), '-check'], env=dict(clean, **env), check=True)
        build_env.update(GOOS='linux', GOARCH='amd64')
        subprocess.run([go, 'build', '-trimpath', '-ldflags=-s -w', '-o', str(binary), './cmd/tunnel'], cwd=ROOT, env=build_env, check=True)
        return package_binary(binary.read_bytes(),routing)
