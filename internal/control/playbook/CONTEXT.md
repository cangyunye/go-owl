# Playbook Execution — Domain Glossary

## Core Concepts

### Execution Mode (`execution_mode`)
The strategy by which a Playbook handles task failures. Set at the Playbook level in YAML.

- **`fail_continue`** (default): All tasks execute sequentially regardless of failures. Errors are recorded, and the final status reflects any failures, but execution does not stop. Suitable for batch processing, monitoring checks.
- **`pipeline`**: On any unignored error, terminate all subsequent tasks immediately. PreTasks failure skips everything; PostTasks must be absent (validated at parse time). Suitable for deployment flows, dependency chains.

### Default Block (`default`)
A YAML block at the Playbook level that provides default values for CLI-level execution parameters. Each field in the `default` block is overridden when the corresponding CLI flag is explicitly provided (CLI wins, full replacement not union).

- **`default.groups`** — Default node groups to target. Overridden by `--group` CLI flag. When multiple groups are specified, nodes belonging to any of the groups are included (union), with duplicates removed.
- **`default.tags`** — Default task tags to execute. Only tasks matching these tags are run. Overridden by `--tags` CLI flag.
- **`default.skip_tags`** — Default task tags to skip. Tasks matching these tags are not run. Overridden by `--skip-tags` CLI flag.
- **`default.timeout`** — Default connect and command timeout values. Overridden by `--default-connect-timeout` / `--default-command-timeout` CLI flags.
- **`default.retry`** — Default retry configuration (max, interval, max_interval). Overridden by `--default-retry` / `--default-retry-interval` / `--default-retry-max-interval` CLI flags.

### Node Selection
The mechanism by which target nodes are chosen for a Playbook run. Priority (highest to lowest):
1. `--nodes` — Explicit node IDs
2. `--group` — Groups from CLI or `default.groups` from YAML
3. `--label` — Label selectors
4. `hosts` — Hosts list in the Playbook YAML
5. All available nodes (fallback)

### Task Filter
The mechanism by which which tasks are executed or skipped. Based on tags:
- `--tags` (CLI) or `default.tags` (YAML): only tasks with matching tags run
- `--skip-tags` (CLI) or `default.skip_tags` (YAML): tasks with these tags are skipped
- If neither is set, all tasks run

### Task Error Handling

- **`ignore_errors: true`** — Prevents an error from being classified as a failure. The task result still records the error, but it does not affect execution flow. Highest priority: if set, neither `pipeline` nor `any_errors_fatal` trigger.
- **`any_errors_fatal: true`** — Marks this task as fail-stop within `fail_continue` mode. Overridden by `pipeline` mode (pipeline treats all tasks as fatal).

### Execution Phases

- **PreTasks** — Initial phase. Failures are terminal by default, blocking Tasks and PostTasks.
- **Tasks** — Main phase. In `fail_continue` mode, failures are logged but execution continues; final status reflects the aggregate.
- **PostTasks** — Cleanup phase. Failures are terminal by default. Not allowed in `pipeline` mode.

### Checkpoint / Resume

- **Default** — Always restart from scratch.
- **`--resume` flag** — Look up the most recent failed execution for the same playbook path in the history database (SQLite `operations` table) and continue from the recorded `current_task_index`/`current_task_phase`.

## Priority Chain

```
ignore_errors (bypasses error detection entirely)
  → pipeline / any_errors_fatal (decides whether to abort on failure)
    → default fail_continue behavior (run-to-completion, aggregate failures)
```

## History Database (SQLite / DuckDB)

`~/.owl/owl.db` — `operations` table extended with:
- `execution_mode` — `pipeline` or `fail_continue`
- `playbook_path` — path to the executed playbook file (used for resume matching)
- `current_task_index` / `current_task_phase` — checkpoint coordinates for resume
