# 设计文档 4: 智能监控与告警体系（Monitoring & Alerting）

## 1. 概述

### 1.1 背景与目标

go-owl 当前是「人发命令」的运维工具：节点管理、批量执行、剧本编排、AI 辅助。本设计将其升级为
**发现问题 → 判定 → 告警（带 ID）→ 对策匹配 → 处置 → 验证** 的闭环系统：

- 自动发现被管理机器的资源与报错（磁盘、内存、负载、网络、服务、日志错误、失联）
- 每条告警带**告警 ID**，通过 ID 可定位到可用对策（Remedy）
- 简单问题可通过脚本自愈；复杂问题给人工排查指引；遵守用户自定对策
- 告警具备通知能力：Web 控制台告警专栏 + 告警历史 + 分级处理 + 邮件推送 + 自定义 Webhook

### 1.2 已确认的决策（2026-06 头脑风暴结论）

| # | 决策点 | 结论 |
|---|--------|------|
| 1 | 采集方式 | **无代理 SSH 轮询**起步；未来 agent 优先复用现成方案（node_exporter、各数据库 exporter），按需再设计，本期不考虑 |
| 2 | 执行机 | owl-serve 兼容当执行机，不引入独立 worker |
| 3 | 存储 | 继续 SQLite；**30 天保留，每天清理一次**；不上独立时序库（详见 5.4） |
| 4 | 通知 | owl 页面告警专栏/历史/分优先级处理；重点告警邮件推送；**不需要**企微/钉钉；暴露**自定义 Webhook** |
| 5 | 自动执行 | **默认全关**，仅按告警 ID 单独配置放行 |
| 6 | 第一期范围 | 简单优先（规则驱动，不依赖 AI） |
| 7 | 生态 | 全中文（UI/文档/告警文案） |

### 1.3 与现有系统关系（复用清单）

- **SSH 层**（`internal/ssh`）：采集器直接复用节点凭据与连接池
- **历史库**（`internal/history`）：`Cleanup(retentionDays)` 已有清理模式，监控清理沿用同风格
- **serve 插件**（`cmd/plugins/serve`）：后台调度、REST API、Web 页面均挂载于此
- **黑名单**（`internal/control/blacklist`）：Phase 2 自愈执行的策略闸门
- **AI agent**（`internal/ai`）：Phase 2 处置引擎，新增 `HandleAlert` 工具
- **nodes 表**：采集目标直接来自已注册节点
- **`internal/metrics`**：是 Prometheus 端点抓取适配器（可选插件 go-owl-metrics），与本设计无关，**不改造**；本设计新建 `internal/monitor`

### 1.4 非目标（本期明确不做）

- 推送式 agent / node_exporter 接入
- 独立时序数据库、趋势预测（磁盘 N 天后耗尽等）
- AI 处置闭环（Phase 2，见 §10 前瞻设计）
- 每节点阈值覆盖（全局阈值足够）、维护窗口 UI（先做全局静默开关）
- 告警报表/SLA 统计

## 2. 总体架构

```
┌────────────────────────── owl-serve（兼执行机） ──────────────────────────┐
│                                                                          │
│  ┌────────────┐    ┌──────────────┐    ┌──────────────┐    ┌──────────┐  │
│  │ Collector   │───▶│ 规则引擎      │───▶│ 告警实例      │───▶│ 通知器    │  │
│  │ (SSH 轮询)  │    │ (阈值+持续)   │    │ (状态机/抑制) │    │ 邮件/Webhook│ │
│  └─────┬──────┘    └──────┬───────┘    └──────┬───────┘    └────┬─────┘  │
│        │                  │                    │                 │        │
│        ▼                  ▼                    ▼                 ▼        │
│  ┌──────────────────────────────────────────────────────────────────┐    │
│  │ SQLite (owl.db)：metrics_YYYYMM 按月分区 + alert_types + alerts   │    │
│  │                    + remedies + notify_channels；每日清理任务      │    │
│  └──────────────────────────────────────────────────────────────────┘    │
│        ▲                  ▲                    ▲                          │
│  ┌─────┴──────┐   ┌───────┴───────┐   ┌───────┴────────┐                │
│  │ REST API   │   │ Web 告警页     │   │ 对策库管理页     │                │
│  └────────────┘   └───────────────┘   └────────────────┘                │
└──────────────────────────────────────────────────────────────────────────┘
        ▲ SSH (df/free/uptime/sar/journalctl ...)
        │
   ┌────┴────┐   ┌────┴────┐        ┌────┴────┐
   │ node-01 │   │ node-02 │  ...   │ node-N  │
   └─────────┘   └─────────┘        └─────────┘
```

