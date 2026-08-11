---
id: "fix-tui_file-upload-button-double-click"
domain: "fix-tui"
slug: "file-upload-button-double-click"
title: "为什么界面版本的文件传输上传按钮要双击?"
status: "resolved"
created: "2026-08-10T22:56:06+08:00"
resolved: "2026-08-10T23:47:23+08:00"
commit: "968164e"
branch: "main"
platform: "darwin"
session: "ses_013d39a59ffe70LQj4gdYxgkwT"
---

# fix-tui_file-upload-button-double-click

## 问题

为什么界面版本的文件传输上传按钮要双击?

## 环境

| 项 | 值 |
|----|----|
| git commit | 968164e |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-10T22:56:06+08:00 |
| 会话 | ses_013d39a59ffe70LQj4gdYxgkwT |

## 调查过程

- [22:56] 建档
- [22:58] 记录日志 (chat): 定位根因:上传按钮单击只切方向,双击才触发传输
- [22:59] 记录日志 (bash): 确认是 commit a32e31a 引入的回归:单击只切方向,传输被移到 dblclick
- [23:16] 记录日志 (chat): 确认修复方案
- [23:44] 记录日志 (bash): E2E: 单击上传按钮弹出"传输任务已提交:57 个节点",无 JS 错误
- [23:44] 记录证据 1 项
- [23:44] 记录终端文本快照
- [23:47] 结案

## 日志与摘录

### [chat] 2026-08-10T22:58:15+08:00 · 定位根因:上传按钮单击只切方向,双击才触发传输

```
files.js 中上传按钮绑定了两种事件:
- 单击 (click, L686-693): 只切换 activeDirection (push/pull) 并更新路径标签,不执行传输
- 双击 (dblclick, L695-696): 才调用 handleTransfer('push'/'pull') 真正发起传输

所以用户必须双击才能上传。这是按钮交互设计的缺陷。
```

### [bash] 2026-08-10T22:59:31+08:00 · 确认是 commit a32e31a 引入的回归:单击只切方向,传输被移到 dblclick

```
git 历史定位: commit a32e31a "feat(staging): add full path column to staging file list"

改前(旧版 files.js L142-143):
  document.getElementById('upload-btn').addEventListener('click', () => handleTransfer('push'));
  document.getElementById('download-btn').addEventListener('click', () => handleTransfer('pull'));
即单击直接触发传输。

改后(当前):
  click 只切 activeDirection + 更新标签;
  dblclick 才 handleTransfer。

结论: 这是回归。该 commit 把按钮点击改成"切换方向指示",却把实际传输挂在 dblclick 上,导致用户必须双击才能上传/下载。
```

### [chat] 2026-08-10T23:16:55+08:00 · 确认修复方案

```
用户确认需要修复。方案: 单击直接触发传输,删除 dblclick。按 TDD 先写测试。
```

### [bash] 2026-08-10T23:44:39+08:00 · E2E: 单击上传按钮弹出"传输任务已提交:57 个节点",无 JS 错误

```
Playwright E2E (chromium, headless) 验证通过:
- 登录 admin → /files 页
- 找到 #upload-btn (text "上传")
- 单击一次 → 弹出 alert "传输任务已提交：57 个节点"
- 无 JS 错误 (pageerror / console.error 均为空)

结论: 修复生效,单击即触发上传,不再需要双击。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: E2E 单击上传按钮后触发传输提交 (alert: 传输任务已提交：57 个节点),无 JS 错误](shots/001-234439.txt)

## 修复方案

web/js/pages/files.js 中把 upload-btn/download-btn 的传输触发从 dblclick 移回 click:
- click 处理器内先 activeDirection=push/pull + updatePathLabels(),随后立即调用 handleTransfer('push'/'pull')
- 删除两个 dblclick 监听器
修复前单击只切换方向指示,双击才发传输;修复后单击即提交传输任务。

## 复盘

回归来自 commit a32e31a: 为支持 staging 多选批量传输重构 files.js 时,把按钮 click 改成了"仅切换方向",却把 handleTransfer 挂到 dblclick 上,导致用户必须双击。教训: 重构交互时若把"动作执行"与"状态切换"拆到不同事件,会改变原有交互预期;改动后应保留 click 直接执行传输的行为,并用 E2E 验证单击路径。
