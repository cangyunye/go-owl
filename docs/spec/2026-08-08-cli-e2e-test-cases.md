# owl CLI 全功能 E2E 测试用例

> 日期：2026-08-08
> 状态：待评审（评审通过后在树莓派节点执行）
> 目标节点：`raspberrypi-kali`（kali@192.168.31.100，真实 SSH）
> 适用：`owl` CLI 二进制（`go build -o owl.exe ./cmd/cli`）

## 一、测试环境与前置条件

| 项 | 值 |
|---|---|
| 测试机 | Windows（本机），CLI 二进制与目标同网段 |
| 目标节点 1 | `raspberrypi-kali`：192.168.31.100:22，user=kali（密码，sudo） |
| 目标节点 2 | `wsl-kube`（WSL Ubuntu）：172.20.214.44:2222，user=kube（**密钥认证**） |
| 目标节点 3-5 | `raspberrypi-e2e-1/2/3`：192.168.31.100:22，user=e2e1/2/3（密码，同一物理机，由 `create-e2e-users.yaml` playbook 创建） |
| 节点数据目录 | `~/.owl/owl.db` |
| AI 配置 | `~/.owl/config.yaml`（provider=deepseek, model=deepseek-chat）+ 环境变量 `OWL_API_KEY` |
| 构建 | `go build -o owl ./cmd/cli`，并确保当前目录存在 `owl` 可执行文件（AI 工具的子进程调用依赖 `getOwlPath`） |
| 管道中文 | Windows PowerShell 5.1 需 `$OutputEncoding = [System.Text.Encoding]::UTF8` 再喂中文给 stdin |

### 测试拓扑注意

- `raspberrypi-e2e-1/2/3` 是**同一台物理机**（树莓派）上的 3 个 SSH 用户，**共享 /tmp**：
  首个用户创建的文件（644）其他用户无法覆盖，`scp`/`cat >` 会报「权限不够」——这是 POSIX 文件属主语义，**不是 owl 缺陷**。
  验证扩散传输时，目标节点应覆盖**不同物理机**（如 wsl-kube + raspberrypi-e2e-1 + raspberrypi-e2e-2），或每次用唯一文件名。
- 本机（Windows）**未安装 rsync**：密钥认证节点会先尝试 rsync 再降级 scp（已修复，见 commit 7598aad）。
- gscp 中继工具无 linux-arm64 预编译产物，扩散中继会提示「部署失败→降级直传」，属预期路径。

### 数据隔离约定

- 所有 E2E 产生的临时节点统一命名 `t-e2e-*`，用例结束后删除；
- 远程文件统一放 `/tmp/owl-e2e/`，用例结束后删除；
- playbook 统一放 `~/.owl/playbooks/`（避免污染仓库 `./playbooks`）；
- settings 修改用例必须还原现场；
- AI 会话用例相互独立，退出方式 `quit`。

### 用例格式

`[自动化]`：可用脚本/命令断言执行；`[手工]`：需人工交互确认。

---

## 二、node — 节点管理（E2E-NODE-*）

### E2E-NODE-001 添加节点 [自动化]
前置：节点 `t-e2e-01` 不存在
步骤：
1. `owl node add t-e2e-01 -n t-e2e-01 -a 127.0.0.1 -p 22 -u root --label env=e2e`
2. `owl node list --no-color`
预期：列表包含 `t-e2e-01`，标签 `env=e2e`
清理：`owl node remove t-e2e-01`

### E2E-NODE-002 列出节点与过滤 [自动化]
步骤：
1. `owl node list --format json --no-color`
2. `owl node list --status online --no-color`
3. `owl node list --label env=e2e --no-color`
预期：① JSON 合法且含目标节点；② online 行存在；③ 仅命中带标签节点
前置：NODE-001 添加的节点保留

### E2E-NODE-003 节点状态 [自动化]
步骤：
1. `owl node status --all --no-color`
2. `owl node status raspberrypi-kali --no-color`
预期：输出含所有节点；② 含 raspberrypi-kali 行