## 3. 模块布局

```
internal/monitor/             # 监控核心引擎（纯逻辑，无 serve 依赖，可单测）
  metric.go                   #   指标定义与输出解析（df/free/uptime/net/...）
  collector.go                #   SSH 轮询采集器（并发、超时、重试）
  store.go                    #   SQLite 存储（按月分区 + 批量写入 + 清理）
  rules.go                    #   规则引擎（阈值 + 持续窗口）
  alerts.go                   #   告警实例生命周期与抑制
  remedies.go                 #   对策库
  notify.go                   #   通知接口定义
cmd/plugins/serve/monitor/    # serve 集成
  engine.go                   #   后台调度（采集循环、规则评估、每日清理）
  email.go                    #   SMTP 通知实现
  webhook.go                  #   自定义 Webhook 实现
  handler.go                  #   REST API
cmd/plugins/serve/handler/    # 已有 handler 目录追加告警页数据接口（或并入 monitor/handler.go）
```

## 4. 采集器设计

### 4.1 采集方式

- 复用 `internal/ssh` 对**全部已注册节点**执行采集命令，无代理、无新组件
- 轮询间隔默认 **60s**，采集超时默认 **10s**，并发上限默认 **10**（可配）
- 单节点一次 SSH 会话内串行执行多条采集命令（减少连接开销），输出按命令分段解析

### 4.2 指标定义（首批）

| 指标名 | 采集命令（Linux） | 说明 |
|--------|-------------------|------|
| `cpu.usage` | `top -bn1` 或读 `/proc/stat` | CPU 使用率 % |
| `load.load1/5/15` | `cat /proc/loadavg` | 负载 |
| `mem.used_pct` | `free -m` | 内存使用率 % |
| `mem.swap_pct` | `free -m` | swap 使用率 % |
| `disk.usage.<mount>` | `df -P` | 每挂载点使用率 % |
| `disk.inodes.<mount>` | `df -Pi` | 每挂载点 inode 使用率 % |
| `net.rx_bytes.<iface>` | `cat /proc/net/dev`（两次采样差值） | 入流量 B/s |
| `net.tx_bytes.<iface>` | 同上 | 出流量 B/s |
| `net.tcp_estab` / `net.tcp_timewait` | `ss -s` / `ss -tan` 计数 | TCP 状态数 |
| `sys.zombies` | `ps` 计数 | 僵尸进程数 |
| `sys.procs` | `ps` 计数 | 进程总数 |
| `svc.<name>.active` | `systemctl is-active <name>`（0/1） | 关键服务状态（注册在节点/全局配置） |
| `err.journal_errors` | `journalctl -p err --since 5min` 行数 | 近 5 分钟错误日志数 |
| `err.oom` | `dmesg | grep -i "out of memory" --since` | OOM 事件计数 |
| `net.latency_ms` | SSH 往返耗时（由采集器统计） | 网络延迟/失联判定 |

> 采集命令集合可在配置中裁剪；`svc.*` 与 `err.*` 需额外指定服务名/关键字，默认采集通用项。

### 4.3 失联判定

SSH 连接失败（超时/认证失败/网络不可达）→ 记录 `node.ssh_unreachable=1`，触发 OWL-OSS-001。

### 4.4 失败处理

单节点采集失败不中断整体轮询；失败计入该节点 `last_collect_error`，连续失败 N 次（默认 3）触发失联告警。

