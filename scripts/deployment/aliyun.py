"""Alibaba FC adapter: submit, verify readback, reconcile HTTP trigger."""
from .errors import UserError
import base64
import json
import time
from .config import domain, deployment_resources

def endpoint_host(url):
    if not url.startswith('https://'):
        raise UserError('FC must return an HTTPS endpoint')
    return domain(url)


def matches_submission(actual, submitted, expected):
    if not submitted.code_checksum or actual.code_checksum != submitted.code_checksum:
        return False
    for key, value in expected.items():
        if key == 'code':
            continue
        observed = getattr(actual, key, None)
        if hasattr(value, 'to_map'):
            if observed is None:
                return False
            actual_map, expected_map = dict(observed.to_map()), value.to_map()
            # FC adds its default health check to custom runtime readback even
            # when omitted from the request. Keep startup fields and explicitly
            # requested health checks strict; never ignore arbitrary extras.
            if key == 'custom_runtime_config' and 'healthCheckConfig' not in expected_map:
                actual_map.pop('healthCheckConfig', None)
            if actual_map != expected_map:
                return False
        elif observed != value:
            return False
    return True



class AliyunFC:
    def __init__(self, client, models, clock=time.monotonic, sleep=time.sleep, timeout=120):
        self.client, self.models = client, models
        self.clock, self.sleep, self.timeout = clock, sleep, timeout

    @classmethod
    def connect(cls, region, profile):
        try:
            from alibabacloud_fc20230330.client import Client
            from alibabacloud_fc20230330 import models
            from alibabacloud_tea_openapi.utils_models import Config
        except ImportError:
            raise UserError('Install cloud dependencies: .bigfoot/venv/bin/pip install -r requirements-cloud.txt') from None
        # Code ZIPs are several MiB; transcontinental uploads can exceed the
        # old 10s socket write deadline (requests uses connect_timeout to send).
        client = Client(Config(access_key_id=profile['access_key_id'], access_key_secret=profile['access_key_secret'], region_id=region, read_timeout=120000, connect_timeout=120000))
        return cls(client, models)

    def apply(self, role, env, artifact, stage, *, function_name=None, resources=None):
        client, m = self.client, self.models
        name = function_name or 'bigfoot-'+role
        settings=deployment_resources({'deployment':{'resources':resources} if resources is not None else {}})
        props = dict(runtime='custom.debian11', handler='tunnel', cpu=settings['cpu'], memory_size=settings['memory_mb'],
                     disk_size=512, timeout=900, instance_concurrency=settings['instance_concurrency'], internet_access=True,
                     description='BigFoot managed code-package deployment',
                     custom_runtime_config=m.CustomRuntimeConfig(command=['/code/tunnel'], port=9000),
                     environment_variables=env, code=m.InputCodeLocation(zip_file=base64.b64encode(artifact).decode()))
        stage('function')
        try:
            old = client.get_function(name, m.GetFunctionRequest()).body
        except Exception as error:
            if getattr(error, 'code', None) != 'FunctionNotFound':
                raise
            old = None
        if old is not None:
            if old.description != props['description']:
                raise UserError(f'{name} exists but is not managed by this deployment script')
            submitted = client.update_function(name, m.UpdateFunctionRequest(body=m.UpdateFunctionInput(**props))).body
        else:
            submitted = client.create_function(m.CreateFunctionRequest(body=m.CreateFunctionInput(function_name=name, **props))).body
        stage('readback')
        deadline = self.clock()+self.timeout
        while True:
            f = client.get_function(name, m.GetFunctionRequest()).body
            status = getattr(f, 'last_update_status', None)
            if status in (None, 'Successful') and matches_submission(f, submitted, props):
                break
            if status == 'Failed' or self.clock() > deadline:
                raise UserError(f'{name}: function update did not succeed')
            self.sleep(2)
        stage('trigger')
        trigger_config = json.dumps({'authType':'anonymous','methods':['GET','HEAD']})
        try:
            client.get_trigger(name, 'https')
        except Exception as error:
            if getattr(error, 'code', None) != 'TriggerNotFound':
                raise
            response = client.create_trigger(name, m.CreateTriggerRequest(body=m.CreateTriggerInput(trigger_name='https', trigger_type='http', trigger_config=trigger_config)))
        else:
            response = client.update_trigger(name, 'https', m.UpdateTriggerRequest(body=m.UpdateTriggerInput(trigger_config=trigger_config)))
        url = response.body.http_trigger.url_internet
        return endpoint_host(url)
