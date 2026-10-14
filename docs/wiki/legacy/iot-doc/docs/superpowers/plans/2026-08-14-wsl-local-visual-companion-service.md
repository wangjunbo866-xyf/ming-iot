# WSL 本机常驻可视化 Companion 服务实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在当前启用 mirrored networking 与 systemd 的 WSL2 发行版运行期间，以固定地址 `http://127.0.0.1:18081` 常驻运行完整的浏览器可视化 Companion。

**Architecture:** systemd 直接托管现有 Codex visual companion runtime，使用固定的 ignored 内容目录、固定 loopback 端口和 `Restart=always`。Windows 浏览器通过 WSL mirrored localhost 访问；服务不绑定局域网接口，不接入 `ming-iot` 产品部署。

**Lifecycle boundary:** systemd service 不会保持 WSL 实例存活。WSL 被回收或关闭时固定入口不可达；下次由 Windows、VS Code 或命令拉起发行版后，enabled unit 自动启动。Windows 登录自动拉起 WSL 仍为非目标，本计划不增加 Windows 计划任务。

**Tech Stack:** WSL 2.7.3、systemd 255、Node.js `/usr/bin/node` v18.19.1、Codex visual companion、HTML、WebSocket、curl。

---

## 执行约束

- 工作区根 `AGENTS.md` 禁止未经授权执行 `git add`、`git commit` 或 `git push`；本计划不包含提交步骤。
- 用户既有的 Scene 文档修改和未跟踪计划均不得改动、暂存或清理。
- 不修改 `%USERPROFILE%\.wslconfig`、`/etc/wsl.conf`、Windows 防火墙或 `portproxy`。
- 不修改 `/root/.codex/superpowers/skills/brainstorming/scripts/` 中的 Codex runtime。
- Node 进程固定以 `wangjunbo:wangjunbo` 运行；不得以 root 身份写入用户可替换的工作区路径。
- Codex runtime 不复制、不修改。unit 使用服务私有的只读 bind mount 映射到 `/run`；路径或依赖文件漂移必须
  通过 namespace 建立失败或非忽略的 `ExecStartPre` 检查失败，使启动尝试明确失败，待人工更新，不得使用
  `Condition*` 产生 `skipped`，也不得静默换实现。`Restart=always` 受 StartLimit 限制后进入 `failed`。
- 正式仓库只保留已批准的设计与本实施计划；运行页和状态必须位于已忽略的 `.superpowers/visual-live/`。
- 宿主机文件写入使用 `apply_patch`；把既有批准 HTML 机械复制到 ignored 目录可使用 `/usr/bin/install`。
- systemd unit 属于宿主机状态，不受 Git worktree 隔离；执行前后都必须单独验证 `/etc` 目标和 Git 工作树。

## 文件与状态边界

**正式文件：**

- Existing: `docs/superpowers/specs/2026-08-14-wsl-local-visual-companion-service-design.md`
- Existing: `docs/superpowers/plans/2026-08-14-wsl-local-visual-companion-service.md`

**宿主机文件：**

- Create: `/etc/systemd/system/iot-visual-companion.service`

**Git 忽略的运行文件：**

- Create directory: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content`
- Create directory: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state`
- Create: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content/rd-graph-overview-v1.html`
- Create temporarily: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content/service-interaction-smoke-v1.html`
- Runtime-owned: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/server-info`
- Runtime-owned: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/events`

### Task 1：建立失败基线与依赖预检

**Files:**

- Read: `docs/superpowers/specs/2026-08-14-wsl-local-visual-companion-service-design.md`
- Read: `/root/.codex/superpowers/skills/brainstorming/scripts/server.cjs`
- Read: `/root/.codex/superpowers/skills/brainstorming/scripts/frame-template.html`
- Read: `/root/.codex/superpowers/skills/brainstorming/scripts/helper.js`

- [ ] **Step 1: 复核环境与 runtime 文件**

Run:

```bash
test "$(systemctl is-system-running)" = "running"
test "$(/usr/bin/node --version)" = "v18.19.1"
test -f /root/.codex/superpowers/skills/brainstorming/scripts/server.cjs
test -f /root/.codex/superpowers/skills/brainstorming/scripts/frame-template.html
test -f /root/.codex/superpowers/skills/brainstorming/scripts/helper.js
/usr/bin/node --check /root/.codex/superpowers/skills/brainstorming/scripts/server.cjs
```

