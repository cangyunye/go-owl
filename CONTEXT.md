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
