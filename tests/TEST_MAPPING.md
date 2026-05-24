# 测试映射表

文档中的测试用例（TC）与自动化测试之间的可追溯映射。

## 状态图例

| 符号 | 含义 |
|------|------|
| ✅ | 已实现 |
| ⚠️ | 部分实现 |
| ❌ | 未实现 |

---

## NODE 模块

| 用户手册 TC | 描述 | L2 Go 测试 | L3 Bash 测试 | 状态 |
|-------------|------|-----------|-------------|------|
| TC-NODE-001 | 添加节点 | `cmd/cli/cmd/node/node_test.go::TestNodeAddFlags` | `tests/scripts/test-node.sh::test_node_add_basic` | ⚠️ |
| TC-NODE-002 | 列出节点 | `cmd/cli/cmd/node/node_test.go::TestNodeListFlags` | `tests/scripts/test-node.sh::test_node_list_all` | ⚠️ |
| TC-NODE-003 | 更新节点 | `cmd/cli/cmd/node/node_test.go::TestNodeUpdateFlags` | `tests/scripts/test-node.sh::test_node_update` | ⚠️ |
| TC-NODE-004 | 删除节点 | `cmd/cli/cmd/node/node_test.go::TestNodeRemoveCmd` | `tests/scripts/test-node.sh::test_node_remove` | ⚠️ |
| TC-NODE-005 | 分组管理 | `cmd/cli/cmd/node/node_test.go::TestNodeGroupsSubcommands` | `tests/scripts/test-node.sh::test_node_groups_add` | ⚠️ |
| TC-NODE-006 | 标签管理 | `cmd/cli/cmd/node/node_test.go::TestNodeLabelsSubcommands` | `tests/scripts/test-node.sh::test_node_labels_set` | ⚠️ |
| TC-NODE-007 | 导入导出 | `cmd/cli/cmd/node/node_test.go::TestNodeImportFlags` | `tests/scripts/test-node.sh::test_node_import_export` | ⚠️ |
| TC-NODE-008 | Ping 检查 | `cmd/cli/cmd/node/node_test.go::TestNodePingFlags` | `tests/scripts/test-node.sh::test_node_ping` | ⚠️ |
| TC-NODE-009 | SSH 连接检查 | `cmd/cli/cmd/node/node_test.go::TestNodeCheckFlags` | `tests/scripts/test-node.sh::test_node_check` | ⚠️ |

---

## EXEC 模块

| 用户手册 TC | 描述 | L2 Go 测试 | L3 Bash 测试 | 状态 |
|-------------|------|-----------|-------------|------|
| TC-EXEC-001 | 单节点命令 | `cmd/cli/cmd/exec/exec_test.go::TestExecRunFlags` | `tests/scripts/test-exec.sh::test_exec_run_single_node` | ⚠️ |
| TC-EXEC-002 | 多节点并行 | `cmd/cli/cmd/exec/exec_test.go::TestExecRunFlags` | `tests/scripts/test-exec.sh::test_exec_run_multi_node` | ⚠️ |
| TC-EXEC-003 | 分组执行 | `cmd/cli/cmd/exec/exec_test.go::TestExecRunFlags` | `tests/scripts/test-exec.sh::test_exec_run_by_group` | ⚠️ |
| TC-EXEC-004 | 命令超时 | `cmd/cli/cmd/exec/exec_test.go::TestExecRunFlags` | `tests/scripts/test-exec.sh::test_exec_run_timeout` | ⚠️ |
| TC-EXEC-005 | JSON 输出 | `cmd/cli/cmd/exec/exec_test.go::TestExecRunFlags` | `tests/scripts/test-exec.sh::test_exec_run_json_output` | ⚠️ |
| TC-EXEC-006 | 错误处理 | `cmd/cli/cmd/exec/exec_test.go::TestExecRunFlags` | `tests/scripts/test-exec.sh::test_exec_run_error` | ⚠️ |
| TC-EXEC-007 | 异步执行 | `cmd/cli/cmd/exec/exec_test.go::TestExecRunFlags` | `tests/scripts/test-exec.sh::test_exec_run_async` | ⚠️ |

---

## FILE 模块