Expected: 所有命令 exit 0；`node --check` 无错误输出。

- [ ] **Step 2: 验证 WSL 网络前提未漂移**

Run:

```bash
/mnt/c/Windows/System32/wsl.exe --version
/mnt/c/Windows/System32/cmd.exe /c "type %UserProfile%\\.wslconfig"
sed -n '1,80p' /etc/wsl.conf
```

Expected: WSL 仍为 2.7.3；`.wslconfig` 包含 `networkingMode=mirrored`；`/etc/wsl.conf` 包含 `systemd=true`。若任一前提变化，停止实施并回到设计评审。

- [ ] **Step 3: 记录服务尚不存在的失败基线**

Run:

```bash
systemctl cat iot-visual-companion.service
```

Expected: 非零退出，并报告 `No files found for iot-visual-companion.service` 或等价的 unit-not-found 信息。

Run:

```bash
if ss -ltn | rg -q '127\.0\.0\.1:18081\b|0\.0\.0\.0:18081\b'; then
  exit 1
fi
```

Expected: exit 0，证明固定端口尚未被占用。

### Task 2：建立稳定运行目录并放入批准的 Graph

**Files:**

- Read source: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/brainstorm/963353-1786671121/content/rd-graph-overview-v1.html`
- Create: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content/rd-graph-overview-v1.html`
- Create directory: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state`

- [ ] **Step 1: 验证批准页面源仍存在**

Run:

```bash
test -f /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/brainstorm/963353-1786671121/content/rd-graph-overview-v1.html
rg -n -m1 '可复刻自动化研发 Graph（V1）' /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/brainstorm/963353-1786671121/content/rd-graph-overview-v1.html
```

Expected: exit 0，并打印 Graph V1 标题所在行。源不存在时停止，不得用空白页冒充批准页面。

- [ ] **Step 2: 创建稳定目录**

Run:

```bash
/usr/bin/install -d -o wangjunbo -g wangjunbo -m 0755 /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live
/usr/bin/install -d -o wangjunbo -g wangjunbo -m 0755 /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content
/usr/bin/install -d -o wangjunbo -g wangjunbo -m 0700 /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state
```

Expected: exit 0；`visual-live`、`content`、`state` 均归属 `wangjunbo:wangjunbo`，模式依次为
`0755`、`0755`、`0700`。重复执行时，既有目录的 owner 与 mode 仍被收敛到这些值。

- [ ] **Step 2a: 验证运行树归属与权限边界**

Run:

```bash
stat -c '%U:%G %a %n' /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state
namei -l /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state
```

Expected: `visual-live`、`content`、`state` 均为 `wangjunbo:wangjunbo`；模式依次为 `0755`、`0755`、`0700`。
此步骤只验证、不改变 owner。页面复制后还必须验证其为 `wangjunbo:wangjunbo 0644`。`namei -l` 应证明
`visual-live` 下运行对象不依赖 root 写入。

- [ ] **Step 3: 机械复制批准页面**

Run:

```bash
/usr/bin/install -o wangjunbo -g wangjunbo -m 0644 /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/brainstorm/963353-1786671121/content/rd-graph-overview-v1.html /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content/rd-graph-overview-v1.html
cmp /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/brainstorm/963353-1786671121/content/rd-graph-overview-v1.html /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content/rd-graph-overview-v1.html
```

Expected: 两条命令 exit 0，`cmp` 无输出；副本为 `wangjunbo:wangjunbo 0644`。重复执行不得将页面恢复为
root owner。

- [ ] **Step 4: 证明运行页不会进入 Git**

Run from `/home/wangjunbo/workspace-iot-platform/iot-doc`:

```bash
git check-ignore -v .superpowers/visual-live/content/rd-graph-overview-v1.html
git status --short -- .superpowers/visual-live
```

Expected: 第一条命中 `.gitignore` 中的 `.superpowers/`；第二条无输出。

### Task 3：安装并启动 systemd unit

**Files:**

- Create: `/etc/systemd/system/iot-visual-companion.service`

- [ ] **Step 1: 使用 `apply_patch` 创建完整 unit**

Create `/etc/systemd/system/iot-visual-companion.service` with exactly:

```ini
[Unit]
Description=IoT Doc Visual Companion
Documentation=file:///home/wangjunbo/workspace-iot-platform/iot-doc/docs/superpowers/specs/2026-08-14-wsl-local-visual-companion-service-design.md
After=network.target
StartLimitIntervalSec=30
StartLimitBurst=5

