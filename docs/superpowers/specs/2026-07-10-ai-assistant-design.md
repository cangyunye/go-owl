# AI 助手设计规格

**目标：** 将 CLI `owl ai` 的 LLM Agent 能力移植到 Web 页面，成为嵌入业务的运维客服助手，同步权限矩阵校验。

**架构模式：** 后端代理 LLM 调用 + RSA 会话密钥加密传输 + 本地 IndexedDB 持久化对话 + 结构化审计。

---

## 1. 整体架构

```
浏览器 (用户本地)
├── ai.js              — 对话界面
├── storage.js         — IndexedDB 对话历史 + localStorage API Key
├── crypto.js          — Web Crypto API 封装
└── audit.js           — 结构化审计上报

服务端 (Go)
├── handler/ai.go      — HTTP 入口 + Agent 编排 + 权限校验
├── internal/ai/       — 复用：prompts / intent_classifier / param_extractor / validator
├── internal/ai/executor.go — 执行器接口（新）
├── handler/aiexecutor.go   — WebExecutor 实现（新）
├── store/ai_audit.go       — 审计表 CRUD（新）
└── 启动参数 --ai-debug     — 调试模式开关
```

---

## 2. 执行器接口分离

当前 `tools.go` 的工具硬编码 `runOwlCommand()` 调 CLI 二进制。改为接口注入。

**`internal/ai/executor.go`（新增）：**

```go
type Executor interface {
    QueryNodes(ctx context.Context, params QueryNodesParams) (*QueryNodesResult, error)
    ExecuteCommand(ctx context.Context, params ExecCommandParams) (*ExecResult, error)
    ExecuteScript(ctx context.Context, params ExecScriptParams) (*ExecResult, error)
    RunPlaybook(ctx context.Context, params RunPlaybookParams) (*RunPlaybookResult, error)
    GeneratePlaybook(ctx context.Context, params GeneratePlaybookParams) (*GeneratePlaybookResult, error)
    TransferFile(ctx context.Context, params TransferFileParams) (*TransferResult, error)
    ListPlaybooks(ctx context.Context) (*ListPlaybooksResult, error)
    PlaybookInfo(ctx context.Context, params PlaybookInfoParams) (*PlaybookInfoResult, error)
    ValidatePlaybook(ctx context.Context, params ValidatePlaybookParams) (*ValidateResult, error)
    NodeCheck(ctx context.Context, params NodeCheckParams) (*NodeCheckResult, error)
    QueryDatabase(ctx context.Context, params QueryDatabaseParams) (*QueryDatabaseResult, error)
}
```

**两套实现：**

| 实现 | 位置 | 调用路径 |
|------|------|---------|
| `CLIExecutor` | 已有 `tools.go` 重构 | `runOwlCommand()` shell 调用 |
| `WebExecutor` | 新增 `handler/aiexecutor.go` | 直接调 Go 内部逻辑: TaskStore, SSH, PlaybookRunStore, TransferRecordStore 等 |

`tools.go` 中的工具构造方法改为接收 `Executor` 接口，不再硬编码执行方式。

---

## 3. 权限校验

AI Chat 降级为 `viewer` 可访问（之前是 `operator`），权限在工具级别校验。

```
query_nodes       → role >= viewer      ✅ 通过
list_playbooks    → role >= viewer      ✅ 通过
playbook_info     → role >= viewer      ✅ 通过
validate_playbook → role >= viewer      ✅ 通过
query_database    → role >= viewer      ✅ 通过

execute_command   → role >= operator    ❌ 否则拒绝 + 提示"权限不足"
execute_script    → role >= operator    ❌ 同上
run_playbook      → role >= operator    ❌ 同上
transfer_file     → role >= operator    ❌ 同上
generate_playbook → role >= operator    ❌ 同上
node_check        → role >= operator    ❌ 同上
```

`handler/ai.go` 新版 `Chat` 方法：
1. 前端发来 `{"message": "...", "session_id": "...", "encrypted_api_key": "..."}`
2. 从 JWT 取用户 role
3. 解密 API Key，调 Agent 路由 → 选提示词 → 提取参数 → 权限校验 → 调 WebExecutor → 返回
4. 异步写入 `ai_audit_log`

增删改查操作（未来扩展）在工具执行前必须：
1. 先查询当前数据展示给用户
2. 等待用户确认后再执行
3. 该流程在 Agent 多轮对话中完成

---

## 3.5 Agent 改造

`internal/ai/agent.go` 的 `NewAgent` 改为接收 `Executor` 接口参数：

```go
func NewAgent(executor Executor, opts ...AgentOption) *Agent
```

每个工具（`QueryNodesTool`、`ExecuteCommandTool` 等）在构造时接收 `Executor`，调用 `e.QueryNodes(...)` 代替 `runOwlCommand(...)`。

CLI 侧传入 `CLIExecutor`，Web 侧传入 `WebExecutor`，Agent 本身的 prompt/路由/参数提取/校验逻辑完全不变。

---

## 4. AI 审计表

