"""用危险配置反例检验隔离边界；仅读取本地 Compose 渲染。"""
import copy
import json
import unittest

import run


class IsolationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.config = json.loads(run.run(run.COMPOSE + ["--profile", "verify", "config", "--format", "json"]))

    def rejects(self, mutate):
        config = copy.deepcopy(self.config)
        mutate(config)
        with self.assertRaises(AssertionError):
            run.check(config)

    def test_normal_profile(self):
        run.check(self.config)

    def test_host_port_is_not_allowed(self):
        self.rejects(lambda c: c["services"]["kafka"].update(ports=[{"published": "9092"}]))

    def test_shared_network_is_not_allowed(self):
        self.rejects(lambda c: c["networks"]["isolated"].update(external=True))

    def test_old_volume_name_is_not_allowed(self):
        self.rejects(lambda c: c["volumes"]["redis_data"].update(name="old_redis_data"))

    def test_host_mount_is_not_allowed(self):
        self.rejects(lambda c: c["services"]["cache-a"].update(volumes=[{"type": "bind", "source": "/", "target": "/host"}]))

    def test_external_broker_environment_is_not_allowed(self):
        self.rejects(lambda c: c["services"]["cache-b"].update(environment={"KAFKA_BROKERS": "remote:9092"}))

    def test_privileged_is_not_allowed(self):
        self.rejects(lambda c: c["services"]["cache-a"].update(privileged=True))


if __name__ == "__main__":
    unittest.main()