### E2E-NODE-004 更新节点 [自动化]
步骤：
1. `owl node add t-e2e-01 -n t-e2e-01 -a 127.0.0.1 -p 22 -u root`
2. `owl node update t-e2e-01 --name t-e2e-01-renamed --label env=prod`
3. `owl node list --no-color`
预期：列表出现 `t-e2e-01-renamed`，标签 `env=prod`
清理：`owl node remove t-e2e-01-renamed`

### E2E-NODE-005 删除节点 [自动化]
步骤：
1. `owl node add t-e2e-02 -n t-e2e-02 -a 127.0.0.1 -p 22 -u root`
2. `owl node remove t-e2e-02`
3. `owl node list --no-color`
预期：删除成功，列表不再包含 t-e2e-02

### E2E-NODE-006 SSH 连通性检查 [自动化]
步骤：
1. `owl node check raspberrypi-kali --no-color`
2. `owl node check --all --no-color`
预期：raspberrypi-kali 标记成功（✓ / online）；check 后可再次 `node status` 验证状态刷新为 online

### E2E-NODE-007 Ping [自动化]
步骤：
1. `owl node ping raspberrypi-kali --count 2 --timeout 3s --no-color`
预期：显示延迟/可达结果，退出码 0

### E2E-NODE-008 分组管理 [自动化]
步骤：
1. `owl node groups add raspberrypi-kali e2e-group`
2. `owl node groups show e2e-group --no-color`
3. `owl node groups list --no-color`
4. `owl node groups remove raspberrypi-kali e2e-group`
预期：① 成功；② 组内含 raspberrypi-kali；③ 列表含 e2e-group；④ 移除后组为空

### E2E-NODE-009 标签管理 [自动化]
步骤：
1. `owl node labels set raspberrypi-kali env=e2e tier=test`
2. `owl node labels show raspberrypi-kali --no-color`
3. `owl node labels remove raspberrypi-kali tier`
4. `owl node labels show raspberrypi-kali --no-color`
预期：② 含 env=e2e 与 tier=test；④ 仅剩 env=e2e
清理：`owl node labels remove raspberrypi-kali env`

### E2E-NODE-010 导入节点 [自动化]
步骤：
1. 准备 `t-e2e-import.yaml`：
   ```yaml
   nodes:
     - id: t-e2e-imp
       name: t-e2e-imp
       address: 127.0.0.1
       port: 22
       user: root
   ```
2. `owl node import --file t-e2e-import.yaml --no-color`
3. `owl node list --no-color`
预期：t-e2e-imp 出现在列表
清理：`owl node remove t-e2e-imp` 并删除 yaml

### E2E-NODE-011 导出节点 [自动化]
步骤：
1. `owl node export --format json --file t-e2e-export.json --no-color`
2. 校验文件存在且 JSON 合法、含 raspberrypi-kali
3. `owl node export --format yaml --no-color`
预期：文件输出与 stdout 输出均有效
清理：删除 t-e2e-export.json

### E2E-NODE-012 示例数据生成 [自动化]
步骤：
1. `owl node sample --no-color`（确认参数：可能需指定数量）
2. `owl node list --no-color`
预期：节点数量显著增加（50 个），分组含 web/db/cache 等
清理：`owl node list --no-color` 提取 `t-e2e`/sample 前缀节点后 `owl node remove` 批量删除（**标注：执行后必须清理，否则污染后续用例**）

---

## 三、exec — 命令执行（E2E-EXEC-*）

### E2E-EXEC-001 基本执行 [自动化]
步骤：`owl exec run "echo owl-e2e-$(date +%s)" --nodes raspberrypi-kali --no-color`
预期：输出含 `owl-e2e-`，节点状态成功

