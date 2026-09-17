# 05 · 能力库与 AI 匹配规范（脚本库 · 剧本库 · 告警自愈）

> 状态：设计评审稿（头脑风暴 → 规范）
> 关联：[04_MONITORING_ALERTING.md](04_MONITORING_ALERTING.md)、[PLAYBOOK_TEMPLATE_SYSTEM.md](PLAYBOOK_TEMPLATE_SYSTEM.md)、[ai-routing-map.md](ai-routing-map.md)

---

## 0. 一句话目标

把「脚本/剧本能修什么」从人的脑子里搬进**机读元数据**，让告警自愈时的 AI 匹配从"读代码猜"变成
**规则召回 + 语义精选**，并在 LLM / embedding 任一缺席时自动降级、处置永不中断。

---

## 1. 背景与现状缺口

### 1.1 已具备的基建

| 能力 | 位置 |
|------|------|
| 自愈管线：建议 → 语法闸门 → 黑名单 → 审批矩阵 → 执行 → 记录 | `internal/monitor/autoheal.go` `AutoHealer.Heal` |
| AI 处置建议器（严格 JSON、优先引用现有对策、失败降级规则） | `internal/monitor/ai_advisor.go` `AIAdvisor` / `ai_disposal.go` `Advisor` 接口 |
| 对策库（含治理字段 source/reviewed/auto_approve 与执行统计） | `internal/monitor/remedy_store.go` `Remedy` |
| 分步执行计划 + 人工审批流 | `internal/monitor/remedy_run.go` `RemedyRun/RemedyStep` |
| 参数 schema 与校验 | `pkg/playbook/template_types.go` `TemplateParameter` + `ValidateParams()` |
| 剧本库（磁盘 YAML 本体 + `playbooks` 元数据索引 + 分类目录） | `cmd/plugins/serve/store/playbook.go`、settings 键 `playbook_library_path` |
| 审批矩阵（高风险永不自动、AI 未审核须审批、canary 先行） | `internal/monitor/approval.go` `DecideApproval` |
| 告警实例级绑定（script/playbook，自动执行） | `internal/monitor/alert_binding_store.go` `AlertBinding` |

### 1.2 五个缺口（本规范要解决的）

| # | 缺口 | 证据位置 |
|---|------|----------|
| G1 | **脚本没有"库"**：只是中转站裸文件 + 内联内容，无名称/语言/用途/参数/风险元数据，AI 无从识别 | `cmd/plugins/serve/handler/staging.go`（仅文件搬运）；`cmd/plugins/serve/handler/exec.go` `script_args`（裸字符串） |
| G2 | **Remedy 内容内联**：与脚本库/剧本库零关联，内容重复、无法参数化 | `internal/monitor/remedy_store.go` `Remedy.Content` |
| G3 | **单一硬绑定**：`Remedy.AlertTypeID` 单值，无法一对多、无症状语义 | `RecommendedRemedies(alertTypeID)` 精确 SQL 匹配 |
| G4 | **剧本类对策被 AI 跳过**：prompt 组装时只给 script 类 | `internal/monitor/ai_advisor.go` `buildPrompt`：`if r.Kind != "script" { continue }` |
| G5 | **反馈与参数未闭环**：`ExecCount/SuccessCount` 已记录但匹配时不用；脚本参数无 schema，AI 无法自动装配 | `RecordExecution` 数据闲置；`execRequest.script_args` |

---

## 2. 能力元数据规范（CapabilityMeta）

### 2.1 统一抽象：处置能力（Capability）

**决策**：脚本与剧本共用**一套**元数据规范，抽象为「处置能力」。现有 `Remedy.Kind`
（script | playbook | sop）顺势承载；匹配层不区分 kind，执行层分发：

```
能力卡片 ──匹配──▶ Remedy(ref_kind+ref_id) ──执行分发──▶ script   → ScriptExecutor
                                                    playbook → 剧本引擎（PlaybookRunner）
                                                    sop      → 展示为人工指引
```

### 2.2 字段规范

