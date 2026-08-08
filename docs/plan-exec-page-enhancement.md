# `/exec` 页面增强计划

## 目标

为 `owl-serve` 的 `/exec` 页面添加与 `owl exec run` CLI 命令对齐的全部功能，包括节点选择、异步执行、输出格式、调试模式、分组/标签筛选、并行/串行选择、重试配置等。

---

## 一、后端改造 (`handler/exec.go`)

### 1.1 扩展 `execRequest` 结构体

添加缺失字段以匹配 CLI flags：

```go
type execRequest struct {
    // 已有字段
    NodeID        string            `json:"node_id"`
    NodeIDs       []string          `json:"node_ids"`
    Command       string            `json:"command"`
    ScriptContent string            `json:"script_content"`
    ScriptName    string            `json:"script_name"`
    ScriptArgs    string            `json:"script_args"`
    Group         string            `json:"group"`
    Labels        map[string]string `json:"labels"`
    Status        string            `json:"status"`
    Force         string            `json:"force,omitempty"`

    // 新增字段
    Async               bool          `json:"async"`
    AsyncMaxPollCount   int           `json:"async_max_poll_count"`
    AsyncPollInterval   string        `json:"async_poll_interval"`
    AsyncRemoteDir      string        `json:"async_remote_dir"`
    AsyncTimeout        string        `json:"async_timeout"`
    Format              string        `json:"format"`
    Debug               bool          `json:"debug"`
    Parallel            bool          `json:"parallel"`
    Serial              bool          `json:"serial"`
    Retry               int           `json:"retry"`
    RetryInterval       string        `json:"retry_interval"`
    RetryMaxInterval    string        `json:"retry_max_interval"`
    NoRetry             bool          `json:"no_retry"`
    ConnectTimeout      string        `json:"connect_timeout"`
    CommandTimeout      string        `json:"command_timeout"`
    Timeout             string        `json:"timeout"`
    NoColor             bool          `json:"no_color"`
    Silent              bool          `json:"silent"`
}
```

### 1.2 修改 `Create` handler

- 将新字段从请求传递到执行上下文
- 当 `async=true` 时，不等待执行完成，返回 task ID 后异步执行
- 当 `parallel=false` 或 `serial=true` 时，串行执行节点
- 当 `retry>0` 时，在失败时自动重试
- 当 `format=detail/json` 时，按格式返回结构化输出
- 当 `debug=true` 时，在输出中包含详细步骤信息

### 1.3 创建 ExecConfig 上下文传递

构建一个内部 `ExecConfig` 结构体，在执行链路中传递：

```go
type ExecConfig struct {
    Command        string
    NodeIDs        []string
    Async          bool
    Format         string
    Debug          bool
    Parallel       bool
    Retry          int
    RetryInterval  time.Duration
    RetryMax       time.Duration
    Force          bool
    ConnectTimeout time.Duration
    CommandTimeout time.Duration
}
```

### 1.4 异步执行支持

- 在 `ExecHandler` 中维护一个异步任务管理器
- 当 `async=true` 时，启动后台 goroutine 轮询/监控
- 通过 WebSocket 或轮询 `/tasks/:id` 返回执行进度
- 支持 fire-and-forget (`async_poll_interval=0`)

### 1.5 重试逻辑

- 在 `executeTask` 中添加重试循环
- 指数退避：`retry_interval * 2^n`，上限 `retry_max_interval`

---

## 二、前端改造 (`web/js/pages/exec.js`)

### 2.1 新增 UI 控件

在 `exec.js` 的侧边栏 `<div class="exec-sidebar">` 中添加以下控件组：

#### A. 节点选择 (已有)
- 节点 chips（已有，保持）
- 状态筛选按钮：全部/在线/离线

#### B. 分组/标签筛选（新增）
- 展开式「筛选」卡片
- 分组下拉框 — 从 `/api/v1/nodes/filters` 获取分组列表
- 标签输入框（key=value 格式，支持多个）
- 显示当前启用的筛选标签（已有 group-tags）

