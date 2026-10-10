# AI 助手左栏：会话搜索 / 删除 / 无限滚动 + 上下文分页 — 实施计划

## 背景

- 会话存于浏览器 IndexedDB（`AIStorage`），`getConversations(userId, limit, offset)` 已支持
  分页，但左栏只取前 50 条、无搜索；删除按钮 `✕` 仅鼠标悬停可见（触屏不可用）。
- 对话上下文来自 `GET /api/v1/ai/context`，固定返回最近 20 条，无翻页。
- 会话多、上下文多时，列表无搜索、看不全更早的历史。

## 目标（已确认）

1. 会话：**标题 + 正文**搜索；**自动无限滚动**加载更早会话；删除按钮**常驻**、点击即删（无确认）。
2. 对话上下文：**自动无限滚动**加载更早操作（后端加分页）。
3. 保持 AI 页左栏「会话 / 对话上下文」两段布局与既有交互（新建/导入/待审批）。

## 设计

### 会话（前端为主，IndexedDB）
- `storage.js`：新增 `getAllConversations(userId)`（复用 `getConversations`，取全部并倒序）。
- `ai.js`：内存态 `allConvs / convQuery / convShown`，页大小 30。
  - 标题 = 首条消息；搜索匹配标题或任一消息正文（忽略大小写）。
  - `renderConvList()` 渲染首屏 + 底部哨兵；`appendConvPage()` 追加下一页。
  - 列表用**事件委托**绑定点击（载入会话 / 删除），追加时无需重新绑定。
  - `IntersectionObserver` 观察哨兵（root = `.ai-conv-list`）触发追加；搜索变化时重置。
  - 删除：从 `allConvs` 移除并重渲染；若为当前会话则清空聊天区。
- `app.css`：`.ai-conv-search` 输入框、`.ai-conv-sentinel`、删除按钮常驻。

### 对话上下文（前后端）
- 后端 `GetContext`：读 `offset`（默认 0）、`limit`（默认 20，上限 100），
  `QueryOptions.Offset/Limit`；响应增加 `has_more`。
- `api.js`：`getAiContext(offset, limit)` 拼 query。
- `ai.js`：内存态 `ctxData / ctxOffset / ctxHasMore / ctxLoading`；
  `loadContext(reset)` 拉一页并追加渲染；哨兵（root = `#ai-ctx-body`）触发下一页。
- 挂载、发送回复后、保活切回均 `loadContext(true)` 重置。

### 测试
- 后端：`TestGetContext_OffsetPagination`（3 条，limit 2 → 第一页 2 + has_more=true；
  offset 2 → 1 + has_more=false）。
- 静态断言（serve 包）：`storage.js` 暴露 `getAllConversations`；`ai.js` 含
  `ai-conv-search` / `ai-conv-sentinel` / `ai-ctx-sentinel` / `IntersectionObserver`。

## 验收

- `go test ./cmd/plugins/serve/...` 全绿。
- 手动：造多条会话 → 搜索命中标题与正文、滚动加载更早；删除常驻 ✕ 生效；
  上下文滚动加载更早操作（后端分页）。
