"""Resumable deployment by reconciling actual provider state on every run."""
from .errors import UserError
from contextlib import contextmanager
import fcntl
import hashlib
import json
import io
import zipfile
import os
import re
import tempfile
from .config import environment, replace_private, export_client, voyagers, load_routing, domain, deployment_resources
from .artifact import build_package, package_binary
from .aliyun import AliyunFC
from .credentials import load_profile
from .verification import check_health, check_link, check_all_routes


@contextmanager
def deployment_lock(path):
    lock = path.parent/'.deploy.lock'
    fd = os.open(lock, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    try:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise UserError('Another deployment is running for this configuration') from None
        yield
    finally:
        os.close(fd)


def safe_error(error):
    code = getattr(error, 'code', type(error).__name__)
    return code if isinstance(code,str) and re.fullmatch(r'[A-Za-z0-9_.-]{1,80}',code) else 'CloudError'


def save_artifact(out, blob):
    digest = hashlib.sha256(blob).hexdigest()
    target = out/(digest+'.zip')
    if not target.exists():
        fd, temp = tempfile.mkstemp(prefix='.artifact-', dir=out)
        try:
            with os.fdopen(fd,'wb') as stream:
                stream.write(blob);stream.flush();os.fsync(stream.fileno())
            os.replace(temp,target)
        finally:
            if os.path.exists(temp):os.unlink(temp)
    return digest


def deploy_package(path, data, only, *, build=build_package, provider_factory=None,
                   health=check_health, mihomo=None, link=check_link, voyager=None):
    if only not in ('all','gate','voyager'):raise UserError('invalid deployment role')
    multi=data.get('version')==3
    if multi and mihomo and only=='voyager':
        raise UserError('All-route Mihomo verification updates Gate; use deploy --only gate --mihomo PATH after Voyager deployment')
    if voyager and (only!='voyager' or not multi or voyager not in voyagers(data)):
        raise UserError('--voyager requires --only voyager and a configured node ID')
    if multi:
        roles=['voyager:'+name for name in voyagers(data) if voyager is None or name==voyager] if only in ('all','voyager') else []
        if only in ('all','gate'):roles.append('gate')
    else:
        roles = ('voyager','gate') if only=='all' else (only,)
    def node_config(job):
        return data['deployment']['voyagers'][job.split(':',1)[1]] if ':' in job else data['deployment'][job]

    with deployment_lock(path):
        # Refuse overwriting edits made after CLI loaded the configuration.
        initial = path.read_bytes()
        if json.loads(initial) != data:
            raise UserError('Configuration changed; rerun deployment')
        out = path.parent/'generated'
        out.mkdir(mode=0o700,exist_ok=True)
        journal = {'version':1,'backend':'code-package','status':'running','role':None,
                   'stage':'preflight','link_verified':False,'completed':[]}
        def record(stage, role=None):
            journal.update(stage=stage,role=role)
            replace_private(out/'deployment.json',journal)
        try:
            record('preflight')
            if only=='gate' and any(not node['domain'] for node in voyagers(data).values()):
                raise UserError('Voyager endpoint required for Gate-only deployment')
            resource_options={'resources':deployment_resources(data)} if 'resources' in data['deployment'] else {}
            routing=load_routing(path,data,preflight=True) if multi else None
            if provider_factory is None:
                profile = load_profile(data['deployment']['access'])
                provider_factory = lambda region: AliyunFC.connect(region,profile)
            # Instantiate every regional client before making any mutations.
            providers = {job:provider_factory(node_config(job)['region']) for job in roles}
            record('build')
            artifact = build(data, roles, routing) if multi else build(data, roles)
            journal['artifact_sha256'] = save_artifact(out,artifact)
            for job in roles:
                role,_,name=job.partition(':')
                record('function',job)
                if multi:
                    if role=='gate':
                        # Preserve the validated rule snapshot; update only the discovered URLs.
                        for key,node in routing['nodes'].items():
                            node['url']='wss://'+domain(voyagers(data)[key]['domain'])+'/tunnel'
                    with zipfile.ZipFile(io.BytesIO(artifact)) as archive:
                        current=package_binary(archive.read('tunnel'),routing if role=='gate' else None)
                    journal.setdefault('artifacts',{})[job]=save_artifact(out,current)
                    host=providers[job].apply(role,environment(data,role,name or None),current,
                        lambda stage:record(stage,job),function_name=node_config(job).get('function_name','bigfoot-gate'),**resource_options)
                else:
                    host=providers[job].apply(role,environment(data,role),artifact,lambda stage:record(stage,job),**resource_options)
                # Persist successfully discovered endpoints even if health fails.
                if path.read_bytes() != initial:
                    raise UserError('Configuration changed during deployment; rerun to reconcile')
                node_config(job)['domain'] = host
                replace_private(path,data)
                initial = path.read_bytes()
                record('health',job)
                health(host)
                journal['completed'].append(job)
                print(f'{job}: deployed and healthy',flush=True)
            if data['deployment']['gate']['domain']:
                target=export_client(path,data)
                print(f'Client configuration: {target}')
            if mihomo:
                record('link')
                if multi:
                    check_all_routes(path,data,mihomo,providers['gate'],current,environment(data,'gate'),
                                     health=health,probe=link,stage=lambda stage:record(stage,'gate'))
                    journal['verified_voyagers']=list(voyagers(data))
                else:link(path,data,mihomo)
                journal['link_verified']=True
            journal['status']='verified' if journal['link_verified'] else 'healthy'
            record('complete')
            if not mihomo:
                print('Deployment healthy; full proxy link NOT verified (use verify --mihomo PATH).')
        except Exception as error:
            journal['status']='failed'
            journal['error_code']=safe_error(error)
            replace_private(out/'deployment.json',journal)
            hint = str(error) if isinstance(error,UserError) else "Inspect the provider error code and local prerequisites."
            raise UserError(f"Deployment failed at {journal['role'] or '-'}:{journal['stage']} ({safe_error(error)}). {hint} Rerun after repair; cloud resources will be reconciled.") from None
