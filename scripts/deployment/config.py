#!/usr/bin/env python3
"""One private configuration; repeatable credential setup and FC deployment."""
from decimal import Decimal
import json
import base64
import os
import pathlib
import re
import secrets
import shutil
import subprocess
import tempfile
from urllib.parse import urlsplit

from client_config import configuration, write_private
from .routing import NODE_ID, credential_env, load_routing

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
    if data.get('version') not in (2,3):
        raise ValueError('unsupported configuration version')
    validate_credentials(data)
    if data['version']==3:
        validate_nodes(data)
    for field in ('session_timeout','idle_timeout'):
        duration_ns(data['runtime'][field],field)
    if duration_ns(data['runtime']['session_timeout'],'session_timeout') > 900_000_000_000:
        raise ValueError('runtime.session_timeout cannot exceed the configured FC 15 minute timeout')
    validate_capacity(data)
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


def initialize(path, *, version=2):
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
    if version==3:
        data['deployment']['resources']={'cpu':0.05,'memory_mb':128,'instance_concurrency':200}
        data['runtime']['max_connections']=200
        data['version']=3
        node=data['deployment'].pop('voyager');node['function_name']='bigfoot-voyager'
        data['deployment']['voyagers']={'hongkong':node}
        secret={key:data['credentials'].pop(key) for key in ('tunnel_token','voyager_identity_ca_pem','voyager_identity_key_pem')}
        data['credentials']['voyagers']={'hongkong':secret}
        data['routing']={'default':'hongkong','rules':[]}
    write_private(path, data)
    print(f'Created {path} (0600). Credentials generated once; do not share this file.')


def domain(value):
    parsed = urlsplit(value if '://' in value else 'https://'+value)
    if parsed.scheme != 'https' or not parsed.hostname or parsed.username is not None or parsed.port not in (None,443) or parsed.path not in ('','/') or parsed.query or parsed.fragment:
        raise ValueError('use an HTTPS hostname or root HTTPS URL, without path or custom port')
    if not re.fullmatch(r'[A-Za-z0-9.-]+',parsed.hostname):
        raise ValueError('invalid domain')
    return parsed.hostname


def environment(data, role, node=None):
    if data.get('version')==3:
        return multi_environment(data,role,node)
    secret = data['credentials']
    runtime = data['runtime']
    validate_capacity(data)
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


def voyagers(data):
    if data.get('version')==3:return data['deployment']['voyagers']
    return {'hongkong':dict(data['deployment']['voyager'],function_name='bigfoot-voyager')}


def node_credentials(data,name):
    return data['credentials']['voyagers'][name] if data.get('version')==3 else data['credentials']


def pem_der(value):
    # Compare decoded PEM blocks, so harmless whitespace cannot bypass isolation.
    match=re.fullmatch(r'\s*-----BEGIN ([A-Z ]+)-----\s*([A-Za-z0-9+/=\s]+)-----END \1-----\s*',value)
    if not match:raise ValueError('invalid Voyager identity PEM')
    try:return base64.b64decode(re.sub(r'\s','',match[2]),validate=True)
    except ValueError:raise ValueError('invalid Voyager identity PEM') from None


def validate_credentials(data):
    credentials=data['credentials']
    values=[credentials['client_password']]
    nodes=credentials['voyagers'].values() if data['version']==3 else [credentials]
    for secret in nodes:
        value=secret['tunnel_token'];values.append(value)
        if not isinstance(value,str) or value==credentials['client_password']:
            raise ValueError('client password and Gate/Voyager token must differ')
        if not isinstance(secret['voyager_identity_ca_pem'],str) or not isinstance(secret['voyager_identity_key_pem'],str):
            raise ValueError('invalid Voyager identity')
    for value in values:
        if not isinstance(value,str) or not 32<=len(value)<=256 or any(not 33<=ord(c)<=126 for c in value):
            raise ValueError('invalid credential field')
    if data['version']==3:
        if len(values)!=len(set(values)):raise ValueError('Voyager tokens must be independent')
        for field in ('voyager_identity_ca_pem','voyager_identity_key_pem'):
            identities=[pem_der(secret[field]) for secret in credentials['voyagers'].values()]
            if len(identities)!=len(set(identities)):raise ValueError('Voyager identities must be independent')


def validate_nodes(data):
    nodes=voyagers(data)
    if not isinstance(nodes,dict) or not nodes:raise ValueError('at least one Voyager required')
    if set(nodes)!=set(data['credentials']['voyagers']):raise ValueError('Voyager credential references do not match')
    locations={(data['deployment']['gate']['region'],'bigfoot-gate')}
    for name,node in nodes.items():
        if not NODE_ID.fullmatch(name):raise ValueError('invalid Voyager ID')
        if not re.fullmatch(r'[A-Za-z_][A-Za-z0-9_-]{0,63}',node['function_name']):raise ValueError('invalid function name')
        if not re.fullmatch(r'[a-z][a-z0-9-]{1,63}',node['region']):raise ValueError('invalid region')
        location=(node['region'],node['function_name'])
        if location in locations:raise ValueError('duplicate Voyager function in one region')
        locations.add(location)
        if node['domain']:domain(node['domain'])
    if data['routing']['default'] not in nodes:raise ValueError('unknown default Voyager')
    if not isinstance(data['routing']['rules'],list):raise ValueError('routing.rules must be a list')
    for rule in data['routing']['rules']:
        if not isinstance(rule,dict) or set(rule)!={'rule_set','voyager'} or rule['voyager'] not in nodes or not isinstance(rule['rule_set'],str):
            raise ValueError('invalid rule set or unknown Voyager')