[Service]
Type=simple
User=wangjunbo
Group=wangjunbo
RuntimeDirectory=iot-visual-companion-runtime
RuntimeDirectoryMode=0755
BindReadOnlyPaths=/root/.codex/superpowers/skills/brainstorming/scripts:/run/iot-visual-companion-runtime
WorkingDirectory=/run/iot-visual-companion-runtime
Environment=BRAINSTORM_DIR=/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live
Environment=BRAINSTORM_PORT=18081
Environment=BRAINSTORM_HOST=127.0.0.1
Environment=BRAINSTORM_URL_HOST=127.0.0.1
ExecStartPre=/usr/bin/test -x /usr/bin/node
ExecStartPre=/usr/bin/test -f /run/iot-visual-companion-runtime/server.cjs
ExecStartPre=/usr/bin/test -f /run/iot-visual-companion-runtime/frame-template.html
ExecStartPre=/usr/bin/test -f /run/iot-visual-companion-runtime/helper.js
ExecStartPre=/usr/bin/install -d -m 0755 /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content
ExecStartPre=/usr/bin/install -d -m 0700 /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state
ExecStartPre=/usr/bin/rm -f /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/server-stopped
ExecStart=/usr/bin/node /run/iot-visual-companion-runtime/server.cjs
Restart=always
RestartSec=2s
TimeoutStopSec=5s
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live
ProtectClock=true
ProtectControlGroups=true
ProtectKernelLogs=true
ProtectKernelModules=true
ProtectKernelTunables=true
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictNamespaces=true
RestrictRealtime=true
RestrictSUIDSGID=true
LockPersonality=true
CapabilityBoundingSet=

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: 静态校验 unit**

Run:

```bash
systemd-analyze verify /etc/systemd/system/iot-visual-companion.service
test "$(rg -c '^Condition(FileIsExecutable|PathExists)=' /etc/systemd/system/iot-visual-companion.service || printf 0)" = 0
test "$(rg -c '^ExecStartPre=/usr/bin/test -(x|f) ' /etc/systemd/system/iot-visual-companion.service)" = 4
rg -q '^ExecStartPre=/usr/bin/rm -f .*/server-stopped$' /etc/systemd/system/iot-visual-companion.service
```

Expected: 全部 exit 0，且 verify 没有 error；unit 中有 0 条 `ConditionFileIsExecutable/ConditionPathExists`、
恰好 4 条 `test` 类型 `ExecStartPre`，并使用固定显式路径的 `rm -f`。若 hardening 选项与当前 systemd
不兼容，只删除明确报错的该项并重新评审 diff，不得移除 loopback、固定端口或依赖失败检查。

缺少 Codex runtime 根目录时，`BindReadOnlyPaths` 的 namespace 建立必须失败；根目录存在但 Node 或三个依赖
文件缺失时，对应 `ExecStartPre` 必须失败。`Restart=always` 的重试受 StartLimit 限制，超过限制后 unit 进入
`failed`，不得只显示 `skipped`。

- [ ] **Step 2a: 验证服务身份与私有 runtime 映射**

Run after service starts:

```bash
systemctl show -p User -p Group iot-visual-companion.service
visual_pid="$(systemctl show -p MainPID --value iot-visual-companion.service)"
test "$visual_pid" != 0
test "$(stat -c '%u' "/proc/$visual_pid")" = 1000
systemctl show -p RuntimeDirectory -p BindReadOnlyPaths iot-visual-companion.service
```

Expected: `User` and `Group` are both `wangjunbo`; the MainPID effective uid is `1000`; `RuntimeDirectory` and
`BindReadOnlyPaths` retain the declared private `/run` runtime mapping. The original root-owned Codex runtime must
remain unmodified.

- [ ] **Step 3: 加载并启动服务**