#### C. 执行模式（新增卡片）
- 并行/串行 切换按钮组（2选1，视觉上不可用按钮和激活按钮）
- 异步执行 切换按钮（带图标，激活时显示异步相关参数）

#### D. 输出格式（新增）
- 下拉框：simple / detail / json
- 默认 simple

#### E. 调试模式（新增）
- 切换开关（checkbox 或 toggle button）
- 默认关闭

#### F. 重试参数（新增）
- 展开式「重试」折叠面板（adv-toggle 风格）
- 最大重试次数 (number input, 默认 3)
- 重试间隔 (number + 单位下拉 s/m, 默认 1s)
- 最大重试间隔 (number + 单位下拉 s/m, 默认 30s)
- 禁用重试 checkbox

#### G. 超时参数（已有，增强）
- 已有 timeout slider（保留）
- 新增：连接超时、命令执行超时 输入框

### 2.2 修改 `handleExec` 函数

将当前简单请求改为收集所有控件值，构造完整请求体：

```javascript
const payload = {
  node_ids: Array.from(selectedNodes),
  command: cmd,
  force: 'true',
  // 新增
  async: document.getElementById('async-toggle').checked,
  format: document.getElementById('format-select').value,
  debug: document.getElementById('debug-toggle').checked,
  parallel: document.getElementById('mode-parallel').checked,
  serial: document.getElementById('mode-serial').checked,
  retry: parseInt(document.getElementById('retry-count').value) || 3,
  retry_interval: document.getElementById('retry-interval').value + 's',
  retry_max_interval: document.getElementById('retry-max-interval').value + 's',
  no_retry: document.getElementById('no-retry').checked,
  // ... etc
};
```

### 2.3 WebSocket/轮询增强

- 对于 async 任务，添加「查看进度」按钮 → 轮询 `/api/v1/tasks/:id`
- 在终端输出区域显示任务 ID、状态

---

## 三、CSS 样式 (`web/css/app.css`)

### 3.1 新增组件样式

- `.exec-mode-group` — 并行/串行 按钮组
- `.exec-toggle` — 切换开关样式
- `.retry-options` — 重试参数折叠面板
- `.filter-selector` — 分组/标签筛选器
- `.format-selector` — 格式下拉框

### 3.2 增强布局

- 侧边栏从 280px 扩展到 300px 以容纳更多控件
- 优化折叠面板的动画和间距

---

## 四、实施顺序

| 步骤 | 内容 | 涉及文件 |
|------|------|---------|
| 1 | 扩展 `execRequest` 结构体，添加所有新字段 | `handler/exec.go` |
| 2 | 修改 `Create` handler，传递新参数到执行上下文 | `handler/exec.go` |
| 3 | 实现 `ExecConfig` 上下文，支持 async/retry/serial | `handler/exec.go`, `handler/ssh_executor.go` |
| 4 | 前端 exec.js：添加分组/标签筛选控件 | `web/js/pages/exec.js` |
| 5 | 前端 exec.js：添加并行/串行、异步、格式下拉 | `web/js/pages/exec.js` |
| 6 | 前端 exec.js：添加调试开关、重试参数面板 | `web/js/pages/exec.js` |
| 7 | 前端 exec.js：修改 handleExec 收集所有参数 | `web/js/pages/exec.js` |
| 8 | CSS：新增控件样式 | `web/css/app.css` |
| 9 | 测试：验证端到端功能 | 手动测试 |

---

## 五、注意事项

- 后端 `resolveNodeIDs()` 已经支持 group/label/status 解析，前端只需将筛选结果传递给 API
- 已有 `api.execAdvanced(data)` 直接传递 JSON，无需修改 API 层
- async 执行需要后端实现异步任务调度（当前 `executeTask` 是同步 goroutine）
- retry 逻辑可以直接在 `executeTask` 中实现循环
- 所有新字段均为可选，向后兼容
