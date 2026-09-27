---
id: "fix-webui_nodes-list-search-broken"
domain: "fix-webui"
slug: "nodes-list-search-broken"
title: "节点管理里面没法搜节点（搜索框输入后列表不按关键字过滤）"
status: "resolved"
created: "2026-09-25T22:07:04+08:00"
resolved: "2026-09-25T22:19:19+08:00"
commit: "dd994ce"
branch: "feat/v1.9.0-app-tabs"
platform: "win32"
session: ""
---

# fix-webui_nodes-list-search-broken

## 问题

节点管理里面没法搜节点（搜索框输入后列表不按关键字过滤）

## 环境

| 项 | 值 |
|----|----|
| git commit | dd994ce |
| 分支 | feat/v1.9.0-app-tabs |
| 平台 | win32 |
| 建档时间 | 2026-09-25T22:07:04+08:00 |
| 会话 | - |

## 调查过程

- [22:07] 建档
- [22:19] 记录日志 (bash): 根因两处：搜索端点不匹配 id、列表端点没有 q 参数（执行页面板搜索因此一直失效）；已修复并实测验证
- [22:19] 结案

## 日志与摘录

### [bash] 2026-09-25T22:19:12+08:00 · 根因两处：搜索端点不匹配 id、列表端点没有 q 参数（执行页面板搜索因此一直失效）；已修复并实测验证

```
【现象】用户报告：节点管理里面没法搜节点。

【定位过程】
1. 先测搜索接口本身（e2e 库）：
   search?q=e2e-a-001 → 1 条；q=zbig → 90 条  ⇒ 接口没坏
2. 用浏览器实测节点页：输入 e2e-a-001 → 列表只剩 1 行，请求 /nodes/search?q=… 正常
   ⇒ 我的测试数据里 name==id，把问题掩盖了
3. 造一个 id 与 name 不同的节点复现：
   POST /nodes {id: "probe-id-search", name: "完全不同的中文名", ...}
   search?q=probe-id-search → **0 条**（应为 1）
   search?q=完全不同的中文名 → 1 条
   ⇒ 根因 1：/nodes/search 的 WHERE 只匹配 name/address/user/groups/labels，不含 id
4. 顺手核对执行页左侧节点面板（exec.js 的 #panel-node-search → loadNodes() → api.nodes({q})）：
   服务端 List **完全没有 q 参数** ⇒ 面板搜索一直「输入没反应」
   ⇒ 根因 2：列表端点缺关键字过滤

【修复】handler/node.go
- List 新增 q：AND (LOWER(id) LIKE ? OR LOWER(name) LIKE ? OR LOWER(address) LIKE ?
  OR LOWER(user) LIKE ? OR LOWER(groups) LIKE ? OR LOWER(labels) LIKE ?)
- Search 的 WHERE 补上 LOWER(id) LIKE ?

【验证（重建后实测）】
  curl search?q=probe-id-search → 1 条
  curl nodes?q=probe-id-search   → 1 条
  UI 节点页 按 id 搜索 → 1 行
  UI 执行页面板 按 id 搜索 → 1 个节点（此前完全失效）
  新增单测 TestNodeList_FilterByQuery / TestNodeSearch_MatchesID 通过
  回归：client_cache / m2 / m1 / m4b 全 PASS；handler 包失败集合与基线一致（20）

【附带结论：命令输出终端容器的 ID】
  执行页输出终端容器是 #term-body（位于 .output-terminal 内，exec.js:902）；
  节点终端页（terminal.js）另有自己的容器。此前 M4b E2E 读不到它，是测试侧问题
  （当时跑在不可达节点上且选择器时机不对），不是代码缺陷。
  现已在 E2E 里用 #term-body 断言「长输出跑完后终端渲染出末行」，实测 20003 行、
  末行 20000 可见；并加了现场诊断（找不到容器时打印 URL/视图/标签/相关元素）。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

## 修复方案

两处独立的真问题，都在 handler/node.go：

1. /nodes/search 的 WHERE 只匹配 name/address/user/groups/labels，**不含 id**。节点通常靠 id 辨识（如 wsl-kube 的 name 是 "WSL Ubuntu"），所以按 id 搜恒为 0 条。已在 WHERE 补上 LOWER(id) LIKE ?。
2. /nodes 列表端点**没有 q 参数**，而执行页左侧节点面板的搜索框走的正是 api.nodes({q})（exec.js:1092 panel-node-search → loadNodes()）——服务端忽略 q，表现为「输入没反应」。已给 List 加 q：AND (LOWER(id) LIKE ? OR LOWER(name) LIKE ? OR LOWER(address) LIKE ? OR LOWER(user) LIKE ? OR LOWER(groups) LIKE ? OR LOWER(labels) LIKE ?)。

验证（重建后实测）：curl search?q=<id> 与 nodes?q=<id> 各命中 1；UI 节点页按 id 搜索命中 1 行；UI 执行页面板按 id 搜索命中 1 个节点（此前完全失效）。新增 TestNodeList_FilterByQuery / TestNodeSearch_MatchesID（用 id 与 name 不同的节点 wsl-kube 复现）；回归 client_cache / m2 / m1 / m4b 全 PASS，handler 包失败集合与基线一致。commit e74dbf5。

## 复盘

根因教训：
1. 「搜不到」先分清是接口没查还是前端没传。/nodes/search 接口本身工作正常（按 name 搜得到），问题在匹配字段不全；而执行页面板是另一个方向——前端传了 q，服务端压根没这个参数。
2. 测试数据会掩盖 bug：我的 e2e 节点 name==id，所以「按 id 搜索」这条路一直没被覆盖。造一个 id 与 name 不同的节点（wsl-kube / "WSL Ubuntu"）才复现——写搜索类测试要专门覆盖「各字段值互不相同」的数据形状。
3. 同一份筛选能力不该有两个实现：搜索端点与列表端点的过滤字段集此前不一致（一个少 id、一个没有 q），这类分叉迟早表现为「有的地方能搜、有的地方不能」。现在两处字段集一致，后续再筛字段应同时改。
4. 顺带澄清用户关于「命令输出终端容器的 ID」的提问：执行页是 #term-body（.output-terminal 内），节点终端页另有容器；M4b E2E 读不到是测试侧问题（跑在不可达节点上 + 选择器时机），不是代码缺陷。已改为用 #term-body 断言渲染结果，并在断言失败时打印现场（URL/视图/标签/相关元素），避免下次再靠猜。
