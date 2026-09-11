#!/usr/bin/env python3
"""Run real Mihomo -> Gate -> Voyager -> HTTPS, with strict certificate verification."""
import argparse
import json
import pathlib
import subprocess
import tempfile

from client_config import configuration, write_private
from smoke import port, ready


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('binary', type=pathlib.Path)
    parser.add_argument('mihomo', type=pathlib.Path)
    parser.add_argument('--auto-identity', action='store_true', help='exercise automatic Voyager certificate renewal mode')
    args = parser.parse_args()
    binary, mihomo = str(args.binary.resolve()), str(args.mihomo.resolve())
    with tempfile.TemporaryDirectory(prefix='bigfoot-mihomo-') as work:
        work = pathlib.Path(work)
        certs = work/'certs'
        subprocess.run(['bash', str(pathlib.Path(__file__).with_name('gencerts.sh')), str(certs), '90'], check=True)
        gate_port, voyager_port, client_port = port(), port(), port()
        processes = []
        log_path = work/'processes.log'
        try:
            with log_path.open('w') as log:
                common = ['-token-file', str(certs/'token.txt'), '-shutdown-timeout', '100ms']
                tls = ['-outer-cert', str(certs/'outer.pem'), '-outer-key', str(certs/'outer.key')]
                commands = [
                    ([binary, '-mode', 'voyager', '-listen', f'127.0.0.1:{voyager_port}',
                      *(['-inner-ca', str(certs/'inner-ca.pem'), '-inner-identity-key', str(certs/'inner-ca.key')] if args.auto_identity else ['-inner-cert', str(certs/'inner.pem'), '-inner-key', str(certs/'inner.key')]), *common, *tls], voyager_port),
                    ([binary, '-mode', 'gate', '-listen', f'127.0.0.1:{gate_port}',
                      '-inner-ca', str(certs/'inner-ca.pem'), '-outer-ca', str(certs/'outer-ca.pem'),
                      '-voyager-url', f'wss://localhost:{voyager_port}/tunnel',
                      '-client-password-file', str(certs/'client-password.txt'), *common, *tls], gate_port),
                ]
                for command, number in commands:
                    proc = subprocess.Popen(command, stdout=log, stderr=log)
                    processes.append(proc)
                    ready(proc, number)
                config = configuration('localhost', gate_port, (certs/'client-password.txt').read_text().strip(),
                                       local_port=client_port, ca=(certs/'outer-ca.pem').read_text())
                config_path = work/'client.yaml'
                write_private(config_path, config)
                subprocess.run([mihomo, '-t', '-d', str(work), '-f', str(config_path)], check=True, stdout=log, stderr=log)
                proc = subprocess.Popen([mihomo, '-d', str(work), '-f', str(config_path)], stdout=log, stderr=log)
                processes.append(proc)
                ready(proc, client_port)
                result = subprocess.run(['curl', '--silent', '--show-error', '--fail', '--max-time', '30',
                                         '--noproxy', '', '--proxy', f'http://127.0.0.1:{client_port}',
                                         'https://example.com/'], capture_output=True, text=True, timeout=35)
                if result.returncode or 'Example Domain' not in result.stdout:
                    raise RuntimeError(f'HTTPS through Mihomo failed: {result.stderr.strip()}')
                print(f'PASS: curl -> Mihomo -> Trojan/WSS Gate -> WSS + inner TLS Voyager -> HTTPS ({len(result.stdout)} bytes)')
                # An independent wrong password must fail with the real client.
                proc.terminate();proc.wait(timeout=5);processes.pop()
                config['proxies'][0]['password'] = 'wrong-client-password-0123456789abcdef'
                bad = work/'bad.yaml';write_private(bad, config)
                proc = subprocess.Popen([mihomo, '-d', str(work), '-f', str(bad)], stdout=log, stderr=log)
                processes.append(proc);ready(proc, client_port)
                bad_result = subprocess.run(['curl', '--silent', '--show-error', '--fail', '--max-time', '5',
                                             '--noproxy', '', '--proxy', f'http://127.0.0.1:{client_port}',
                                             'https://example.com/'], capture_output=True, timeout=8)
                if bad_result.returncode == 0:
                    raise AssertionError('wrong password accepted')
                print('PASS: real client with wrong password rejected')
        except Exception:
            print(log_path.read_text())
            raise
        finally:
            for proc in reversed(processes):
                if proc.poll() is None:
                    proc.terminate()
            for proc in processes:
                try:
                    code = proc.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    proc.kill();proc.wait()
                    raise RuntimeError('process failed to shut down within 8 seconds')
            print('Processes stopped; temporary credentials removed.')


if __name__ == '__main__':
    main()
