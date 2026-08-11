---
id: "fix-webui_files-upload-btn-gray-and-double-click"
domain: "fix-webui"
slug: "files-upload-btn-gray-and-double-click"
title: "1.文件传输的上传按钮字体颜色为什么一直是灰色的失效感觉,2.文件传输的上传按钮还是要点两次"
status: "resolved"
created: "2026-08-11T18:53:37+08:00"
resolved: "2026-08-11T19:29:29+08:00"
commit: "65e17e7"
branch: "main"
platform: "darwin"
session: "ses_00f9b7a9affebqulkoHEEl6Yks"
---

# fix-webui_files-upload-btn-gray-and-double-click

## 问题

1.文件传输的上传按钮字体颜色为什么一直是灰色的失效感觉,2.文件传输的上传按钮还是要点两次

## 环境

| 项 | 值 |
|----|----|
| git commit | 65e17e7 |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-11T18:53:37+08:00 |
| 会话 | ses_00f9b7a9affebqulkoHEEl6Yks |

## 调查过程

- [18:53] 建档
- [19:13] 记录日志 (chat): 定位两个根因:1) 灰字是 CSS .btn-primary.active 未设 color,继承了 .btn.active 的 color:var(--accent),低对比度看似失效;2) 双击是 build/owl-serve 二进制陈旧(8-10 构建,内嵌旧 files.js 仍含 dblclick),源码已修复但用户跑的是旧二进制。
- [19:29] 新增 E2E 用例: 文件传输上传按钮灰字 + 单击即可提交
- [19:29] 记录证据 1 项
- [19:29] 记录终端文本快照
- [19:29] 结案

## 日志与摘录

### [chat] 2026-08-11T19:13:29+08:00 · 定位两个根因:1) 灰字是 CSS .btn-primary.active 未设 color,继承了 .btn.active 的 color:var(--accent),低对比度看似失效;2) 双击是 build/owl-serve 二进制陈旧(8-10 构建,内嵌旧 files.js 仍含 dblclick),源码已修复但用户跑的是旧二进制。

```
根因分析:
1. 灰字/失效感: files.js 上传按钮 HTML 是 class="btn btn-primary active"。CSS 里 .btn.active 设置了 color:var(--accent),而 .btn-primary.active 只覆盖了 background/border 没有覆盖 color,于是主按钮文字变成 accent 色、压在 accent-hover 背景上,对比度极低,看起来像灰色禁用。
2. 仍要点两次: 当前源码 (65e17e7) 已改为 click 事件内直接调用 handleTransfer('push'),单次点击即可。但 build/owl-serve (Aug 10 22:31) 二进制内嵌的 files.js 仍是旧版: click 只切方向, handleTransfer 挂在 dblclick 上。darwin-arm64/owl-serve (Aug 11) 已无 dblclick。用户运行的是 build/owl-serve 旧二进制。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|
| 1 | 文件传输上传按钮灰字 + 单击即可提交 | 1. 登录进入文件传输页 2. 检查上传按钮文字颜色 3. 填入源/目标路径后单击"上传" 4. 拖放文件到中转站 | 上传按钮文字为白色;单击一次即弹出"传输任务已提交";拖放文件自动上传 | pass |

## 证据截图

[文本快照: E2E 验证通过:按钮白色文字、单击提交、拖放上传均 PASS](shots/001-192907.txt)

## 修复方案

两个问题分别处理:
1. 灰字/失效感: CSS bug。app.css 里 .btn-primary.active 只覆盖了 background/border,没有覆盖 color,于是文字继承自 .btn.active 的 color:var(--accent),accent 色文字压在 accent-hover 背景上对比度低,看起来像灰色禁用。修复:在 .btn-primary.active 加 color:#fff,恢复白色主按钮外观。
2. 仍要点两次: 非源码问题,是二进制陈旧。build/owl-serve 是 8-10 构建,内嵌的 files.js 仍是旧版(click 只切方向,handleTransfer 挂在 dblclick 上)。当前源码在 65e17e7 已改为单击直接提交。修复:重新编译 build/owl-serve(go build 直接编译),验证新二进制无 dblclick、含 staging-dropzone。

## 复盘

根因教训:
1. 修改了源代码后忘记重建 go:embed 二进制,导致用户一直跑旧版本看到旧行为。web 资源是 go:embed 进二进制的,改完 js/css 必须重新编译才会生效。以后改前端要顺带重新 build-serve。
2. CSS 组合类要检查继承: .btn-primary.active 这种高特异性组合应显式声明 color,不能依赖 .btn-primary 的默认值被 .btn.active 覆盖。
3. Makefile 在本机因 SHELL 内嵌 $$ 解析问题报错(unterminated call to function shell),改用 go build 直接编译绕过。
