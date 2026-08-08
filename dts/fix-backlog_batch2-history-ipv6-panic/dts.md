---
id: "fix-backlog_batch2-history-ipv6-panic"
domain: "fix-backlog"
slug: "batch2-history-ipv6-panic"
title: "批次二:history 迁移竞态容错(J)、operations 读路径暴露 forced(K)、playbook_engine panic 预防(O)、IPv"
status: "resolved"
created: "2026-08-05T23:48:54+08:00"
resolved: "2026-08-06T00:03:19+08:00"
commit: "d41463e"
branch: "feat/ui-redesign-phase0"
platform: "darwin"
session: "ses_02d630ac9ffeCiZ4SCwswT7by4"
---

# fix-backlog_batch2-history-ipv6-panic

## 问题

批次二:history 迁移竞态容错(J)、operations 读路径暴露 forced(K)、playbook_engine panic 预防(O)、IPv6 拨号地址拼接硬化(P)

## 环境

| 项 | 值 |
|----|----|
| git commit | d41463e |
| 分支 | feat/ui-redesign-phase0 |
| 平台 | darwin |
| 建档时间 | 2026-08-05T23:48:54+08:00 |
| 会话 | ses_02d630ac9ffeCiZ4SCwswT7by4 |

## 调查过程

- [23:48] 建档
- [00:03] 新增 E2E 用例: J: legacy 表先手动 ALTER 加 forced,再调 addForcedColumn/EnsureForcedColumn 断言无错(根+serve 双侧)
- [00:03] 新增 E2E 用例: K: 写 Forced:true/false 的 operation,serve Query 读回 Forced 与写入一致
- [00:03] 记录证据 1 项
- [00:03] 记录终端文本快照
- [00:03] 结案

## 日志与摘录

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|
| 1 | J: legacy 表先手动 ALTER 加 forced,再调 addForcedColumn/EnsureForcedColumn 断言无错(根+serve 双侧) | 建 legacy operations 表(无 forced) → 手动 ALTER ADD forced → 调 addForcedColumn → 调 EnsureForcedColumn/ensureForcedColumn | 全部 NoError,duplicate column 被容忍 | pass |
| 2 | K: 写 Forced:true/false 的 operation,serve Query 读回 Forced 与写入一致 | Init → RecordOperation(Forced:true) + RecordOperation(Forced:false) → Query by TaskID → 断言 Operation.Forced | 读回 true / false 分别匹配 | pass |

## 证据截图

[文本快照: 双侧门禁全绿与两个提交](shots/001-000311.txt)

## 修复方案

(J) EnsureForcedColumn/ensureForcedColumn 拆出 addForcedColumn：ALTER 返回 duplicate column name 视为成功（与 playbook_run.go 模式一致），根+serve 双侧；(K) serve Query SELECT/Scan 补 forced 列并映射 Operation.Forced；(O) 删除 executePlaybookRunV2 冗余第二次 runs.Get，用已取到的 run.Status 判 cancelled，消除 nil 解引用恐慌；(P) 6 处拨号地址改 net.JoinHostPort。提交 fa42336(根) + 5f1477e(serve)。

## 复盘

根因：两处 ALTER 迁移先查后改仍留竞态窗口（两进程同时通过前置检查），必须对 duplicate column 错误做幂等容错，不能只靠前置检查；playbook_engine 二次 Get 忽略错误是典型 nil 恐慌源，冗余读直接删。IPv6 教训：凡流向网络拨号的 addr 拼接一律 net.JoinHostPort，展示用途除外。TDD 经验：并发竞态分支在单进程无法确定性编排，拆 seam 方法（addForcedColumn）白盒命中是可靠切面。
