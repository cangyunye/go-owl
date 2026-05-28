# P2P 中继传输 Spec

## Why
当前 `owl file transfer` 的扩散传输只是"控制节点分批直传"，源节点不实际参与文件转发，没有实现真正的 P2P 中继。需要改为：控制节点仅传给首批源节点，后续由源节点之间通过 bash 脚本接力传输（仅限密码认证节点），密钥认证节点始终由控制节点直传。

## What Changes
- 新增一个 bash 中继脚本 `owl-relay.sh`，负责在源节点上 SCP 文件到子节点，返回结构化 CSV 结果
- 修改 `runDiffusionTransfer`：首批仍由控制节点直传；后续批次中，密码认证节点由源节点接力，密钥认证节点始终由控制节点直传
- 控制节点监控每个源节点的脚本执行结果，收集结构化的成功/失败/超时信息
- 已完成节点可继续接收新子任务，支持失败重分配
- **BREAKING**: 扩散传输行为变更——后续批次不再全部从控制节点发出

## Impact
- Affected specs: 无（新功能）
- Affected code:
  - `cmd/cli/cmd/file/transfer.go` — 修改 `runDiffusionTransfer`
  - `internal/control/transfer/` — 新增中继任务调度逻辑
  - `internal/ssh/` — 可能需要扩展远程脚本执行能力
  - 新增 `scripts/owl-relay.sh` — bash 中继脚本

## ADDED Requirements

### Requirement: 安全策略——密钥认证节点控制节点直传
出于安全考虑，仅使用 SSH 密钥认证（无密码）的目标节点 SHALL 始终由控制节点直接传输，不得分配给源节点做中继。

#### Scenario: 密钥节点不参与中继
- **WHEN** 控制节点构建中继子任务时
- **THEN** SHALL 过滤掉 `SSHPassword` 为空的节点，将其列入控制节点直传列表
- **AND** 仅将 `SSHPassword` 非空的节点分配给源节点进行中继传输

### Requirement: Bash 中继脚本 (owl-relay.sh)
系统 SHALL 提供一个 bash 脚本 `owl-relay.sh`，部署到源节点后能独立完成文件 SCP 转发并返回结构化结果。

#### Scenario: 脚本接收参数执行 SCP 转发
- **WHEN** 控制节点通过 SSH 远程调用脚本，传入参数
- **THEN** 脚本解析参数，对每个目标节点执行 SCP 传输，返回 CSV 格式的批量结果

#### Scenario: 密码认证
- **WHEN** 源节点需要向目标节点 SCP 文件
- **THEN** 脚本 SHALL 接收 `--passwords` 参数（逗号分隔的密码列表，与 `--targets` 按索引一一对应，等长）
- **AND** 脚本 SHALL 通过 `sshpass` + `SSHPASS` 环境变量传递密码进行 SCP
- **AND** 若 `sshpass` 不可用，对应目标标记为 `auth_failed: sshpass not available`

#### Scenario: 跳过严格主机密钥检查
- **WHEN** 目标节点的主机密钥未在源节点的 known_hosts 中
- **THEN** 脚本 SHALL 自动添加 `-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null` 跳过检查

#### Scenario: 脚本超时处理
- **WHEN** 单个目标节点的 SCP 操作超过配置的超时时间（默认 30s）
- **THEN** 脚本 SHALL 终止该节点的传输，在结果中标记为 `timeout`，继续处理其余目标

#### Scenario: 脚本返回规范化 CSV 结果
- **WHEN** 脚本执行完成（无论成功或失败）
- **THEN** 脚本 SHALL 输出 CSV 格式（第一行为表头 `target,status,error,duration_ms`），后续每行一个目标结果。`status` 取值：`success` / `failed` / `timeout` / `auth_failed`。CSV 中的字段值如果包含逗号或换行，需用双引号包裹。

#### Scenario: 连接失败
- **WHEN** 目标节点不可达
- **THEN** 脚本 SHALL 返回 `status: "failed"` 及 `error` 字段包含具体原因

### Requirement: 认证凭据传递
控制节点 SHALL 仅将密码凭据传给源节点，**不得**传递 SSH 私钥。

#### Scenario: 仅传递密码
- **WHEN** 控制节点构建中继子任务时
- **THEN** SHALL 从 `ResolvedNode.SSHPassword` 提取目标节点密码，通过 `--passwords` 参数传给脚本
- **AND** SHALL NOT 将控制节点的 SSH 私钥文件复制或引用到源节点

### Requirement: 中继任务调度
控制节点 SHALL 在首批源节点完成传输后，将剩余节点分为两组：密码认证节点分配给源节点中继，密钥认证节点由控制节点直传。

#### Scenario: 首批控制节点直传
- **WHEN** 用户执行 `owl file transfer` 且节点数 >= threshold
- **THEN** 控制节点 SHALL 先将文件 SCP 到前 `source-count` 个源节点

#### Scenario: 节点分流
- **WHEN** 首批源节点传输完成
- **THEN** 控制节点 SHALL 将剩余目标节点按认证方式分流：`SSHPassword` 非空 → 加入中继任务列表；`SSHPassword` 为空 → 加入控制节点直传列表

#### Scenario: 源节点接力（密码认证节点）
- **WHEN** 中继任务列表非空
- **THEN** 控制节点 SHALL 均衡分配给已完成源节点，生成子任务列表

#### Scenario: 控制节点直传（密钥认证节点）
- **WHEN** 直传列表非空
- **THEN** 控制节点 SHALL 并行 SCP 文件到这些节点（与现有的直接传输行为一致）

#### Scenario: 控制节点分发脚本并调用
- **WHEN** 中继子任务列表生成完毕
- **THEN** 控制节点 SHALL 通过 SSH 将 `owl-relay.sh` 上传到源节点、添加执行权限，然后远程执行脚本并传入子任务参数（含 `--passwords`）

#### Scenario: 控制节点收集脚本结果
- **WHEN** 源节点上的脚本执行完成
- **THEN** 控制节点 SHALL 解析 CSV 输出，更新各目标节点的状态，并汇总到 `DiffusionTransfer`

#### Scenario: 已完成节点接收新子任务
- **WHEN** 某个源节点完成了本轮分配的子任务，且仍有未完成的中继节点
- **THEN** 控制节点 SHALL 可以将新的子任务分配给该已完成节点

#### Scenario: 失败回退到控制节点
- **WHEN** 某个源节点执行脚本整体失败
- **THEN** 控制节点 SHALL 将其子节点转由控制节点直接传输

#### Scenario: 全部完成汇总
- **WHEN** 所有目标节点都已有最终状态
- **THEN** 控制节点 SHALL 打印汇总：成功数、失败数、超时数，并记录历史

## MODIFIED Requirements
无。这是对扩散传输内部实现的增强，不改变现有上传/下载的行为。
