---
id: "fix-exec_exec-detail-same-as-simple"
domain: "fix-exec"
slug: "exec-detail-same-as-simple"
title: "命令执行菜单 /exec 里 detail 和 simple 的 JSON 输出为什么一样?"
status: "resolved"
created: "2026-08-12T19:35:34+08:00"
resolved: "2026-08-12T19:55:28+08:00"
commit: "08f8c4d"
branch: "main"
platform: "darwin"
session: "ses_00a3e7526ffei6FcYwyTqbiFhN"
---

# fix-exec_exec-detail-same-as-simple

## 问题

命令执行菜单 /exec 里 detail 和 simple 的 JSON 输出为什么一样?

## 环境

| 项 | 值 |
|----|----|
| git commit | 08f8c4d |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-12T19:35:34+08:00 |
| 会话 | ses_00a3e7526ffei6FcYwyTqbiFhN |

## 调查过程

- [19:35] 建档
- [19:36] 记录日志 (chat): 初步定位:format 只在任务结束时作用于最终 outputStr,前端实时终端只显示 WS 原始行
- [19:55] 记录日志 (manual): 移除 /exec 前端输出格式选择器
- [19:55] 结案

## 日志与摘录

### [chat] 2026-08-12T19:36:06+08:00 · 初步定位:format 只在任务结束时作用于最终 outputStr,前端实时终端只显示 WS 原始行

```
后端 handler/exec.go:604-611 确实区分格式:
- json:  outputStr = fmt.Sprintf(`{"node_id":"%s","command":"%s","exit_code":%d,"output":%s}`, ...)
- detail: outputStr = "Node: %s\nCommand: %s\nExit Code: %d\n---\n%s"
- simple: 原始输出

但 /exec 前端(exec.js:630-635)实时终端只消费 WS task_output 事件(原始行流),从不渲染任务完成时存储的最终 outputStr(task_update 只用来显示 ✓/✗ 状态)。因此 detail/json/simple 在实时视图里看起来完全一样。格式差异只体现在 history/任务详情/日志里保存的最终输出。
```

### [manual] 2026-08-12T19:55:24+08:00 · 移除 /exec 前端输出格式选择器

```
修改 cmd/plugins/serve/web/js/pages/exec.js:
1. buildExecPayload 移除 formatEl 读取(payload 不再带 format 字段)
2. 「输出选项」卡片删除格式下拉框(simple/detail/json),仅保留调试模式
后端 handler/exec.go 未改动(CLI 仍用 format)。node --check 通过。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

## 修复方案

删除 /exec 页面前端「输出选项」卡片中的格式下拉框(simple/detail/json),并移除 buildExecPayload 中对 format-select 的读取与 payload.format 传递。后端保留 format 支持供 CLI 使用。因前端实时终端只消费 WS 原始行流,格式选择对 /exec 实时视图无任何作用,故直接移除而非重写渲染逻辑。

## 复盘

web 端的 format 参数只在任务完成时作用于最终 outputStr,实时终端(WS task_output 行流)始终显示原始输出,两者不共享渲染路径。前端选项与后端实际生效点分离时,应避免只加控件不做端到端验证。
