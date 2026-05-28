# Tasks

- [x] Task 1: 创建 blacklist 包
  - [x] 创建 `internal/control/blacklist/blacklist.go` — 定义 `Config`、`Rule` 结构体、内置默认规则 `DefaultRules()`、从 `~/.owl/blacklist.yaml` 加载配置的 `LoadConfig()`、保存配置的 `SaveConfig()`
  - [x] 创建 `internal/control/blacklist/checker.go` — 定义 `Checker` 结构体，实现 `Check(user, command string) *CheckResult` 方法，按行分割命令并逐行匹配规则模式
  - [x] `CheckResult` 包含：是否命中、匹配到的规则列表、匹配行列表、涉及的节点用户信息

- [x] Task 2: 编写 blacklist 包单元测试
  - [x] 测试用例：配置文件存在时正确加载
  - [x] 测试用例：配置文件不存在时使用默认规则
  - [x] 测试用例：root 用户命中 rm 规则
  - [x] 测试用例：普通用户不命中 root 专属规则
  - [x] 测试用例：任意用户命中 `*` 全局规则
  - [x] 测试用例：空命令/安全命令不命中
  - [x] 测试用例：多条规则命中

- [x] Task 3: `owl exec run` 集成黑名单检查
  - [x] 在 `cmd/cli/cmd/exec/run.go` 中增加 `--force` / `-f` flag
  - [x] 在 `runExecRun()` 中，解析目标节点后、执行命令前，先解析节点用户信息，调用 `blacklist.Check()` 进行危险命令检测
  - [x] 命中黑名单时：输出警告信息（包含危险命令行、匹配规则、节点用户），等待用户输入 y/N 确认
  - [x] `--force` 标志或用户确认 y 时继续执行；用户输入 N 或非 y 时中止

- [x] Task 4: `owl exec script` 集成黑名单检查
  - [x] 在 `cmd/cli/cmd/exec/script.go` 中增加 `--force` / `-f` flag
  - [x] 在 `runScript()` 中，解析目标节点后、执行脚本前，读取脚本内容并调用 `blacklist.Check()` 进行危险命令检测
  - [x] 命中黑名单时：输出警告信息（包含危险命令行、匹配规则、节点用户），等待用户输入 y/N 确认
  - [x] `--force` 标志或用户确认 y 时继续执行；用户输入 N 或非 y 时中止

# Task Dependencies
- Task 2 依赖 Task 1
- Task 3 依赖 Task 1
- Task 4 依赖 Task 1
- Task 3 和 Task 4 可并行执行
- Task 2 可与 Task 3、Task 4 并行执行
