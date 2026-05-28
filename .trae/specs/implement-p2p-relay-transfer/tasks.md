# Tasks

- [ ] Task 1: 编写 bash 中继脚本 `scripts/owl-relay.sh`
  - [ ] 1.1 创建脚本框架：参数解析（`--source` 源文件路径, `--targets` 逗号分隔目标列表（格式 `user@host:/path`）, `--timeout` 每目标超时秒数, `--passwords` 逗号分隔密码列表（与 `--targets` 等长，按索引一对一对应））
  - [ ] 1.2 实现逐目标 SCP 传输逻辑，自动添加 `-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null`
  - [ ] 1.3 实现密码认证：按索引取 `--passwords` 中对应密码，通过 `sshpass` + `SSHPASS` 环境变量；若 `sshpass` 不可用，标记为 `auth_failed`
  - [ ] 1.4 实现每个目标的 timeout 机制（使用 `timeout` 命令包装 scp，默认 30s）
  - [ ] 1.5 实现 CSV 结果输出：先输出表头行 `target,status,error,duration_ms`，再逐目标追加一行 CSV，字段值含逗号或换行时用双引号包裹

- [ ] Task 2: 定义 Relay 子任务数据结构和 CSV 协议
  - [ ] 2.1 在 `internal/control/transfer/` 新增 `relay_task.go`，定义 `RelaySubTask`（源节点 ID、目标列表、文件信息、超时、每目标密码）
  - [ ] 2.2 定义 `RelayTargetResult` 结构体（Target/Status/Error/DurationMs），与脚本 CSV 输出一致
  - [ ] 2.3 实现 `RelaySubTask.ToShellArgs()` 方法：将子任务序列化为 `--source` `--targets` `--timeout` `--passwords` 命令行参数
  - [ ] 2.4 实现 `ParseRelayResults(csvOutput string) ([]RelayTargetResult, error)`：使用 Go `encoding/csv` 解析，跳过表头，容错 malformed 行

- [ ] Task 3: 实现源节点远程脚本执行能力
  - [ ] 3.1 在 `internal/control/transfer/` 新增 `relay_executor.go`，实现 `RelayExecutor` 结构体
  - [ ] 3.2 实现 `DeployScript(ctx, nodeID string) error`：通过 TransferManager/SCP 将 owl-relay.sh 上传到源节点 `/tmp/owl-relay.sh` 并 chmod +x
  - [ ] 3.3 实现 `ExecuteRelay(ctx, nodeID string, task *RelaySubTask) ([]RelayTargetResult, error)`：SSH 远程执行脚本，收集 stdout CSV 并解析

- [ ] Task 4: 改造 `runDiffusionTransfer` 实现真正 P2P 中继
  - [ ] 4.1 修改 `cmd/cli/cmd/file/transfer.go` 中的 `runDiffusionTransfer`：保留首批控制节点直传逻辑不变
  - [ ] 4.2 实现节点分流：将剩余未传输节点分为两组——`SSHPassword` 非空（中继列表）和 `SSHPassword` 为空（直传列表）
  - [ ] 4.3 密钥认证节点直传：控制节点并行 SCP 文件到直传列表，直接完成
  - [ ] 4.4 实现 `scheduleRelayTasks()`：将中继列表均衡分配给已完成源节点，生成 `RelaySubTask` 列表
  - [ ] 4.5 实现 `dispatchRelayRound()`：并行向各已完成源节点部署脚本 + 远程执行，收集结果并更新状态
  - [ ] 4.6 实现已完成源节点的再调度：一轮完成后，已完成源节点可接收新的中继子任务
  - [ ] 4.7 实现失败回退：源节点执行失败时，其子节点转由控制节点直接传输

- [ ] Task 5: 更新文档和测试
  - [ ] 5.1 更新 `docs/dev/FILE_TRANSFER_ARCHITECTURE.md` 第 6 章：反映真正 P2P 中继架构 + 节点分流策略
  - [ ] 5.2 更新 `docs/user/FILE.md` 第 4 章：更新工作原理说明和示例输出
  - [ ] 5.3 为 `scripts/owl-relay.sh` 编写单元测试（使用 mock 目标节点）
  - [ ] 5.4 为 `RelaySubTask`/`RelayExecutor`/`ParseRelayResults` 编写 Go 单元测试

# Task Dependencies
- Task 2 依赖 Task 1（脚本协议确定后定义结构体）
- Task 3 依赖 Task 2（RelayExecutor 需要 RelaySubTask 类型）
- Task 4 依赖 Task 2, Task 3（扩散传输改造需要调度器和执行器）
- Task 5 依赖 Task 4（文档反映最终实现）

Task 1 可独立启动。
Task 2 和 Task 1 可部分并行（协议层面先敲定字段）。
