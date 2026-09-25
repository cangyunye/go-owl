---
id: "feat-ai-alert_curl-gbk-body-breaks-ai-intent"
domain: "feat-ai-alert"
slug: "curl-gbk-body-breaks-ai-intent"
title: "E2E 验证 AI 告警路由时，Web 端 /ai/chat 对\"有哪些节点有告警\"始终返回兜底帮助文案，CLI 端同样问题正常"
status: "resolved"
created: "2026-09-19T19:34:24+08:00"
resolved: "2026-09-19T19:35:05+08:00"
commit: "1e202fc"
branch: "main"
platform: "win32"
session: ""
---

# feat-ai-alert_curl-gbk-body-breaks-ai-intent

## 问题

E2E 验证 AI 告警路由时，Web 端 /ai/chat 对"有哪些节点有告警"始终返回兜底帮助文案，CLI 端同样问题正常

## 环境

| 项 | 值 |
|----|----|
| git commit | 1e202fc |
| 分支 | main |
| 平台 | win32 |
| 建档时间 | 2026-09-19T19:34:24+08:00 |
| 会话 | - |

## 调查过程

- [19:34] 建档
- [19:34] 记录日志 (chat): 根因是 curl 命令行中文被 GBK 转码，非产品缺陷；UTF-8 文件体重发即恢复
- [19:34] 记录日志 (bash)
- [19:35] 结案
- [19:35] 记录证据 1 项
- [19:35] 记录终端文本快照

## 日志与摘录

### [chat] 2026-09-19T19:34:40+08:00 · 根因是 curl 命令行中文被 GBK 转码，非产品缺陷；UTF-8 文件体重发即恢复

```
排查过程：
1. CLI 端 owl ai "有哪些节点有告警" 正常返回告警列表；Web 端 POST /api/v1/ai/chat 同样消息返回 FormatUncertainHelp 兜底文案，响应仅 915µs。
2. 干净重建 owl-serve.exe、更换端口进程后依旧 → 排除二进制陈旧。
3. agent.Process 单测（同构造）通过；handler 层 session.Send 单测通过；serve 包全服务器 httptest 单测通过（返回"当前没有符合条件的告警"= alert_list 工具真实执行）。
4. 决定性对照：查询 ai_audit_log 中 prompt_text 的原始字节，hex 为 efbfbd...（UTF-8 U+FFFD 替换符）+ 残留 GBK 字节。
根因：Git Bash on Windows (zh-CN, ANSI=GBK) 下 curl 命令行内联中文 body 被按 GBK 传递/转码，服务端收到乱码，本地意图分类器关键词（告警/节点/列出）全部无法匹配 → IntentUncertain → 兜底帮助文案。属测试工具编码问题，非产品缺陷。
修复：请求体改用 UTF-8 文件 + curl --data-binary @body.json（或 python requests 发送）。重发后 Web 端正常返回 50 条 OWL-OSS-001 告警。
```

### [bash] 2026-09-19T19:34:50+08:00

```
$ python -c "import sqlite3; db=sqlite3.connect('build/e2e/owl.db'); row=db.execute(\"SELECT prompt_text FROM ai_audit_log ORDER BY rowid DESC LIMIT 1\").fetchone(); print(row[0].encode('utf-8','surrogateescape')[:40].hex())"
efbfbdefbfbdefbfbdefbfbdd0a9efbfbddab5efbfbdefbfbdd0b8e6beafefbfbdefbfbdefbfbdef

修复后请求体（UTF-8 文件）：
$ curl --data-binary @build/e2e/body1.json ...
reply: 共 50 条告警：| OWL-OSS-001 | web-nginx-03 | critical | open | 节点 SSH 连续 3 次采集失败（失联）| ...
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: 终端文本快照](shots/001-193524.txt)

## 修复方案

无需改产品代码。E2E 测试方法修正：Windows (zh-CN) 下 curl 命令行内联中文会按 GBK 传递，导致服务端意图分类失败。改用 UTF-8 编码的 JSON 文件 + curl --data-binary @file 发送（或 python requests），Web 端 AI 告警查询/修复方案场景全部恢复正常。教训：中文 E2E 请求永远走文件体，不要内联在命令行；排查"AI 返回兜底文案"类问题先查 ai_audit_log 里 prompt_text 的原始字节。

## 复盘

根因：Git Bash/Windows 命令行参数经 ANSI 代码页（GBK）转码，中文 body 变成 U+FFFD 替换符序列，UTF-8 关键词匹配全部失效。踩坑点：三层单测全绿 + 干净重建二进制后 HTTP 仍失败，极易误判为"装配差异"；真正差异在测试客户端而非服务端。下次遇到 AI 意图识别异常，第一步先看审计表中存的 prompt 字节是不是合法 UTF-8。