def multi_environment(data,role,node=None):
    validate_credentials(data);validate_nodes(data)
    runtime=data['runtime']
    validate_capacity(data)
    env={'BIGFOOT_MODE':role,'BIGFOOT_BEHIND_TLS_PROXY':'true','PORT':'9000',
         'BIGFOOT_SESSION_TIMEOUT':runtime['session_timeout'],'BIGFOOT_IDLE_TIMEOUT':runtime['idle_timeout'],
         'BIGFOOT_SHUTDOWN_TIMEOUT':'5s','BIGFOOT_MAX_CONNECTIONS':str(runtime['max_connections'])}
    if role=='gate':
        env['BIGFOOT_CLIENT_PASSWORD']=data['credentials']['client_password']
        env['BIGFOOT_ROUTING_CONFIG']='/code/routing.json'
        for name in voyagers(data):
            secret=node_credentials(data,name)
            env[credential_env(name,'TOKEN')]=secret['tunnel_token']
            env[credential_env(name,'CA_PEM')]=secret['voyager_identity_ca_pem']
    elif role=='voyager':
        if node is None:
            if len(voyagers(data))!=1:raise ValueError('Voyager ID required')
            node=next(iter(voyagers(data)))
        secret=node_credentials(data,node)
        env.update(BIGFOOT_TOKEN=secret['tunnel_token'],BIGFOOT_INNER_CA_PEM=secret['voyager_identity_ca_pem'],
                   BIGFOOT_INNER_IDENTITY_KEY_PEM=secret['voyager_identity_key_pem'])
    else:raise ValueError('invalid deployment role')
    return env


def migrate(path):
    with config_lock(path):
        data=read_bundle(path)
        if data['version']==3:return
        backup=path.with_name(path.name+'.v2.bak')
        original=config_snapshot(path,data)
        if backup.exists() or backup.is_symlink():
            if backup.is_symlink() or backup.stat().st_mode&0o077 or backup.read_bytes()!=original:
                raise ValueError('migration backup conflicts with current configuration; preserve it before retry')
        else:
            fd=os.open(backup,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
            with os.fdopen(fd,'wb') as stream:stream.write(original);stream.flush();os.fsync(stream.fileno())
        data['version']=3
        node=data['deployment'].pop('voyager');node['function_name']='bigfoot-voyager'
        data['deployment']['voyagers']={'hongkong':node}
        secret={key:data['credentials'].pop(key) for key in ('tunnel_token','voyager_identity_ca_pem','voyager_identity_key_pem')}
        data['credentials']['voyagers']={'hongkong':secret}
        data['routing']={'default':'hongkong','rules':[]}
        validate_nodes(data);validate_credentials(data)
        if path.read_bytes()!=original:raise ValueError('configuration changed during migration')
        replace_private(path,data)


def config_snapshot(path,data):
    original=path.read_bytes()
    if json.loads(original)!=data:raise ValueError('configuration changed; retry the operation')
    return original


def config_lock(path):
    from .workflow import deployment_lock
    return deployment_lock(path)


def add_voyager(path,name,region):
    if not NODE_ID.fullmatch(name) or not re.fullmatch(r'[a-z][a-z0-9-]{1,63}',region):raise ValueError('invalid Voyager ID or region')
    with config_lock(path):
        data=read_bundle(path)
        if data['version']!=3:raise ValueError('run migrate before adding Voyager nodes')
        if name in voyagers(data):
            if voyagers(data)[name]['region']!=region:raise ValueError('existing Voyager has a different region')
            return
        original=config_snapshot(path,data)
        with tempfile.TemporaryDirectory(prefix='bigfoot-new-node-') as temporary:
            seed=pathlib.Path(temporary)/'config.json';initialize(seed);secret=read_bundle(seed)['credentials']
        secret.pop('client_password')
        data['deployment']['voyagers'][name]={'region':region,'domain':'','function_name':'bigfoot-voyager-'+name}
        data['credentials']['voyagers'][name]=secret
        validate_nodes(data);validate_credentials(data)
        if path.read_bytes()!=original:raise ValueError('configuration changed while adding Voyager')
        replace_private(path,data)


def deployment_resources(data):
    if 'resources' not in data['deployment']:
        # Existing saved configurations retain their previous sizing until edited.
        return {'cpu':1,'memory_mb':1024,'instance_concurrency':128}
    resources=data['deployment']['resources']
    if not isinstance(resources,dict) or set(resources)!={'cpu','memory_mb','instance_concurrency'}:
        raise ValueError('deployment.resources requires cpu, memory_mb and instance_concurrency')
    cpu,memory,concurrency=(resources[key] for key in ('cpu','memory_mb','instance_concurrency'))
    if type(cpu) not in (int,float):raise ValueError('cpu must be a number')
    cores=Decimal(str(cpu))
    if not cores.is_finite() or not Decimal('0.05')<=cores<=16 or cores%Decimal('0.05'):
        raise ValueError('cpu must be 0.05..16 in steps of 0.05')
    if type(memory) is not int or not 128<=memory<=32768 or memory%64:
        raise ValueError('memory_mb must be 128..32768 in steps of 64')
    if not cores*1024<=memory<=cores*4096:
        raise ValueError('CPU to memory (GiB) ratio must be between 1:1 and 1:4')
    if type(concurrency) is not int or not 1<=concurrency<=200:
        raise ValueError('instance_concurrency must be 1..200')
    return dict(resources)


def validate_capacity(data):
    resources=deployment_resources(data)
    count=data['runtime']['max_connections']
    limit=resources['instance_concurrency']
    if type(count) is not int or not 1<=count<=limit:
        raise ValueError(f'runtime.max_connections must be 1..{limit}, within FC instance_concurrency')