| 用户手册 TC | 描述 | L2 Go 测试 | L3 Bash 测试 | 状态 |
|-------------|------|-----------|-------------|------|
| TC-FILE-001 | 单节点上传 | `cmd/cli/cmd/file/file_test.go::TestFileUploadFlags` | `tests/scripts/test-file.sh::test_file_upload_single` | ⚠️ |
| TC-FILE-002 | 多节点上传 | `cmd/cli/cmd/file/file_test.go::TestFileUploadFlags` | `tests/scripts/test-file.sh::test_file_upload_multi` | ⚠️ |
| TC-FILE-003 | 分组上传 | `cmd/cli/cmd/file/file_test.go::TestFileUploadFlags` | `tests/scripts/test-file.sh::test_file_upload_group` | ⚠️ |
| TC-FILE-004 | 单节点下载 | `cmd/cli/cmd/file/file_test.go::TestFileDownloadFlags` | `tests/scripts/test-file.sh::test_file_download_single` | ⚠️ |
| TC-FILE-005 | 上传覆盖 | `cmd/cli/cmd/file/file_test.go::TestFileUploadFlags` | `tests/scripts/test-file.sh::test_file_upload_overwrite` | ⚠️ |
| TC-FILE-006 | 上传不覆盖 | `cmd/cli/cmd/file/file_test.go::TestFileUploadFlags` | `tests/scripts/test-file.sh::test_file_upload_no_overwrite` | ⚠️ |
| TC-FILE-007 | 多节点下载(后缀) | `cmd/cli/cmd/file/file_test.go::TestFileDownloadFlags` | `tests/scripts/test-file.sh::test_file_download_multi_suffix` | ⚠️ |
| TC-FILE-008 | 多节点下载(子目录) | `cmd/cli/cmd/file/file_test.go::TestFileDownloadFlags` | `tests/scripts/test-file.sh::test_file_download_subdir` | ⚠️ |
| TC-FILE-009 | 文件不存在 | - | `tests/scripts/test-file.sh::test_file_not_found` | ❌ |

---

## PLAYBOOK 模块

| 用户手册 TC | 描述 | L2 Go 测试 | L3 Bash 测试 | 状态 |
|-------------|------|-----------|-------------|------|
| TC-PLAY-001 | 列出剧本 | `cmd/cli/cmd/playbook/playbook_test.go::TestPlaybookListFlags` | `tests/scripts/test-playbook.sh::test_playbook_list` | ⚠️ |
| TC-PLAY-002 | 剧本信息 | `cmd/cli/cmd/playbook/playbook_test.go::TestPlaybookInfoCmd` | `tests/scripts/test-playbook.sh::test_playbook_info` | ⚠️ |
| TC-PLAY-003 | 执行剧本 | `cmd/cli/cmd/playbook/playbook_test.go::TestPlaybookRunFlags` | `tests/scripts/test-playbook.sh::test_playbook_run` | ⚠️ |
| TC-PLAY-004 | 传递变量 | `cmd/cli/cmd/playbook/playbook_test.go::TestPlaybookRunFlags` | `tests/scripts/test-playbook.sh::test_playbook_vars` | ⚠️ |
| TC-PLAY-005 | 验证语法 | `cmd/cli/cmd/playbook/playbook_test.go::TestPlaybookValidateCmd` | `tests/scripts/test-playbook.sh::test_playbook_validate` | ⚠️ |

---

## SESSION 模块

| 用户手册 TC | 描述 | L2 Go 测试 | L3 Bash 测试 | 状态 |
|-------------|------|-----------|-------------|------|
| TC-SESSION-001 | 单节点连接 | `cmd/cli/cmd/session/session_test.go::TestSessionAttachFlags` | - (交互式) | ❌ |
| TC-SESSION-002 | 会话帮助 | - | - (交互式) | ❌ |
| TC-SESSION-003 | 会话历史 | `cmd/cli/cmd/session/session_test.go::TestSessionHistoryFlags` | - | ❌ |
| TC-SESSION-004 | 多节点连接 | `cmd/cli/cmd/session/session_test.go::TestSessionAttachFlags` | - (交互式) | ❌ |

---

## AI 模块

