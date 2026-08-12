---
id: "feat-ai-session_ai-session-user-isolation"
domain: "feat-ai-session"
slug: "ai-session-user-isolation"
title: "AI助手会话与前端对话历史未按用户区分:前端 IndexedDB 固定库名所有用户共享,服务端 Session 内存键仅 sessionID 不绑 user_i"
status: "resolved"
created: "2026-08-12T00:09:01+08:00"
resolved: "2026-08-12T07:53:35+08:00"
commit: "55fa9cd"
branch: "main"
platform: "darwin"
session: "ses_00e6cdafcffenTzLmhuizVdh2N"
---

# feat-ai-session_ai-session-user-isolation

## 问题

AI助手会话与前端对话历史未按用户区分:前端 IndexedDB 固定库名所有用户共享,服务端 Session 内存键仅 sessionID 不绑 user_id。用户确认要按用户隔离。

## 环境

| 项 | 值 |
|----|----|
| git commit | 55fa9cd |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-12T00:09:01+08:00 |
| 会话 | ses_00e6cdafcffenTzLmhuizVdh2N |

## 调查过程

- [00:09] 建档
- [00:18] 记录日志 (chat): 根因确认与修复方案
- [07:53] 新增 E2E 用例: 同一 session_id 下 admin/alice 两用户的 AI 会话隔离与审计归属
- [07:53] 记录证据 1 项
- [07:53] 记录终端文本快照
- [07:53] 结案

## 日志与摘录

### [chat] 2026-08-12T00:18:17+08:00 · 根因确认与修复方案

```
根因:
1. 前端 IndexedDB 库 owl_ai_chat 固定库名, conversations 表无 userId 字段 → 同浏览器所有用户共享对话列表。
2. 服务端 AuthMiddleware 未设置 user_id 上下文 → AIHandler.Chat 里 c.GetString("user_id") 恒为空, 全部记为 "anonymous"。
3. 服务端 SessionManager 内存 map 仅以 sessionID 为键 → 不同用户提交相同 sessionID 会共享对话上下文。

修复方案:
- auth.go: AuthMiddleware 增加 c.Set("user_id", claims.Username)。
- ai.go Chat: 会话键改为 userID:sessionID (anonymous 保持裸 sessionID), fallback 键同样命名空间。
- storage.js: DB_VERSION 1→2, conversations 增加 user_id 索引; saveConversation(conv, userId) 打标; getConversations(userId, ...) 按 userId 过滤并按 createdAt 倒序。
- ai.js: 定义 userId = user.id||user.username; saveCurrentConv 用 userId::Date.now() 作为会话 id 防跨用户撞 id; 存取都传 userId。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|
| 1 | 同一 session_id 下 admin/alice 两用户的 AI 会话隔离与审计归属 | 1. 启动 ./build/owl-serve --reset-admin --port 8080 2. 用 admin 创建 operator 用户 alice 3. 分别用 admin 与 alice 的 token 调用 POST /api/v1/ai/chat, session_id 均为 e2e-shared-session-001 4. 查询 ai_audit_log 的 user_id 列 | 两条新审计记录的 user_id 分别为 admin 与 alice(而非 anonymous); 两次 chat 均 200。 | pass: 新记录 user_id=alice 与 admin, 旧 8-7 记录仍为 anonymous(修复前) |

## 证据截图

[文本快照: E2E 验证通过: 同 session_id 双用户 chat + 审计归属 + 前端文件已内嵌新逻辑](shots/001-075328.txt)

## 修复方案

服务端:
- handler/auth.go: AuthMiddleware 增加 c.Set("user_id", claims.Username), 让 AI chat 等 handler 拿到真实用户而非空 → 修掉所有用户审计都被记为 "anonymous" 的问题。
- handler/ai.go Chat: 会话键从 sessionID 改为 userID:sessionID(匿名保持原样), fallback 会话键同步命名空间 → 同一 sessionID 在不同用户下互不共享/续接上下文。

前端:
- web/js/storage.js: DB_VERSION 1→2, conversations 对象库新增 user_id 索引(含升级分支); saveConversation(conv, userId) 打标 userId; getConversations(userId,...) 用 IDBKeyRange.only(userId) 过滤, 按 createdAt 倒序分页。
- web/js/pages/ai.js: 取当前登录用户 userId = user.id||user.username; 会话 id 改为 userId::Date.now() 防跨用户撞 id; 存取历史均传 userId。

测试:
- 新增 TestAuthMiddleware_SetsUserID、TestChat_SessionIsolatedPerUser(断言同 sessionID 两用户 ListSessions()==2)、storage.js/ai.js 内容断言测试。
E2E: 双用户同 session_id chat 均 200, ai_audit_log.user_id 正确归属 admin/alice。

## 复盘

根因: 前端 IndexedDB 用固定库名且无 userId 字段, 服务端 AuthMiddleware 只设 username/role 不设 user_id, SessionManager 仅以 sessionID 为键。三点叠加导致: 同浏览器多用户共享会话列表、AI 审计全部记为 anonymous、跨用户可共享 session 上下文。教训: 多用户 web 应用做会话隔离时要同时覆盖存储层(打归属字段)、身份层(中间件注入 user_id)、内存层(键加用户命名空间); 并且 user_id 注入与读取要成对检查, 否则 C.GetString("user_id") 恒空会静默回退到 anonymous, 单测很难发现。
