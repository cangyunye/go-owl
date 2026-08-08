# Web 操作历史子系统设计规格

**目标：** 为 Web 控制台补齐 CLI `owl history` 对应的操作历史能力，记录并展示通过 Web 发起的命令执行、文件传输、剧本运行、节点变更，并与 CLI 历史**统一**（同一数据库、同一批表）。

**架构模式：** serve module 内自建 `HistoryStore`（纯 Go modernc），复用 `internal/history` 的**逐字相同 schema** 写入共享的 `~/.owl/owl.db`；后端在现有 handler 处埋点记录；前端重写 `history.js` 提供多维过滤、详情钻取、导出与清理。

---

## 0. 背景与关键约束

差距分析结论：Web 完全没有 CLI `owl history` 对应的历史子系统。现有 `history.js` 仅列出 `tasks` 表，且状态过滤失效。

三条经核实的约束决定了方案选型：

1. **CLI 与 Web 共用同一个数据库文件** `~/.owl/owl.db`
   - CLI `internal/history` 的 `DefaultConfig()` 返回 `~/.owl/owl.db`（`internal/history/db.go:29`）
   - Web `owl-serve` 同样使用 `~/.owl/owl.db`（`cmd/owl-serve/main.go:36`）
2. **两个独立 Go module，serve 刻意纯 Go**
   - 根 module 依赖 CGO 的 `mattn/go-sqlite3` / `duckdb`
   - `cmd/plugins/serve` 是独立 module，仅依赖纯 Go 的 `modernc.org/sqlite`；`make build-serve` 为无 CGO 交叉编译
3. **CLI 实际记录的 op_type**：`command` / `script` / `file_transfer` / `playbook`
   - `node_manage` 仅出现在测试中，CLI 节点命令并不记录（Web 本期主动补充该类型）

### 方案选型

| 方案 | 说明 | 结论 |
|------|------|------|
| A. serve import `internal/history` | 复用 CLI 代码 | ❌ 会把 CGO（mattn/duckdb）引入 serve，破坏纯 Go 交叉编译 |
| **B. serve 内自建 HistoryStore，同 schema 同库** | modernc 写同一 owl.db 的同名表 | ✅ **采用**：纯 Go + 与 CLI 历史天然统一 |
| C. 聚合现有 web store（tasks/transfer_records…） | union 异构表 | ❌ 捕获不到 CLI 操作；不含 node_manage；不匹配 CLI 模型 |

方案 B 之所以能与 CLI 统一：两者写**同一文件**的**同名同 schema** 表（`CREATE TABLE IF NOT EXISTS` 幂等，先建者生效）。CLI 操作显示在 Web，Web 操作也出现在 `owl history`。

---

## 1. 数据层 — `store/history.go`（新增）

复用 serve 现有 modernc `*sql.DB`。表结构与 `internal/history/db_sqlite3.go` **逐字一致**（文件头部注释标明"需与 internal/history 保持同步"）。本期仅创建操作历史相关的 4 张表（`sessions`/`session_commands`/`nodes`/`aichat` 属其他子系统，不在本期创建；Web 已有自己的 `nodes` 表）。

