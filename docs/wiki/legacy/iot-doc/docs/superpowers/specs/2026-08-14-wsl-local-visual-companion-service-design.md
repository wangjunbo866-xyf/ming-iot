# WSL 本机常驻可视化 Companion 服务设计

> 日期：2026-08-14  
> 状态：已实施并验收（WSL 发行版运行期间）  
> 范围：在当前 WSL2 发行版运行期间常驻浏览器可视化辅助服务，供 Windows 宿主机通过 localhost 访问。

## 1. 背景与目标

当前 brainstorming visual companion 由会话临时启动，使用随机端口，并在 30 分钟无访问后退出。
会话结束、owner 进程退出或 idle timeout 都可能导致浏览器链接失效，不适合作为持续评审研发 Graph
和架构图的固定入口。

当前环境已经确认：

- WSL 版本为 2.7.3；
- WSL2 内核为 6.6.114.1；
- `%USERPROFILE%\.wslconfig` 使用 `networkingMode=mirrored`；
- `/etc/wsl.conf` 已启用 systemd。

Microsoft 的 mirrored networking 约定允许 Windows 与 WSL 通过 `localhost` 互访，因此不依赖会变化的
WSL 网卡 IP，也不需要 Windows `netsh interface portproxy`。参见
[Microsoft WSL 网络说明](https://learn.microsoft.com/windows/wsl/networking)。

本设计目标如下：

1. Windows 与 WSL 均固定使用 `http://127.0.0.1:18081`；
2. companion 只监听 `127.0.0.1:18081`，不接受局域网远程连接；
3. 保留“自动显示最新页面、页面自动刷新、记录浏览器点击事件”的完整 companion 能力；
4. 由 WSL 内的 systemd 在发行版启动时自动启动，并在进程退出后自动恢复，实现 WSL 发行版运行期间常驻；
5. 不修改 `ming-iot` 的生产部署，不把临时可视化内容纳入 Git。

## 2. 非目标与安全边界

首版不包含：

- Windows `portproxy`、ECS 穿透、公网发布、域名或 HTTPS；
- 局域网其他设备访问；
- 登录认证、多人权限或审计；
- Windows 登录后主动拉起 WSL 的计划任务；
- Docker 镜像和 `ming-iot/deploy/docker-compose.yml` 集成；
- 将 companion 升格为 `ming-iot` 产品组件；
- 自研或长期维护一份 companion server 实现。

服务仅监听 WSL loopback。Windows 宿主机依靠 mirrored networking 使用同一个 localhost 访问；局域网
其他设备不能直接连接。页面中仍不得放置密码、Token、私钥、生产凭据或其他敏感内容。

systemd unit 的“enabled”含义是：WSL 发行版启动后自动启动服务。Microsoft WSL 官方文档明确说明，
systemd service 不会保持 WSL 实例存活；WSL 被 Windows 回收或主动关闭后，固定入口不可达。下次由
Windows、VS Code 或命令重新拉起该发行版时，enabled unit 才会自动启动并恢复入口。因此本设计只承诺
WSL 发行版运行期间常驻，不保证 Windows 刚开机、WSL 尚未被拉起时服务已经存在。Windows 登录后自动拉起 WSL 仍属于非目标，不增加 Windows 计划任务。

## 3. 方案选择

首版选择“systemd 托管现有 Codex visual companion runtime”。与其他方案相比：

- 相比静态 HTTP 服务，它保留最新页面切换、WebSocket 自动刷新和点击事件；
- 相比自研项目内服务，它不引入重复实现和维护成本；
- 相比 Docker 化，它不增加镜像、Compose 和挂载管理。

代价是服务依赖当前 Codex 安装路径。该依赖属于 WSL 宿主开发工具依赖，不进入产品依赖图；Codex
安装位置变化后，服务应显式失败并更新 unit，不得静默切换到未知实现。这里的“显式失败”不是
`Condition*` 不满足产生的 `skipped`：只读 namespace 无法建立或四条 `ExecStartPre=/usr/bin/test ...`
任一失败都会使启动尝试失败；`Restart=always` 重试受 `StartLimitIntervalSec` 和 `StartLimitBurst` 限制，
达到限制后 unit 进入 `failed`。

Node 进程固定以 `wangjunbo:wangjunbo` 运行，避免以 root 身份写入位于用户工作区、其祖先目录可被用户
替换的路径。Codex runtime 仍由 root 安装并保持原样；systemd 通过服务私有的只读 bind mount 将其映射到
`/run/iot-visual-companion-runtime`。该 `/run` 映射只在本服务的 mount namespace 中可见，服务停止后随
`RuntimeDirectory` 消失。Codex 升级若使原路径或三个依赖文件变化，unit 必须明确失败，待人工复核并更新；
不得复制 runtime 或降级为未知实现。

## 4. 运行架构

以下入口只在 WSL 发行版运行期间存在；systemd 不负责维持 WSL 实例本身的生命周期。

```text
Windows 浏览器
  │ http://127.0.0.1:18081
  │ WSL mirrored localhost
  ▼
WSL 127.0.0.1:18081
  ▼
iot-visual-companion.service
  │ wangjunbo 进程、固定环境变量与固定端口
  ▼
服务私有 /run/iot-visual-companion-runtime/server.cjs（只读映射自 Codex runtime）
  ├── 读取 / 监听 .superpowers/visual-live/content/*.html
  ├── WebSocket 通知浏览器刷新
  └── 写入 .superpowers/visual-live/state/events
```

固定目录如下：

```text
/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/
├── content/     # AI 写入的新页面；服务展示修改时间最新的 HTML
└── state/       # server-info、server-stopped 和浏览器点击事件
```

`.superpowers/` 已被 `iot-doc/.gitignore` 忽略，因此页面和运行状态不会污染正式文档工作树。
`visual-live`、`content`、`state` 及其中页面均归属 `wangjunbo:wangjunbo`；模式分别固定为
`0755`、`0755`、`0700`、`0644`，使服务用户可写运行状态而不需要 root 权限。

## 5. systemd 服务契约

系统服务名固定为 `iot-visual-companion.service`，unit 位于 `/etc/systemd/system/`。运行参数固定为：

| 参数 | 值 | 含义 |
| --- | --- | --- |
| `BRAINSTORM_DIR` | `.../iot-doc/.superpowers/visual-live` | 稳定内容与状态目录 |
| `BRAINSTORM_PORT` | `18081` | 固定端口 |
| `BRAINSTORM_HOST` | `127.0.0.1` | 只监听 WSL loopback |
| `BRAINSTORM_URL_HOST` | `127.0.0.1` | Windows 与 WSL 的稳定展示地址 |

不设置 `BRAINSTORM_OWNER_PID`，避免会话进程结束导致服务退出。服务配置必须包含：

- `Restart=always` 和短暂 `RestartSec`，使 idle timeout、异常退出和人工杀进程后自动恢复；
- 固定端口，重启后 URL 不变；
- 不使用 `ConditionFileIsExecutable` 或 `ConditionPathExists` 表达强依赖，避免依赖缺失时只得到
  `skipped`；启动前用四条非忽略失败的 `ExecStartPre=/usr/bin/test ...` 检查 Node 可执行文件以及
  `server.cjs`、frame template、helper script；
- 固定 `User=wangjunbo`、`Group=wangjunbo`；运行目录由该用户写入，避免 root 写入用户可替换路径；
- 固定 `RuntimeDirectory=iot-visual-companion-runtime`、`RuntimeDirectoryMode=0755`，并以
  `BindReadOnlyPaths=/root/.codex/superpowers/skills/brainstorming/scripts:/run/iot-visual-companion-runtime`
  映射未复制、未修改的 Codex runtime；`WorkingDirectory=/run/iot-visual-companion-runtime`，
  `ExecStart=/usr/bin/node /run/iot-visual-companion-runtime/server.cjs`；
- 启动前以固定显式路径执行 `/usr/bin/rm -f .../state/server-stopped`，安静清理上一次运行留下的
  marker，但保留 HTML 和点击事件；
- Codex runtime 根目录缺失时，`BindReadOnlyPaths` 的 namespace 建立失败；目录存在但依赖文件缺失时，
  对应 `ExecStartPre` 失败。两种情况都属于启动失败，`Restart=always` 重试受 StartLimit 限制后进入
  `failed`，不得退化成 `skipped` 或无限快速重启；
- `NoNewPrivileges`、私有临时目录、内核与设备保护等不影响 Node 运行的 systemd hardening。

现有 server 在 30 分钟无请求后会正常退出。首版不修改 Codex runtime；systemd 在同一端口自动重启，
因此在 WSL 发行版运行期间入口保持稳定，最多出现一次短暂重连。浏览器重新连接后继续展示稳定目录中的
最新页面。WSL 实例停止后 systemd 也随之停止，不能把进程级自动恢复解释为 Windows 开机级常驻。

## 6. 页面发布与交互流程

1. AI 在 `content/` 中使用新的语义化文件名写入 HTML，不覆盖既有页面；
2. server 的文件监听器发现新页面，清空上一屏的临时点击事件并广播 reload；
3. Windows 浏览器自动加载修改时间最新的页面；
4. 用户点击选项后，事件以 JSON Lines 写入 `state/events`；
5. AI 在下一轮读取事件，并以终端文字反馈为主、点击事件为辅继续设计；
6. 已确认的正式图另行写入 `iot-doc` 正式文档，不能把 `.superpowers` 当作长期规范真源。

## 7. 故障处理与运维

| 故障 | 行为 |
| --- | --- |
| companion idle timeout | systemd 在固定端口自动重启 |
| Node 进程异常退出 | systemd 自动重启并保留页面 |
| `18081` 被占用 | 启动失败并在 journal 中报告，不改用随机端口 |
| Codex runtime 根路径不存在 | 只读 namespace 建立失败；重试受 StartLimit 限制后 unit 进入 `failed` |
| Node 或 runtime 依赖文件不存在 | 四条 `ExecStartPre` 中对应检查失败；重试受 StartLimit 限制后 unit 进入 `failed`，而不是 `skipped` |
| WSL 发行版重启 | 下次发行版被拉起后，enabled unit 随 systemd 启动并继续使用 `127.0.0.1:18081` |
| WSL 实例被回收或关闭 | systemd service 不会保持 WSL 实例存活，固定入口暂时不可达；等待 Windows、VS Code 或命令再次拉起发行版 |
| `.wslconfig` 改回 NAT | 停止依赖本设计，重新验证 Windows 到 WSL 的访问方式 |
| 页面损坏 | 推送一个新文件恢复；不原位覆盖旧页面 |

WSL 发行版运行期间，常用运维入口为 `systemctl status iot-visual-companion`、
`sudo systemctl restart/stop iot-visual-companion` 和对应的 `journalctl` 日志。停用服务时只停止并禁用
unit，不自动删除 `.superpowers/visual-live` 中的页面和事件。Windows 登录自动拉起 WSL 不在本设计范围内。

## 8. 验收标准

实施完成必须提供以下新鲜证据：

1. WSL 发行版运行期间，unit 为 `enabled` 且 `active (running)`；
2. WSL 中 `ss` 显示 `127.0.0.1:18081` 正在监听，且没有 `0.0.0.0:18081`；
3. WSL 中 `curl http://127.0.0.1:18081/` 返回 HTTP 200；
4. Windows `curl.exe http://127.0.0.1:18081/` 返回 HTTP 200，证明 mirrored localhost 可达；
5. 返回页面包含已批准的“可复刻自动化研发 Graph（V1）”标记；
6. 写入一个新屏幕后，journal 出现 `screen-added`，浏览器自动刷新；
7. 浏览器点击后，`state/events` 出现对应 JSON 事件；
8. 杀死 Node 主进程后，systemd 自动拉起新 PID，固定 URL 仍返回 HTTP 200；
9. `git status --short` 证明除本设计文档、实施计划和用户既有改动外，没有新增正式项目文件。

## 9. 回滚

如需撤销，使用 `sudo systemctl disable --now iot-visual-companion.service` 停止并禁用服务，使用
`sudo /usr/bin/unlink /etc/systemd/system/iot-visual-companion.service` 移除 unit，再执行
`sudo systemctl daemon-reload` 与 `sudo systemctl reset-failed iot-visual-companion.service`。
默认保留被 Git 忽略的 `visual-live` 内容；只有用户明确要求时才删除这些页面和事件。
