---
id: "fix-playbook_run-history-view-no-response"
domain: "fix-playbook"
slug: "run-history-view-no-response"
title: "剧本管理菜单的执行后的运行历史里的 view 按钮没有响应"
status: "resolved"
created: "2026-08-12T12:44:13+08:00"
resolved: "2026-08-12T13:06:44+08:00"
commit: "b6b0339"
branch: "main"
platform: "darwin"
session: "ses_00e6cdafcffenTzLmhuizVdh2N"
---

# fix-playbook_run-history-view-no-response

## 问题

剧本管理菜单的执行后的运行历史里的 view 按钮没有响应

## 环境

| 项 | 值 |
|----|----|
| git commit | b6b0339 |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-12T12:44:13+08:00 |
| 会话 | ses_00e6cdafcffenTzLmhuizVdh2N |

## 调查过程

- [12:44] 建档
- [13:06] 记录日志 (chat): 根因:重绘吞掉 click 事件;修复:pointerdown/pointerup 委托
- [13:06] 新增 E2E 用例: 重绘竞态下 View 点击不丢失
- [13:06] 记录证据 1 项
- [13:06] 记录终端文本快照
- [13:06] 结案

## 日志与摘录

### [chat] 2026-08-12T13:06:28+08:00 · 根因:重绘吞掉 click 事件;修复:pointerdown/pointerup 委托

```
根因定位:
1. renderRuns() 把 click 监听器绑在每个 .view-run-btn 上;而 loadRuns()(由 WS playbook_run_update 触发)用 innerHTML 整体重绘运行历史表格。
2. 若重绘发生在 mousedown→mouseup 之间,原按钮被移除,DOM 变更期间浏览器不会派发 click 事件 → 点击被吞掉,表现为「View 没有响应」。
3. Playwright 复现证实:race 期间 pointerdown/mousedown/pointerup/mouseup 都触发,唯独 click 不触发(Chromium 对 mousedown 目标被移除时不派发 click)。
4. 附带问题:view 处理 .catch(() => {}) 吞掉加载错误;且未设置 run-detail 的 data-run-id,导致 WS 无法自动刷新已打开的详情。

修复:
- 事件委托改为 pointerdown 记录意图 + pointerup 执行,绑定在表格容器上仅一次(pointer 事件不受重绘影响);取消按钮同样处理。
- View 成功时设置 data-run-id 并滚动到详情卡片;失败时调用 showRunDetailError 显示错误而非静默吞掉。
- 初始化 ?run= 恢复路径与 WS 处理器同样增加错误显示与 null 保护。
- 新增内容断言测试 TestPlaybooksJS_RunViewClickSurvivesRerender。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|
| 1 | 重绘竞态下 View 点击不丢失 | 1. 打开 /playbooks,对 .view-run-btn 执行 mouse.down() 2. 在 down 与 up 之间用 JS 重绘 #playbook-runs-list 的 innerHTML(模拟 WS playbook_run_update 触发 loadRuns) 3. mouse.up() 并断言 URL 与 run-detail | 重绘发生在 mousedown→mouseup 之间时,View 仍能更新 URL 并渲染运行详情 | pass: race 后 URL 更新为 ?run=... 且详情渲染;活跃执行中连点 6 次 View 均正常,无 console 错误 |

## 证据截图

[文本快照: 修复后 E2E:竞态点击与活跃执行中点击均正常](shots/001-130636.txt)

## 修复方案

改动 cmd/plugins/serve/web/js/pages/playbooks.js:
- renderRuns 不再逐按钮绑 click,改为容器级事件委托 delegateRunActions(仅绑一次):pointerdown 记录意图(view/cancel + run id),pointerup 执行。pointer 事件在重绘替换按钮后仍会触发,彻底解决重绘竞态导致的点击丢失。
- View 成功时设置 run-detail 的 data-run-id(WS 自动刷新可命中)并 smooth 滚动到详情卡片;失败时 showRunDetailError 显示错误,不再静默吞掉。
- 页面初始化 ?run= 恢复路径与 WS 处理器同样接入 showRunDetailError 与 null 保护。
- filesjs_test.go 新增内容断言测试 TestPlaybooksJS_RunViewClickSurvivesRerender。

验证:Playwright 复现竞态(mousedown→重绘→mouseup)修复前 URL 不更新/详情不渲染,修复后正常;活跃执行中连点 6 次 View 全部正常;普通点击无回归;serve 模块全量测试 + vet 通过。

## 复盘

根因:WS 推送触发的 loadRuns() 用 innerHTML 整体重绘表格,若重绘发生在 mousedown→mouseup 之间,浏览器不会派发 click 事件,导致按钮点击被静默吞掉。教训:动态重绘列表上的按钮交互不应绑 click,应使用 pointerdown/pointerup 委托;且 .catch(()=>{}) 会掩盖这类「点击无响应」问题,错误应展示出来便于定位。
