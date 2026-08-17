---
id: "feat-tui_nodes-menu-pagination"
domain: "feat-tui"
slug: "nodes-menu-pagination"
title: "owl TUI 的 nodes 菜单需要翻页功能,节点太多时看不到顶上菜单选项,怀疑有全屏高度限制"
status: "resolved"
created: "2026-08-17T22:20:02+08:00"
resolved: "2026-08-17T22:40:21+08:00"
commit: "576964e"
branch: "main"
platform: "darwin"
session: "ses_fefe82fe3ffeFY3UWeoOYmyBzy"
---

# feat-tui_nodes-menu-pagination

## 问题

owl TUI 的 nodes 菜单需要翻页功能,节点太多时看不到顶上菜单选项,怀疑有全屏高度限制

## 环境

| 项 | 值 |
|----|----|
| git commit | 576964e |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-17T22:20:02+08:00 |
| 会话 | ses_fefe82fe3ffeFY3UWeoOYmyBzy |

## 调查过程

- [22:20] 建档
- [22:21] 记录日志 (chat): 根因: nodes 模型未跟踪 Height,listPane 无高度裁剪,全部渲染导致溢出
- [22:38] 记录证据 1 项
- [22:38] 记录终端文本快照
- [22:38] 记录日志 (chat): 修复已落地:高度跟踪+窗口滚动+PgUp/Dn翻页+位置指示,并修复表头换行与尾空行两处溢出 bug
- [22:40] 结案

## 日志与摘录

### [chat] 2026-08-17T22:21:39+08:00 · 根因: nodes 模型未跟踪 Height,listPane 无高度裁剪,全部渲染导致溢出

```
根因确认: nodes.Model 只接收 WindowSizeMsg 的 Width,忽略 Height(cmd/cli/cmd/tui/nodes/model.go:212)。listPane()(view.go:66-83)对 m.visible() 全部渲染,无高度上限。App.View() 顶部有 3 行 chrome(menuBar/path/分隔线)+ statusBar 1 行。节点一多输出就超出终端高度,alt-screen 溢出导致顶菜单被顶出屏幕。
对照: ai/chat.go:126 已有 msg.Height-8 的高度处理惯例(8 = 顶部chrome3 + 边框2 + header1 + separator1 + statusbar1 之外的预留)。
```

### [chat] 2026-08-17T22:38:15+08:00 · 修复已落地:高度跟踪+窗口滚动+PgUp/Dn翻页+位置指示,并修复表头换行与尾空行两处溢出 bug

```
修复方案:
1. nodes.Model 新增 height/offset 字段,WindowSizeMsg 同时更新 height(tui 惯例按 ai/chat.go 的 msg.Height-8 预留顶部 chrome)。
2. listPane 只渲染 offset..offset+listRows 窗口,listRows = height-9(顶部chrome3+状态栏1+边框2+表头1+分隔1+滚动提示行1)。
3. 光标移动/翻页自动滚动窗口(clampCursor 联动 offset);新增 PgUp/PgDn/Ctrl+u/Ctrl+d 整页翻页;g/G 跳首尾并归位窗口。
4. 状态栏显示 [n/total] 位置 + 列表内滚动提示(↓ 还有 N 项 / ↑ 已滚动)。
5. 顺带修复两处既有布局 bug(导致输出比屏幕高 2 行、顶菜单被挤出屏幕):
   - 表头列宽按 avail-5 分配,实际宽度 = 5+sum+len(cols) > avail,多出 len(cols) 字符触发换行多占 1 行 → 改为 avail-5-len(cols)。
   - 列表内容以尾部换行结尾,边框渲染后多一个空行 → 改为按行拼接(Join)无尾换行。
6. detailPane 高度按 rows+3 截断(truncateLines),小终端下不溢出。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: PTY 24x100 运行 owl tui,30 节点初始帧:顶部菜单可见,列表只渲染 15 行 + 滚动提示,位置 [1/30]](shots/001-223804.txt)

## 修复方案

nodes 列表翻页与高度裁剪:
1. Model 新增 height/offset,WindowSizeMsg 同步更新 height;listRows = height-9 计算可见行数。
2. listPane 只渲染 offset..offset+rows 窗口,新增 PgUp/PgDn/Ctrl+u/Ctrl+d 整页翻页,g/G 跳首尾并归位窗口,光标越界自动滚动。
3. 状态栏显示 [n/total] 位置,列表内提示 ↓ 还有 N 项/↑ 已滚动;detailPane 按 rows+3 截断。
4. 修复两处既有溢出 bug(共同导致输出比屏幕高 2 行、顶菜单被挤出屏):
   - 表头列宽分配用 avail-5,实际占宽 5+sum+len(cols) 超 avail 触发换行 → 改为 avail-5-len(cols)。
   - 列表内容以尾换行结束,边框多出空行 → 改为按行 Join。
验证: 单元测试(高度限制/滚动/翻页/首尾跳转/位置显示)+ PTY 24 行 E2E(30 节点,顶菜单可见、首屏不含 node-30、PgDown/G 翻页正常)+ 全仓 go test ./... 通过。

## 复盘

根因不是"全屏高度限制",而是 nodes.Model 从未跟踪 WindowSizeMsg 的 Height,且 listPane 无条件全量渲染。做高度计算时务必对最终渲染逐行核对:alt-screen 下内容超过终端行数会从顶部挤出,症状是"菜单没了"而不是"内容被截断"。header 宽度计算忽略了列间空格占位(len(cols))导致换行、字符串尾部换行会让边框多一空行,这类 +1/-1 误差只有在真实终端(PTY 定行数)下才暴露,单测的 strings.Contains 断言查不出。E2E 必须用固定 winsize 的 PTY 而非只靠 strip ANSI。
