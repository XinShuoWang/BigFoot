import copy
import pathlib
import sys
import unittest
from types import SimpleNamespace as NS
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]/'scripts'))
from deployment.aliyun import AliyunFC

class CloudError(Exception):
    def __init__(self,code):self.code=code

class Models:
    def __getattr__(self,name):return lambda **kw:NS(**kw)

class FakeFC:
    def __init__(self):
        self.function=None;self.trigger=None;self.calls=[];self.fail=None;self.stale=False
    def get_function(self,*_):
        if self.fail=='denied':raise CloudError('AccessDenied')
        if self.function is None:raise CloudError('FunctionNotFound')
        f=copy.deepcopy(self.function)
        if self.stale:f.code_checksum='old'
        return NS(body=f)
    def create_function(self,request):
        self.calls.append('create');self.function=request.body;self.function.code_checksum='new'
        if self.fail=='create':raise CloudError('TimeoutAfterCommit')
        return NS(body=copy.deepcopy(self.function))
    def update_function(self,name,request):
        self.calls.append('update');self.function=request.body;self.function.code_checksum='new'
        return NS(body=copy.deepcopy(self.function))
    def get_trigger(self,*_):
        if self.trigger is None:raise CloudError('TriggerNotFound')
        return NS(body=self.trigger)
    def create_trigger(self,*_):
        self.calls.append('trigger-create')
        if self.fail=='trigger':raise CloudError('TriggerFailure')
        self.trigger=NS(http_trigger=NS(url_internet='https://voyager.example.com'))
        return NS(body=self.trigger)
    def update_trigger(self,*_):
        self.calls.append('trigger-update');return NS(body=self.trigger)

class AdapterTests(unittest.TestCase):
    def test_create_timeout_after_commit_and_trigger_failure_recover(self):
        for failure in ('create','trigger'):
            client=FakeFC();client.fail=failure
            adapter=AliyunFC(client,Models())
            with self.assertRaises(CloudError):adapter.apply('voyager',{},b'zip',lambda _:None)
            self.assertIsNotNone(client.function)
            client.fail=None
            self.assertEqual(adapter.apply('voyager',{},b'zip',lambda _:None),'voyager.example.com')
            self.assertEqual(client.calls.count('create'),1)
            self.assertEqual(client.calls.count('update'),1)
            adapter.apply('voyager',{},b'zip',lambda _:None)
            self.assertEqual(client.calls[-1],'trigger-update')

    def test_permission_error_must_not_be_treated_as_missing_function(self):
        client=FakeFC();client.fail='denied'
        with self.assertRaises(CloudError):AliyunFC(client,Models()).apply('voyager',{},b'zip',lambda _:None)
        self.assertEqual(client.calls,[])

    def test_stale_readback_times_out_without_creating_trigger(self):
        client=FakeFC();client.stale=True
        now=[0]
        def sleep(seconds):now[0]+=seconds
        adapter=AliyunFC(client,Models(),clock=lambda:now[0],sleep=sleep,timeout=3)
        with self.assertRaises(ValueError):adapter.apply('voyager',{},b'zip',lambda _:None)
        self.assertEqual(client.calls,['create'])
