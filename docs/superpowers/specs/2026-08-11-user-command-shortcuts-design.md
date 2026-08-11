# 用户级快捷命令设计

日期:2026-08-11
状态:已确认

## 目标

在「命令执行」页面为每个用户提供一组可自由管理(增/删/改/拖拽排序)的快捷命令。快捷命令横向展示在「命令/脚本」标签上方,点击即填入命令输入框。

新建用户时通过「用户创建触发层」自动追加 3 条默认快捷命令;已有用户不受影响。触发层为后续新功能(如默认分组、默认设置等)预留扩展点,无需再改 `UserHandler`。

## 需求要点

- 快捷命令为**用户级别**:每个用户只能看到和管理自己的。
- 每个新用户初始具备 3 条默认指令:`df -h`、`ps -fu $LOGNAME`、`free -h`(仅作为示例,可被用户修改/删除)。
- 新用户创建时自动追加默认指令;**已有用户不追加**。
- 用户可自由添加、删除、更新、拖拽排序自己的快捷命令。
- 点击快捷命令 chip → 切到「命令」模式并填入命令输入框,沿用现有执行流程(不直接执行)。
- 默认指令是**创建时快照**:未来默认集变更,老用户不补播(见 ADR-0001)。

## 领域术语(见 CONTEXT.md)

- **User (用户)**:Web 控制台账号,带角色,是快捷命令的所有者。
- **Shortcut Command (快捷命令)**:用户私有的命名命令模板,`name` + 一条 Command。
- **New-User Defaults (新用户默认指令)**:建号时一次播种、永不同步补播。

## 已确认的边界与规则(头脑风暴定稿)

- **仅命令模式**:快捷命令只填命令输入框(支持多行);脚本模式为非目标,暂不做「快捷脚本」。
- **归属与生命周期**:私有;删除用户级联删除其快捷命令,不转交、不留孤儿。
- **权限**:所有已登录用户(含 viewer)可管理自己的快捷命令;执行权限仍由 `/exec` 接口按 operator+ 把关。
- **名称语义**:允许重名(名称仅显示标签,不唯一);名称与命令都必填非空。
- **`$LOGNAME` 语义**:默认命令原样存储,由目标节点 shell 展开为节点的 ssh 登录用户,前端不替换。
- **数量上限**:不设硬上限(个人数据,自约束)。

## 数据模型

新表 `user_commands`:

```sql
CREATE TABLE IF NOT EXISTS user_commands (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES web_users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    command    TEXT NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_user_commands_user ON user_commands(user_id, position);
```

- `user_id` 外键关联 `web_users.id`,用户删除时级联删除其快捷命令。
- `position` 控制横向展示顺序,拖拽排序通过重写 position 实现。

## 后端

### Store:`store/command_store.go` 的 `CommandStore`

- `Init(ctx)` — 建表
- `ListByUser(ctx, userID)` — 按 position 升序返回
- `Create(ctx, cmd)` — 追加,position = 当前最大值 + 1
- `Update(ctx, cmd)` — 更新 name/command
- `Delete(ctx, id, userID)` — 仅允许删除自己的
- `Reorder(ctx, userID, orderedIDs)` — 按给定 id 顺序重写 position
- `CountByUser(ctx, userID)` — 用于 hook 幂等判断(可选)

### Handler:`handler/command.go` 的 `ShortcutHandler`

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/shortcuts` | 当前用户的快捷命令 |
| POST | `/api/v1/shortcuts` | 新增 |
| PUT | `/api/v1/shortcuts/:id` | 修改名称/命令 |
| DELETE | `/api/v1/shortcuts/:id` | 删除 |
| PUT | `/api/v1/shortcuts/reorder` | 批量保存排序 |

- 当前用户身份:从 JWT `claims.username` 经 `UserStore.FindByUsername` 取 `id`。不改 JWT Claims,避免已有 token 失效。
- 权限:所有已登录用户可管理自己的快捷命令(读者组,`RoleViewer`+)。命令执行本身仍受 `operator+` 限制不变。
- 所有写操作按 `user_id` 校验归属,用户 A 不能修改/删除用户 B 的快捷命令。

### 用户创建触发层(轻量 Hook 注册表)

```go
type UserCreatedHook func(ctx context.Context, userID int64) error
```

- `Server` 维护 `hooks []UserCreatedHook` 与 `RegisterUserCreatedHook(hook)`。
- 在 `UserHandler.Create` 成功创建用户后逐个调用。
- 首次运行的 `ensureAdmin` 创建管理员后也调用,保证全新安装的管理员同样有默认指令;已有安装不受影响。
- 快捷命令注册一个 hook:为新用户插入 3 条默认指令。
- 后续新功能只需再注册 hook,不再改 `UserHandler`。
- Hook 执行失败仅记日志,不阻塞用户创建(快捷命令是可恢复的个人数据,不因默认数据写入失败而让建号失败)。

## 默认快捷命令(代码常量)

| name | command |
|---|---|
| 磁盘占用 | `df -h` |
| 我的进程 | `ps -fu $LOGNAME` |
| 内存 | `free -h` |

`$LOGNAME` 原样存储,由目标节点远端 shell 展开。

## 前端(`web/js/pages/exec.js`)

- 在「命令/脚本」seg 上方渲染横向快捷命令条(新增容器,不改现有 `.editor-header` 结构)。
- 加载:`renderExec` 初始化时 `GET /api/v1/shortcuts`。
- 点击 chip:`switchExecMode('command')` + 填入 `#cmd-input`,触发 `updateExecButton()`。
- 管理:
  - chip 悬停显示「编辑/删除」。
  - 条末尾「+」按钮打开新增弹窗(名称 + 命令)。
  - 编辑弹窗复用同一表单。
  - **拖拽排序**:HTML5 drag & drop 或 Pointer Events 实现 chip 拖动,松手后以新顺序调用 `PUT /api/v1/shortcuts/reorder`。
- 命令模式与脚本模式互不干扰(填入仅影响命令输入框)。
- 本地无状态刷新:增删改排序后重新拉取列表重绘。

## 测试

- `command_store` 单测:CRUD、position 追加、拖拽排序重写、删除用户级联删除、跨用户隔离。
- `ShortcutHandler` 单测:API CRUD + reorder、未登录 401、用户 A 操作用户 B 数据 403/404。
- Hook 测试:新建用户自动生成 3 条默认;已有用户不追加;hook 失败不阻塞建号。
- 前端静态断言(filesjs_test.go 风格):exec.js 包含拖拽排序、点击填入、新增/编辑/删除相关标记。

## 非目标(本次不做)

- 管理员级全局快捷命令/模板。
- 拖拽外的复杂排序方式(如置顶)。
- 默认指令管理员可配置化(先硬编码常量,YAGNI)。
- 脚本快捷方式(仅命令)。