## 5. 存储设计（SQLite，30 天保留）

### 5.1 指标表（按月分区）

```sql
-- 动态指标：metrics_YYYYMM，每月一张表
CREATE TABLE IF NOT EXISTS metrics_<YYYYMM> (
  node_id TEXT NOT NULL,
  metric  TEXT NOT NULL,
  ts      INTEGER NOT NULL,      -- unix 秒
  value   REAL NOT NULL,
  PRIMARY KEY (node_id, metric, ts)
) WITHOUT ROWID;
```

- 写入：`INSERT OR REPLACE`，**批量事务**（每批 5000 行），WAL 模式
- 300 节点 × 60 指标 × 60s ≈ 18,000 行/分钟（300 行/秒），modernc SQLite 完全可承受
- 空间估算：30 天 ≈ 2,600 万行 ≈ 1.5~2 GB（不含磁盘指标时更低），可接受

### 5.2 告警与配置表

```sql
CREATE TABLE alert_types (            -- 告警类型注册表（告警 ID 的载体）
  id          TEXT PRIMARY KEY,       -- 如 OWL-DSK-001
  category    TEXT NOT NULL,          -- disk|mem|cpu|net|svc|err|avail
  name        TEXT NOT NULL,          -- 中文名
  description TEXT NOT NULL DEFAULT '',
  default_severity TEXT NOT NULL DEFAULT 'warn',   -- info|warn|critical
  default_params TEXT NOT NULL DEFAULT '{}',       -- 阈值 JSON，如 {"disk_pct":90,"duration":120}
  auto_approve INTEGER NOT NULL DEFAULT 0,         -- ★ 自动执行放行，默认全关
  notifiable  INTEGER NOT NULL DEFAULT 1,          -- 是否参与通知
  enabled     INTEGER NOT NULL DEFAULT 1,
  builtin     INTEGER NOT NULL DEFAULT 1           -- 内置种子不可删
);

CREATE TABLE alerts (                 -- 告警实例
  id          TEXT PRIMARY KEY,       -- 实例 ID（可读，如 AL-<unix>-<seq>）
  alert_type_id TEXT NOT NULL,
  node_id     TEXT NOT NULL,
  severity    TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'open',   -- open|acked|resolved
  message     TEXT NOT NULL,                  -- 中文描述（含指标快照摘要）
  metric_snapshot TEXT NOT NULL DEFAULT '{}', -- 触发时指标快照 JSON
  first_seen  INTEGER NOT NULL,
  last_seen   INTEGER NOT NULL,
  resolved_at INTEGER,
  remedy_id   TEXT
);
CREATE UNIQUE INDEX idx_alerts_active
  ON alerts(alert_type_id, node_id) WHERE status != 'resolved';  -- 活跃告警去重

CREATE TABLE remedies (               -- 对策库
  id            TEXT PRIMARY KEY,
  alert_type_id TEXT NOT NULL,
  name          TEXT NOT NULL,
  kind          TEXT NOT NULL,        -- script|playbook|sop
  content       TEXT NOT NULL,
  risk          TEXT NOT NULL,        -- low|medium|high
  rollback      TEXT NOT NULL DEFAULT '',
  source        TEXT NOT NULL,        -- builtin|ai|user
  reviewed      INTEGER NOT NULL DEFAULT 0,   -- ai 生成须审核
  auto_approve  INTEGER NOT NULL DEFAULT 0,   -- 该对策是否允许自动执行（Phase 2）
  exec_count    INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);

CREATE TABLE notify_channels (        -- 通知渠道
  id          TEXT PRIMARY KEY,
  kind        TEXT NOT NULL,          -- email|webhook
  name        TEXT NOT NULL,          -- 中文名
  config      TEXT NOT NULL,          -- JSON：smtp 服务器/账号/收件人 或 webhook URL/鉴权头
  severity_min TEXT NOT NULL DEFAULT 'warn',   -- 最低通知级别
  alert_types TEXT NOT NULL DEFAULT '',        -- 空=全部；逗号分隔告警 ID
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_at  INTEGER NOT NULL
);
```