Run:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now iot-visual-companion.service
systemctl is-enabled iot-visual-companion.service
systemctl is-active iot-visual-companion.service
```

Expected: 后两条分别输出 `enabled` 和 `active`。

- [ ] **Step 4: 检查启动日志**

Run:

```bash
journalctl -u iot-visual-companion.service -n 30 --no-pager
```

Expected: 出现 `server-started`，其中 `port=18081`、`host=127.0.0.1`、`url=http://127.0.0.1:18081`；没有启动循环或权限错误。

### Task 4：验证 WSL 与 Windows localhost

**Files:**

- Runtime-read: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/server-info`

- [ ] **Step 1: 验证只监听 loopback**

Run:

```bash
ss -ltnp | rg '127\.0\.0\.1:18081\b'
if ss -ltn | rg -q '0\.0\.0\.0:18081\b'; then
  exit 1
fi
```

Expected: 第一条显示 Node 监听 `127.0.0.1:18081`；第二条 exit 0，证明没有 wildcard 监听。

- [ ] **Step 2: 从 WSL 验证 HTTP 与 Graph 内容**

Run:

```bash
curl -fsS -o /dev/null -w 'HTTP %{http_code}\n' http://127.0.0.1:18081/
curl -fsS http://127.0.0.1:18081/ | rg -m1 '可复刻自动化研发 Graph（V1）'
```

Expected: 输出 `HTTP 200`，并打印 Graph V1 标题行。

- [ ] **Step 3: 从 Windows 网络栈验证 mirrored localhost**

Run from WSL:

```bash
/mnt/c/Windows/System32/curl.exe -sS -o NUL -w "%{http_code}\n" http://127.0.0.1:18081/
```

Expected: 输出 `200`。如果 WSL curl 成功而 Windows curl 失败，停止并记录 Windows/WSL 网络证据，不得把服务标记为完成。

- [ ] **Step 4: 验证 server-info 固定地址**

Run:

```bash
rg -n '"port":18081' /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/server-info
rg -n '"host":"127\.0\.0\.1"' /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/server-info
rg -n '"url":"http://127\.0\.0\.1:18081"' /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/server-info
```

Expected: 三个字段均命中。

### Task 5：验证自动刷新与浏览器点击事件

**Files:**

- Create temporarily: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content/service-interaction-smoke-v1.html`
- Runtime-read: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/events`

- [ ] **Step 1: 使用 `apply_patch` 新增交互测试屏**

Create `service-interaction-smoke-v1.html` with exactly:

```html
<h2>Visual Companion 常驻服务交互验收</h2>
<p class="subtitle">点击下面的选项，验证 WebSocket 与事件落盘。</p>
<div class="options">
  <div class="option" data-choice="service-ok" onclick="toggleSelect(this)">
    <div class="letter">✓</div>
    <div class="content">
      <h3>服务交互正常</h3>
      <p>固定端口页面已经自动刷新，点击事件应写入 state/events。</p>
    </div>
  </div>
</div>
```

- [ ] **Step 2: 验证文件监听与新屏切换**

Run:

```bash
journalctl -u iot-visual-companion.service --since '-2 minutes' --no-pager | rg 'screen-added.*service-interaction-smoke-v1\.html'
curl -fsS http://127.0.0.1:18081/ | rg -m1 'Visual Companion 常驻服务交互验收'
```

Expected: journal 命中新文件，HTTP 返回新屏标题。

- [ ] **Step 3: 执行一次真实浏览器点击**

Open `http://127.0.0.1:18081` in the Windows browser, click **服务交互正常**, then return to the terminal.

Expected: 页面显示所选项，终端收到用户确认。该步骤验证真实浏览器、WebSocket 和 helper script，不以人工写 events 文件代替。

- [ ] **Step 4: 验证点击事件落盘**

Run:

```bash
test -s /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/events
tail -n 1 /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/state/events | rg '"type":"click".*"choice":"service-ok"'
```

Expected: exit 0，最后一条 JSON Lines 事件包含 `type=click` 和 `choice=service-ok`。

### Task 6：验证进程故障后的自动恢复

**Files:**

- Runtime-read: systemd service state and journal only.

- [ ] **Step 1: 记录当前主进程并注入 SIGKILL**

Run:

```bash
visual_old_pid="$(systemctl show -p MainPID --value iot-visual-companion.service)"
test -n "$visual_old_pid"
test "$visual_old_pid" != "0"
sudo systemctl kill --kill-whom=main --signal=SIGKILL iot-visual-companion.service
```

