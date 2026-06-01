# AI 输入补全功能规格

## Why

在 owl AI 会话交互模式下，用户经常需要重复输入类似的命令或查询语句。当前用户每次都需要完整输入完整的文本，降低了交互效率。通过实现基于历史输入的智能补全功能，用户只需输入前缀即可快速补全历史中相似的内容，提升操作效率。

## What Changes

- **新增 AI 输入补全 API**：在 `internal/history` 包中新增 `QueryInputCompletions` 函数，支持前缀匹配查询历史用户输入
- **扩展 history 接口**：在 `history.go` 中新增全局函数 `QueryInputCompletionsGlobal`
- **数据库优化**：在 `aichat` 表的 `input` 和 `created_at` 字段上创建复合索引，优化前缀查询性能
- **前端集成**：在 `cmd/cli/cmd/ai/ai.go` 的交互式输入循环中集成补全功能，使用 readline 库实现带补全的输入

## Impact

- Affected specs: `enhance-ai-dialog-capabilities`
- Affected code:
  - `internal/history/aichat.go` — 新增 `QueryInputCompletions` 函数
  - `internal/history/history.go` — 新增 `QueryInputCompletionsGlobal` 导出函数
  - `internal/history/interface.go` — 更新接口定义
  - `internal/history/db_sqlite3.go` — SQLite 数据库创建复合索引
  - `internal/history/db_duckdb.go` — DuckDB 数据库创建复合索引
  - `cmd/cli/cmd/ai/ai.go` — 交互式输入循环集成补全功能
  - `go.mod` — 添加 readline 依赖

## ADDED Requirements

### Requirement: AI 输入补全 API

系统 SHALL 提供 `QueryInputCompletions` 函数，允许根据输入前缀查询历史用户输入，并按最近时间优先返回匹配结果。

#### Function Signature

```go
func QueryInputCompletions(db *sql.DB, prefix string, limit int) ([]string, error)
```

#### Parameters

| 参数 | 类型 | 说明 |
|------|------|------|
| db | *sql.DB | 数据库连接 |
| prefix | string | 匹配前缀（至少 1 个字符） |
| limit | int | 返回结果上限 |

#### Return Value

返回匹配的输入字符串切片，按 `created_at DESC` 排序（最近优先），最多返回 `limit` 条。

#### Scenario: 前缀匹配查询

- **WHEN** 调用 `QueryInputCompletions(db, "查询", 5)`
- **THEN** 返回所有 `role='user'` 且 `input` 以 "查询" 开头的结果
- **AND** 结果按 `created_at` 降序排列
- **AND** 最多返回 5 条

#### Scenario: 无匹配结果

- **WHEN** 调用 `QueryInputCompletions(db, "xyz", 5)` 且无匹配
- **THEN** 返回空切片 `[]string{}`
- **AND** 不返回错误

#### Scenario: 空前缀

- **WHEN** 调用 `QueryInputCompletions(db, "", 5)`
- **THEN** 返回错误 `"prefix cannot be empty"`

### Requirement: 全局查询函数

系统 SHALL 提供 `QueryInputCompletionsGlobal` 全局函数，封装数据库连接获取和错误处理。

#### Function Signature

```go
func QueryInputCompletionsGlobal(prefix string, limit int) ([]string, error)
```

#### Scenario: 全局查询

- **WHEN** 调用 `QueryInputCompletionsGlobal("测试", 10)`
- **THEN** 自动获取全局数据库连接
- **AND** 调用 `QueryInputCompletions`
- **AND** 返回匹配结果

#### Scenario: 数据库未初始化

- **WHEN** 全局数据库为 nil
- **THEN** 返回空切片 `[]string{}`
- **AND** 不返回错误

### Requirement: 交互式输入补全

CLI 交互模式 SHALL 在用户输入时提供智能补全功能，通过 Tab 键触发补全建议。

#### Trigger Condition

- 用户输入至少 1 个字符后
- 用户按下 Tab 键

#### Behavior

- **WHEN** 用户在输入过程中按下 Tab
- **THEN** 系统查询与当前输入前缀匹配的历史输入
- **AND** 显示匹配结果供用户选择
- **AND** 如果只有 1 个匹配，自动补全
- **AND** 如果多个匹配，显示列表供选择

#### Scenario: 单个匹配自动补全

- **WHEN** 用户输入 "查询" 后按 Tab
- **AND** 历史中只有 "查询所有节点"
- **THEN** 自动补全为 "查询所有节点"
- **AND** 光标移动到行尾

#### Scenario: 多个匹配显示列表

- **WHEN** 用户输入 "查询" 后按 Tab
- **AND** 历史中有 "查询所有节点" 和 "查询在线节点"
- **THEN** 显示列表：
  ```
  > 查询所有节点
  > 查询在线节点
  ```
- **AND** 等待用户选择或继续输入

#### Scenario: 无匹配

- **WHEN** 用户输入 "xyz" 后按 Tab
- **AND** 历史中无匹配
- **THEN** 不做任何补全
- **AND** 发出提示音或显示 "无匹配"

## MODIFIED Requirements

### Requirement: aichat 表结构优化

**原因**: 提升前缀查询性能

**Migration**:
1. 为 `aichat` 表的 `(role, input, created_at)` 创建复合索引
2. SQLite: `CREATE INDEX idx_aichat_user_input_time ON aichat(role, input, created_at)`
3. DuckDB: `CREATE INDEX idx_aichat_user_input_time ON aichat(role, input, created_at)`

### Requirement: ai.go 交互式输入循环优化

**原代码**: 使用 `bufio.NewScanner(os.Stdin)` 简单读取
**新代码**: 使用 `github.com/chzyer/readline` 库实现带补全的交互式输入

**Migration**:
1. 导入 `github.com/chzyer/readline` 包
2. 创建 `*readline.Instance` 实例
3. 配置补全器 `readline.Config.AutoComplete`
4. 使用 `rl.Readline()` 替代 `scanner.Text()`
5. 在读取循环中集成补全逻辑

## REMOVED Requirements

无

## Technical Design

### Database Schema

```sql
-- 假设 aichat 表已存在，以下是索引优化
CREATE INDEX idx_aichat_user_input_time ON aichat(role, input, created_at);

-- 查询优化后的 SQL
SELECT input FROM aichat
WHERE role = 'user'
  AND input LIKE '查询%'
ORDER BY created_at DESC
LIMIT 5;
```

### Prefix Matching Implementation

使用 SQL 的 `LIKE 'prefix%'` 实现前缀匹配，无需全文索引。

### Recent-First Ordering

使用 `ORDER BY created_at DESC` 确保最近输入优先返回。

### Frontend Completion Integration

使用 `chzyer/readline` 库的自动补全功能：
1. 创建补全器实例，实现 `Complete(word string) []string`
2. 在补全器中调用 `QueryInputCompletionsGlobal(prefix, limit)`
3. 配置 readline 使用该补全器
4. 在输入循环中处理补全结果

### Error Handling

- 数据库连接失败：返回空切片，不影响主流程
- 查询失败：记录日志，返回空切片
- 输入为空：返回错误提示

## Configuration

无新增配置项。

## Testing Strategy

### Unit Tests

1. `QueryInputCompletions` - 测试前缀匹配、边界条件、排序
2. `QueryInputCompletionsGlobal` - 测试全局函数封装

### Integration Tests

1. 补全 API 与数据库集成
2. 交互式输入补全流程

### Manual Testing

1. 启动 `owl ai` 交互模式
2. 输入历史中的命令前缀
3. 按 Tab 触发补全
4. 验证补全结果正确性