### 5.3 每日清理任务

- serve 启动时立即执行一次，此后每 **24h** 执行一次（固定 02:00 附近）
- 逻辑：`DELETE FROM metrics_<本月/上月> WHERE ts < now-30d`；**DROP 早于保留期的整张分区表**；同时清理 `alerts` 中 `resolved_at < now-30d` 的记录
- 与 `history.Cleanup` 互不干扰，各管各的表

### 5.4 为什么不引入独立时序库（决策记录）

300 节点规模下 SQLite + WAL + 批量写入 + 按月分区即可满足吞吐与查询；独立时序库（bbolt 自研、
VictoriaMetrics 单机、Prometheus tsdb 嵌入）均增加部署/维护成本。将来若需 PromQL 或规模破千，
`internal/monitor/store.go` 已隔离存储层，可平移到 VictoriaMetrics，无需改规则/告警引擎。

## 6. 告警规则与告警 ID

### 6.1 告警 ID 编码

`OWL-<类别>-<三位编号>`，类别：`DSK/MEM/CPU/NET/SVC/ERR/OSS(可用性)`。

### 6.2 内置告警类型种子（第一期）

| 告警 ID | 名称 | 默认判定（阈值 + 持续时长） | 级别 |
|---------|------|------------------------------|------|
| OWL-DSK-001 | 磁盘使用率过高 | usage > 90% 持续 2 次采样 | warn |
| OWL-DSK-002 | inode 使用率过高 | inodes > 90% 持续 2 次 | warn |
| OWL-MEM-001 | 内存使用率过高 | used_pct > 90% 持续 3 次 | warn |
| OWL-MEM-002 | swap 使用过高 | swap_pct > 50% 持续 3 次 | warn |
| OWL-CPU-001 | 负载持续过高（卡顿） | load1 > 核数×1.5 持续 3 次 | warn |
| OWL-NET-001 | 入/出流量异常 | rx/tx > 阈值（默认 500MB/s，可配）持续 2 次 | warn |
| OWL-NET-002 | TCP 连接堆积 | TIME_WAIT > 5000 持续 2 次 | warn |
| OWL-SVC-001 | 关键服务停止 | svc.active = 0 持续 2 次 | critical |
| OWL-SVC-002 | 服务反复重启 | 5 分钟内 restart 计数 > 3 | critical |
| OWL-ERR-001 | 日志高频错误 | journal 错误新增 > 50 条/5 分钟 | warn |
| OWL-ERR-002 | OOM 事件 | dmesg 出现 OOM | critical |
| OWL-OSS-001 | 节点失联 | SSH 连续失败 ≥ 3 次 | critical |

- 阈值存于 `alert_types.default_params`，admin 可改；`builtin=1` 的行禁止删除、允许改阈值与开关
- 规则引擎为纯函数：`(metric 最新值, 参数, 历史窗口) → 是否触发`

### 6.3 告警实例状态机

```
            触发(规则满足)          人工/超时            指标恢复持续 N 次
  ──▶ open ──────────────▶ acked ──────────────▶ resolved
       │  ▲                    │                       ▲
       │  └── 升级(severity 提升, 重新通知)            │
       └───────────────────────────────────────────────┘   (自动恢复)
```

- **去重/抑制**：部分唯一索引保证同一 (类型, 节点) 仅一个活跃实例；`last_seen` 持续刷新，避免刷屏
- **自动恢复**：指标回落并持续 N 次采样（默认 3），实例自动置为 resolved（记录原因 `auto_recovered`）
- **升级**：warn 持续超过 1 小时未 ack → 升级 critical 并重新通知（默认开启，可配）

### 6.4 静默开关

全局 `monitor.silence`（布尔 + 可选截止时间）：静默期内不创建新实例、不通知，已有实例正常流转。

## 7. 对策库（Remedy）

### 7.1 三种来源

