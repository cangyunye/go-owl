# AI 助手「对话上下文」面板 — 实施计划

## 背景 / 问题

- AI 页左侧壳层面板标题为「对话上下文」（`app.js` `PANEL_TITLES.ai`），但 `pages/ai.js`
  做的是 `scope.panel.setContent('')` + `sidePanel.style.display='none'`——面板被清空并隐藏。
- 后端 `GET /api/v1/ai/context` 是空桩（`handler/ai.go:307`），前端 `api.getAiContext()`
  定义后从未调用，`app.js` 里引用的 `agentContextPanel` 从未创建。
- 保活切回（`mountActiveTab`，`app.js:220`）只重设面板标题、未重新隐藏，导致空面板"漏"回
  （用户截图现象）。

## 目标

把「对话上下文」做成可用的运行时上下文面板：显示**当前用户近期由 AI 发起的操作**
（命令/脚本执行、文件传输、剧本运行），点击可按类型跳转对应页面。

## 范围决策

- **用户级近期**，不做会话级：现有 `operations`/`tasks`/`transfer_records`/`playbook_runs`
  均无 `session_id`，唯一可用的 AI 标记是 `operations.origin='ai'` + `username`。
- 数据源统一为 `operations` 单表（已含 origin/status/targets/op_type/command/created_at），
  避免多表 join。
- 补齐 AI 剧本运行的 origin 标记：当前 `WebExecutor.RunPlaybook` 未写 `operations`
  记录，AI 触发的剧本运行与 web 运行无法区分。

## 变更清单

### 后端
1. `store/history.go`
   - `QueryOptions` 增 `Origin string`；`Query` 增 `AND origin = ?`。
2. `handler/aiexecutor.go`
   - `RunPlaybook` 收尾时 `History.RecordOperation`：`OpType:"playbook"`、`Origin:"ai"`、
     `TaskID: run.ID`、`Command: pb.Name`、`Targets: nodeIDs`、`Status` 由 run 状态映射。
3. `handler/ai.go` `GetContext`
   - 读 `c.GetString("user_id")`；
     `Query(Origin:"ai", User:userID, Limit:20, SummaryOnly:true)`。
   - 按 `op_type` 分组为 `tasks`（command/script）/`transfers`（file_transfer）/
     `playbook_runs`（playbook）三个数组；每项
     `{id, task_id, op_type, command, targets, status, created_at}`。
   - `executor`/`History` 为 nil 时返回空数组（保持既有契约）。

### 前端
4. `pages/ai.js`
   - 不再隐藏 `sidePanel`/`panelToggle`；改为 `scope.panel.setContent(renderContext(...))`。
   - 新增 `loadContext()`：`api.getAiContext()` → 渲染分组列表 + 空态。
   - 挂载时加载；助手回复完成后刷新；`scope.onResume` 刷新。
   - 点击项 `navigate` 到 `/history`（命令/脚本、剧本）或 `/files`（传输）。
5. `css/app.css` 新增 `.ai-ctx-*` 样式。

## 测试（TDD，先失败后实现）

- `store`: `TestHistoryStore_QueryOriginFilter`
- `handler`: `TestGetContext_ReturnsAIOperations`（seed operations，断言分组 + 用户隔离）
- `handler`: `TestWebExecutor_RunPlaybook_RecordsAIOperation`（origin=ai）

## 验收

- `go test ./cmd/plugins/serve/...` 全绿。
- 手动：AI 执行一条命令后，左侧「对话上下文」出现该条，状态/目标节点正确；空态有提示；
  切标签返回后仍在且有内容。
