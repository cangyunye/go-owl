# dts 问题档案索引

共 16 条 · 未解决 0 · 已解决 16 · 更新于 2026-08-11T20:00:15+08:00

## fix-webui

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-webui_ui-alignment-and-playbook-files-redesign](fix-webui_ui-alignment-and-playbook-files-redesign/dts.md) | 2026-08-11 | 1.节点管理搜索框和筛选状态按钮要对齐高度;2.剧本管理路径输入框不要中间圆角(嵌套感),刷新按钮点击闪现Refresh变宽;3.文件传输传输记录/任务详情及命 | resolved |
| [fix-webui_files-upload-btn-gray-and-double-click](fix-webui_files-upload-btn-gray-and-double-click/dts.md) | 2026-08-11 | 1.文件传输的上传按钮字体颜色为什么一直是灰色的失效感觉,2.文件传输的上传按钮还是要点两次 | resolved |
| [fix-webui_file-transfer-stuck-in-progress](fix-webui_file-transfer-stuck-in-progress/dts.md) | 2026-08-10 | owl-serve 界面文件传输在大批节点任务时执行任务卡在"进行中"不刷新,是否因获取状态太快且一次性,刷新无反应;历史里显示所有节点已处理完成 | resolved |

## fix-tui

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-tui_file-upload-button-double-click](fix-tui_file-upload-button-double-click/dts.md) | 2026-08-10 | 为什么界面版本的文件传输上传按钮要双击? | resolved |

## fix-ai

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-ai_settings-query-model-no-dropdown](fix-ai_settings-query-model-no-dropdown/dts.md) | 2026-08-07 | AI设置里的查询模型，为什么查询成功，但是没有下拉框提示可选择模型 | resolved |

## feat-ai-chat

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [feat-ai-chat_markdown-render-tool-results](feat-ai-chat_markdown-render-tool-results/dts.md) | 2026-08-07 | Web AI 对话里"列出所有节点"等工具结果是纯文本,能否通过 Markdown 表格渲染?并做查询/渲染层分离,补上 Groups/Labels 列。 | resolved |

## fix-ui

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-ui_history-missing-pagination-buttons](fix-ui_history-missing-pagination-buttons/dts.md) | 2026-08-07 | 任务历史缺少翻页按键，只有显示当前页码，这是什么情况 | resolved |
| [fix-ui_dark-theme-menu-text-invisible](fix-ui_dark-theme-menu-text-invisible/dts.md) | 2026-08-06 | 前端暗色主题下概览、节点列表等菜单左侧文字显示灰色看不见,要求与右侧数据表字体颜色一致改为白色 | resolved |

## fix-ai-chat

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-ai-chat_list-nodes-answers-ansible](fix-ai-chat_list-nodes-answers-ansible/dts.md) | 2026-08-07 | 为什么在 AI 对话中询问"列出所有节点"没有执行 owl node list,而是返回了 ansible 命令提示(用了 ansible-inventory) | resolved |

## fix-auth

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-auth_rbac-reader-group-requires-admin](fix-auth_rbac-reader-group-requires-admin/dts.md) | 2026-08-07 | RBACMiddleware 取 allowedRoles 的最大等级而非最小，导致 reader(writer/operator) 分组实际要求 admin  | resolved |

## fix-db

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-db_no-such-table-settings-execution-mode](fix-db_no-such-table-settings-execution-mode/dts.md) | 2026-08-06 | 1. owl-serve 报错 no such table settings;2. 前端节点执行命令提示 has no column named executi | resolved |

## fix-exec

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-exec_exec-no-output-no-history](fix-exec_exec-no-output-no-history/dts.md) | 2026-08-06 | 运维中心命令执行可执行但看不到输出:后端不广播task_output,exec页不渲染WS流,历史详情不展示stdout/stderr | resolved |

## fix-dashboard

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-dashboard_overview-node-count-wrong](fix-dashboard_overview-node-count-wrong/dts.md) | 2026-08-06 | serve概览总节点未统计:dashboard只取首页100条计数,顶栏在线/离线恒为0 | resolved |

## fix-backlog

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-backlog_batch2-history-ipv6-panic](fix-backlog_batch2-history-ipv6-panic/dts.md) | 2026-08-05 | 批次二:history 迁移竞态容错(J)、operations 读路径暴露 forced(K)、playbook_engine panic 预防(O)、IPv | resolved |

## fix-serve

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-serve_backlog-1-consistency-fixes](fix-serve_backlog-1-consistency-fixes/dts.md) | 2026-08-05 | Backlog 批次一：serve handler 四个独立小修复（playbook forced 审计、AI 空选择全量扇出守卫、IPv6 地址格式、term | resolved |

## feat-serve-nodeselect

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [feat-serve-nodeselect_panel-select-all-default-all](feat-serve-nodeselect_panel-select-all-default-all/dts.md) | 2026-08-05 | owl serve 界面节点选择缺全选;未选择时默认按组/标签过滤全部节点;右侧过滤应联动左侧;未选不应阻止提交 | resolved |
