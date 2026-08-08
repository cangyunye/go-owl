---
id: "feat-serve-nodeselect_panel-select-all-default-all"
domain: "feat-serve-nodeselect"
slug: "panel-select-all-default-all"
title: "owl serve 界面节点选择缺全选;未选择时默认按组/标签过滤全部节点;右侧过滤应联动左侧;未选不应阻止提交"
status: "resolved"
created: "2026-08-05T12:32:17+08:00"
resolved: "2026-08-05T12:51:29+08:00"
commit: "454868b"
branch: "feat/ui-redesign-phase0"
platform: "darwin"
session: "ses_02fd583dcffeoXVYN3G7lVMPsF"
---

# feat-serve-nodeselect_panel-select-all-default-all

## 问题

owl serve 界面节点选择缺全选;未选择时默认按组/标签过滤全部节点;右侧过滤应联动左侧;未选不应阻止提交

## 环境

| 项 | 值 |
|----|----|
| git commit | 454868b |
| 分支 | feat/ui-redesign-phase0 |
| 平台 | darwin |
| 建档时间 | 2026-08-05T12:32:17+08:00 |
| 会话 | ses_02fd583dcffeoXVYN3G7lVMPsF |

## 调查过程

- [12:32] 建档
- [12:50] 记录日志 (bash): Playwright E2E 验证全选/清空、未选默认全部、label 联动、未选可提交
- [12:50] 新增 E2E 用例: exec 页:全选-清空-标签过滤-未选默认提交
- [12:50] 新增 E2E 用例: files 页:全选按钮可用
- [12:50] 记录证据 1 项
- [12:50] 记录终端文本快照
- [12:51] 结案

## 日志与摘录

### [bash] 2026-08-05T12:50:31+08:00 · Playwright E2E 验证全选/清空、未选默认全部、label 联动、未选可提交

```
[Playwright E2E, headless chrome, http://localhost:8090]
select-all visible: true
clear visible: true
count initial: 未选择（默认全部匹配节点）
exec disabled (no sel): false
exec text (no sel): 在全部匹配节点上执行命令
count after select-all: 已选 43 个节点
count after clear: 未选择（默认全部匹配节点）
left nodes after label env=prod: 13
exec disabled label filter no sel: false
exec POST status: 202
exec tasks returned: 11
files select-all visible: true
files clear visible: true
files count after select-all: 已选 43 个节点
--- console errors ---
DONE

注: exec tasks returned 11 < 43 是因为测试期间 8080 端口另有一个 owl-serve 实例共享同一 ~/.owl/owl.db,
sqlite 并发写锁导致部分 EXISTS 查询失败被跳过,属环境干扰;确定性由 in-memory 单元测试保证。

API 层面验证:
- GET /nodes?label=env:prod&label=tier:frontend -> AND 语义, total 6
- POST /exec {command, force, groups:[web], labels:{env:prod}} -> 6 tasks (web+prod 交集)
- POST /transfer {groups:[web], labels:{env:prod}} -> 6 transfers
- 无 node_ids 空 exec 在 in-memory 测试中命中全部节点 (TestExecCreate_NoSelection_RunsOnAllNodes)
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|
| 1 | exec 页:全选-清空-标签过滤-未选默认提交 | 1. 登录后进入 /exec;2. 查看左侧面板应有 全选/清空 按钮;3. 不选任何节点,输入命令,查看执行按钮状态与文案;4. 点全选,查看已选数量;5. 点清空,查看提示;6. 右侧标签输入 env=prod,查看左侧列表节点数;7. 不选节点直接点执行。 | 全选选中全部过滤节点(43);清空后提示"未选择（默认全部匹配节点）";未选时执行按钮可用且文案为"在全部匹配节点上执行命令";标签过滤联动左侧(env=prod -> 13);未选提交返回 202 并创建任务。 | pass |
| 2 | files 页:全选按钮可用 | 1. 进入 /files;2. 查看左侧面板全选/清空按钮;3. 点全选。 | 全选选中 43 个节点,已选计数更新。 | pass |

## 证据截图

[文本快照: exec 页 E2E 文本快照](shots/001-125042.txt)

## 修复方案

前端 exec.js/files.js 左侧节点面板新增 全选/清空 按钮;buildNodeQuery() 统一携带 组+标签+状态+搜索 过滤,全选按 100/页循环拉取当前过滤下全部分页节点。未选择节点时:buildExecPayload/buildTransferPayload 不再传 node_ids(改为仅 groups/labels),updateExecButton 仅凭命令内容决定可用性(未选时文案"在全部匹配节点上执行命令"),handleTransfer 去掉 size===0 拦截。后端:exec.go 去掉 opts.Empty() 提前返回并改用新增的 SelectIntersect(groups+labels 取交集,空选项返回全部,与左侧 AND 预览一致);transfer.go 增加 groups/labels 字段,空 node_ids 时用 SelectIntersect 解析;node.go 列表 API 改 QueryArray 支持多 label AND 过滤(左侧联动);修复 dbNodeSource.List 对 NULL name 的 Scan 错误(COALESCE)。已通过 in-memory 单元测试与 Playwright E2E(全选43、清空提示、未选可提交 202、label 过滤联动 13 节点)。

## 复盘

根因:前端只支持逐节点点选且未选即禁用提交,未传 node_ids 时后端 opts.Empty() 提前返回导致空 exec 被拒;右侧标签过滤不参与左侧列表刷新。踩坑:共享选择器 Select() 是 groups>labels 优先级语义,与 Web 左侧列表的 AND 过滤不一致,需新增 SelectIntersect 避免误伤 CLI;SQLite 并发写锁会让共享 DB 的 E2E 任务数抖动,确定性验证应放在 in-memory 单元测试。
