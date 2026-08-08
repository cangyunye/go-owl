---
id: "fix-serve_backlog-1-consistency-fixes"
domain: "fix-serve"
slug: "backlog-1-consistency-fixes"
title: "Backlog 批次一：serve handler 四个独立小修复（playbook forced 审计、AI 空选择全量扇出守卫、IPv6 地址格式、term"
status: "resolved"
created: "2026-08-05T23:30:45+08:00"
resolved: "2026-08-05T23:44:25+08:00"
commit: "808832e"
branch: "feat/ui-redesign-phase0"
platform: "darwin"
session: "ses_02d75a0a8ffeFfPflIhwyXx3a1"
---

# fix-serve_backlog-1-consistency-fixes

## 问题

Backlog 批次一：serve handler 四个独立小修复（playbook forced 审计、AI 空选择全量扇出守卫、IPv6 地址格式、terminal 拨号 ctx）

## 环境

| 项 | 值 |
|----|----|
| git commit | 808832e |
| 分支 | feat/ui-redesign-phase0 |
| 平台 | darwin |
| 建档时间 | 2026-08-05T23:30:45+08:00 |
| 会话 | ses_02d75a0a8ffeFfPflIhwyXx3a1 |

## 调查过程

- [23:30] 建档
- [23:43] 记录日志 (bash): 门禁全绿：serve 模块 build/vet/test 通过（vet 零告警），根模块 cli/internal/pkg 回归无失败
- [23:44] 结案

## 日志与摘录

### [bash] 2026-08-05T23:43:28+08:00 · 门禁全绿：serve 模块 build/vet/test 通过（vet 零告警），根模块 cli/internal/pkg 回归无失败

```
cd cmd/plugins/serve: go build ./... && go vet ./... && go test -count=1 ./...
→ vet 零告警（IPv6 "%s:%d" 告警已消失）；serve / handler / service / store 全部 ok
根目录: go test -count=1 ./cmd/cli/... ./internal/... ./pkg/... → 全部 ok
新增失败测试（修复前红）：TestPlaybookRun_ForcedAudit、TestWebExecutor_ResolveAINodeIDs_AllParamsEmpty、TestWebExecutor_ExecuteCommand/ExecuteScript_NoTargetParams_NoFanout、TestPing_IPv6Address（dial tcp: address ::1:50580: too many colons in address）；修复后全绿，-count=2 复跑稳定。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

## 修复方案

四处修复一次提交 d41463e：playbook.go:245 补 Forced: req.DangerConfirmed；resolveAINodeIDs 四参数全空提前返回 nil 阻断全量扇出；node.go Ping/checkNodeSSH 改 net.JoinHostPort（vet 清零）；terminal 拨号传请求 ctx 并删除 dial 包装。修复前先落 5 个红测试（forced 审计、空参数零扇出×3、真实 IPv6 ping），serve build/vet/test 全绿且根模块无回归。

## 复盘

三个坑：1) modernc sqlite :memory: 每个池连接是独立空库，测试必须 db.SetMaxOpenConns(1)；2) handler 后台 goroutine（executePlaybookRunV2）与测试共用内存 DB 会写竞争甚至 db 关闭后 nil panic，测试用独立库且不 Close 规避；3) store.HistoryStore.Query 未 SELECT forced 列导致审计写入可见性断裂，发现后按范围纪律只报告不改，留作后续 backlog。
