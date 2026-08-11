---
id: "fix-webui_file-transfer-stuck-in-progress"
domain: "fix-webui"
slug: "file-transfer-stuck-in-progress"
title: "owl-serve 界面文件传输在大批节点任务时执行任务卡在\"进行中\"不刷新,是否因获取状态太快且一次性,刷新无反应;历史里显示所有节点已处理完成"
status: "resolved"
created: "2026-08-10T21:52:11+08:00"
resolved: "2026-08-10T22:37:46+08:00"
commit: "2d3a4b0"
branch: "main"
platform: "darwin"
session: "ses_0140e1d77ffedqPfRZq2v3Jfb4"
---

# fix-webui_file-transfer-stuck-in-progress

## 问题

owl-serve 界面文件传输在大批节点任务时执行任务卡在"进行中"不刷新,是否因获取状态太快且一次性,刷新无反应;历史里显示所有节点已处理完成

## 环境

| 项 | 值 |
|----|----|
| git commit | 2d3a4b0 |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-10T21:52:11+08:00 |
| 会话 | ses_0140e1d77ffedqPfRZq2v3Jfb4 |

## 调查过程

- [21:52] 建档
- [22:26] 记录日志 (chat): 根因确认:非前端刷新问题,而是服务端 UpdateNodeResult 的 read-modify-write 竞态
- [22:35] 记录证据 1 项
- [22:35] 记录终端文本快照
- [22:35] 记录日志 (chat): 已实施修复并验证通过
- [22:37] 结案

## 日志与摘录

### [chat] 2026-08-10T22:26:29+08:00 · 根因确认:非前端刷新问题,而是服务端 UpdateNodeResult 的 read-modify-write 竞态

```
用户怀疑是前端轮询/刷新问题,实际根因在服务端 store/transfer_record.go 的 UpdateNodeResult:

并发测试复现:20 个并发 UpdateNodeResult(success=true)后 success_count 只到 3~4,status 卡在 running。

原因:transfer.go Create() 里 parallel=true 时对每个节点 go h.runTransfer(...),每个 goroutine 结束时调用 recordStore.UpdateNodeResult,该函数先 SELECT node_count/success_count/failed_count,本地 +1 后再 UPDATE 写回。这是经典的 read-modify-write 竞态 —— 多个 goroutine 读到同一个旧值,各自 +1 后互相覆盖,丢失增量。因此 success_count 永远达不到 node_count,status 永远停在 running,前端怎么刷新都没用。而每个节点的 task 本身状态是分别 UpdateStatus 更新的,所以历史里看所有节点都处理完成,只有那条聚合 transfer_record 卡住。

验证:TransferHandler.Create 并行默认开(parallel := req.Parallel == nil || *req.Parallel),前端 files.js 也默认勾选并行。
```

### [chat] 2026-08-10T22:35:41+08:00 · 已实施修复并验证通过

```
修复内容:
1. store/transfer_record.go UpdateNodeResult 改为单条原子 UPDATE:success_count/failed_count 直接 SQL 自增,并用同一语句内的 CASE 推导 status + completed_at。彻底消除 read-modify-write 竞态丢失增量的问题。
2. store/transfer_record.go MarkRunning 增加 WHERE status='pending' 守卫:避免启动 goroutine 前的快速失败(节点解析失败全部计入 failed)被 MarkRunning 覆盖回 running。
3. handler/transfer.go Create 把 MarkRunning 调用移到 goroutine 启动之前:防止并发传输在 MarkRunning 之前全部完成、状态被回写为 running 的时序竞态。

验证:单测 20 并发 UpdateNodeResult 修复前 success_count 只剩 3~4、status 卡 running;修复后全部 20 正确计数并 completed。-race 下多次通过。E2E 20 节点并行传输聚合记录最终到达 failed(全部失败),completed_at 写入。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: E2E 验证:20 个节点并发文件传输,聚合记录最终进入终态 failed(全部解析/SSH 失败),completed_at 已写入;修复前会卡在 running 且计数远小于 node_count](shots/001-223515.txt)

## 修复方案

1) transfer_record.go UpdateNodeResult:原来先 SELECT 再本地 +1 再 UPDATE,是典型 read-modify-write 竞态。20 并发下多个 goroutine 读到同一旧值,各自 +1 互相覆盖,success_count 永远到不了 node_count,status 卡 running。改为单条原子 UPDATE:success_count/failed_count 用 SQL 自增,status 和 completed_at 在同一个语句里用 CASE 推导,SQLite 串行写后即原子。
2) transfer_record.go MarkRunning:加 WHERE status='pending' 守卫,避免启动 goroutine 前的节点解析失败(已计入 failed)被无条件 MarkRunning 覆盖回 running。
3) transfer.go Create:把 MarkRunning 移到 goroutine 启动之前,消除"全部传输在 MarkRunning 前完成被回写 running"的时序竞态。

## 复盘

用户描述的现象(历史里每节点都完成、聚合记录卡进行中、刷新无效)是经典"聚合计数竞态"信号:单节点 task 状态各自写库没问题,聚合 record 靠读-改-写累加则并发丢增量。排查方向不应只盯前端轮询,应先确认后端聚合状态是否真的到达终态。教训:凡是并发 goroutine 更新同一行计数,必须用原子 SQL(自增或条件更新),不能先 SELECT 后 UPDATE;状态初始化(mark running)要放在并发启动之前并加状态守卫。
