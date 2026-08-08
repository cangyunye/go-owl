# 统一执行共享层设计规格（节点选择 / SSH 拨号 / 安全黑名单）

**目标：** 消除 CLI 与 Web（serve）两个前端在"执行目标选择、SSH 连接、危险命令检查"三处的重复实现与行为不一致，以 CLI 语义为准建立共享执行层；serve 保留自己的流式编排（WebSocket 逐行输出）。

**架构模式：** 共享逻辑落在根模块 `internal/`，serve 通过现有 `replace` 依赖引用（保持现有三模块划分不变）；serve handler 变为瘦适配层。

---

## 0. 背景与关键事实

工程质量分析发现的三组不一致（均已核实到代码位置）：

1. **执行目标选择**
   - CLI 精确匹配：解析 groups JSON 数组后精确比较
   - serve 子串匹配：`handler/exec.go:131` `SELECT id FROM nodes WHERE groups LIKE '%web%'` —— 在 web 组执行命令会误伤 `web-k8s`、`webserver` 组节点；labels 同理（`exec.go:153` 用 JSON 子串匹配）
   - **注意区分**：`handler/node.go:105,130,234` 的 LIKE 属于节点列表/搜索 API（界面搜索框），是合理的模糊查询功能，**本次不动**
2. **SSH 连接**
   - CLI：`internal/control/command.Executor` + `internal/ssh` 池/`native_executor`（带连接超时、重试）
   - serve：`handler/ssh_executor.go`（183 行）每次调用重新 dial，`Timeout: 0` 无限等，无 ProxyJump，无 keyboard-interactive 认证
   - serve 自研执行器的唯一真实差异是 **逐行 stdout/stderr 流式推送 WebSocket**（`ExecuteStream`），CLI 的 `RunStreaming` 是按节点完成粒度，直接替换会打断 Web 实时输出
   - ProxyJump 现状（查证修正）：`internal/node/ssh_config_source.go:85` 只解析、全项目无任何 dial 路径消费它——CLI 与 serve 都不支持跳板，统一拨号器中实现属"补齐"而非"对齐 CLI"
3. **安全黑名单**
   - `internal/control/blacklist`（Checker + 可配置规则）只被 CLI `exec run` 使用（`cmd/cli/cmd/exec/run.go:197-244`：命中→交互式 y/n 确认，`--force` 跳过）
   - serve 的 `/api/v1/exec`、playbook 执行、AI WebExecutor **零检查**

历史成因：serve 执行栈来自早期 web console 提交（f777b3b、652c45e），未复用 internal，非有意架构决策。

### 方案选型

| 方案 | 说明 | 结论 |
|------|------|------|
| **A. 统一连接层，保留 serve 流式编排** | 共享选择/拨号/黑名单，serve 编排层保留 | ✅ **采用** |
| B. 只统一策略 + 契约测试，保留双执行器 | 改动最小 | ❌ 双 SSH 栈长期漂移风险不消除 |
| C. 连节点模型一起彻底统一 | 5+ 份节点结构收敛 | ❌ 范围过大，单独立项 |

已确认边界决策：
- 保持现有 go.work 三模块划分，共享代码放 `internal/`
- 行为分歧一律以 CLI 语义为准（精确匹配、超时、force 语义）
- 界面搜索框模糊查询不动

---

## 1. 统一节点选择 — `internal/node/select`（新增）

```go
package select

type NodeRow struct {
    ID, Name string
    Groups   []string
    Labels   map[string]string
    Status   string
}

type NodeSource interface {
    List(ctx context.Context) ([]NodeRow, error)
}

type Selector struct { source NodeSource }

type SelectOptions struct {
    NodeIDs []string            // 按 id 或 name，精确
    Groups  []string            // 精确成员匹配
    Labels  map[string]string   // k=v 全匹配
    Status  string
}

func (s *Selector) Select(ctx context.Context, opts SelectOptions) ([]NodeRow, error)
```

**语义规则（以 CLI 为准）：**
- groups：解析 JSON 数组后精确比较，`web` ≠ `web-k8s`
- labels：键值完全相等；多 label 为 AND 关系（对齐 CLI `--label env=prod,zone=a`）
- 多条件并存时沿用 CLI 优先级 nodes > groups > labels，**不做交集**，避免 Web 端意外变严
- NodeIDs 按 id 优先、name 兜底；解析失败**整体报错**，不再静默跳过（serve 现状是 `continue` 吞错，`exec.go:132-134`）

**NodeSource 实现：**
- serve：一次 `SELECT id,name,groups,labels,status FROM nodes`，Go 内解析 JSON 过滤（节点量级小，全量拉取）
- CLI：包装现有 `NodeResolver` 的数据（其已合并 local/API/ssh-config 三个来源），选择逻辑收敛到 Selector