```sql
CREATE TABLE IF NOT EXISTS operations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT,
    op_type TEXT,
    command TEXT,
    targets TEXT,
    status TEXT,
    execution_mode TEXT DEFAULT '',
    playbook_path TEXT DEFAULT '',
    current_task_index INTEGER DEFAULT 0,
    current_task_phase TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_operations_task_id ON operations (task_id);
CREATE INDEX IF NOT EXISTS idx_operations_op_type ON operations (op_type);
CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at);

CREATE TABLE IF NOT EXISTS command_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT,
    node_id TEXT,
    command TEXT,
    exit_code INTEGER,
    stdout TEXT,
    stderr TEXT,
    duration_ms INTEGER,
    success INTEGER,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_executions_task_id ON command_executions (task_id);
CREATE INDEX IF NOT EXISTS idx_executions_node_id ON command_executions (node_id);
CREATE INDEX IF NOT EXISTS idx_executions_created_at ON command_executions (created_at);

CREATE TABLE IF NOT EXISTS file_transfers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT,
    node_id TEXT,
    file_name TEXT,
    file_size INTEGER,
    transfer_type TEXT,
    status TEXT,
    progress REAL,
    error TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_transfers_task_id ON file_transfers (task_id);
CREATE INDEX IF NOT EXISTS idx_transfers_node_id ON file_transfers (node_id);
CREATE INDEX IF NOT EXISTS idx_transfers_created_at ON file_transfers (created_at);

CREATE TABLE IF NOT EXISTS node_communications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT,
    node_id TEXT,
    node_address TEXT,
    direction TEXT,
    message_type TEXT,
    payload TEXT,
    success INTEGER,
    error TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_communications_task_id ON node_communications (task_id);
CREATE INDEX IF NOT EXISTS idx_communications_node_id ON node_communications (node_id);
CREATE INDEX IF NOT EXISTS idx_communications_created_at ON node_communications (created_at);
```

### 类型定义（镜像 `internal/history`）

```go
type Operation struct {
    ID               int64
    TaskID           string
    OpType           string
    Command          string
    Targets          []string
    Status           string
    ExecutionMode    string
    PlaybookPath     string
    CurrentTaskIndex int
    CurrentTaskPhase string
    CreatedAt        time.Time
}

type CommandExecution struct {
    ID         int64
    TaskID     string
    NodeID     string
    Command    string
    ExitCode   int
    Stdout     string
    Stderr     string
    DurationMs int64
    Success    bool
    CreatedAt  time.Time
}

type FileTransfer struct {
    ID           int64
    TaskID       string
    NodeID       string
    FileName     string
    FileSize     int64
    TransferType string
    Status       string
    Progress     float64
    Error        string
    CreatedAt    time.Time
}

type NodeCommunication struct {
    ID          int64
    TaskID      string
    NodeID      string
    NodeAddress string
    Direction   string
    MessageType string
    Payload     string
    Success     bool
    Error       string
    CreatedAt   time.Time
}

// Record 统一记录结构（与 CLI 一致）
type Record struct {
    Operation         *Operation
    CommandExecutions []*CommandExecution
    Transfers         []*FileTransfer
    Communications    []*NodeCommunication
}

type QueryOptions struct {
    TaskID    string
    NodeID    string
    OpType    string
    Status    string
    StartTime time.Time
    EndTime   time.Time
    Limit     int
    Offset    int
}

// Stats 侧栏统计：按 op_type 与 status 的计数
type Stats struct {
    Total     int            `json:"total"`
    ByOpType  map[string]int `json:"by_op_type"`  // command / file_transfer / playbook / node_manage / script
    ByStatus  map[string]int `json:"by_status"`   // completed / failed / running / ...
}
```

### HistoryStore 方法

```go
type HistoryStore struct{ db *sql.DB }

func NewHistoryStore(db *sql.DB) *HistoryStore
func (s *HistoryStore) Init(ctx context.Context) error                       // 建 4 表 + 索引（幂等）
func (s *HistoryStore) RecordOperation(ctx, op *Operation) error
func (s *HistoryStore) RecordCommandExecution(ctx, exec *CommandExecution) error
func (s *HistoryStore) RecordFileTransfer(ctx, t *FileTransfer) error
func (s *HistoryStore) RecordNodeCommunication(ctx, c *NodeCommunication) error
func (s *HistoryStore) Query(ctx, opts *QueryOptions) ([]*Record, int, error) // 过滤 + 嵌套明细 + total
func (s *HistoryStore) GetByTaskID(ctx, taskID string) (*Record, error)
func (s *HistoryStore) Cleanup(ctx, retentionDays int) (int64, error)         // 删 4 表过期数据，返回删除数
func (s *HistoryStore) Stats(ctx) (*Stats, error)                             // 按 op_type/status 计数（供侧栏）
```

