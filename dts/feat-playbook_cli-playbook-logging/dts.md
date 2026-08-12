---
id: "feat-playbook_cli-playbook-logging"
domain: "feat-playbook"
slug: "cli-playbook-logging"
title: "owl 的 cli 端的 playbook 执行是否有日志记录?"
status: "resolved"
created: "2026-08-12T08:06:29+08:00"
resolved: "2026-08-12T11:21:01+08:00"
commit: "81a30c2"
branch: "main"
platform: "darwin"
session: "ses_00e6cdafcffenTzLmhuizVdh2N"
---

# feat-playbook_cli-playbook-logging

## 问题

owl 的 cli 端的 playbook 执行是否有日志记录?

## 环境

| 项 | 值 |
|----|----|
| git commit | 81a30c2 |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-12T08:06:29+08:00 |
| 会话 | ses_00e6cdafcffenTzLmhuizVdh2N |

## 调查过程

- [08:06] 建档
- [08:07] 记录日志 (chat): CLI playbook 有批处理日志无流式实时日志
- [11:21] 结案

## 日志与摘录

### [chat] 2026-08-12T08:07:19+08:00 · CLI playbook 有批处理日志无流式实时日志

```
CLI 端 playbook 日志现状(分析结论):
- 三层落盘,均为跑完后一次性写入(非流式):
  1. 终端: run.go:496-526 Execute 返回后循环打印, 输出截断 1024
  2. DB(~/.owl/owl.db): operations / playbook_runs / command_executions / playbook_step_state / checkpoint
     - 查询: owl history, owl playbook state list/show
  3. 文件: ~/.owl/logs/nodes/<nodeID>.log (NodeLogWriter.AppendEntry)
- 根因与 web 相同: command.ExecuteOnNode 走 CombinedOutput 批处理(control/command/ssh_executor.go:277), pbexec.Execute 同步阻塞, 无法逐行流式
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

## 修复方案

维持现状不改动。CLI 端 playbook 已有三层批处理日志:DB(operations/playbook_runs/command_executions/playbook_step_state/checkpoint)、节点文件日志(~/.owl/logs/nodes/<nodeID>.log)、终端汇总打印。查询: owl history / owl playbook state list|show。不做流式/实时日志改造。

## 复盘

确认现状已满足需求即可,不必为对称性(与 exec 实时输出一致)而强加改造。