| 来源 | 说明 | 自动执行资格 |
|------|------|--------------|
| builtin | 随二进制发布的常用对策（磁盘清理、重启服务、清缓存） | 需该告警 ID 放行（auto_approve） |
| ai | Phase 2 由 AI 现场生成 | **必须人工审核**（reviewed=1）且告警 ID 放行 |
| user | 用户自定 SOP/脚本，绑定到告警 ID | 用户自定放行策略 |

### 7.2 与告警 ID 的关系

- 每条 remedy 绑定一个 `alert_type_id`；同一类型可有多条对策（如「磁盘清理」低风险 vs 「扩容提示」SOP）
- 告警详情页展示该类型全部可用对策，**按风险排序**
- 用户对策优先级最高：同类型同时存在 user 与 builtin 时，页面/AI 优先推荐 user 对策

### 7.3 自动执行放行（决策 #5 的落地）

- `alert_types.auto_approve` **默认 0（全关）**
- admin 按告警 ID 逐个开启；开启后 Phase 2 允许该类型低风险对策无人值守执行
- 放行记录（谁、何时、为何）入审计日志

## 8. 通知

### 8.1 通知规则

- 每条通知渠道独立配置：`severity_min`（info/warn/critical）+ `alert_types`（空=全部）
- 触发时机：新告警 open、severity 升级、active 超时未处理（可选）
- 邮件与 Webhook 都是**异步发送**，失败重试 3 次，失败记录入日志不阻塞主流程

### 8.2 邮件（重点告警）

- SMTP 配置存于 notify_channels.config（JSON）：`host/port/username/password/from/to[]`
- 正文模板：告警 ID、节点、级别、中文描述、指标快照、对策入口（Web 链接）

### 8.3 自定义 Webhook

- 配置：`url` + 可选 `headers`（如鉴权 Token）
- 载荷固定 JSON 结构：

```json
{
  "alert_id": "AL-1750000000-1",
  "alert_type": "OWL-DSK-001",
  "alert_type_name": "磁盘使用率过高",
  "node_id": "node-web-01",
  "node_name": "web-01",
  "severity": "warn",
  "status": "open",
  "message": "磁盘使用率 93.5%（挂载点 /），阈值 90%",
  "metric_snapshot": {"disk.usage./": 93.5},
  "first_seen": 1750000000,
  "web_url": "http://owl-host/alerts/AL-1750000000-1"
}
```

- admin 提供「发送测试」按钮校验连通性

## 9. Web 控制台（owl-serve）

### 9.1 告警专栏

- **告警列表**：按级别（critical 置顶）/状态/节点筛选 + 分页；行内展示 ID、类型中文名、节点、级别、首次/最近时间、状态徽标
- **告警详情**：指标快照表格、时间线（open→acked→resolved）、该类型可用对策列表（风险排序 + 「采纳为对策」入口，Phase 2）、处置按钮（ack/resolve）
- **告警历史**：已解决列表，支持按类型/节点/时间段检索；保留 30 天

### 9.2 设置页

- 告警类型管理：开关、阈值、默认级别、放行（auto_approve）
- 对策库管理：CRUD、来源/审核状态展示、测试执行（Phase 2）
- 通知渠道：邮件/Webhook 配置 + 测试发送
- 全局静默开关

### 9.3 REST API（权限对齐现有 viewer/editor/operator/admin）

```
reader:   GET  /api/v1/alerts            GET /api/v1/alerts/:id
          GET  /api/v1/alert-types       GET /api/v1/remedies?alert_type_id=
          GET  /api/v1/metrics?node_id=&metric=&from=&to=        (图表)
operator: POST /api/v1/alerts/:id/ack    POST /api/v1/alerts/:id/resolve
admin:    PUT  /api/v1/alert-types/:id   POST/PUT/DELETE /api/v1/remedies
          GET/POST/PUT/DELETE /api/v1/notify-channels
          POST /api/v1/notify-channels/:id/test
```

## 10. AI 处置闭环（Phase 2，已部分落地）

