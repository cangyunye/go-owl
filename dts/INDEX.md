# dts 问题档案索引

共 8 条 · 未解决 0 · 已解决 8 · 更新于 2026-08-07T00:09:15+08:00

## fix-auth

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-auth_rbac-reader-group-requires-admin](fix-auth_rbac-reader-group-requires-admin/dts.md) | 2026-08-07 | RBACMiddleware 取 allowedRoles 的最大等级而非最小，导致 reader(writer/operator) 分组实际要求 admin  | resolved |

## fix-ui

| id | 日期 | 摘要 | 状态 |
|----|------|------|------|
| [fix-ui_dark-theme-menu-text-invisible](fix-ui_dark-theme-menu-text-invisible/dts.md) | 2026-08-06 | 前端暗色主题下概览、节点列表等菜单左侧文字显示灰色看不见,要求与右侧数据表字体颜色一致改为白色 | resolved |

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