Expected: 命令 exit 0；只杀死本计划创建的 service main process。

- [ ] **Step 2: 条件等待新进程**

Run:

```bash
visual_new_pid=""
for visual_attempt in $(seq 1 20); do
  visual_candidate_pid="$(systemctl show -p MainPID --value iot-visual-companion.service)"
  if test "$(systemctl is-active iot-visual-companion.service)" = "active" && test "$visual_candidate_pid" != "0" && test "$visual_candidate_pid" != "$visual_old_pid"; then
    visual_new_pid="$visual_candidate_pid"
    break
  fi
  sleep 0.5
done
test -n "$visual_new_pid"
test "$visual_new_pid" != "$visual_old_pid"
```

Expected: 10 秒内得到不同于旧 PID 的非零新 PID，命令 exit 0。

- [ ] **Step 3: 验证固定端口和内容恢复**

Run:

```bash
systemctl is-active iot-visual-companion.service
curl -fsS -o /dev/null -w 'HTTP %{http_code}\n' http://127.0.0.1:18081/
journalctl -u iot-visual-companion.service --since '-2 minutes' --no-pager | rg 'server-started'
```

Expected: 输出 `active`、`HTTP 200`，journal 显示新的 `server-started`。

### Task 7：恢复 Graph 首页并完成最终验收

**Files:**

- Create: `/home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content/rd-graph-overview-v1-live.html`
- Verify: all service, runtime and Git state.

- [ ] **Step 1: 让批准的 Graph 重新成为最新页面**

Run:

```bash
/usr/bin/install -o wangjunbo -g wangjunbo -m 0644 /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/brainstorm/963353-1786671121/content/rd-graph-overview-v1.html /home/wangjunbo/workspace-iot-platform/iot-doc/.superpowers/visual-live/content/rd-graph-overview-v1-live.html
curl -fsS http://127.0.0.1:18081/ | rg -m1 '可复刻自动化研发 Graph（V1）'
```

Expected: Graph V1 标题命中；服务根页面不再停留在 smoke screen；新页面为 `wangjunbo:wangjunbo 0644`。

- [ ] **Step 2: 运行完整服务验收**

Run:

```bash
systemctl is-enabled iot-visual-companion.service
systemctl is-active iot-visual-companion.service
systemd-analyze verify /etc/systemd/system/iot-visual-companion.service
ss -ltnp | rg '127\.0\.0\.1:18081\b'
curl -fsS -o /dev/null -w 'WSL HTTP %{http_code}\n' http://127.0.0.1:18081/
/mnt/c/Windows/System32/curl.exe -sS -o NUL -w "Windows HTTP %{http_code}\n" http://127.0.0.1:18081/
```

Expected: `enabled`、`active`、unit verify exit 0、仅 loopback 监听、WSL 与 Windows 均返回 HTTP 200。

- [ ] **Step 3: 验证正式工作树没有被运行产物污染**

Run from `/home/wangjunbo/workspace-iot-platform/iot-doc`:

```bash
git diff --check
git status --short
git status --short -- .superpowers/visual-live
```

Expected: `git diff --check` 无输出；最后一条无输出。完整 status 只包含用户既有修改、已批准设计和本实施计划，不出现 systemd unit 或 visual-live 页面。

- [ ] **Step 4: 向用户交付固定入口和运维命令**

Report:

```text
固定入口（WSL 发行版运行期间）：http://127.0.0.1:18081
查看状态：systemctl status iot-visual-companion
查看日志：journalctl -u iot-visual-companion -f
手工重启：sudo systemctl restart iot-visual-companion
```

同时明确：systemd service 不会保持 WSL 实例存活；服务只在 WSL 发行版运行期间常驻，不保证在 Windows
尚未拉起 WSL 时可用。Windows 登录自动拉起 WSL 仍为非目标；未执行 git commit。

## 回滚手册（仅在用户明确要求撤销时执行）

```bash
sudo systemctl disable --now iot-visual-companion.service
sudo /usr/bin/unlink /etc/systemd/system/iot-visual-companion.service
sudo systemctl daemon-reload
sudo systemctl reset-failed iot-visual-companion.service
```

回滚默认保留 `.superpowers/visual-live`。删除 ignored 页面与事件属于独立的破坏性操作，必须再次取得用户授权。
