"""Read local, bounded rule sets without putting secrets in deployment artifacts."""
import ipaddress
import pathlib
import re

MAX_FILE=8*1024*1024
MAX_TOTAL=32*1024*1024
MAX_RULES=200000
NODE_ID=re.compile(r'[a-z][a-z0-9-]{0,31}')


def credential_env(node, kind):
    return 'BIGFOOT_NODE_'+node.upper().replace('-','_')+'_'+kind


def validate_line(line):
    line=line.strip()
    if not line or line.startswith('#'):return False
    parts=line.split(',')
    if len(parts)!=2:raise ValueError('expected TYPE,value')
    kind,value=(part.strip() for part in parts)
    if kind in ('DOMAIN','DOMAIN-SUFFIX'):
        try:
            name=value.removesuffix('.').encode('idna').decode('ascii')
        except UnicodeError:
            raise ValueError('invalid domain') from None
        if not name or len(name)>253 or any(not re.fullmatch(r'[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?',label) for label in name.split('.')):
            raise ValueError('invalid domain')
        try:ipaddress.ip_address(name)
        except ValueError:pass
        else:raise ValueError('domain must not be an IP')
    elif kind in ('IP-CIDR','IP-CIDR6'):
        try:network=ipaddress.ip_network(value,strict=False)
        except ValueError:raise ValueError('invalid IP prefix') from None
        if network.version!=(4 if kind=='IP-CIDR' else 6) or (network.version==6 and network.network_address.ipv4_mapped):
            raise ValueError('wrong IP family')
    elif kind=='DST-PORT':
        if not re.fullmatch(r'[0-9]+',value) or not 1<=int(value)<=65535:raise ValueError('invalid port')
    else:raise ValueError('unknown rule type')
    return True


def load_routing(path,data,*,preflight=False):
    from .config import domain, voyagers
    nodes=voyagers(data)
    routing=data.get('routing',{'default':'hongkong','rules':[]})
    if routing['default'] not in nodes:raise ValueError('unknown default Voyager')
    result={'default':routing['default'],'nodes':{},'rules':[]}
    for name,node in nodes.items():
        result['nodes'][name]={'url':'wss://'+domain(node['domain'] or ('preflight.invalid' if preflight else ''))+'/tunnel',
            'token_env':credential_env(name,'TOKEN'),'ca_env':credential_env(name,'CA_PEM')}
    root=path.parent.resolve();total=0;count=0
    for rule in routing['rules']:
        if set(rule)!={'rule_set','voyager'} or rule['voyager'] not in nodes:raise ValueError('invalid rule set or unknown Voyager')
        relative=pathlib.Path(rule['rule_set'])
        if relative.is_absolute() or '..' in relative.parts:raise ValueError('rule path must stay inside configuration directory')
        candidate=root
        for part in relative.parts:
            candidate=candidate/part
            if candidate.is_symlink():raise ValueError('rule paths must not contain symbolic links')
        if not candidate.is_file():raise ValueError('rule file is missing')
        with candidate.open('rb') as stream:raw=stream.read(MAX_FILE+1)
        total+=len(raw)
        if len(raw)>MAX_FILE or total>MAX_TOTAL:raise ValueError('rule files too large')
        try:lines=raw.decode('utf-8').splitlines()
        except UnicodeError:raise ValueError('rule file must be UTF-8') from None
        for number,line in enumerate(lines,1):
            try:count+=validate_line(line)
            except ValueError as error:raise ValueError(f'{relative}:{number}: {error}') from None
            if count>MAX_RULES:raise ValueError('too many rules')
        result['rules'].append({'source':relative.as_posix(),'voyager':rule['voyager'],'lines':lines})
    return result