**调用方改造：**
- serve `handler/exec.go`：删除 `resolveNodeIDs`（:111-181），改用共享 Selector + DB NodeSource
- serve AI：`handler/aiexecutor.go:55-87` 的 `resolveAINodeIDs` 同样替换
- CLI `exec run`：替换为共享 Selector，行为不变
- `/api/v1/exec` 请求体字段不变，仅选择结果被修正

---

## 2. 统一 SSH 拨号器 — `internal/ssh`（扩展）

现有三个 dial 实现：CLI `internal/ssh/native_executor.go:57,122`、CLI `internal/session/connection_pool.go:99`、serve `handler/ssh_executor.go:67,113`。收敛为：

```go
type DialOptions struct {
    ConnectTimeout time.Duration  // 默认 10s，替代 serve 的 Timeout: 0
    ProxyJump      string         // "jump-host" 或 "jump-host:2222"，新增能力
    SSHKeyContent  string         // DB 内联密钥（serve 场景）
    KeyFile        string         // 密钥文件（CLI 场景）
    Password       string
}

func Dial(ctx context.Context, addr, user string, opts DialOptions) (*ssh.Client, error)
```

**实现要点：**
- 认证链复用 `native_executor.go:182-251` 现有逻辑（password + key + keyboard-interactive + 默认 key 尝试）
- ProxyJump：先 dial 跳板机，在跳板连接上转发到目标——CLI/serve 同时获得新能力，数据源为已解析的 ssh config（`ssh_config_source.go:85-87`）
- 连接超时走 ctx；`HostKeyCallback` 维持 `InsecureIgnoreHostKey` 现状（known_hosts 校验单独立项）
- 错误统一包装为 `*ConnectionError`（复用 `native_executor.go:301` 类型，区分 auth/timeout/connection），替代现有按错误字符串猜测的分类（`command/ssh_executor.go:164-168`）

**改造后：**
- serve `sshExecutor.Execute/ExecuteStream` 改调共享 Dial，删除自有 dial/auth 代码（约 -100 行），**逐行流式逻辑保留**
- CLI `native_executor`、session 连接池改调共享 Dial，删除重复拨号
- `command.Executor`、serve WS/retry/async 编排层均不动

---

## 3. 黑名单统一接入 — `internal/control/blacklist`（扩展）

```go
// API 场景薄封装
func (c *Checker) CheckForExec(user, command string, force bool) (*CheckResult, error)
// force=true：命中也返回 nil error，但 CheckResult 仍返回（供审计）
// force=false：命中返回携带匹配详情的 error
```

**接入点：**

| 入口 | 现状 | 改造 |
|------|------|------|
| serve `POST /api/v1/exec` | 无检查 | 执行前按每节点 user 检查 |
| serve playbook 执行 | 无检查 | 每个 command/script step 前检查 |
| serve AI WebExecutor | 无检查 | AI 生成命令执行前检查 |
| CLI `exec run` | 已有交互确认 | 仅底层共用 Checker 加载，行为不变 |

**API 行为：**
- 命中且无 force：`403` + `{"blocked": true, "matches": [{"node": "web-01", "pattern": "rm -rf ", "line": "..."}]}`，前端据此渲染确认弹窗
- 带 `"force": true` 重提：放行，审计记录标记 forced
- playbook：`force` 作为 run 参数对整个 run 生效（对齐 CLI `--force`）

**配置：** 两端共用现有 `blacklist.LoadConfig()`（`~/.owl/blacklist.yaml` 或默认规则）。

**审计：** `history.Operation` 增加 `Forced bool` 字段，operations 表加列；sqlite 不支持 `ADD COLUMN IF NOT EXISTS`，迁移时先查 `PRAGMA table_info` 再决定（serve 与 CLI 各自建表的现状下两端都需容错）。

---

## 4. 错误处理原则

- 选择阶段：任一 id/name 解析失败 → 整体 400，不静默跳过
- 拨号失败：统一 `*ConnectionError`，serve API 层映射为结构化错误
- serve API 错误统一 `{"error": "...", "detail": "..."}`，替换现有裸 `"failed"`（如 `handler/exec.go:401,410,428`）

---

## 5. 测试策略

- `internal/node/select`：单测覆盖 `web` vs `web-k8s`、label AND、错误 id 报错、优先级规则
- `internal/ssh` Dial：单测 + ProxyJump 集成测试走现有 `OWL_TEST_ENABLED` 门禁（`tests/integration`）
- 黑名单：`CheckForExec` force 路径单测；serve handler 用 httptest 补 403 / force 场景
- 回归线：serve 现有 16 个 `handler/*_test.go` 与 CLI 相关测试全绿；CLI 行为不得变化（交互确认、`--force`）
- 清理：删除 `tests/unit/command_test.go`、`variable_test.go`（测试自己复制的辅助函数，不覆盖生产代码）

---

## 6. 明确范围外（单独立项）

- known_hosts 校验（替换全部 `InsecureIgnoreHostKey`）
- 节点模型统一（5+ 份结构收敛）
- LLM 客户端合并、文件传输统一
- 数据库 schema 单源化与 migration 机制
- `internal/control/task` 死代码调度器清理
