#!/usr/bin/env python3
"""Generate a private Mihomo configuration (JSON is valid YAML). No network calls."""
import argparse
import json
import os
import pathlib
import re


def configuration(server, port, password, path='/proxy', local_port=7890, ca=None):
    if not re.fullmatch(r'[A-Za-z0-9.-]+', server):
        raise ValueError('server must be a DNS hostname or IPv4 address, without scheme/port')
    if not 1 <= port <= 65535 or not 1 <= local_port <= 65535:
        raise ValueError('port must be in 1..65535')
    if not re.fullmatch(r'/[A-Za-z0-9/_.-]*', path) or path in ('/healthz', '/readyz') or path.startswith('//'):
        raise ValueError('invalid WebSocket path')
    if not 32 <= len(password) <= 256 or any(not 33 <= ord(c) <= 126 for c in password):
        raise ValueError('password must contain 32..256 non-space printable ASCII characters')
    result = {
        'mixed-port': local_port, 'allow-lan': False, 'bind-address': '127.0.0.1',
        'mode': 'rule', 'log-level': 'warning', 'ipv6': False,
        'proxies': [{'name': 'BigFoot', 'type': 'trojan', 'server': server,
                     'port': port, 'password': password, 'sni': server,
                     'skip-cert-verify': False, 'udp': False, 'network': 'ws',
                     'alpn': ['http/1.1'], 'ws-opts': {'path': path},
                     'smux': {'enabled': False}}],
        'rules': ['MATCH,BigFoot'],
    }
    if ca:
        # Mihomo's upstream field is deliberately spelled "certifactes".
        result['tls'] = {'custom-certifactes': [ca]}
    return result


def write_private(path, data):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as stream:
        json.dump(data, stream, indent=2, ensure_ascii=False)
        stream.write('\n')


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--server', required=True)
    p.add_argument('--port', type=int, default=443)
    p.add_argument('--password-file', type=pathlib.Path, required=True)
    p.add_argument('--path', default='/proxy')
    p.add_argument('--local-port', type=int, default=7890)
    p.add_argument('--ca', type=pathlib.Path, help='optional local test CA; normal deployment uses system trust')
    p.add_argument('--output', type=pathlib.Path, required=True)
    args = p.parse_args()
    password = args.password_file.read_text().strip().splitlines()[0]
    data = configuration(args.server, args.port, password, args.path, args.local_port,
                         args.ca.read_text() if args.ca else None)
    write_private(args.output, data)
    print(f'Created {args.output}; contains client credentials, keep private.')


if __name__ == '__main__':
    main()
