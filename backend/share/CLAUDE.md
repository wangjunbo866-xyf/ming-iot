# Share 仓库

> 公共基础库，承载缓存、上下文、错误码、事件总线、存储等跨模块共享能力。

## 仓库信息

| 项目 | 值 |
|------|-----|
| 远程 | `git@codeup.aliyun.com:642f7dca8b47795dae985084/ee/share.git` |
| 当前分支 | `saas`（最新） |
| 模块 | `gitee.com/ming-iot/share` |
| Go 版本 | 1.25.4 |

## 关键目录

```
share/
├── cache/          # 缓存封装（Redis）
├── clients/        # 外部客户端封装
├── conf/           # 配置加载
├── containers/     # 容器工具
├── ctxs/           # 上下文封装（租户、用户、权限）
├── def/            # 常量与枚举定义
├── errors/         # 错误码体系
├── event/          # 事件总线（NATS）
├── oss/            # 对象存储封装
├── stores/         # 数据存储封装
├── tests/          # 测试辅助工具
└── utils/          # 通用工具函数
```

## 依赖升级顺序

Share 是依赖链最底层，升级后需依次打 tag：
```text
share -> core -> things -> things-ee -> app
```

## 常用操作

```bash
# 查看最新 tag
git tag --sort=-creatordate | head -5

# 打新 tag
git tag v1.5.xx
git push origin v1.5.xx
git push gitee v1.5.xx
```