| 字段组 | 字段 | 类型 | 消费方 | 说明 |
|--------|------|------|--------|------|
| 身份 | `name` | string | 人 + AI 卡片 | 简短名称 |
| 身份 | `description` | string | AI 卡片 | 一句人话，**语义匹配的主锚点** |
| 身份 | `kind` | enum | 执行分发 | script / playbook / sop（沿用） |
| 身份 | `language` | enum | 执行器 | script 类：bash / python3 / perl / ruby / go / expect |
| **适用性** | `alert_types` | []string | L0 硬召回 | 精确映射 `OWL-XXX-YYY`，**多绑定**（替代 `Remedy.AlertTypeID` 单值） |
| **适用性** | `symptoms` | []string | R1 语义召回 + AI 卡片 | 自然语言症状，泛化到未知告警类型的唯一通道 |
| **适用性** | `category` | enum | L0 硬召回 + UI | disk / mem / cpu / net / svc / err / avail / generic（对齐 `alert_type.go` 内置分类） |
| 节点兼容 | `applicable_os` | []string | L0 过滤 | 依赖 nodes 表补 OS 字段（见 §10 开放问题） |
| 节点兼容 | `node_groups` / `node_labels` | []string | L0 过滤 | 与 `internal/node/select` 选择器语义一致 |
| 参数 | `parameters` | []TemplateParameter | L2 装配校验 | **复用** `pkg/playbook` `TemplateParameter`（name/type/required/default/options/pattern） |
| 安全 | `risk` | enum | 审批矩阵 | low / medium / high（沿用） |
| 安全 | `destructive` | bool | 卡片 + 审批展示 | 是否有破坏性操作 |
| 安全 | `side_effects` | string | 审批展示 | 人话描述副作用 |
| 安全 | `requires_root` / `requires` | bool / string | 前置检查 | 前置条件 |
| 安全 | `rollback` | string | 失败回滚 | 回滚脚本路径或描述（沿用 `Remedy.Rollback`） |
| 安全 | `idempotent` / `timeout` | bool / int | 重试与执行控制 | 幂等标记 / 超时秒数 |
| 治理 | `source` | enum | 执行门槛 | user / builtin / ai（沿用） |
| 治理 | `reviewed` / `auto_approve` | bool | 执行门槛 | **AI 生成或 AI 起草的元数据默认未审核**（沿用） |
| 治理 | `exec_count` / `success_count` / `last_exec_at` | int/int64 | 卡片先验 | 执行统计（沿用 + 补时间戳） |

**设计要点**：

1. `alert_types[]` 与 `symptoms[]` **双通道**是规范灵魂：前者保证已知告警**确定性命中**
   （零 LLM 成本），后者让自定义检查（`alert_types.check_cmd`）、未来入站 webhook 等
   未知告警也能**泛化匹配**。
2. **AI 生成的元数据只影响匹配排序，不影响执行权限**。执行权限仍由
   `source / reviewed / auto_approve` 三元组决定（`CanAutoExecute` 与
   `DecideApproval` 语义不变）。

### 2.3 与现有数据的兼容策略

- **内置对策不动**：`internal/monitor/builtin_remedies.go` 的 7 条 sop 种子保持
  `source=builtin, kind=sop`，无 ref，语义即"人工指引"。
- **remedies 表加列**（`ALTER TABLE ADD COLUMN` 幂等迁移，`playbook_runs` 已有先例）：
  `alert_types(JSON)`、`symptoms(JSON)`、`category`、`ref_kind`、`ref_id`、
  `parameters(JSON)`、`language`、`timeout`、`idempotent`、`destructive` 等。
- **内联内容兼容保留**：`ref_id` 为空时回退 `Content` 执行——旧数据零迁移成本。

---

## 3. 脚本库实体设计

### 3.1 决策：混合模式（对齐剧本库）

脚本**本体存磁盘**、**元数据存 SQLite**、Web CRUD 管理——与剧本库
（磁盘 YAML + `playbooks` 表）心智完全一致。

```
~/.owl/scripts/                 # settings 键 script_library_path（对齐 playbook_library_path 模式）
  disk/clean_disk_space.sh      # 一级子目录 = category（对齐剧本库 SyncFromDir）
  mem/oom_guard.py
  svc/restart_service.sh
```

### 3.2 scripts 表 DDL 草案