| 用户手册 TC | 描述 | L2 Go 测试 | 状态 |
|-------------|------|-----------|------|
| TC-AI-001 | AI 对话 | `cmd/cli/cmd/ai/ai_test.go::TestAIFlags` | ❌ |
| TC-AI-002 | 模型列表 | `cmd/cli/cmd/ai/ai_test.go::TestAIModelsFlags` | ❌ |
| TC-AI-003 | 初始化配置 | `cmd/cli/cmd/ai/ai_test.go::TestAIConfigSubcommands` | ❌ |
| TC-AI-004 | 显示配置 | `cmd/cli/cmd/ai/ai_test.go::TestAIConfigSubcommands` | ❌ |
| TC-AI-005 | 提供商验证 | `cmd/cli/cmd/ai/ai_test.go::TestAIProviders` | ❌ |

---

## HISTORY 模块

| 用户手册 TC | 描述 | L2 Go 测试 | L3 Bash 测试 | 状态 |
|-------------|------|-----------|-------------|------|
| TC-HIST-001 | 查看历史 | `cmd/cli/cmd/history/history_test.go::TestHistoryFlags` | `tests/scripts/test-history.sh::test_history_list` | ❌ |
| TC-HIST-002 | 按节点筛选 | `cmd/cli/cmd/history/history_test.go::TestHistoryFlags` | `tests/scripts/test-history.sh::test_history_by_node` | ❌ |
| TC-HIST-003 | JSON 输出 | `cmd/cli/cmd/history/history_test.go::TestHistoryFlags` | `tests/scripts/test-history.sh::test_history_json_output` | ❌ |
| TC-HIST-004 | 相对时间 | `cmd/cli/cmd/history/history_test.go::TestHistoryFlags` | `tests/scripts/test-history.sh::test_history_relative_time` | ❌ |
| TC-HIST-005 | 清理历史 | `cmd/cli/cmd/history/history_test.go::TestHistoryCleanFlags` | `tests/scripts/test-history.sh::test_history_clean` | ❌ |

---

## SETTINGS 模块

| 用户手册 TC | 描述 | L2 Go 测试 | L3 Bash 测试 | 状态 |
|-------------|------|-----------|-------------|------|
| TC-SETTINGS-001 | 显示设置 | `cmd/cli/cmd/settings/settings_test.go::TestSettingsShowCmd` | `tests/scripts/test-settings.sh::test_settings_show` | ❌ |
| TC-SETTINGS-002 | 设置值 | `cmd/cli/cmd/settings/settings_test.go::TestSettingsSetCmd` | `tests/scripts/test-settings.sh::test_settings_set` | ❌ |
| TC-SETTINGS-003 | 默认目标 | `cmd/cli/cmd/settings/settings_test.go::TestSettingsTargetFlags` | `tests/scripts/test-settings.sh::test_settings_target` | ❌ |

---

## 根命令

| 测试项 | L2 Go 测试 | 状态 |
|--------|-----------|------|
| 根命令存在 | `cmd/cli/cmd/root_test.go::TestRootCmdExists` | ✅ |
| 版本信息 | `cmd/cli/cmd/root_test.go::TestRootCmdVersion` | ✅ |
| 全部子命令 | `cmd/cli/cmd/root_test.go::TestAllSubCommands` | ✅ |
| 帮助信息 | `cmd/cli/cmd/root_test.go::TestRootCmdHelp` | ✅ |

---

## 统计

| 模块 | TC 总数 | L2 已实现 | L3 已实现 | 整体完成度 |
|------|--------|----------|----------|-----------|
| NODE | 9 | 9 | 0 | 50% |
| EXEC | 7 | 7 | 0 | 50% |
| FILE | 9 | 8 | 0 | 44% |
| PLAYBOOK | 5 | 5 | 0 | 50% |
| SESSION | 4 | 0 | 0 | 0% |
| AI | 5 | 0 | 0 | 0% |
| HISTORY | 5 | 0 | 0 | 0% |
| SETTINGS | 3 | 0 | 0 | 0% |
| 根命令 | 4 | 4 | - | 100% |
| **总计** | **51** | **33** | **0** | **65%** |