```sql
CREATE TABLE ai_audit_log (
    id                TEXT PRIMARY KEY,
    user_id           TEXT NOT NULL,
    intent            TEXT NOT NULL,           -- 路由分类: query_nodes / exec_run / run_playbook / ...
    tool              TEXT NOT NULL,           -- 工具名
    params_snapshot   TEXT NOT NULL DEFAULT '{}', -- JSON 工具参数（不含原始 prompt）
    result            TEXT NOT NULL,           -- success / failed / rejected
    target_type       TEXT DEFAULT '',         -- task / transfer_record / playbook_run / playbook / query
    target_ids        TEXT DEFAULT '[]',       -- JSON array of 关联记录 ID
    prompt_text       TEXT DEFAULT '',         -- 调试用，仅 --ai-debug 时记录
    reply_text        TEXT DEFAULT '',         -- 调试用，仅 --ai-debug 时记录
    llm_model         TEXT DEFAULT '',
    llm_duration_ms   INTEGER DEFAULT 0,
    created_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_ai_audit_user ON ai_audit_log(user_id);
CREATE INDEX idx_ai_audit_time ON ai_audit_log(created_at);
```

**记录时机：** `/ai/chat` 后端操作完成后由 `WebExecutor` 同步写入，不依赖前端上报。`POST /api/v1/ai/audit` 为备用端点，仅当前端直连 LLM（方案 B）时由 `audit.js` 调用。

**调试模式：** 启动参数 `--ai-debug`，开启时额外记录 `prompt_text` 和 `reply_text`。前端检测到调试模式后显示黄色横幅 `🔧 调试模式已开启，所有对话将被记录`。

**增删改查的确认：** 变更操作在用户确认前，审计记录 `result=awaiting_confirmation`；用户确认后才执行并更新为 `success`。

---

## 5. API 端点

| 方法 | 路径 | 角色 | 说明 |
|------|------|------|------|
| GET | `/api/v1/ai/session-key` | viewer+ | 获取 RSA 公钥 + session_id |
| GET | `/api/v1/ai/context` | viewer+ | 获取用户最近 20 条操作摘要（不调 LLM，纯 DB 查） |
| POST | `/api/v1/ai/chat` | viewer+ | AI 对话（权限在工具级校验） |
| POST | `/api/v1/ai/audit` | viewer+ | 前端直接上报结构化审计（备用） |

---

## 6. API Key 加密传输

**流程：**

```
服务端启动 → 生成 RSA-2048 OAEP keypair（仅内存，永不落盘）
                  ↓
前端加载 AI 页面 → GET /api/v1/ai/session-key
                  ← { session_id, public_key_spki }
                  ↓
前端 Web Crypto API:
  1. crypto.subtle.importKey('spki', publicKey, { name: 'RSA-OAEP', hash: 'SHA-256' }, false, ['encrypt'])
  2. crypto.subtle.encrypt({ name: 'RSA-OAEP' }, publicKey, apiKeyBytes)
  3. 结果 base64 编码
                  ↓
POST /api/v1/ai/chat
  {
    message: "查一下节点状态",
    session_id: "sess_xxx",
    encrypted_api_key: "base64_encrypted_blob"
  }
                  ↓
后端用 session 关联的私钥解密 → API Key 仅在内存中用于 LLM 调用
→ 显式过滤 encrypted_api_key 字段，不写入任何日志
```

---

## 7. 前端架构

**`storage.js`（新增）：**
- `localStorage`：存 API Key（AES-GCM 加密，密钥派生自 `用户ID + session_id`）+ Provider/Model 偏好
- `IndexedDB`：存完整对话历史（AES-GCM 加密），支持分页查询

**`crypto.js`（新增）：**
- `encryptApiKey(publicKeySpki, apiKey)` → base64
- `encryptLocal(data, userId)` → AES-GCM 加密后存 localStorage
- `decryptLocal(encrypted, userId)` → 解密读取

**`audit.js`（新增）：**
- `reportAudit(tool, params, result, targetType, targetIds)` → `POST /api/v1/ai/audit`

**`ai.js`（重构）：**
- 现有 ChatUI 增强：多轮对话渲染、加载状态、错误处理
- 加载时检查 localStorage 是否有 API Key，无则引导到 settings 配置
- 对话存 IndexedDB，刷新恢复上次会话
- 自动注入 System Prompt：当前用户 role + 最近操作摘要（最近 20 条 task/transfer/playbook_run 记录，由 `GET /api/v1/ai/context` 提供，不调 LLM）

**设置入口：** `settings` 页面新增 AI 配置区块：
- API Key 输入（写入 localStorage）
- Provider 选择（OpenAI / Anthropic / DeepSeek / 自定义）
- Model 选择
- 检测 `--ai-debug` 状态，调试模式时显示横幅

---

## 8. 对话生命周期

```
页面加载 → 检查 localStorage API Key
         → 无 → 提示用户去 settings 配置
         → 有 → GET /api/v1/ai/session-key → 加密 Key → 就绪
         ↓
用户输入 → POST /api/v1/ai/chat
         → 后端 Agent 路由 → 权限校验 → 执行 → 回复
         → 前端渲染回复
         → 异步：IndexedDB 存对话 + audit API 上报
         ↓
多轮循环（上下文中携带历史摘要）
```

---

## 9. 后续扩展：增删改查的确认流程

```
用户: "删掉 web-01 节点"
  ↓
Agent 路由 → node_delete（需要确认）
  ↓
1. 先查询节点信息 → 展示给用户
   "确认要删除节点 web-01 (10.0.0.1) 吗？"
  ↓
2. 用户确认 → 执行删除 → 记录审计
   用户取消 → 记录审计 result=cancelled
```

审计记录在确认前: `result=awaiting_confirmation`
审计记录在确认后: `result=success`
审计记录在取消后: `result=cancelled`