```sql
CREATE TABLE IF NOT EXISTS scripts (
    id            TEXT PRIMARY KEY,            -- 文件路径 sha256 前 6 字节 hex（对齐 playbookID()）
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    language      TEXT NOT NULL DEFAULT 'bash',-- bash|python3|perl|ruby|go|expect
    category      TEXT NOT NULL DEFAULT 'generic',
    file_path     TEXT NOT NULL,
    file_exists   INTEGER NOT NULL DEFAULT 1,
    alert_types   TEXT NOT NULL DEFAULT '[]',  -- JSON ["OWL-DSK-001", ...]
    symptoms      TEXT NOT NULL DEFAULT '[]',  -- JSON ["磁盘使用率过高", ...]
    parameters    TEXT NOT NULL DEFAULT '[]',  -- JSON []TemplateParameter
    risk          TEXT NOT NULL DEFAULT 'medium',
    rollback      TEXT NOT NULL DEFAULT '',
    timeout       INTEGER NOT NULL DEFAULT 300,
    requires_root INTEGER NOT NULL DEFAULT 0,
    idempotent    INTEGER NOT NULL DEFAULT 0,
    destructive   INTEGER NOT NULL DEFAULT 0,
    tags          TEXT NOT NULL DEFAULT '[]',
    source        TEXT NOT NULL DEFAULT 'user',
    reviewed      INTEGER NOT NULL DEFAULT 0,
    auto_approve  INTEGER NOT NULL DEFAULT 0,
    exec_count    INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);
```

### 3.3 API 草案

RBAC 对齐现状：读 viewer+，写 operator+，配置 admin。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/scripts` / `/scripts/:id` / `/scripts/:id/file` | 列表 / 详情 / 内容 |
| POST | `/api/v1/scripts/upload` | 上传（内容或 multipart），可带元数据 |
| PUT | `/api/v1/scripts/:id` / `/scripts/:id/meta` | 更新内容 / 仅元数据 |
| DELETE | `/api/v1/scripts/:id` | 删除（文件 + 索引） |
| POST | `/api/v1/scripts/refresh` | 从库目录重扫（对齐 `POST /playbooks/refresh`） |
| POST | `/api/v1/scripts/:id/ai-meta` | AI 元数据草稿（§8）：**只返回草稿不落库**，前端确认后调 meta 更新 |

### 3.4 与相邻概念的边界

| 概念 | 定位 | 关系 |
|------|------|------|
| 中转站（staging） | 临时文件搬运区 | 保持不变；脚本库脚本下发时内部复用其上传通道 |
| 剧本库 | 剧本资产 | 同构（磁盘本体 + DB 索引 + refresh）；剧本 `script:` 动作引用脚本库路径（是否支持按 ID 引用见 §10） |
| remedies（对策） | **匹配条目**（面向告警） | 经 `ref_kind/ref_id` 指向脚本库/剧本库实体；内联 content 兼容回退 |
| alert_bindings | 告警实例级绑定 | 与能力匹配互补：绑定=人工指定的确定关系，匹配=AI 推荐的模糊关系 |

---

## 4. 剧本 YAML meta 扩展

在剧本文件头扩展字段（`internal/control/playbook/parser.go` `Playbook` struct 增加；
`parameters` 段已由模板系统支持，全库统一复用）：

```yaml
name: 清理磁盘空间
description: 清理 journal 日志与包缓存，释放磁盘空间
version: "1.0"
category: disk                      # 新增
alert_types: ["OWL-DSK-001", "OWL-DSK-002"]   # 新增：多绑定
symptoms:                           # 新增：语义召回语料
  - 磁盘使用率超过阈值
  - 根分区剩余空间不足
risk: low                           # 新增
rollback: ""                        # 新增：回滚剧本路径或描述
timeout: 600                        # 新增
applicable_os: ["linux"]            # 新增
parameters:                         # 已有（TemplateMeta.parameters）
  - name: keep_days
    type: int
    required: false
    default: 3