`Query` 行为对齐 CLI `internal/history.Query`：先按条件查 `operations`（`ORDER BY created_at DESC` + `LIMIT/OFFSET`），再对每条按 `task_id` 拉取关联的 `command_executions` / `file_transfers` / `node_communications`。`targets` 以 JSON 字符串存储，读取时反序列化为 `[]string`。

---

## 2. 并发与连接配置（必需改动）

**风险：** serve 模块当前**没有任何 PRAGMA 设置**（已核实，无 WAL / busy_timeout）。Web 长驻连接与 CLI 短驻连接同写 `owl.db`，缺 WAL + busy_timeout 会触发 `database is locked`。

**改动：** `server.go` 的 `Init()` 在 `sql.Open("sqlite", dbPath)` 之后，对连接执行（对齐 `internal/history/db_sqlite3.go:43-45` 并补 busy_timeout）：

```go
db.ExecContext(ctx, "PRAGMA journal_mode=WAL")
db.ExecContext(ctx, "PRAGMA synchronous=NORMAL")
db.ExecContext(ctx, "PRAGMA foreign_keys=ON")
db.ExecContext(ctx, "PRAGMA busy_timeout=5000")
```

> `journal_mode=WAL` 是库文件级持久属性，`busy_timeout` 是连接级、每次打开都需设置。

---

## 3. 记录接入点（后端 handler 埋点）

`Server` 增加 `History *store.HistoryStore` 字段，`Init()` 中初始化并注入相关 handler。

**核心原则：记录失败只记日志，绝不阻断主操作**（与 CLI 一致，CLI 多处忽略 `RecordOperation` 错误）。

| Handler | 触发点 | op_type | 明细记录 |
|---------|--------|---------|----------|
| `ExecHandler.Create`（exec.go） | 命令任务创建/完成 | `command` | 每节点 `command_executions`（node_id / exit_code / stdout=output / duration_ms / success） |
| `TransferHandler.Create`（transfer.go） | 传输任务创建/完成 | `file_transfer` | 每节点 `file_transfers`（file_name / file_size / transfer_type=push\|pull / status） |
| `PlaybookHandler.Run`（playbook.go） | 剧本运行 | `playbook` | `operations.playbook_path` + 每步 `command_executions` |
| `NodeHandler` Create/Update/Delete/Import/BatchGroups（node.go） | 节点变更 | `node_manage` | `command`=动作描述（如 `node create web-01`），`targets`=[节点 id] |

**task_id 关联：** 每次操作生成一个 operation uuid 作为 `operations.task_id`，并用同一 id 关联其 `command_executions` / `file_transfers` 明细行，保证详情钻取可聚合。

**script 类型：** 本期 Web 尚无真正的脚本执行路径（属后续"脚本执行"子系统），故暂不埋点 `script`；待该子系统实现时调用同一 `HistoryStore.RecordOperation(op_type="script")` 即可，无需改动历史子系统。

---

## 4. HTTP API — `handler/history.go`（新增 HistoryHandler）

路由注册于 `server.go`，挂在 `auth` 组下：

| 方法 | 路径 | 角色 | 说明 |
|------|------|------|------|
| GET | `/api/v1/history` | viewer+ | 查询。参数：`op_type` `status` `node_id` `task_id` `start_time` `end_time` `last`（如 `1h`/`24h`/`7d`）`limit` `offset`。返回 `{ data: [Record], meta: { total } }` |
| GET | `/api/v1/history/:task_id` | viewer+ | 单条 Record + 嵌套明细 |
| GET | `/api/v1/history/export` | viewer+ | 同查询参数 + `format=json\|yaml`，返回文件下载 |
| DELETE | `/api/v1/history` | admin | `?days=N`（N>0），返回 `{ deleted: <count> }` |
| GET | `/api/v1/history/stats` | viewer+ | 按 op_type / status 的计数（供前端侧栏） |

