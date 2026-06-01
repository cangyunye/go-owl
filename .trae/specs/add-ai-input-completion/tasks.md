# Tasks: AI 输入补全功能实现

## Task 1: 数据库索引优化

为 `aichat` 表创建复合索引，优化前缀查询性能。

- [ ] 1.1: 在 `db_sqlite3.go` 中添加 `idx_aichat_user_input_time` 索引创建逻辑
- [ ] 1.2: 在 `db_duckdb.go` 中添加 `idx_aichat_user_input_time` 索引创建逻辑
- [ ] 1.3: 在 `db.go` 的数据库初始化逻辑中添加索引创建调用
- [ ] 1.4: 测试索引创建是否成功

## Task 2: 新增补全查询函数

在 `internal/history` 包中新增补全查询功能。

- [ ] 2.1: 在 `aichat.go` 中实现 `QueryInputCompletions` 函数
  - [ ] 2.1.1: 实现 SQL 查询逻辑（前缀匹配 + 排序）
  - [ ] 2.1.2: 处理空数据库连接情况
  - [ ] 2.1.3: 添加单元测试
- [ ] 2.2: 在 `history.go` 中实现 `QueryInputCompletionsGlobal` 全局函数
  - [ ] 2.2.1: 封装全局数据库连接获取
  - [ ] 2.2.2: 处理数据库未初始化情况
  - [ ] 2.2.3: 添加单元测试
- [ ] 2.3: 在 `interface.go` 中添加接口定义（如果需要）

## Task 3: CLI 交互式补全集成

在 `cmd/cli/cmd/ai/ai.go` 中集成补全功能。

- [ ] 3.1: 添加 readline 依赖到 `go.mod`
  - [ ] 3.1.1: 使用 `go get github.com/chzyer/readline` 添加依赖
- [ ] 3.2: 修改 `runAI` 函数中的输入循环
  - [ ] 3.2.1: 导入 readline 包
  - [ ] 3.2.2: 创建 `*readline.Instance` 实例
  - [ ] 3.2.3: 实现补全器 `Completer` 结构体
  - [ ] 3.2.4: 配置 readline 使用补全器
  - [ ] 3.2.5: 使用 `rl.Readline()` 替代 `scanner.Text()`
- [ ] 3.3: 实现补全器逻辑
  - [ ] 3.3.1: 实现 `Complete` 方法调用 `QueryInputCompletionsGlobal`
  - [ ] 3.3.2: 处理 Tab 键触发补全
  - [ ] 3.3.3: 处理单个匹配自动补全
  - [ ] 3.3.4: 处理多个匹配显示列表
- [ ] 3.4: 更新帮助信息，说明 Tab 补全功能

## Task 4: 集成测试与验证

验证整个补全功能的正确性。

- [ ] 4.1: 手动测试补全功能
  - [ ] 4.1.1: 启动 `owl ai` 交互模式
  - [ ] 4.1.2: 输入一些测试命令
  - [ ] 4.1.3: 再次输入前缀并按 Tab 测试补全
  - [ ] 4.1.4: 验证补全结果正确性
- [ ] 4.2: 测试边界条件
  - [ ] 4.2.1: 测试空前缀（应报错）
  - [ ] 4.2.2: 测试无匹配情况
  - [ ] 4.2.3: 测试单个匹配自动补全
  - [ ] 4.2.4: 测试多个匹配显示列表

## Task Dependencies

- [Task 2] 依赖于 [Task 1]（需要索引优化查询性能）
- [Task 3] 依赖于 [Task 2]（需要补全查询函数）
- [Task 4] 依赖于 [Task 3]（需要集成后的功能）

## Implementation Notes

### Task 1 详细说明

索引创建应该在数据库初始化时自动执行，确保每次启动都能验证索引存在。

### Task 2 详细说明

`QueryInputCompletions` 函数的核心 SQL：

```sql
SELECT input FROM aichat
WHERE role = 'user'
  AND input LIKE ? || '%'
  AND input IS NOT NULL
  AND input != ''
ORDER BY created_at DESC
LIMIT ?
```

### Task 3 详细说明

使用 readline 的 `AutoComplete` 接口：

```go
type Completer struct{}

func (c *Completer) Complete(word string) []string {
    completions, _ := QueryInputCompletionsGlobal(word, 10)
    return completions
}

cfg := &readline.Config{
    AutoComplete: c,
}
rl, _ := readline.NewEx(cfg)
```

### Task 4 详细说明

测试时注意清理测试数据，避免影响正式环境。
