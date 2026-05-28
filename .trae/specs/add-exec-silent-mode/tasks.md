# Tasks

- [x] Task 1: Add silent mode to `owl exec run` command
  - [x] SubTask 1.1: Add `execSilent` bool variable and register `--silent` / `-s` flag in `NewRunCmd()` (run.go)
  - [x] SubTask 1.2: Modify `runExecRun()` — when `execSilent` is true, skip all informational prints (command banner, node count, mode, task ID, summary emoji line), print table header before execution starts, and alter `processResult` closure to output one table row per result instead of calling `printResult()`
  - [x] SubTask 1.3: Handle `--format` priority — when `--format json` or `--format detail` is explicitly set alongside `--silent`, the explicit format takes precedence and silent is ignored
  - [x] SubTask 1.4: Print summary row after all results: `Total: N success, M failed`

- [x] Task 2: Add silent mode to `owl exec script` command
  - [x] SubTask 2.1: Add `scriptSilent` bool variable and register `--silent` / `-s` flag in `NewScriptCmd()` (script.go)
  - [x] SubTask 2.2: Modify `runScript()` — when `scriptSilent` is true, skip informational prints (script path, node count, execution mode, dest dir, keep, args) but keep blacklist check warnings and interactive confirmation; print table header before execution; output one table row per result in the result loop; print summary row after the loop

- [x] Task 3: Update tests
  - [x] SubTask 3.1: Add `--silent` flag existence and default value test to `TestExecRunFlags` in exec_test.go
  - [x] SubTask 3.2: Add `--silent` flag existence and default value test to `TestExecScriptFlags` in exec_test.go

# Task Dependencies
- Task 2 depends on Task 1 (shared understanding of table format, but can be done in parallel since script.go is independent)
- Task 3 depends on Task 1 and Task 2
