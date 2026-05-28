# go-owl 自动化测试方案：保证每项功能与用户使用手册匹配

## 一、现状分析

### 1.1 文档体系

`docs/user/` 目录包含 12 份用户手册，涵盖了工具全部功能模块：

| 文档 | 模块 | 子命令数 | 内嵌测试用例 |
|------|------|---------|-------------|
| QUICKSTART.md | 快速入门 | — | 8 步 |
| NODE.md | 节点管理 | 10 | TC-NODE-001 ~ TC-NODE-009 |
| EXEC.md | 批量命令执行 | 2 | TC-EXEC-001 ~ TC-EXEC-007 |
| FILE.md | 文件传输 | 3 | TC-FILE-001 ~ TC-FILE-009 |
| PLAYBOOK.md | 剧本管理 | 5 | TC-PLAY-001 ~ TC-PLAY-007 |
| SESSION.md | 会话管理 | 3 | TC-SESSION-001 ~ TC-SESSION-004 |
| AI.md | AI 助手 | 5 | TC-AI-001 ~ TC-AI-006 |
| HISTORY.md | 历史记录 | 2 | TC-HIST-001 ~ TC-HIST-006 |
| SETTINGS.md | 系统设置 | 3 | TC-SETTINGS-001 ~ TC-SETTINGS-003 |
| SESSION_USAGE.md | 会话补充 | — | — |
| USAGE.md | AI 使用指南 | — | — |

**总计约 55+ 个内嵌测试用例**，均为人工操作描述，非自动化测试。

### 1.2 现有测试覆盖