```

`playbooks` 表同步扩展列（alert_types / symptoms / risk / rollback / timeout …），
`SyncFromDir` 时解析入库；剧本上传/编辑表单同步暴露这些字段。

---

## 5. 匹配管线规范

### 5.1 六层漏斗

```
告警 (type, message, severity, metric_snapshot, node)
 │
 ├─ R0 SQL 硬召回（必选，零 AI）
 │    alert_types 命中 ∪ category 匹配告警类型 category
 │    ∩ 节点兼容（applicable_os / groups / labels）
 │    ∩ 治理过滤（未删除；AI 来源未审核 → 仅标记展示，不进自动推荐）
 │
 ├─ R1 语义召回（可选层）
 │    embeddings 余弦 top-K（阈值 ≥0.5，K≤20）；缺席 → 跳过
 │
 ├─ L1 LLM 卡片精选（可用时）
 │    候选能力卡片 + 告警上下文 → 单次调用 → 严格 JSON（§5.3 契约）
 │
 ├─ L2 参数装配
 │    LLM 从 metric_snapshot 提取参数值 → ValidateParams() 校验
 │    必填缺失 / 校验失败 → 该步转人工
 │
 ├─ L3 安全闸门（现状复用）
 │    SyntaxGate（仅 AI 生成）→ blacklist → DecideApproval 审批矩阵
 │    risk=high 永远人工；confidence<0.6 至少转审批（§6）
 │
 ├─ L4 执行与验证
 │    执行分发（script/playbook/sop）；exit_code=0 ≠ 修复成功
 │    验证窗口内告警自动 resolved 才记 heal_success（§7）
 │
 └─ L5 反馈闭环
      exec/success 统计回写 → 成功率进下次 L1 卡片先验 → 失败触发 AI 复盘（§7）
```

### 5.2 召回层与 embeddings 表（不引入向量库）

**选型论证**：脚本库+剧本库现实量级为几十~几百条。一条 1024 维 float32 向量 4KB，
千条共 4MB；Go 暴力余弦千条 <1ms、万条 <10ms，ANN 索引（HNSW/IVF，面向百万~亿级）
完全用不上。项目是"单二进制 + 单 SQLite 文件"定位，引 Qdrant/Milvus 破坏轻部署；
`modernc.org/sqlite` 为纯 Go 驱动不支持加载 sqlite-vec 等 C 扩展，换驱动/引 CGO
得不偿失。**结论：SQLite 存向量 + Go 暴力余弦。**

```sql
CREATE TABLE IF NOT EXISTS embeddings (
    ref_kind   TEXT NOT NULL,          -- script | playbook
    ref_id     TEXT NOT NULL,
    model      TEXT NOT NULL,          -- 向量化模型标识
    dim        INTEGER NOT NULL,
    vector     BLOB NOT NULL,          -- float32 little-endian
    state      TEXT NOT NULL DEFAULT 'ready',  -- pending | ready | stale
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (ref_kind, ref_id)
);
```

规则：

1. **向量化失败不阻塞入库**：state=pending，该项仅参与 R0 硬召回；后台补算 / 手动重建。
2. **换模型自动重建**：model 或 dim 不匹配 → 全量置 stale → 重新向量化。
3. 向量化语料 = `name + description + symptoms + category + tags` 拼接；
   查询语料 = 告警类型 name + description + 告警 message（截断）。
4. `internal/ai` 需新增 `Embeddings` 能力（OpenAI 兼容 `/embeddings`），provider 不支持
   时 R1 整层缺席（降级链自动生效）。能力探测见 §10。

### 5.3 LLM 精选契约（能力卡片）

**卡片内容规范**（替代现状 `buildPrompt` 截 200 字符脚本正文——正文对匹配无用且烧 token）：

```
- id=<ref_id> kind=<script|playbook> name=<名称>
  描述: <description>
  症状: <symptoms，最多 3 条>
  分类: <category> 风险: <risk> 幂等: <bool>
  参数: <参数名:类型 列表，无则省略>
  近期战绩: 近 30 天执行 N 次成功 M 次（无记录则"无记录"）