### E2E-EXEC-002 执行模式与过滤 [自动化]
步骤：
1. `owl exec run "hostname" --nodes raspberrypi-kali --serial --no-color`
2. `owl exec run "hostname" --nodes raspberrypi-kali --format json --no-color`
预期：① serial 输出；② JSON 结构含 node/status/output 字段

### E2E-EXEC-003 异步执行 [自动化]
步骤：
1. `owl exec run "sleep 3 && echo done" --nodes raspberrypi-kali --async --no-color`
预期：返回 task-id（不阻塞等待）
关联：配合 E2E-ASYNC-* 验证任务流转

### E2E-EXEC-004 脚本执行 [自动化]
步骤：
1. 本地写脚本 `t-e2e.sh`：`#!/bin/bash\necho "script-ok-$(hostname)"`
2. `owl exec script t-e2e.sh --nodes raspberrypi-kali --no-color`
预期：输出 `script-ok-raspberrypi-kali`
清理：删除 t-e2e.sh

---

## 四、file — 文件传输（E2E-FILE-*）

### E2E-FILE-001 上传 [自动化]
步骤：
1. 本地写文件 `t-e2e-up.txt` 内容 `owl-file-e2e`
2. `owl file upload t-e2e-up.txt --nodes raspberrypi-kali --dest /tmp/owl-e2e --no-color`
3. `owl exec run "cat /tmp/owl-e2e/t-e2e-up.txt" --nodes raspberrypi-kali --no-color`
预期：③ 输出 `owl-file-e2e`

### E2E-FILE-002 下载 [自动化]
步骤：
1. 前置：FILE-001 文件在远程
2. `owl file download /tmp/owl-e2e/t-e2e-up.txt --nodes raspberrypi-kali --dest ./t-e2e-dl --no-color`
3. 对比本地 `t-e2e-dl/t-e2e-up.txt` 内容
预期：内容一致（md5 相同）
清理：删除 t-e2e-dl

### E2E-FILE-003 多节点扩散传输（diffusion）[自动化]
前置：
- 可用节点 ≥2（跨物理机最佳）：`wsl-kube` + `raspberrypi-e2e-1` + `raspberrypi-e2e-3`
- 远端目录已创建并可写：`owl exec run "mkdir -p /tmp/owl-e2e" --nodes wsl-kube,raspberrypi-e2e-1,raspberrypi-e2e-3`
步骤：
1. 本地写文件 `t-diff-<毫秒时间戳>.txt`（**唯一文件名**，避免覆盖残留文件）
2. `owl file transfer t-diff-<ts>.txt --nodes wsl-kube,raspberrypi-e2e-1,raspberrypi-e2e-3 --dest /tmp/owl-e2e --threshold 1 --fan-out 2 --source-count 1`
预期：
- 输出「模式: 扩散传输 (fan-out=2, threshold=1)」+ 扩散树（源节点 → 子节点）
- 密钥节点可先见 rsync 提示（本机无 rsync 时自动降级 scp，commit 7598aad）
3. 验证：`owl exec run "cat /tmp/owl-e2e/t-diff-<ts>.txt" --nodes wsl-kube,raspberrypi-e2e-1,raspberrypi-e2e-3`
预期：**全部节点输出文件内容一致**（cat 全部成功即通过；transfer 自身报告含共享 /tmp 的 EACCES 假象时以 cat 为准）
清理：`owl exec run "rm -f /tmp/owl-e2e/t-diff-*.txt" --nodes ...`（注意用单文件模式避免触发 `rm -rf /` 黑名单）

### E2E-FILE-004 同机多用户覆盖语义（边界说明）[条件]
说明：`raspberrypi-e2e-1/2` 传输到**同一路径**时，后写者会因文件属主（644）报「权限不够」——验证该行为符合预期即可，不作为缺陷。跨物理机场景不存在此现象。

---

## 五、playbook — 剧本（E2E-PB-*）

### E2E-PB-001 列出剧本 [自动化]
步骤：`owl playbook list --no-color`
预期：列出默认库 `~/.owl/playbooks` 下的剧本

