#!/usr/bin/env python3
"""BigFoot configuration, deployment and verification commands."""
import argparse
import json
import pathlib
import subprocess
import sys
from deployment.config import ROOT, initialize, read_bundle, export_client


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config',type=pathlib.Path,default=ROOT/'.bigfoot/config.json')
    sub=parser.add_subparsers(dest='command',required=True)
    sub.add_parser('init')
    for name in ('render','deploy'):
        command=sub.add_parser(name)
        if name=='deploy':
            command.add_argument('--only',choices=['all','gate','voyager'],default='all')
            command.add_argument('--mihomo',type=pathlib.Path,help='also verify full proxy link using this client')
    command=sub.add_parser('verify')
    command.add_argument('--mihomo',type=pathlib.Path)
    args=parser.parse_args()
    path=args.config.absolute()
    if args.command=='init':
        initialize(path)
        return
    data=read_bundle(path)
    if args.command=='verify':
        from deployment.verification import verify
        verify(path,data,args.mihomo)
        return
    if args.command=='render':
        print(f'Client configuration: {export_client(path,data)}')
    else:
        from deployment.workflow import deploy_package
        deploy_package(path,data,args.only,mihomo=args.mihomo)


if __name__=='__main__':
    try:
        main()
    except (ValueError,KeyError,OSError,subprocess.SubprocessError) as error:
        if isinstance(error,(KeyError,json.JSONDecodeError)):
            message='invalid/incomplete configuration; existing file was not replaced'
        elif isinstance(error,subprocess.SubprocessError):
            message='external command failed; inspect tool installation and retry'
        else:message=str(error)
        print(f'BigFoot: {message}',file=sys.stderr)
        sys.exit(1)
