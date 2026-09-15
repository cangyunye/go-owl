# owl-serve Web 控制台图解

本文档通过截图介绍 owl-serve Web 控制台（OWL Console）的各页面功能。API 与部署细节见 [serve-user.md](serve-user.md)。

**截图环境说明**

- 浏览器视口 1440×900（2x 缩放），默认「深空」主题
- 演示数据：通过 `POST /api/v1/nodes/seed` 生成 50 个模拟节点（分组：web/db/cache/worker/monitor/gateway 等）
- 模拟节点 SSH 不可达，因此部分截图中的任务状态为「失败」，属预期现象
- 所有截图存于 `docs/images/serve/`

> 截图可通过工作流一键重拍（前端 UI 变更后）：见 `.agent/skills/serve-screenshots/SKILL.md`，
> 脚本位于 `scripts/serve-screenshots/`（`seed_demo_data.py` 准备演示数据，`capture.py` 采集并自校验 26 张图）。

---

## 1. 登录

首次访问任意页面会跳转到登录页。使用服务首次启动时输出的 admin 凭据登录。

![登录页](images/serve/01-login.png)

---

## 2. 仪表盘

登录后进入仪表盘（快捷键 `Alt+1`），总览节点与任务状态。

![仪表盘](images/serve/02-dashboard.png)

- **统计卡片** — 总节点数、在线率、在线/离线节点数
- **节点分布** — 在线/离线/告警环形图
- **最近任务** — 最新执行的任务及状态，点击「查看全部」进入任务历史
- **快捷操作** — 快速执行命令、运行剧本、添加节点

---

## 3. 节点管理

左侧为分组面板（可折叠、可搜索、复选过滤），顶部支持按状态（在线/离线/告警）筛选，搜索框输入节点名称/IP/标签后 100ms 防抖自动过滤。

![节点列表](images/serve/03-nodes-list.png)

搜索 `web` 即时过滤：

![节点搜索](images/serve/04-nodes-search.png)

点击「新建节点」弹出录入表单，支持地址/端口/用户/密码/SSH Key、分组与标签（`key:value` 格式自动校验）：

![新建节点](images/serve/05-node-add-modal.png)

勾选节点后出现批量操作栏：执行命令、依次标签（合并到各节点）、同批标签（统一替换）、管理分组、Ping、SSH 检查、删除：

![批量操作](images/serve/06-nodes-batch.png)

分组和标签以彩虹色阶区分，相同值颜色一致。

---

## 4. 节点详情

在节点列表点击节点名称进入 `/nodes/:id`，查看连接信息、分组标签、创建/更新时间：

![节点详情](images/serve/07-node-detail.png)

底部操作栏提供 Ping、SSH 检查、**终端**（浏览器内 WebSocket 交互式终端）以及编辑、删除。

---

## 5. 命令执行

「命令执行」页（`Alt+3`）左侧为节点选择面板（支持搜索、按在线状态过滤、分页、全选/清空），右侧为编辑区与输出终端。

**命令模式** — 输入命令（支持多行），通过分组/标签过滤目标节点，选择并行或串行：

![命令执行](images/serve/08-exec-command.png)

**脚本模式** — 切换到「脚本」Tab，可直接内联脚本内容，或勾选「从文件中转站选择」复用已上传脚本：

![脚本执行](images/serve/09-exec-script.png)

点击执行后，下方终端实时流式输出每个节点的任务创建与执行结果：

![实时输出](images/serve/10-exec-running.png)

右侧还提供：活跃分组 chips、快捷命令（保存常用命令）、超时/重试设置、调试模式。

---

## 6. 任务历史与详情

「任务历史」页（`Alt+7`）按操作维度聚合展示执行记录，支持状态/时间/用户/节点/命令多重过滤与分页，可导出 JSON/YAML：

![任务历史](images/serve/11-history.png)

点击任意记录打开详情弹窗：目标节点列表、退出码、耗时、每节点输出，并支持打包下载日志（zip）：

![任务详情](images/serve/12-history-detail.png)

---

## 7. 剧本管理

「剧本管理」页（`Alt+4`，operator 及以上可见）左侧为剧本分类，主区为剧本列表（名称/分类/描述/任务），每条提供 Run、二次编辑、下载；下方为运行历史与运行详情：

![剧本管理](images/serve/13-playbooks.png)

点击行查看剧本详情（YAML 内容、任务清单、变量）：

![剧本详情](images/serve/14-playbook-detail.png)