### E2E-PB-002 验证剧本 [自动化]
步骤：
1. 写合法 `t-e2e-ok.yaml`：
   ```yaml
   name: e2e-ok
   hosts: [raspberrypi-kali]
   tasks:
     - name: say
       action: command
       args: {cmd: "echo pb-ok"}
   ```
2. 写非法 `t-e2e-bad.yaml`（缺失 tasks）
3. `owl playbook validate t-e2e-ok.yaml --no-color`
4. `owl playbook validate t-e2e-bad.yaml --no-color`
预期：③ 通过；④ 报错

### E2E-PB-003 运行剧本 [自动化]
步骤：
1. 前置：PB-002 合法剧本
2. `owl playbook run t-e2e-ok.yaml --no-color`
3. `owl playbook state list --playbook e2e-ok --no-color`
预期：② 输出 `pb-ok`；③ 状态为成功/success

### E2E-PB-004 剧本运行状态 [自动化]
步骤：
1. 前置：PB-003 已产生 run-id
2. `owl playbook state show <run-id> --no-color`
预期：展示每节点每任务结果、退出码

### E2E-PB-005 模板列表/详情/导出 [自动化]
步骤：
1. `owl playbook template list --no-color`
2. `owl playbook template info e2e-tpl --no-color`（若不存在则先执行 PB-006 创建）
3. `owl playbook template export e2e-tpl --to ./t-e2e-tpl.yaml --no-color`
预期：① 列表；② 详情；③ 导出文件存在

### E2E-PB-006 模板实例化 new [自动化]
步骤：`owl playbook new --from e2e-tpl --var app_version=1.0 -o ./t-e2e-new.yaml --no-color`
预期：生成含变量替换的剧本文件

### E2E-PB-008 骨架生成 scaffold [自动化]
步骤：`owl playbook scaffold --type basic --no-color`
预期：输出 basic 骨架 YAML（header + echo hello 任务）

---

## 六、settings — 设置（E2E-SET-*）

### E2E-SET-001 查看设置 [自动化]
步骤：`owl settings show --no-color`
预期：输出当前配置（output/default/target 段）

### E2E-SET-002 修改并还原 [自动化]
步骤：
1. `owl settings set output.format json`
2. `owl settings show --no-color` 验证 format=json
3. `owl settings set output.format table`（还原）
预期：② 生效；③ 还原成功

### E2E-SET-003 非法设置值 [自动化]
步骤：`owl settings set output.format xml`
预期：报错 `invalid format`，退出码非 0，现场未被修改

### E2E-SET-004 默认目标 [自动化]
步骤：
1. `owl settings set target.nodes raspberrypi-kali`
2. `owl exec run "hostname" --no-color`（不带 --nodes）
预期：默认命中 raspberrypi-kali
清理：`owl settings set target.nodes ""`（或还原原值）

### E2E-SET-005 设置模板帮助 [自动化]
步骤：`owl settings template --no-color`
预期：列出全部可用 key 与格式说明

---

## 七、async — 异步任务（E2E-ASYNC-*）

### E2E-ASYNC-001 异步任务全流程 [自动化]
步骤：
1. `owl exec run "sleep 3 && echo async-done" --nodes raspberrypi-kali --async --no-color` → 记录 task-id
2. `owl async list --no-color`：任务出现在列表
3. `owl async status <task-id> --no-color`：状态 running/pending
4. `owl async wait <task-id> --no-color`：等待完成后输出结果
预期：最终状态成功、结果含 `async-done`

### E2E-ASYNC-002 取消任务 [自动化]
步骤：
1. `owl exec run "sleep 60" --nodes raspberrypi-kali --async --no-color` → task-id
2. `owl async cancel <task-id> --no-color`
3. `owl async status <task-id> --no-color`
预期：③ 状态为 canceled

