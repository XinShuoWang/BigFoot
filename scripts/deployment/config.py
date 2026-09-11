#!/usr/bin/env python3
"""One private configuration; repeatable credential setup and FC deployment."""
from decimal import Decimal
import json
import os
import pathlib
import re
import secrets
import shutil
import subprocess
import tempfile
from urllib.parse import urlsplit

from client_config import configuration, write_private

ROOT = pathlib.Path(__file__).resolve().parents[2]


def duration_ns(value, field):
    if not isinstance(value,str) or not re.fullmatch(r'(?:[0-9]+(?:\.[0-9]+)?(?:ns|us|ms|s|m|h))+', value):
        raise ValueError(f'runtime.{field} must be a positive duration such as 30s, 2m or 1h30m')
    scales={'ns':1,'us':1000,'ms':1000000,'s':1000000000,'m':60000000000,'h':3600000000000}
    total=sum(int(Decimal(number)*scales[unit]) for number,unit in re.findall(r'([0-9]+(?:\.[0-9]+)?)(ns|us|ms|s|m|h)',value))
    if not 0<total<=2**63-1:
        raise ValueError(f'runtime.{field} must be a positive Go duration')
    return total


def read_bundle(path):
    if path.is_symlink():
        raise ValueError('configuration must not be a symbolic link')
    if path.stat().st_mode & 0o077:
        raise ValueError(f'configuration must be private: chmod 600 {path}')
    if path.stat().st_size > 131072:
        raise ValueError('configuration too large')
    data = json.loads(path.read_text())
    if data.get('version') != 2:
        raise ValueError('unsupported configuration version')
    for key in ('tunnel_token', 'client_password'):
        value = data['credentials'][key]
        if not isinstance(value, str) or not 32 <= len(value) <= 256 or any(not 33 <= ord(c) <= 126 for c in value):
            raise ValueError(f'invalid credential field: {key}')
    if data['credentials']['tunnel_token'] == data['credentials']['client_password']:
        raise ValueError('client password and Gate/Voyager token must differ')
    for field in ('session_timeout','idle_timeout'):
        duration_ns(data['runtime'][field],field)
    if duration_ns(data['runtime']['session_timeout'],'session_timeout') > 900_000_000_000:
        raise ValueError('runtime.session_timeout cannot exceed the configured FC 15 minute timeout')
    return data


def replace_private(path, data):
    path = pathlib.Path(path)
    if path.is_symlink():
        raise ValueError('refusing to replace a symbolic link')
    fd, temporary = tempfile.mkstemp(prefix='.bigfoot-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w') as f:
            json.dump(data, f, indent=2, ensure_ascii=False)
            f.write('\n')
            f.flush()
            os.fsync(f.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def initialize(path):
    if path.exists() or path.is_symlink():
        read_bundle(path)
        print(f'Reusing {path}; existing passwords and identity unchanged.')
        return
    if not shutil.which('openssl'):
        raise ValueError('OpenSSL is required for first-time initialization')
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    with tempfile.TemporaryDirectory(prefix='bigfoot-identity-') as temporary:
        directory = pathlib.Path(temporary)
        conf = directory/'ca.cnf'
        conf.write_text('[req]\ndistinguished_name=dn\nx509_extensions=ext\nprompt=no\n[dn]\nCN=BigFoot Voyager identity\n[ext]\nbasicConstraints=critical,CA:TRUE,pathlen:0\nkeyUsage=critical,keyCertSign\n')
        subprocess.run(['openssl', 'req', '-x509', '-sha256', '-newkey', 'ec',
                        '-pkeyopt', 'ec_paramgen_curve:P-256', '-pkeyopt', 'ec_param_enc:named_curve',
                        '-nodes', '-config', str(conf), '-days', '18250',
                        '-keyout', str(directory/'identity.key'), '-out', str(directory/'identity.pem')],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        data = {'version': 2,
                'deployment': {'access': 'default',
                               'gate': {'region': 'cn-shanghai', 'domain': ''},
                               'voyager': {'region': 'cn-hongkong', 'domain': ''}},
                'runtime': {'session_timeout': '14m', 'idle_timeout': '2m', 'max_connections': 128},
                'credentials': {'tunnel_token': secrets.token_hex(32), 'client_password': secrets.token_hex(32),
                                'voyager_identity_ca_pem': (directory/'identity.pem').read_text(),
                                'voyager_identity_key_pem': (directory/'identity.key').read_text()}}
    write_private(path, data)
    print(f'Created {path} (0600). Credentials generated once; do not share this file.')


def domain(value):
    parsed = urlsplit(value if '://' in value else 'https://'+value)
    if parsed.scheme != 'https' or not parsed.hostname or parsed.username is not None or parsed.port not in (None,443) or parsed.path not in ('','/') or parsed.query or parsed.fragment:
        raise ValueError('use an HTTPS hostname or root HTTPS URL, without path or custom port')
    if not re.fullmatch(r'[A-Za-z0-9.-]+',parsed.hostname):
        raise ValueError('invalid domain')
    return parsed.hostname


def environment(data, role):
    secret = data['credentials']
    runtime = data['runtime']
    if not isinstance(runtime['max_connections'],int) or not 1 <= runtime['max_connections'] <= 128:
        raise ValueError('max_connections must be 1..128 with the default FC capacity')
    env = {'BIGFOOT_MODE':role, 'BIGFOOT_BEHIND_TLS_PROXY':'true', 'PORT':'9000',
           'BIGFOOT_TOKEN':secret['tunnel_token'], 'BIGFOOT_INNER_CA_PEM':secret['voyager_identity_ca_pem'],
           'BIGFOOT_SESSION_TIMEOUT':runtime['session_timeout'], 'BIGFOOT_IDLE_TIMEOUT':runtime['idle_timeout'],
           'BIGFOOT_SHUTDOWN_TIMEOUT':'5s', 'BIGFOOT_MAX_CONNECTIONS':str(runtime['max_connections'])}
    if role == 'gate':
        env.update(BIGFOOT_CLIENT_PASSWORD=secret['client_password'],
                   BIGFOOT_VOYAGER_URL='wss://'+domain(data['deployment']['voyager']['domain'])+'/tunnel')
    else:
        env['BIGFOOT_INNER_IDENTITY_KEY_PEM'] = secret['voyager_identity_key_pem']
    return env



def export_client(path, data):
    host = data['deployment']['gate']['domain']
    if not host:
        raise ValueError('Gate HTTPS endpoint is required; deploy Gate first or fill deployment.gate.domain')
    out = path.parent/'generated'
    out.mkdir(mode=0o700, exist_ok=True)
    target = out/'client.yaml'
    replace_private(target, configuration(domain(host),443,data['credentials']['client_password']))
    return target
