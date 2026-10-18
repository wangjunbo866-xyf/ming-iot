"""验证隔离合同能拒绝端口、外部网络和宿主挂载反例。"""
import copy
import json
import subprocess
import unittest
from pathlib import Path
from safety import validate


class IsolationTest(unittest.TestCase):
    """对真实展开配置注入越界资源并要求失败。"""
    @classmethod
    def setUpClass(cls):
        """读取固定 profile，不启动容器。"""
        profile = Path(__file__).resolve().parent
        cls.config = json.loads(subprocess.check_output(['docker', '--host', 'unix:///var/run/docker.sock', 'compose', '--env-file', '/dev/null', '-f', str(profile / 'compose.yaml'), '--profile', 'verify', 'config', '--format', 'json']))

    def test_baseline(self):
        """合法 profile 必须通过。"""
        validate(self.config)

    def test_local_volume_driver(self):
        cfg = copy.deepcopy(self.config)
        cfg['volumes']['postgres_postgis_data']['driver'] = 'local'
        validate(cfg)

    def test_reject_external_volume_driver(self):
        cfg = copy.deepcopy(self.config)
        cfg['volumes']['postgres_postgis_data']['driver'] = 'remote-storage'
        with self.assertRaises(ValueError):
            validate(cfg)

    def test_reject_unsafe_resources(self):
        """四种实际危险配置必须被合同阻断。"""
        for mutation in ('port', 'external', 'bind', 'volume'):
            with self.subTest(mutation=mutation):
                cfg = copy.deepcopy(self.config)
                if mutation == 'port': cfg['services']['syssvr']['ports'] = [{'published': '9180'}]
                if mutation == 'external': cfg['networks']['isolated']['internal'] = False
                if mutation == 'bind': cfg['services']['syssvr']['volumes'] = [{'type': 'bind', 'source': '/var/run/docker.sock'}]
                if mutation == 'volume': cfg['volumes']['postgres_postgis_data']['name'] = 'existing-production'
                with self.assertRaises(ValueError): validate(cfg)
