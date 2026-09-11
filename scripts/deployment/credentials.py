"""Local credentials; never log returned values."""
from .errors import UserError
import pathlib
import json

def load_profile(alias, path=None):
    profile_path = path or pathlib.Path.home()/'.aliyun/config.json'
    if profile_path.stat().st_mode & 0o077:
        raise UserError('Alibaba CLI configuration must have mode 0600')
    profiles = json.loads(profile_path.read_text())['profiles']
    profile = next((p for p in profiles if p['name'] == alias), None)
    if not profile or profile.get('mode') != 'AK':
        raise UserError('Code deployment requires a dedicated AK CLI profile matching deployment.access')
    return profile
