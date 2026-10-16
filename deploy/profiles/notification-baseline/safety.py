#!/usr/bin/env python3
"""校验展开后的 Compose，拒绝超出通知隔离 profile 的资源连接。"""
import json
import sys


def require(condition, message):
    """即使 Python 优化模式开启，也拒绝越界配置。"""
    if not condition:
        raise ValueError(message)


def validate(config):
    """固定项目、网络与卷边界，阻断外联和宿主资源暴露。"""
    require(config['name'] == 'ming-iot-notification-baseline', 'unexpected project')
    require(set(config['networks']) == {'isolated'}, 'unexpected network')
    network = config['networks']['isolated']
    require(network.get('internal') is True and not network.get('external'), 'network must be internal')
    require(network.get('name') == 'ming-iot-notification-baseline_isolated', 'network name')
    require(set(config['volumes']) == {'postgres_postgis_data', 'nats_data', 'redis_data'}, 'unexpected volume')
    for key, volume in config['volumes'].items():
        require(not volume.get('external') and not volume.get('driver_opts'), 'external volume')
        require(volume.get('name') == 'ming-iot-notification-baseline_' + key, 'volume name')
    require(set(config['services']) == {'postgres', 'redis', 'nats', 'syssvr', 'probe'}, 'unexpected service')
    for name, service in config['services'].items():
        require(set(service.get('networks', {})) == {'isolated'}, name + ': network')
        for forbidden in ('ports', 'network_mode', 'privileged', 'devices', 'extra_hosts', 'pid', 'ipc', 'cap_add', 'volumes_from', 'build'):
            require(not service.get(forbidden), name + ': ' + forbidden)
        for volume in service.get('volumes', []):
            require(volume['type'] == 'volume' and volume['source'] in config['volumes'], name + ': mount')
        require(service.get('pull_policy') == 'never', name + ': pull policy')
        expected_images = {'postgres': 'postgis/postgis:17-3.5@sha256:30f0632e34063d8ad3897d172273c44393eb483cd2e974e472b4353df7b6d6ee', 'redis': 'redis:7.4-alpine@sha256:ff02b58f971e7d7d156a1267e283fcbbeee91773b6aa36c49dac28ecfe28eadf', 'nats': 'nats:2-alpine@sha256:c11af972c99ae542de8925e6a7d9c533aa1eb039660420d2074beed6089b3bf0', 'syssvr': 'ming-iot-notification-baseline:local', 'probe': 'ming-iot-notification-baseline:local'}
        require(service['image'] == expected_images[name], name + ': image')
        require(not service.get('env_file'), name + ': inherited environment')


if __name__ == '__main__':
    validate(json.load(sys.stdin))
    print('PASS notification isolation contract')
