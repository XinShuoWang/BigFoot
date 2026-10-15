#!/usr/bin/env python3
"""Verify deployed Gate/Voyager through a real Mihomo client, including wrong-password rejection."""
import argparse
import json
import pathlib
import subprocess
import tempfile
from client_config import write_private
from smoke import port, ready


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mihomo',type=pathlib.Path)
    parser.add_argument('--config',type=pathlib.Path,default=pathlib.Path(__file__).resolve().parents[1]/'.bigfoot/generated/client.yaml')
    args=parser.parse_args()
    config=json.loads(args.config.read_text())
    binary=str(args.mihomo.resolve())
    with tempfile.TemporaryDirectory(prefix='bigfoot-cloud-smoke-') as d:
        work=pathlib.Path(d)
        for valid in (True,False):
            number=port()
            config['mixed-port']=number
            if not valid:config['proxies'][0]['password']='invalid-password-0123456789abcdef'
            filename=work/('valid.yaml' if valid else 'invalid.yaml')
            write_private(filename,config)
            with (work/'mihomo.log').open('w') as log:
                proc=subprocess.Popen([binary,'-d',str(work),'-f',str(filename)],stdout=log,stderr=log)
                try:
                    ready(proc,number)
                    result=subprocess.run(['curl','--silent','--show-error','--fail','--max-time','40','--noproxy','','--proxy',f'http://127.0.0.1:{number}','https://example.com/'],capture_output=True,timeout=45)
                    ok=result.returncode==0 and b'Example Domain' in result.stdout
                    if valid and not ok:
                        raise RuntimeError(f'Cloud HTTPS test failed (curl exit {result.returncode})')
                    if not valid and result.returncode==0:
                        raise RuntimeError('Wrong password accepted')
                    print('PASS: Mihomo -> Gate -> selected Voyager -> HTTPS' if valid else 'PASS: wrong client password rejected',flush=True)
                finally:
                    proc.terminate()
                    try:proc.wait(timeout=8)
                    except subprocess.TimeoutExpired:proc.kill();proc.wait()

if __name__=='__main__':main()
