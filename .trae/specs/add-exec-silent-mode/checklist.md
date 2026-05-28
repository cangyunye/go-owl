# Checklist

- [x] `owl exec run --help` 输出中包含 `--silent` / `-s` 参数说明
- [x] `owl exec script --help` 输出中包含 `--silent` / `-s` 参数说明
- [x] `--silent` 模式下 `run` 命令不打印 emoji 图标和节点输出内容
- [x] `--silent` 模式下 `run` 命令以表格形式展示每节点结果（Node、Status、Exit Code、Duration 列）
- [x] `--silent` 模式下 `run` 命令每完成一个节点立即追加一行（流式输出）
- [x] `--silent` 模式下 `run` 命令表格末尾打印汇总统计行
- [x] `--silent` 模式下 `script` 命令不打印脚本信息和节点输出内容
- [x] `--silent` 模式下 `script` 命令以表格形式展示每节点结果
- [x] `--silent` 模式下 `script` 命令每完成一个节点立即追加一行
- [x] `--silent` 模式下 `script` 命令表格末尾打印汇总统计行
- [x] `--silent` 与 `--format json` 同时使用时，`--format json` 优先生效
- [x] `--silent` 模式下黑名单危险命令警告和交互确认仍然正常显示（不影响安全性）
- [x] 测试文件中 `TestExecRunFlags` 验证 `--silent` 标志存在且默认值为 `false`
- [x] 测试文件中 `TestExecScriptFlags` 验证 `--silent` 标志存在且默认值为 `false`
- [x] 现有测试全部通过（`go test ./cmd/cli/cmd/exec/...`）