点击「Run」弹出执行配置：目标节点/分组、任务标签、Extra Vars，并可引用中转站文件：

![运行剧本](images/serve/15-playbook-run-modal.png)

---

## 8. 文件传输

「文件传输」页（`Alt+5`）支持节点间上传/下载，右侧筛选条件与传输选项（覆盖已有文件、并行传输、断点续传、权限位）：

![文件传输](images/serve/16-files-upload.png)

「传输记录 / 任务详情」Tab 切换视图，记录含状态（成功/失败/进行中/部分成功）与重试入口。

「文件中转站」为服务端暂存区（`~/.owl/staging`），上传到中转站的文件可在命令执行/剧本运行中引用，并在系统设置中调整暂存地址与最小剩余空间：

![传输记录](images/serve/17-files-records.png)

---

## 9. AI 助手

「AI 助手」页（`Alt+6`）是对话式运维入口：管理节点、执行命令、运行剧本、传输文件。左侧为会话列表（按登录用户隔离），右侧为话术建议；供应商在系统设置中配置后即可对话：

![AI 助手](images/serve/18-ai-chat.png)

- 快捷能力按钮：故障诊断、性能监控、配置查询、告警管理、下载日志
- 输入 `/` 呼出命令补全
- 工具调用结果以 Markdown 渲染；写操作按 RBAC 限制（viewer 只读）

---

## 10. 告警中心

监控采集异常时产生告警，列表按分级（紧急/重要/次要）与状态（待处理/已确认/已解决）标记，支持按节点过滤、确认/解决处置，并可查看详情与推荐对策：

![告警中心](images/serve/19-alerts.png)

「监控配置」（admin）提供静默窗口、告警类型阈值、通知渠道与对策库管理：

![监控配置](images/serve/20-alerts-config.png)

---

## 11. 系统设置

「系统设置」（admin only）分两部分：

- **KV 配置** — 全局键值表，内置键（如 `staging_dir`、`staging_min_free`）带中文 Description 与默认值标记，编辑保存后写入数据库
- **AI 供应商配置** — Anthropic/DeepSeek/OpenAI/千问/火山引擎/MiniMax/小米 MiMo/自定义，支持 API Key、Base URL、模型查询、思考强度与 JSON 微调

![系统设置](images/serve/21-settings.png)

编辑配置项（`staging_min_free` 校验正整数）：

![编辑配置](images/serve/22-settings-edit.png)

---

## 12. 用户管理

「用户管理」（admin only）提供用户 CRUD 与角色分配（viewer/editor/operator/admin），支持搜索与分页：

![用户管理](images/serve/23-users.png)

![添加用户](images/serve/24-users-add-modal.png)

角色权限矩阵见 [serve-user.md](serve-user.md) 的「RBAC 角色」一节。

---

## 13. 主题

顶栏可切换三套主题：深空（默认）、青瓷（浅色，纸白底 + 纯白面板 + 柔和阴影）、暖阳（暖色）：

![青瓷主题](images/serve/25-theme-light.png)

![暖阳主题](images/serve/26-theme-warm.png)

---

## 键盘快捷键

| 快捷键 | 功能 |
|--------|------|
| `Alt+1` | 仪表盘 |
| `Alt+2` | 节点管理 |
| `Alt+3` | 命令执行 |
| `Alt+4` | 剧本管理 |
| `Alt+5` | 文件传输 |
| `Alt+6` | AI 助手 |
| `Alt+7` | 任务历史 |

---

## 附：CLI 文档终端截图插入点（待补）

以下位置建议后续补充终端截图（本轮仅标注，见各文档内 `TODO(screenshot)` 注释）：

| 文档 | 位置 | 建议内容 |
|------|------|----------|
| [serve-user.md](serve-user.md) | 「首次运行」 | `owl-serve` 首次启动输出（URL/Username/Password） |
| [serve-user.md](serve-user.md) | 「重置管理员密码」 | `--reset-admin` 输出 |
| [../README.md](../README.md) | 「快速开始」6 小节 | node/exec/file/playbook/session/ai 命令输出 |
| [user/QUICKSTART.md](user/QUICKSTART.md) | 8 个步骤 | 各命令示例输出 |
| [user/NODE.md](user/NODE.md) 等 | 「示例输出」块 | 对应 CLI 命令真实输出 |
| [user/TUI.md](user/TUI.md) | 全文 | TUI 界面（Nodes/Exec/File/AI 面板）截图 |
