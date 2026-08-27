# go-owl Domain Glossary

## Task (任务)

A `Task` is an execution record in the Web Console (`store.Task`). Each task
corresponds to one command or script run on **one node**. A single `owl exec`
invocation targeting N nodes produces N tasks, all sharing the same `batch_id`.

Contrast with playbook: a playbook step is also called "task" in YAML, but is a
different concept — a step in a sequential pipeline within a playbook run.

## Batch (批次)

A "batch" is purely a **UI grouping concept** — there is no backend `Batch`
entity. When a user executes a command on multiple nodes (via
`--nodes`/`--groups`/`--label`), each node gets its own `Task` record, and all
are linked by a shared `batch_id`. The UI groups them for display.

## Node (节点)

A managed SSH-accessible machine. Has `id`, `name`, `address`, `port`, `user`,
optional `password`/`ssh_key`, `groups` (string list), `labels` (key-value map).

## Group (分组)

A string attribute on a node for organizational grouping. Used for
filtering/selecting nodes. E.g. `web`, `db`, `prod`.

## Label (标签)

A key-value metadata pair on a node, e.g. `{"env": "production"}`. More
flexible than groups — arbitrary key-value pairs.

## Execution (执行)

The act of running a command or script on one or more nodes. Execution can be
parallel or sequential, with configurable timeouts and retry.

## Command vs Script

- **Command**: a single shell command string, run via `ssh <node> <command>`.
- **Script**: executable content (could be multi-line), run inline via
  `echo '<content>' | ssh <node> bash` or uploaded then executed.

## User (用户)

A Web Console account that can log in and operate the console. Has a `role`
(viewer/editor/operator/admin) governing what it may do. Owns its Shortcut
Commands; deleting a user deletes their shortcuts.
_Avoid_: account, operator (when meaning the user)

## New-User Defaults (新用户默认指令)

The initial set of Shortcut Commands granted to a User exactly once, at account
creation. A creation-time snapshot: when the default set later changes, existing
users are not retroactively seeded — they add new ones manually.
_Avoid_: seeding, provisioning (when implying ongoing sync)

## Shortcut Command (快捷命令)

A user-owned, named command template displayed horizontally in the Execution
Console for quick reuse. Composed of a `name` (display label) and a `command`
(one Command). Distinct from Command: a shortcut wraps a Command with a name and
a user owner; a Command is just the raw string.
_Avoid_: shortcut template, saved command, 快捷指令

## i18n

## Message Catalog (消息目录)

A per-language collection of key→translation entries used to render tool
copy. The CLI never embeds user-facing Chinese literals; it references a
message key and the active language decides which string is shown.
_Avoid_: translation map, locale file

## Character Encoding (字符编码)

The byte-level encoding used to emit and consume text (UTF-8 / GBK / Big5).
Determined independently of language: an English user can still be on GBK,
a Chinese user on UTF-8.
_Avoid_: charset only where it means bytes

## Tool Copy (工具文案)

Strings the tool itself generates and which are looked up in the Message
Catalog. Only these are translated.
_Avoid_: literal string, hardcoded text

## Pass-through Data (透传数据)

Content originating from remote hosts or files (node names, command output,
playbook YAML bodies). It is never translated — only character-encoded.
_Avoid_: translated data, localized output

## Monitoring (监控)

The umbrella capability covering collection, alerting, remedies and
notifications — the closed loop from "detect a problem" to "disposed and
verified". See docs/design/04_MONITORING_ALERTING.md.

## Metric (指标)

A periodic numeric data point collected from a managed node, uniquely
identified by (node, metric name, timestamp). Metrics are the input to alert
rules. The node itself is never inferred — a metric without a node_id is
meaningless.
_Avoid_: 采样点, 监控点

## Collector (采集器)

A background task that periodically runs SSH collection commands against
registered nodes and writes results into the metric store. Agentless — it
reuses the node's existing SSH credentials; no agent is deployed on targets.
_Avoid_: agent, 探针, 采集代理

## Alert Type (告警类型)

An identifiable class of problem, identified by a stable Alert ID
(`OWL-<category>-<nnn>`), carrying a detection rule, default severity, and
references to remedies. The Alert ID is the key used to look up applicable
remedies — the type is identity, the rule is the detection expression.
_Avoid_: 告警规则 (a rule is the detection expression, not the identity)

## Alert (告警实例)

One concrete occurrence of an Alert Type on one node, with a lifecycle status
(open/acked/resolved) and a metric snapshot. At most one active Alert exists
per (type, node); dedup is by partial unique index on non-resolved rows.
_Avoid_: 告警事件, alert event

## Remedy (对策)

A disposal plan bound to an Alert Type: a script, a playbook, or a manual
runbook (SOP). Distinguished by source (builtin / ai / user) and review
status; user-supplied remedies outrank builtin ones for the same type.
_Avoid_: 处理方案 (口语), runbook

## Auto-Approval (自动执行放行)

A per-Alert-Type switch deciding whether its remedies may execute unattended.
**Off by default for every type**; only types explicitly configured by an
admin are approved, and the approval action is audited.
_Avoid_: 自愈开关, auto-run

## Suppression (抑制)

Within a window, the same (Alert Type, node) does not create a new Alert
instance; the existing one's `last_seen` refreshes instead. Prevents alert
flapping.
_Avoid_: 去重 (dedup is a data-layer concern)

## Retention (保留期)

The retention period for metrics and alert records (default 30 days),
enforced by a daily cleanup task that deletes expired rows and drops
partition tables older than the cutoff.
_Avoid_: 清理周期, cleanup interval