```

**输出 JSON 契约**：

```json
{
  "reasoning": "根因解读与选择理由（中文，简洁）",
  "steps": [
    {"ref_kind": "script", "ref_id": "abc123", "confidence": 0.9,
     "why": "磁盘使用率 92% 且候选脚本对症", "params": {"keep_days": 3}}
  ]
}
```

**铁律**：

1. **宁缺毋滥**：无合适候选 → `steps` 为空 + reasoning 说明，禁止硬凑。
2. `confidence < 0.6` 的步骤不得自动执行（L3 强制至少转审批）。
3. 引用不存在的 `ref_id` → 丢弃该步（沿用 `normalize` 现有防幻觉逻辑）。
4. **内容以库为准，不信任 LLM 改写**（`ai_advisor.go normalize` 原则扩展到 ref 场景）。
5. 剧本类卡片与脚本类卡片同场竞选（修复 G4）。
6. 单次调用，最多 `maxSteps` 步（沿用 3 步上限）；LLM 失败 → 降级链（§5.4）。

### 5.4 四级降级链（每级独立可用）

```
L0  SQL 硬召回        —— 永远可用，零 AI；元数据规范的价值所在
L0.5 embedding 召回   —— 可选层；provider 不支持/未向量化 → 跳过
L1  LLM 卡片精选      —— 可用时启用；候选本就少时直选即够，embedding 只是库变大后的 prompt 瘦身优化
L2  规则兜底          —— RuleBasedAdvisor 风险排序推荐（现状保留，今天就在生产路径上）
```

LLM、embedding、规则三者任一缺席，告警自愈照常工作，只是"聪明程度"降档。

### 5.5 Agent 工具出口（MatchService 双出口）

匹配服务（MatchService，P1 建）**一套实现、两个出口**：

```
                     ┌─▶ Advisor 接口（§5.1 管线）—— 告警自动自愈（无人值守）
 MatchService ───────┤
 （R0 召回 + L1 精选） └─▶ internal/ai ToolRegistry —— Web AI 助手对话式触发（人在环路）
```

Web AI 助手按请求装配 Agent（`internal/ai/agent.go`），工具注册于 `ToolRegistry`
（`internal/ai/tools.go` `Tool` 接口：Name / Description / Parameters / Validate /
Execute）。新增 4 个工具：

| 工具 | 输入 | 行为 | 安全约束 |
|------|------|------|----------|
| `list_capabilities` | 过滤条件（category / kind） | 列能力卡片（含治理状态与统计） | 只读 |
| `match_remediation` | 告警 ID 或 类型+节点 | R0+R1+L1 → 排序卡片 + confidence + 理由 | 只读，不执行 |
| `draft_script_meta` | 脚本内容 | §8 静态分析 + LLM 草稿 | 只返回草稿不落库 |
| `run_capability` | ref_kind / ref_id + params + 目标节点 | L2 校验 → L3 三道闸 → 提交 RemedyRun | 用户确认即审批形态之一；矩阵规则不变 |

**配套仓库级 skill**：`skills/capability/`（SKILL.md + schema.md）供外部编码 agent
（opencode / zcode 等）在录入脚本/剧本时产出合规元数据——与产品内工具消费同一 schema，
录入侧与运行侧的元数据契约由 `skills/capability/schema.md` 单点维护。

`run_capability` 的执行权限完全复用 §6 闸门，不新增旁路。

---

## 6. 安全与验证规范

- **三道闸全复用**：`SyntaxGate`（仅 AI generated）→ `blacklist.Checker` →
  `DecideApproval` 审批矩阵。匹配管线的产出只是"建议"，**执行权限判定权完全留在现有矩阵**。
- **审批矩阵微扩展**：`ApprovalInput` 增加 `Confidence float64`；
  `confidence < 0.6 → 至少 DecisionApproval`（只升不降，其余矩阵规则不变）。
- `risk=high` 永远 `DecisionHuman`（矩阵第 2 条已保证）；critical 告警须审批、
  多节点须审批（canary 先行）均不变。
- **修复成功判据**：`exit_code=0 ≠ 修复成功`。规范：执行完成后进入验证窗口
  （settings 键 `monitor.heal_verify_window`，默认 600 秒），窗口内告警自动
  `resolved` → 记 `heal_success`；超时未 resolved → 记 `heal_attempted`。
  `Alert.RemedyID` 字段已存在（`internal/monitor/alert.go`），用于关联闭环。

---

## 7. 反馈闭环规范

1. **统计双维度**：`exec_success`（步骤退出码）与 `heal_success`（验证窗口内告警 resolved）
   分开记录；`exec_count/success_count` 以 `heal_success` 为准回写能力来源表
   （scripts / playbooks），remedies 表统计沿用 `RecordExecution`。
2. **先验注入**：L1 卡片的"近期战绩"字段取自上述统计（滚动 30 天），让匹配随使用变准。
3. **失败复盘钩子**：`heal_success=false` 且 LLM 可用时，异步生成复盘建议
   （调参 / 换对策 / 修订 symptoms 元数据），落 `RemedyRun` 备注，人工审阅后可一键更新元数据。

---

## 8. AI 辅助录入流程（规范的可持续性）

> 规范最大的敌人是"没人愿意填元数据"。录入时让 AI 打草稿、人来拍板。

```
上传脚本 ──▶ 静态分析（纯本地，零成本）──▶ LLM 草稿 ──▶ 前端表单预填 ──▶ 人工确认 ──▶ reviewed=true 落库
```

**静态分析规则表**：

| 信号 | 提取 | 实现方式 |
|------|------|----------|
| shebang `#!/usr/bin/env python3` | `language` 草稿 | 字符串前缀 |
| `getopts` / `getopt` / `argparse` / `$1..$9` / `usage()` | `parameters` 草稿 | 正则提取 |
| `rm -rf` / `mkfs` / `dd` 写盘 / `reboot` / `shutdown` / `kill -9` | `risk=high` + `destructive=true` 草稿 | 复用 `internal/control/blacklist` 词表 |
| `systemctl restart/stop` | `risk=medium` + side_effects 草稿 | 模式匹配 |
| 纯只读命令（df/free/status/du/journalctl） | `risk=low` 草稿 | 模式匹配 |

