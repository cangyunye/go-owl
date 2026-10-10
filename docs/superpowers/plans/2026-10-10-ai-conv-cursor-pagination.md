# 会话列表：复合索引 + 游标分页 — 实施计划

## 背景

上一步会话列表用「IndexedDB 全量取回（含全部消息）→ 内存搜索 + 切片」实现搜索与
无限滚动。单用户会话上千条且消息体很大时，每次挂载/保存都全量读入内存，成本高。

## 目标

- 列表**浏览**改为真正的**游标分页**：只读当前一页的会话，不再一次加载全部。
- 搜索仍覆盖标题 + 正文（偶发操作，允许一次全量扫描）。

## 设计

### storage.js（DB v2 → v3）
- `onupgradeneeded` 改为幂等确保索引存在，并新增**复合索引** `user_created`
  （keyPath `['userId','createdAt']`）。
- `getConversationsPage(userId, limit, beforeCreatedAt)`：用 `user_created` 索引、
  `prev` 方向、`IDBKeyRange.bound([userId,''],[userId, cursor], false, !!cursor)`，
  倒序取一页；返回 `{ items, nextCursor }`（`nextCursor` 取末条 `createdAt`，
  用于取更早一页；取满才给，否则为 null 表示到底）。
- `searchConversations(userId, query)`：全量扫描 + 标题/正文过滤（倒序）。
- `getConversation(id)`：按主键取单条（点开某会话时才读消息）。
- 移除 `getAllConversations`（不再需要）。

### ai.js
- 会话列表状态改为取数器模型：`convFetcher`（下一页取数闭包）、`convDone`、
  `convLoading`、`convQuery`。
  - 无搜索：`getConversationsPage` 游标分页（只读一页）。
  - 有搜索：`searchConversations` 一次扫描后本地切片。
- `loadConvPage(first)`：重置并取首页 / 追加下一页；`updateConvSentinel` 维护底部哨兵。
- `loadConversationById(id)`：改为 `getConversation(id)` 按需读取。
- 删除：删库后重载首页。
- 事件委托、搜索框、常驻删除保持不变。

### 测试
- 静态断言更新：`storage.js` 含 `user_created` 复合索引 / `getConversationsPage` /
  `searchConversations`；`ai.js` 含 `getConversationsPage(userId`；
  `filesjs` 的用户隔离断言改为 `getConversationsPage(userId`。

## 验收

- `go test ./cmd/plugins/serve/...` 全绿。
- 手动：40 条会话首屏 30 + 哨兵、滚动加载更早、搜索标题/正文命中、点开/删除正常；
  复合索引升级在旧库（v2）上自动完成。