**P2-M1 手动处置编排（已完成，提交 efbef16）**：用户在告警详情多选对策、拖拽排序、
按序串行执行——这是 AI 自愈的执行底座：AI 处置只是把「谁选对策/谁排序」从人换成模型，
执行引擎（RemedyRun）完全复用。执行引擎要点：

- 脚本经 base64 管道执行，避免转义问题；sop 人工步骤自动跳过
- 黑名单闸门：命中危险命令不执行（默认不允许 force）
- 失败即停 / 失败继续可选；执行中可停止；对策内容执行时快照
- 每步完成回写 `RecordExecution`，为 AI 推荐积累成功率数据

**P2-M2（AI 处置引擎，接口层 + 闸门链已完成，真实 LLM 待凭据）**：AI 选对策 + 排序 + 现场生成脚本，闸门链：

```
告警触发 → AI 富化(根因解读) → 对策匹配 → 未命中则 AI 起草脚本
  → ★语法闸门(bash -n / YAML 校验，失败关闭)         ✅ 已实现
  → ★策略闸门(复用黑名单 + 禁自动清单)              ✅ 已实现
  → ★审批矩阵(severity×risk×scope → auto/审批/仅人工) ✅ 已实现
  → ★执行(复用 RunExecutor，入历史库)               ✅ 已实现
  → 反馈(RecordExecution)                            ✅ 已实现
```

已落地：`DisposalAdvisor` 接口（AI 实现与规则兜底可插拔）、`RuleBasedAdvisor`、
`AutoHealer` 管线、引擎接线（新告警且类型放行 → 自愈）。
`internal/monitor` 只暴露「告警 → 上下文（指标快照+日志摘要）」与「处置结果回写」两个接口，
AI 引擎在 `internal/ai` 扩展，二者不互相依赖。

## 11. 里程碑拆分（TDD，每步先写测试）

| 里程碑 | 内容 | 关键测试 |
|--------|------|----------|
| **M1 采集+存储** | `internal/monitor`：指标解析（df/free/loadavg/net）、SSH 采集器、SQLite 按月分区 + 批量写入 + 清理任务 | 解析各命令输出；写入/查询/清理；WAL 下并发写 |
| **M2 规则+告警** | 规则引擎（阈值+持续窗口）、告警类型注册表（内置种子）、告警实例状态机 + 去重 + 自动恢复 + 升级 | 触发/恢复/去重/升级各状态转换；内置种子完整性 |
| **M3 对策库** | remedies CRUD、与告警类型绑定、用户对策优先、放行开关（默认关） | 来源/审核/放行权限逻辑 |
| **M4 通知** | 通知接口 + 邮件 SMTP + Webhook（含重试、测试发送） | 载荷格式；失败重试；severity/类型过滤 |
| **M5 serve 集成** | 后台调度引擎（采集循环/规则评估/每日清理/静默）、REST API、Web 告警页与设置页 | API 权限矩阵；调度生命周期（停止/重启） |
| **M6（Phase 2）** | AI 处置引擎（闸门链、审批流、反馈闭环） | 语法闸门拦截、审批矩阵、canary 流程 |

## 12. 配置（config.yaml 增量）

```yaml
monitor:
  enabled: true          # serve 启动时是否启用监控引擎
  interval: 60           # 采集间隔（秒）
  timeout: 10            # 单节点采集超时（秒）
  concurrency: 10        # 并发采集节点数
  retention_days: 30     # 指标与告警保留天数
  silence_until: ""      # 全局静默截止时间（空=不静默）
```

## 13. 开放问题

1. SMTP 凭据存 DB（可页面配置）还是 config.yaml？—— 倾向 DB（notify_channels.config），与页面管理一致
2. 轮询间隔 60s / 保留 30 天是否满足实际运维节奏？后续可在设置页放宽
3. `svc.*` 关键服务清单由节点标签驱动（如 label `watch=nginx,mysql`）还是全局配置？—— 倾向标签驱动，与现有 label 体系一致
4. Web 告警页是否需要自动刷新（SSE/轮询）？—— 第一期用页面手动刷新 + 轮询 30s 即可
