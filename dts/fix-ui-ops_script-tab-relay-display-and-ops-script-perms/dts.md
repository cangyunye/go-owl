---
id: "fix-ui-ops_script-tab-relay-display-and-ops-script-perms"
domain: "fix-ui-ops"
slug: "script-tab-relay-display-and-ops-script-perms"
title: "1. 命令执行脚本标签里的中转站显示叠层三角形异常；2. 运维角色缺少剧本写权限（需能删除/保存本地脚本）；3. 批量执行需增加\"不输出节点响应\"选择按钮（右侧"
status: "resolved"
created: "2026-09-22T22:28:38+08:00"
resolved: "2026-09-22T23:29:45+08:00"
commit: "b030776"
branch: "feat/v1.7.0-ai-chain"
platform: "darwin"
session: "ses_f367b4a76ffeMd8CCDbIJPmYcX"
---

# fix-ui-ops_script-tab-relay-display-and-ops-script-perms

## 问题

1. 命令执行脚本标签里的中转站显示叠层三角形异常；2. 运维角色缺少剧本写权限（需能删除/保存本地脚本）；3. 批量执行需增加"不输出节点响应"选择按钮（右侧）；4. 增加仓库地址签名和作者签名

## 环境

| 项 | 值 |
|----|----|
| git commit | b030776 |
| 分支 | feat/v1.7.0-ai-chain |
| 平台 | darwin |
| 建档时间 | 2026-09-22T22:28:38+08:00 |
| 会话 | ses_f367b4a76ffeMd8CCDbIJPmYcX |

## 调查过程

- [22:28] 建档
- [23:29] 记录日志 (bash): 确认中转站下拉框箭头在主题模式下平铺的原因与修复效果
- [23:29] 记录日志 (bash): 运维角色剧本保存/删除权限修复与验证
- [23:29] 记录日志 (chat): 批量执行抑制输出开关与签名功能实现及验证
- [23:29] 记录证据 1 项
- [23:29] 记录终端文本快照
- [23:29] 结案

## 日志与摘录

### [bash] 2026-09-22T23:29:27+08:00 · 确认中转站下拉框箭头在主题模式下平铺的原因与修复效果

```
问题1根因定位（Playwright computed style）：#script-staging-select 使用 class="exec-input"，其 `background: var(--bg)` 简写把 background-repeat/position 重置为初始值(no-repeat 被覆盖成 repeat、position 0 0)；而主题覆盖规则 `html[data-theme="dark-warm"] select` / `html[data-theme="light-sky"] select`（特异性 0,1,2）又重新写回 background-image（箭头），于是箭头在整条 select 上平铺成"叠层的三角形"。
修复：改用已有的 .exec-select（自带 appearance:none + 箭头 no-repeat + right 8px center + padding-right:24px）。
验证(修复后): dark-warm/light-sky 下 bgRepeat=no-repeat, bgPos=calc(100% - 8px) 50%, padRight=24px，截图单个箭头。
```

### [bash] 2026-09-22T23:29:32+08:00 · 运维角色剧本保存/删除权限修复与验证

```
问题2：POST /playbook/template 原为 admin-only（保存/新建本地剧本），且无删除接口，与 RBAC 矩阵「operator 剧本管理 ✓」矛盾。
修复：server.go 将 POST /playbook/template 移入 operator 组；新增 PlaybookHandler.Delete（删本地 YAML + 清 DB 记录）并注册 operator.DELETE /playbooks/:id；前端详情页加「删除」按钮（两段式确认，遵循 playbooks.js 禁原生 confirm 的约束）。
E2E（operator ops1）：POST /playbook/template=201；DELETE /playbooks/:id=200；删除后列表不再包含；POST /playbook/refresh 仍 403（保持 admin）；UI 两段式删除 删除→确认删除→生效。
```

### [chat] 2026-09-22T23:29:36+08:00 · 批量执行抑制输出开关与签名功能实现及验证

```
问题3：exec 右栏「输出选项」新增「不输出节点响应」开关(#suppress-output)。勾选后 handleExec 不打印 task_output 流、不重建补全，仅显示任务状态与"输出已抑制，完整输出见任务历史"；多节点创建时改显示汇总行。前端展示层抑制，不改变后端采集（日志/历史仍完整）。
问题4：登录页卡片下方与左侧导航栏底部新增签名 github.com/cangyunye/go-owl · cangyunye（新增 icon-git symbol；nav 折叠仅图标、展开显示文字）。
验证：go test ./cmd/plugins/serve/... 通过（含 playbooksui 禁 alert/confirm 断言）；make build-serve 成功；Playwright 截图确认。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: E2E 终端输出：4 项改动全部通过](shots/001-232940.txt)

## 修复方案

1) exec.js 中转站下拉框 class 由 exec-input 改为 exec-select，消除主题模式下箭头平铺。2) server.go 将 POST /playbook/template 从 admin 移入 operator 组；playbook.go 新增 Delete（删本地 YAML + 清 DB 记录）并注册 operator.DELETE /playbooks/:id；api.js 加 deletePlaybook；playbooks.js 详情页加两段式确认的删除按钮。3) exec.js 输出选项新增 #suppress-output，勾选后不打印各节点 task_output、不重建补全，仅显示状态与历史提示。4) login.js 登录卡片下、app.js 导航栏底部加仓库地址+作者签名（新增 icon-git symbol + CSS）。

## 复盘

根因一：CSS 简写 `background: var(--bg)` 会重置 background-repeat/position，而主题覆盖 `html[data-theme] select` 只补回 background-image，特异性更高导致箭头平铺。教训：给 select 做自定义样式时用专门的 .exec-select 类，别用仅面向文本输入的 .exec-input；跨主题的 select 覆盖规则要么写全 background 属性，要么只改颜色。根因二：RBAC 文档矩阵与路由实际权限脱节——operator 标称「剧本管理✓」但保存/删除接口缺失或限 admin。教训：改权限时对照 docs/serve-user.md 的角色矩阵，保证文档、路由、前端按钮三者一致。另注意 playbooks.js 有单测禁止原生 alert/confirm，删除确认需用页内两段式/自定义弹窗。
