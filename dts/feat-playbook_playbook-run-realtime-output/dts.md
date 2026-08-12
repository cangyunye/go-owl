---
id: "feat-playbook_playbook-run-realtime-output"
domain: "feat-playbook"
slug: "playbook-run-realtime-output"
title: "剧本管理菜单里的执行内容,能否设计个和命令执行一样的实时打印输出?"
status: "resolved"
created: "2026-08-12T08:03:52+08:00"
resolved: "2026-08-12T11:21:00+08:00"
commit: "2b6a201"
branch: "main"
platform: "darwin"
session: "ses_00e6cdafcffenTzLmhuizVdh2N"
---

# feat-playbook_playbook-run-realtime-output

## 问题

剧本管理菜单里的执行内容,能否设计个和命令执行一样的实时打印输出?

## 环境

| 项 | 值 |
|----|----|
| git commit | 2b6a201 |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-12T08:03:52+08:00 |
| 会话 | ses_00e6cdafcffenTzLmhuizVdh2N |

## 调查过程

- [08:03] 建档
- [11:21] 结案

## 日志与摘录

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

## 修复方案

经分析后决定不实施:playbook 执行保持"跑完一次性落盘"现状,不做行级实时流式。serve 前端维持现有 playbook_run_update 终态推送 + 结果表;CLI 端维持三层批处理日志(DB operations/playbook_runs/command_executions/playbook_step_state + ~/.owl/logs/nodes/*.log + 终端汇总)。代码零改动。

## 复盘

实时流式改造需打通三层(ssh ExecuteStream → WS playbook_output → 前端终端),成本不低;用户评估后认为 playbook 场景可接受批处理结果,需求取消。下次先与用户确认"是否必须实时"再出方案,避免过度设计。