**LLM 草稿**：输入 = 脚本正文（截断）+ 静态分析结果；输出 JSON =
description / symptoms / category / 建议 alert_types / 参数语义描述。

**审核铁律**：AI 起草的元数据**永不直接落库为已审核**；前端确认后
`reviewed=true`。剧本上传同理（解析 YAML tasks 摘要作为 LLM 输入）。

---

## 9. 分期实施路线

每期遵循 AGENTS.md：TDD（/tdd）+ E2E 通过后原子提交。

### P0 · 脚本库实体
- scripts 表 + `script_library_path` settings 键 + SyncFromDir + CRUD API + 前端脚本库页
- **验收**：上传 → 分类目录正确 → 列表/详情/编辑/删除可用；服务重启后索引一致（E2E）

### P1 · 匹配与执行打通
- remedies/scripts/playbooks 扩展列（§2.3、§4）；`MatchService`（R0 硬召回）
- `AIAdvisor.buildPrompt` 卡片化 + 剧本类放行（修 G4）；ScriptExecutor 按 language 选解释器
- Remedy `ref_kind/ref_id` 关联 + 内联回退兼容
- **验收**：种子磁盘告警 → 自愈选中脚本库脚本 → 按正确解释器执行成功 → 告警 resolved（E2E）

### P2 · 语义增强
- `internal/ai` Embeddings 能力 + 能力探测；embeddings 表 + R1 召回 + 降级链装配
- AI 辅助录入（ai-meta 草稿）
- Agent 工具出口：`list_capabilities` / `match_remediation` / `draft_script_meta` / `run_capability`（§5.5）
- **验收**：关闭 embedding provider 时管线照常（降级链 E2E）；ai-meta 草稿 → 确认 → reviewed 入库；AI 助手对话"XX 告警能自动修吗"返回候选能力卡片，确认后经闸门执行成功

### P3 · 反馈闭环
- 验证窗口 + heal_success 判定 + stats 先验注入 + 失败复盘钩子
- **验收**：执行后告警 resolved 记 success；下一轮卡片显示真实成功率（E2E）

---

## 10. 开放问题

1. **nodes 表 OS 字段**：`applicable_os` 过滤依赖节点操作系统信息；采集方式
   （collector 是否已有 uname 抓取、持久化到哪）待定。
2. **embedding provider 支持矩阵**：openai / dashscope（text-embedding-v3/v4）支持；
   deepseek / anthropic 不支持。`internal/ai` 需统一 `Embeddings` 接口 + 启动时能力探测，
   探测失败 → R1 永久缺席（仅提示）。
3. **剧本 `script:` 动作按 ID 引用脚本库**：现为文件路径引用（`{{PLAYBOOK_DIR}}`），
   是否扩展 `script_ref` 语义指向库 ID，与 preflight 检查联动。
4. **入站 webhook 告警源**：外部告警接入后，`symptoms` 语义通道价值放大；
   需要外部告警 → 内部告警类型的规范化映射层。
5. **remedies 与 alert_bindings 的 UI 边界**：类型级能力匹配（自动）与实例级绑定
   （人工指定）并存，告警详情页的信息架构需避免混淆。

---

*最后更新: 2026-09-17*