### E2E-ASYNC-003 清理过期任务 [自动化]
步骤：`owl async cleanup --no-color`
预期：清理完成提示，残留任务移除

---

## 八、history — 执行历史（E2E-HIST-*）

### E2E-HIST-001 历史查询 [自动化]
前置：已执行过 ≥1 次 exec（如 EXEC-001）
步骤：
1. `owl history --no-color`
2. `owl history --node-id raspberrypi-kali --no-color`
3. `owl history --op-type exec --format json --no-color`
预期：① 有记录；② 过滤命中；③ JSON 合法

### E2E-HIST-002 清理历史 [自动化]
步骤：`owl history clean --days 1 --force --no-color`
预期：清理成功；`owl history --no-color` 记录减少/为空

---

## 九、session — SSH 会话（E2E-SESS-*）

### E2E-SESS-001 会话列表 [自动化]
步骤：`owl session list --no-color`
预期：列出（可能为空）会话记录

### E2E-SESS-002 会话历史 [自动化]
步骤：`owl session history --no-color`
预期：输出会话历史或空提示

### E2E-SESS-003 交互式 attach [手工]
步骤：`owl session attach raspberrypi-kali` → 进入远程 shell → `echo attached` → `exit`
预期：进入 kali 的 shell，命令执行成功
说明：全屏交互，需人工评审时确认；自动化跳过

---

## 十、ai — 智能助手（E2E-AI-*）

> 前置：`~/.owl/config.yaml` 已配置 deepseek；环境变量 `OWL_API_KEY`；`owl` 可执行文件在当前目录（工具子进程调用）。

### E2E-AI-001 单次查询节点 [自动化]
步骤：`owl ai "列出所有节点"`
预期：返回节点列表（含 raspberrypi-kali）

### E2E-AI-002 单次模式写操作拒绝 [自动化]
步骤：`owl ai "在 raspberrypi-kali 上执行 uptime"`
预期：返回「该操作（execute_command）需要交互确认，请进入交互模式执行」；**未实际执行任何命令**

### E2E-AI-003 交互查询 [自动化]
步骤：管道输入 `列出所有节点\nquit\n`
预期：AI 返回节点表格，随后退出

### E2E-AI-004 确认门：确认后执行 [自动化]
步骤：管道输入 `在 raspberrypi-kali 上执行 uptime\n是\nquit\n`
预期：① 出现「即将执行：execute_command(...) 是否继续？」；② 输入「是」后返回**真实 uptime 输出**（`up x days, load average...`）
验证：输出含树莓派 load average，确认非 mock（mock 输出为 `✅ [x] 成功`）

### E2E-AI-005 确认门：取消 [自动化]
步骤：管道输入 `在 raspberrypi-kali 上执行 df -h\n否\nquit\n`
预期：返回「已取消该操作」，无命令执行

### E2E-AI-006 会话记忆追问 [自动化]
步骤：管道输入 `在 raspberrypi-kali 上执行 hostname\n是\n刚才那个操作结果如何\nquit\n`
预期：追问轮能引用之前的操作记录（输出含 hostname 或执行摘要）

### E2E-AI-007 文件下载意图 [自动化]
步骤：管道输入 `把 /etc/hostname 从 raspberrypi-kali 下载到本地\n是\nquit\n`
预期：确认后执行 `owl file download`，本地出现 hostname 文件，内容为 `raspberrypi-kali`
清理：删除下载文件

### E2E-AI-008 生成剧本并保存 [自动化]
步骤：管道输入 `生成一个安装 nginx 的 playbook 并保存\n是\nquit\n`
预期：确认后保存到 `~/.owl/playbooks/install-nginx.yaml`，返回路径
清理：删除生成的剧本（保留亦可）

### E2E-AI-009 不支持功能提示 [自动化]
步骤：`owl ai "使用 metrics 监控节点"`（或交互输入）
预期：返回「该功能不支持 AI 操作」或路由到通用工具目录后的合理拒绝