`last` 解析复用 CLI 的相对时间语义（`h`=小时、`d`=天，其余走 `time.ParseDuration`）。

---

## 5. WS 实时刷新

复用现有 `wsHub`。handler 记录操作后广播 `history_update` 事件（沿用现有 `task_update` 模式），前端历史页订阅后增量刷新。事件 payload 仅需 `{ type: "history_update" }`，前端收到后重新拉取当前过滤条件下的列表。

---

## 6. 前端 — 重写 `web/js/pages/history.js`

替换现有"仅任务列表"实现，对齐 CLI `owl history` 能力：

- **操作类型 tab**（侧栏或顶部）：全部 / 命令 / 脚本 / 文件传输 / 剧本 / 节点，带计数（来自 `/history/stats`）
- **状态过滤**（修复现有失效逻辑，真正传入查询）：全部 / 成功 / 失败 / 进行中
- **时间范围**：最近 1h / 24h / 7d / 30d / 自定义
- **节点过滤**：节点搜索输入
- **列表行**：类型图标 + 命令/描述 + 目标 chips + 状态徽章 + 相对时间
- **详情钻取**（对齐 CLI `--verbose`）：点击行展开/弹窗，分三表展示
  - Command Executions：node / exit code / duration / status / command
  - File Transfers：node / file / size / type / status
  - Communications：node / direction / type / status（若有）
- **导出**：JSON / YAML 下载按钮
- **清理**（仅 admin 可见）：保留天数输入 + 二次确认
- **分页** + **WS 实时刷新**（订阅 `history_update`）

`web/js/api.js` 增加：`historyList(params)` / `historyGet(taskId)` / `historyExport(params, format)` / `historyClean(days)` / `historyStats()`。

---

## 7. 错误处理

- 记录埋点失败：`log.Printf` 记录，不返回、不阻断主操作
- 查询参数非法（如 `days<=0`、时间格式错误）：返回 400 + 明确 message
- 清理权限不足：RBAC 中间件返回 403
- 数据库错误：返回 500 + message
- 并发：依赖 §2 的 WAL + busy_timeout

---

## 8. 测试（TDD，遵循 AGENTS.md `/tdd`）

| 测试文件 | 覆盖 |
|----------|------|
| `store/history_test.go` | Init 建表；RecordOperation + Query 各过滤器 roundtrip；嵌套明细聚合；Cleanup 仅删过期数据；Stats 计数；**schema 与 internal/history 兼容性断言** |
| `handler/history_test.go` | 列表过滤；详情；导出 json/yaml；清理 RBAC（非 admin 403）；`days<=0` 校验 400；`last` 解析 |
| 记录集成（扩展现有 handler 测试） | exec/transfer/playbook/node 操作后断言 `operations` 表有对应记录 |
| E2E | seed 节点 → API 执行命令 → `/api/v1/history` 可见 → `owl history` CLI 同样可见（验证统一） |

---

## 9. 构建与验证

- `make build-serve` 必须仍成功（**纯 Go，不引入任何 CGO 依赖**；仅用 modernc）
- serve module `go test ./...` 全绿
- 按 AGENTS.md 手工 E2E：`--reset-admin` 启动 → seed 50 节点 → 登录 → 触发 exec/transfer/playbook/node 操作 → 历史页验证过滤/详情/导出/清理

---

## 10. 范围外（后续子系统）

- `script` op_type 记录（随"脚本执行"子系统）
- `node_communications` 细粒度写入（本期建表但暂不埋点）
- 历史 `tasks` 表存量数据迁移（历史从本子系统启用后新写入开始，旧 tasks 仍由原任务页负责）
- 交互式 SSH 会话、监控指标等其他缺失子系统
