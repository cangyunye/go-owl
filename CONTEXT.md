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
`--nodes`/`--group`/`--label`), each node gets its own `Task` record, and all
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