### E2E-AI-010 模型列表 [自动化]
步骤：`owl ai models --provider deepseek`
预期：列出 deepseek 可用模型
说明：依赖 API key 与网络

### E2E-AI-011 AI 配置管理 [自动化]
步骤：
1. `owl ai config init --provider deepseek --model deepseek-chat`（或交互）
2. `owl ai config show`
预期：② 显示当前配置
说明：若 init 为交互向导则标注手工；完成后需还原配置文件

### E2E-AI-012 AI 会话历史 [自动化]
前置：已产生 ≥1 次 AI 对话（AI-003/004）
步骤：
1. `owl ai history list --limit 10`
2. `owl ai history show <session-id>`
3. `owl ai history clean --days 30`
预期：① 有会话记录；② 展示步骤明细；③ 清理成功

---

## 十二、边界与异常（E2E-EDGE-*）

### E2E-EDGE-001 未知命令 [自动化]
步骤：`owl no-such-command`
预期：报错 unknown command，退出码非 0

### E2E-EDGE-002 帮助信息 [自动化]
步骤：`owl --help`、`owl node --help`、`owl exec --help`、`owl playbook --help`、`owl ai --help`
预期：各命令显示子命令与 flags

### E2E-EDGE-003 必填参数缺失 [自动化]
步骤：`owl node add`（缺 name/address）
预期：报错 required flag，退出码非 0

### E2E-EDGE-004 不存在的节点 [自动化]
步骤：`owl exec run "hostname" --nodes no-such-node --no-color`
预期：节点选择失败提示，无执行

### E2E-EDGE-005 断网/不可达节点 [自动化]
前置：添加一个不可达临时节点 `t-e2e-dead`（10.255.255.1:22）
步骤：`owl node check t-e2e-dead --no-color`、`owl exec run "hostname" --nodes t-e2e-dead --no-color`
预期：check 失败标记、exec 超时/失败提示
清理：`owl node remove t-e2e-dead`

---

## 十三、执行顺序与回归矩阵

建议执行顺序（避免数据互相污染）：

```
1. E2E-EDGE-*（命令可用性冒烟）
2. E2E-NODE-*（增删改查 → 分组标签 → 导入导出 → sample）
3. E2E-EXEC-*（基本 → 模式 → 异步）
4. E2E-ASYNC-*（承接 EXEC-003）
5. E2E-FILE-*（上传 → 下载 → transfer）
6. E2E-PB-*（validate → run → state → template 系列）
7. E2E-HIST-*（承接 EXEC 记录）
8. E2E-SET-*（**放在最后阶段，避免影响其他用例默认值**）
9. E2E-SESS-*
10. E2E-AI-*（独立，随时可跑）
```

| 模块 | 用例数 | 自动化 | 手工/条件 |
|---|---|---|---|
| node | 12 | 12 | sample 需清理 |
| exec | 4 | 4 | — |
| file | 4 | 4 | — |
| playbook | 7 | 7 | — |
| settings | 5 | 5 | — |
| async | 3 | 3 | — |
| history | 2 | 2 | — |
| session | 3 | 2 | attach 手工 |
| ai | 12 | 12 | — |
| edge | 5 | 5 | — |
| **合计** | **57** | **56** | **1** |

## 十四、评审确认点

1. **E2E-NODE-012**：`owl node sample` 是否生成 50 个固定前缀节点，清理策略是否可接受；
2. **E2E-FILE-003/004**：扩散传输已在真实环境验证（wsl-kube + raspberrypi-e2e-1/3，`--threshold 1`），同机多用户共享 /tmp 的 EACCES 行为按预期处理；
3. **E2E-AI-008**：生成的 nginx playbook 是否允许保留在 `~/.owl/playbooks`（影响后续 PB 用例列表预期）；
4. AI 用例统一使用 `owl` 子进程（getOwlPath 依赖当前目录 `owl` 可执行文件），评审是否改为主机 PATH 安装。