| 层级 | 已有测试 | 覆盖情况 |
|------|---------|---------|
| **internal/ 核心逻辑** | 16 个 `*_test.go` 文件 | 较好：command、retry、blacklist、playbook parser/executor、diffusion、async、llm 等 |
| **cmd/cli/cmd/ CLI 命令** | 4 个 `*_test.go` 文件 | **严重不足**：仅 history、file/transfer、session/list、playbook/run |
| **tests/unit/** | 2 个 Go 文件 | command_test、variable_test |
| **tests/integration/** | 1 个 Go 文件 | exec_test |
| **tests/scripts/** | 3 个 Bash 脚本 | test-exec、test-node、test-history，覆盖较浅 |
| **tests/FUNCTIONAL_TESTS.md** | 35 个人工用例 | 无自动化 |

### 1.3 核心缺口

以下 CLI 命令模块**完全没有自动化测试**：

- `node/` — 9 个子命令均无 Go 测试（add、list、update、remove、status、groups、labels、import、ping、check）
- `exec/` — run.go 和 script.go 无 Go CLI 测试
- `file/` — upload.go、download.go 无测试（仅 transfer 有）
- `playbook/` — list、validate、info 无测试（仅 run 有）
- `session/` — attach、history 无测试（仅 list 有）
- `ai/` — 全部无测试
- `settings/` — 全部无测试
- `async/` — 全部无测试
- `common/` — ParseLabels、ParseNodeList、OutputFormatter 等核心函数无测试

---

## 二、测试策略

采用**三层金字塔模型**，由底向上逐步构建：

```
        ╱  Layer 3: E2E 测试 ╲        对真实节点执行 owl 命令，验证端到端行为
       ╱  (Bash 脚本, ~30 个)  ╲       — 需要 SSH 测试节点环境
      ╱───────────────────────────╲
     ╱  Layer 2: CLI 单元测试     ╲     测试命令结构、flag 注册、helper 函数
    ╱   (Go _test.go, ~50 个文件)  ╲    — 无需 SSH，纯代码级别
   ╱─────────────────────────────────╲
  ╱  Layer 1: 核心逻辑单元测试       ╲   测试 internal/ 中的纯逻辑、算法
 ╱  (已有 16 个，补充 ~5 个)          ╲   — 无需 SSH，可完全 Mock
╱───────────────────────────────────────╲
```

### 三条核心原则

1. **文档即契约**：用户手册中描述的每个命令、每个 flag、每种输出格式，都必须有对应的自动化断言
2. **分层隔离**：能不依赖 SSH 的测试尽量不依赖 — CLI 结构测试和逻辑测试占比 >80%，E2E 测试占比 <20%
3. **可维护性**：测试用例编号与用户手册内嵌的 TC 编号保持一致，形成可追溯的映射表

---

## 三、实施步骤

### 阶段一：基础设施准备

#### 步骤 1.1 — 统一测试辅助工具包

- 新建 `cmd/cli/cmd/common/common_test.go`
  - 测试 `ParseLabels()` 函数：正常、边界、非法格式
  - 测试 `ParseNodeList()` 函数：单节点、多节点、空字符串、尾部逗号
  - 测试 `ParseGroupList()` 函数
  - 测试 `OutputFormatter.FormatNodes()` 函数：table/json/yaml 三种格式输出到 buffer
  - 测试 `OutputFormatter.FormatNode()` 函数：单节点详情
  - 测试 `NewOutputFormatter()` 函数：各种 format 参数解析

#### 步骤 1.2 — 建立 CLI 命令测试辅助函数

- 新建 `cmd/cli/cmd/testutil/command_test_helpers.go`
  - 提供 `AssertCommandExists(t, parent, name)` — 验证子命令存在
  - 提供 `AssertFlagExists(t, cmd, flagName, shortHand, defaultValue, usage)` — 验证 flag 定义与文档一致
  - 提供 `AssertHelpContains(t, cmd, text)` — 验证帮助信息包含文档描述的内容
  - 提供 `ExecuteCommand(t, cmd, args...) string` — 执行命令并捕获输出

#### 步骤 1.3 — 建立文档-测试映射表

- 在 `tests/` 目录下创建 `TEST_MAPPING.md` 作为自动化测试与用户手册的映射索引
- 每个文档中的 TC 编号对应到具体的自动化测试函数

---

### 阶段二：Layer 1 — 补充核心逻辑单元测试

> 目标：internal/ 层覆盖率提升至 80%+

#### 步骤 2.1 — common/model 测试补充

- 在 `internal/common/model/node_test.go` 中补充：
  - `Node` 结构体序列化/反序列化（JSON/YAML）
  - `NodeStatus` 常量值验证

#### 步骤 2.2 — 节点解析器测试

- `internal/node/local_source_test.go` 已有部分测试，补充：
  - 测试从 JSON 文件加载节点配置
  - 测试节点分组/标签过滤

#### 步骤 2.3 — history 数据访问层测试

- 新建 `internal/history/db_test.go`（如果不存在）
  - 测试 DuckDB 数据库 CRUD 操作
  - 测试 session 记录的写入和查询
  - 测试历史清理逻辑

#### 步骤 2.4 — SSH 配置测试补充

- 补充 `internal/ssh/timeout_test.go`：
  - 测试 `TimeoutConfig` 各字段默认值和验证逻辑

---

### 阶段三：Layer 2 — CLI 命令结构性测试（核心交付）

> 目标：每个 CLI 模块的每个子命令都有结构性验证 + helper 函数单元测试

#### 步骤 3.1 — node 模块测试（最复杂模块，9 个子命令）

新建 `cmd/cli/cmd/node/node_test.go`：

| 测试函数 | 对应文档 TC | 验证内容 |
|---------|-----------|---------|
| `TestNodeCmdExists` | TC-NODE-000 | node 父命令存在，含 11 个子命令 |
| `TestNodeAddFlags` | TC-NODE-001 | `--name`(必填)、`--address`(必填)、`--port`(默认22)、`--user`(默认root)、`--password`、`--ssh-key`、`--proxy-jump`、`--groups`、`--labels`/-l |
| `TestNodeListFlags` | TC-NODE-002 | `--group`、`--label`、`--status`、`--format`(table/json) |
| `TestNodeUpdateFlags` | TC-NODE-003 | `--name`、`--address`、`--port`、`--user`、`--password`、`--ssh-key`、`--groups`、`--labels`、`--status` |
| `TestNodeRemoveFlags` | TC-NODE-004 | 支持多 ID 参数 |
| `TestNodeStatusFlags` | TC-NODE-005 | `--all`、`--output` |
| `TestNodeGroupsSubcommands` | TC-NODE-005 | add/remove/list/show 四个子命令 |
| `TestNodeLabelsSubcommands` | TC-NODE-006 | set/remove/show 三个子命令 |
| `TestNodeImportFlags` | TC-NODE-007 | `-f/--file`、`--overwrite`、`--skip-existing`、`--dry-run`、`--template` |
| `TestNodeExportFlags` | TC-NODE-007 | `-f/--file`、`--nodes`、`--groups`、`--labels`、`-o/--format` |
| `TestNodePingFlags` | TC-NODE-008 | `--all`、`-t/--timeout`(默认3s) |
| `TestNodeCheckFlags` | TC-NODE-009 | `--all`、`-t/--timeout`(默认10s)、`-w/--workers`(默认5)、`--update` |
| `TestNodeSampleCmd` | — | sample 命令存在 |

#### 步骤 3.2 — exec 模块测试

新建 `cmd/cli/cmd/exec/exec_test.go`：

| 测试函数 | 对应文档 TC | 验证内容 |
|---------|-----------|---------|
| `TestExecCmdExists` | — | exec 父命令存在，含 run/script 子命令 |
| `TestExecRunFlags` | TC-EXEC-001~004 | `--nodes`、`--group`、`--label/-l`、`--status`、`--parallel`(默认true)、`--serial`、`--timeout`(默认60s)、`--connect-timeout`、`--command-timeout`、`--retry`、`--retry-interval`、`--no-retry`、`--async`、`-o/--output`(simple/detail/json)、`--no-color`、`-f/--force` |
| `TestExecScriptFlags` | TC-EXEC-005~006 | `--nodes`、`--group`、`--label/-l`、`--dest`(默认/tmp)、`--args`、`--timeout`(默认5m)、`--inline`、`--keep`、`-f/--force` |
| 新建 `cmd/cli/cmd/exec/run_test.go` | TC-EXEC-001~004 | `parseNodeList` 等 helper 函数测试 |
| 新建 `cmd/cli/cmd/exec/script_test.go` | TC-EXEC-005~006 | script 相关 helper 函数测试 |

#### 步骤 3.3 — file 模块测试

新建 `cmd/cli/cmd/file/file_test.go`：

| 测试函数 | 对应文档 TC | 验证内容 |
|---------|-----------|---------|
| `TestFileCmdExists` | — | file 父命令存在，含 upload/download/transfer 子命令 |
| `TestFileUploadFlags` | TC-FILE-001~003,005~006 | `--nodes`、`--group`、`--label/-l`、`-d/--dest`(默认/tmp)、`--mode`(默认0644)、`--parallel`(默认true)、`--overwrite`(默认true)、`--no-overwrite`、`--resume` |
| `TestFileDownloadFlags` | TC-FILE-004,007~008 | `--nodes`、`--group`、`--label/-l`、`-d/--dest`(默认.)、`--node`(单节点)、`--parallel`、`--subdir`、`--name-format`、`--resume` |
| `TestFileTransferFlags` | TC-FILE-009 | `--nodes`、`--all-nodes`、`--group`、`--label/-l`、`-d/--dest`(默认/tmp)、`--source-count`、`--fan-out`、`--threshold` |

已有的 `cmd/cli/cmd/file/transfer_test.go` 已有 helper 测试，保持不动，补充 flag 测试到 file_test.go。

#### 步骤 3.4 — playbook 模块测试

新建 `cmd/cli/cmd/playbook/playbook_test.go`：

| 测试函数 | 对应文档 TC | 验证内容 |
|---------|-----------|---------|
| `TestPlaybookCmdExists` | — | playbook 父命令存在，含 list/run/info/validate/create 子命令 |
| `TestPlaybookListFlags` | TC-PLAY-001 | `--library`、`-o/--output` |
| `TestPlaybookRunFlags` | TC-PLAY-003~004 | `--nodes`、`--group`、`--label/-l`、`--tags`、`--skip-tags`、`--extra-vars`、`--check`、`--diff`、`--default-connect-timeout`、`--default-command-timeout`、`--default-retry` |
| `TestPlaybookInfoCmd` | TC-PLAY-002 | info 命令存在 |
| `TestPlaybookValidateCmd` | TC-PLAY-005 | validate 命令存在 |

已有的 `cmd/cli/cmd/playbook/run_test.go` 包含 `adapterNodeManager` 测试，保持不动。

#### 步骤 3.5 — session 模块测试

新建 `cmd/cli/cmd/session/session_test.go`：

| 测试函数 | 对应文档 TC | 验证内容 |
|---------|-----------|---------|
| `TestSessionCmdExists` | — | session 父命令存在，含 attach/list/history 子命令 |
| `TestSessionAttachFlags` | TC-SESSION-001,004 | `--nodes`、`--ssh-config`、`--key`、`--timeout`(默认30m)、`--mode`(single/multi) |
| `TestSessionHistoryFlags` | TC-SESSION-003 | `<session-id>`、`--node`、`--last`、`--verbose`、`-n/--limit` |

已有的 `cmd/cli/cmd/session/list_test.go` 保持不动。

#### 步骤 3.6 — ai 模块测试

新建 `cmd/cli/cmd/ai/ai_test.go`：

| 测试函数 | 对应文档 TC | 验证内容 |
|---------|-----------|---------|
| `TestAICmdExists` | TC-AI-001 | ai 父命令存在，含 models、config 子命令 |
| `TestAIFlags` | TC-AI-001 | `--model`(默认gpt-4o)、`--provider`(默认openai)、`--api-key`、`--base-url`、`--timeout`(默认120s)、`--session` |
| `TestAIModelsFlags` | TC-AI-002 | `--provider`、`--api-key`、`--base-url`、`--timeout` |
| `TestAIConfigSubcommands` | TC-AI-003~004 | init/show 子命令存在 |
| `TestAIProviders` | USAGE.md | 验证代码中注册的 Provider 与文档描述一致：OpenAI、Anthropic、DashScope、DeepSeek |

#### 步骤 3.7 — history 模块测试

新建 `cmd/cli/cmd/history/history_test.go`（已有，但需补充）：

补充：
| 测试函数 | 对应文档 TC | 验证内容 |
|---------|-----------|---------|
| `TestHistoryCmdExists` | — | history 父命令存在，含 clean 子命令 |
| `TestHistoryFlags` | TC-HIST-001~004 | `--task-id`、`--node-id`、`--op-type`(command/file_transfer/playbook/node_manage)、`--status`、`--start-time`、`--end-time`、`--last`(1h/24h/7d)、`--limit`(默认50)、`--offset`、`--format`(table/json/yaml)、`--output`(导出文件)、`--verbose` |
| `TestHistoryCleanFlags` | TC-HIST-005 | `--days`(默认30)、`--force` |

#### 步骤 3.8 — settings 模块测试

新建 `cmd/cli/cmd/settings/settings_test.go`：

| 测试函数 | 对应文档 TC | 验证内容 |
|---------|-----------|---------|
| `TestSettingsCmdExists` | — | settings 父命令存在，含 show/set/target 子命令 |
| `TestSettingsShowCmd` | TC-SETTINGS-001 | show 命令存在 |
| `TestSettingsSetCmd` | TC-SETTINGS-002 | set 命令存在，支持 `server.address/timeout`、`output.format/color`、`diffusion.fan-out/source-count`、`defaults.timeout` |
| `TestSettingsTargetFlags` | TC-SETTINGS-003 | `--group`、`--label/-l`、`--nodes` |

#### 步骤 3.9 — async 模块测试

新建 `cmd/cli/cmd/async/async_test.go`：

| 测试函数 | 对应文档 TC | 验证内容 |
|---------|-----------|---------|
| `TestAsyncCmdExists` | — | async 父命令存在，含 list/status/wait/cancel/cleanup 子命令 |
| `TestAsyncFlags` | — | status 的 `--poll-interval` 等参数 |

#### 步骤 3.10 — 根命令测试

新建 `cmd/cli/cmd/root_test.go`：

| 测试函数 | 验证内容 |
|---------|---------|
| `TestRootCmdExists` | owl 根命令存在，Use="owl" |
| `TestRootCmdVersion` | version 命令/flag 存在 |
| `TestAllSubCommands` | 9 个子命令全部注册：node/exec/file/playbook/session/ai/history/settings/async/tui |
| `TestRootCmdHelp` | 帮助信息包含各模块描述 |

---

### 阶段四：Layer 3 — E2E Bash 测试脚本扩展

> 目标：覆盖所有可在测试节点上验证的命令行为

#### 步骤 4.1 — 重构现有 Bash 测试脚本为标准化框架

- 提取公共函数到 `tests/scripts/test_common.sh`
  - `check_env()` — 检查 owl 命令和测试节点
  - `assert_contains()` — 断言输出包含指定文本
  - `assert_output_format()` — 断言 table/json/yaml 格式正确
  - `assert_exit_code()` — 断言命令退出码
  - `summary()` — 输出测试统计摘要

#### 步骤 4.2 — 扩展 test-node.sh

新的 `tests/scripts/test-node.sh` 应覆盖：

| 脚本测试函数 | 文档 TC | 测试内容 |
|-------------|--------|---------|
| `test_node_add_basic` | TC-NODE-001 | 添加节点（密码认证） |
| `test_node_add_sshkey` | TC-NODE-001 | 添加节点（密钥认证） |
| `test_node_add_nonstandard_port` | TC-NODE-001 | 添加节点（非标准端口） |
| `test_node_add_with_groups_labels` | TC-NODE-001 | 添加节点（分组+标签） |
| `test_node_list_all` | TC-NODE-002 | 列出所有节点（验证 User 列） |
| `test_node_list_by_group` | TC-NODE-002 | 按分组筛选 |
| `test_node_list_by_label` | TC-NODE-002 | 按标签筛选 |
| `test_node_list_json_format` | TC-NODE-002 | JSON 格式输出 |
| `test_node_update` | TC-NODE-003 | 更新节点信息 |
| `test_node_groups_add` | TC-NODE-005 | 添加分组 |
| `test_node_groups_list` | TC-NODE-005 | 列出分组 |
| `test_node_labels_set` | TC-NODE-006 | 设置标签 |
| `test_node_labels_show` | TC-NODE-006 | 显示标签 |
| `test_node_labels_remove` | TC-NODE-006 | 删除标签 |
| `test_node_import_export` | TC-NODE-007 | 导出再导入 |
| `test_node_remove` | TC-NODE-004 | 删除节点 |

#### 步骤 4.3 — 扩展 test-exec.sh

新的 `tests/scripts/test-exec.sh` 应覆盖：

| 脚本测试函数 | 文档 TC | 测试内容 |
|-------------|--------|---------|
| `test_exec_run_single_node` | TC-EXEC-001 | 单节点执行简单命令 |
| `test_exec_run_multi_node` | TC-EXEC-002 | 多节点并行执行 |
| `test_exec_run_by_group` | TC-EXEC-003 | 按分组执行 |
| `test_exec_run_timeout` | TC-EXEC-004 | 超时处理 |
| `test_exec_run_json_output` | TC-EXEC-005 | JSON 格式输出 |
| `test_exec_run_error` | TC-EXEC-006 | 错误命令处理 |
| `test_exec_run_async` | TC-EXEC-007 | 异步执行 |
| `test_exec_script_file` | TC-EXEC | 脚本文件执行 |
| `test_exec_script_inline` | TC-EXEC | 内联模式执行 |
| `test_exec_script_with_args` | TC-EXEC | 带参数脚本 |

#### 步骤 4.4 — 新增 test-file.sh

新建 `tests/scripts/test-file.sh`：

| 脚本测试函数 | 文档 TC | 测试内容 |
|-------------|--------|---------|
| `test_file_upload_single` | TC-FILE-001 | 单节点上传 |
| `test_file_upload_multi` | TC-FILE-002 | 多节点上传 |
| `test_file_upload_group` | TC-FILE-003 | 分组上传 |
| `test_file_download_single` | TC-FILE-004 | 单节点下载 |
| `test_file_upload_overwrite` | TC-FILE-005 | 覆盖策略 |
| `test_file_upload_no_overwrite` | TC-FILE-006 | 不覆盖策略 |
| `test_file_download_multi_suffix` | TC-FILE-007 | 多节点下载（后缀命名） |
| `test_file_download_subdir` | TC-FILE-008 | 多节点下载（子目录） |
| `test_file_not_found` | TC-FILE-009 | 文件不存在错误处理 |

#### 步骤 4.5 — 新增 test-playbook.sh

新建 `tests/scripts/test-playbook.sh`：

| 脚本测试函数 | 文档 TC | 测试内容 |
|-------------|--------|---------|
| `test_playbook_list` | TC-PLAY-001 | 列出剧本 |
| `test_playbook_info` | TC-PLAY-002 | 查看剧本详情 |
| `test_playbook_run` | TC-PLAY-003 | 执行剧本 |
| `test_playbook_vars` | TC-PLAY-004 | 传递变量 |
| `test_playbook_validate` | TC-PLAY-005 | 验证语法 |

#### 步骤 4.6 — 新增 test-settings.sh

新建 `tests/scripts/test-settings.sh`：

| 脚本测试函数 | 文档 TC | 测试内容 |
|-------------|--------|---------|
| `test_settings_show` | TC-SETTINGS-001 | 显示配置 |
| `test_settings_set` | TC-SETTINGS-002 | 设置配置项 |
| `test_settings_target` | TC-SETTINGS-003 | 默认目标选择 |

#### 步骤 4.7 — 新增 test-history.sh（扩展）

扩展现有 `tests/scripts/test-history.sh`：

| 脚本测试函数 | 文档 TC | 测试内容 |
|-------------|--------|---------|
| `test_history_list` | TC-HIST-001 | 查看历史 |
| `test_history_by_node` | TC-HIST-002 | 按节点筛选 |
| `test_history_json_output` | TC-HIST-003 | JSON 输出 |
| `test_history_relative_time` | TC-HIST-004 | 相对时间筛选 |
| `test_history_clean` | TC-HIST-005 | 清理历史 |

#### 步骤 4.8 — 新增 test-version.sh

新建 `tests/scripts/test-version.sh`：

| 脚本测试函数 | 测试内容 |
|-------------|---------|
| `test_version_display` | `owl version` 显示版本号 |

---

### 阶段五：测试基础设施整合

#### 步骤 5.1 — 更新 Makefile 测试目标

- 根 `Makefile` 已有的 test 目标保持，确认 `test-all` 正确串联所有测试
- `tests/Makefile` 补充新脚本的调用：
  ```makefile
  test-bash:
      bash ./scripts/test-exec.sh
      bash ./scripts/test-node.sh
      bash ./scripts/test-file.sh
      bash ./scripts/test-playbook.sh
      bash ./scripts/test-settings.sh
      bash ./scripts/test-history.sh
      bash ./scripts/test-version.sh
  ```

#### 步骤 5.2 — 创建测试映射文档

- 在 `tests/TEST_MAPPING.md` 中建立 **文档 TC → 自动化测试函数** 的映射表
- 格式：

  | 用户手册 | TC 编号 | 测试层级 | 测试文件 | 测试函数 |
  |---------|--------|---------|---------|---------|
  | NODE.md | TC-NODE-001 | L2 | cmd/cli/cmd/node/node_test.go | TestNodeAddFlags |
  | NODE.md | TC-NODE-001 | L3 | tests/scripts/test-node.sh | test_node_add_basic |

#### 步骤 5.3 — 最终验证

```bash
# 1. 确保编译通过
make build

# 2. 运行所有单元测试（Layer 1 + 2，不需要 SSH）
make test-unit

# 3. 运行全部测试（需要 SSH 测试节点）
OWL_TEST_ENABLED=true OWL_TEST_NODES=self-test-1 make test-all

# 4. 生成覆盖率报告
make test-coverage
```

---

## 四、预计新增文件清单

### Go 测试文件（新增 ~15 个）

| 序号 | 文件路径 | 对应模块 |
|------|---------|---------|
| 1 | `cmd/cli/cmd/common/common_test.go` | 通用工具 |
| 2 | `cmd/cli/cmd/testutil/command_test_helpers.go` | 测试辅助 |
| 3 | `cmd/cli/cmd/root_test.go` | 根命令 |
| 4 | `cmd/cli/cmd/node/node_test.go` | 节点管理 |
| 5 | `cmd/cli/cmd/exec/exec_test.go` | 命令执行 |
| 6 | `cmd/cli/cmd/exec/run_test.go` | exec run helper |
| 7 | `cmd/cli/cmd/exec/script_test.go` | exec script helper |
| 8 | `cmd/cli/cmd/file/file_test.go` | 文件传输 |
| 9 | `cmd/cli/cmd/playbook/playbook_test.go` | 剧本管理 |
| 10 | `cmd/cli/cmd/session/session_test.go` | 会话管理 |
| 11 | `cmd/cli/cmd/ai/ai_test.go` | AI 助手 |
| 12 | `cmd/cli/cmd/settings/settings_test.go` | 系统设置 |
| 13 | `cmd/cli/cmd/async/async_test.go` | 异步任务 |
| 14 | `internal/history/db_test.go` | 历史数据访问层 |
| 15 | ⚠️ 可能需额外补充 1~2 个 internal 测试 |

### Bash 测试脚本（新增 ~5 个）

| 序号 | 文件路径 | 对应模块 |
|------|---------|---------|
| 1 | `tests/scripts/test_common.sh` | 公共测试函数库 |
| 2 | `tests/scripts/test-file.sh` | 文件传输 E2E |
| 3 | `tests/scripts/test-playbook.sh` | 剧本 E2E |
| 4 | `tests/scripts/test-settings.sh` | 设置 E2E |
| 5 | `tests/scripts/test-version.sh` | 版本命令 E2E |

### 需修改的现有文件

| 序号 | 文件路径 | 修改内容 |
|------|---------|---------|
| 1 | `tests/Makefile` | 补充新脚本调用 |
| 2 | `tests/scripts/test-node.sh` | 大幅扩展测试用例 |
| 3 | `tests/scripts/test-exec.sh` | 大幅扩展测试用例 |
| 4 | `tests/scripts/test-history.sh` | 扩展测试用例 |
| 5 | `cmd/cli/cmd/history/history_test.go` | 补充 flag 测试和 clean 测试 |

---

## 五、实现优先级建议

| 优先级 | 内容 | 理由 |
|--------|------|------|
| **P0** | 步骤 1.1~1.3（基础设施） | 所有后续测试依赖这些辅助工具 |
| **P0** | 步骤 3.10（根命令测试） | 验证整体命令结构正确性 |
| **P1** | 步骤 3.1（node 测试） | 最核心、最复杂模块，9 个子命令 |
| **P1** | 步骤 3.3（file 测试） | 用户手册标识了 4 个未完整实现的功能 |
| **P1** | 步骤 2.1~2.4（核心逻辑测试） | internal 层测试覆盖率提升 |
| **P2** | 步骤 3.2、3.4~3.9（其余 CLI 模块） | 按模块逐个补齐 |
| **P2** | 步骤 4.1~4.8（Bash E2E 脚本） | 需要 SSH 环境，可并行开展 |
| **P3** | 步骤 5.1~5.3（整合和最终验证） | 收尾工作 |

---

## 六、测试运行命令速查

```bash
# 仅运行 Go 单元测试（不需要 SSH 节点，开发阶段高频使用）
make test-unit

# 运行所有 Go 测试（需先 build）
go test ./cmd/cli/cmd/... ./internal/... ./tests/unit/...

# 运行 E2E Bash 测试（需要 OWL_TEST_ENABLED=true 和测试节点）
OWL_TEST_ENABLED=true make test-bash

# 运行全部（单元 + 集成 + Bash）
OWL_TEST_ENABLED=true make test-all

# 覆盖率报告
make test-coverage
```

---

## 七、风险与前提条件

| 项目 | 说明 |
|------|------|
| **Go 编译环境** | 项目依赖 DuckDB CGO，需确保 C 编译工具链可用（或使用 SQLite3 构建标签） |
| **SSH 测试节点** | Layer 3 E2E 测试需要至少 1 个可 SSH 连接的测试节点，建议用 Docker 容器或本地虚拟机 |
| **测试数据隔离** | 所有 E2E 测试使用唯一的临时目录和节点 ID（如 `owl-test-{timestamp}`），测试后自动清理 |
| **AI 模块测试限制** | AI 模块的 E2E 测试依赖第三方 API，不适合自动化；仅做结构性验证 |
| **session 交互式测试** | 交互式会话需用 expect 或类似工具，初期先做结构性验证，后续补充交互测试 |
