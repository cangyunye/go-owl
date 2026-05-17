# owl settings 命令详解

系统设置模块，用于配置和管理 owl 的各种选项。

---

## 1. 命令列表

```
owl settings - 系统设置
├── owl settings show   - 显示设置
├── owl settings set   - 设置配置
├── owl settings get   - 获取配置
└── owl settings target - 目标配置
```

---

## 2. owl settings show

显示当前所有配置。

### 使用方法

```bash
owl settings show
owl settings show --format json
```

### 示例输出

```
 Owl 设置
────────────────────────────────────────

AI 配置:
  Provider:    openai
  Model:      gpt-4o
  API Key:    已设置
  Base URL:   https://api.openai.com/v1
  Timeout:    120s

SSH 配置:
  默认端口:    22
  超时:       30s
  保持连接:   ✓

日志配置:
  级别:       info
  格式:       json
  输出:       stdout

数据库:
  类型:       sqlite3
  路径:       ~/.owl/owl.db
```

---

## 3. owl settings set

设置配置项。

### 使用方法

```bash
owl settings set <key> <value>
```

### 可配置项

| 配置项 | 说明 | 示例 |
|--------|------|------|
| `ai.provider` | AI Provider | openai, anthropic, dashscope |
| `ai.model` | AI 模型 | gpt-4o, claude-3.5-sonnet |
| `ai.api_key` | API Key | sk-xxx |
| `ai.base_url` | API Base URL | https://api.openai.com/v1 |
| `ai.temperature` | 温度参数 | 0.7 |
| `ai.max_tokens` | 最大 Token | 4096 |
| `ssh.port` | 默认 SSH 端口 | 22 |
| `ssh.timeout` | 连接超时 | 30s |
| `ssh.keep_alive` | 保持连接 | true/false |
| `log.level` | 日志级别 | debug, info, warn, error |
| `log.format` | 日志格式 | json, text |

### 示例

```bash
# 设置 AI Provider
owl settings set ai.provider openai

# 设置 AI 模型
owl settings set ai.model gpt-4o

# 设置 API Key
owl settings set ai.api_key "sk-xxx"

# 设置日志级别
owl settings set log.level debug

# 设置 SSH 超时
owl settings set ssh.timeout 60s
```

---

## 4. owl settings get

获取单个配置项的值。

### 使用方法

```bash
owl settings get ai.provider
owl settings get ssh.port
```

### 示例

```bash
$ owl settings get ai.provider
openai

$ owl settings get ssh.timeout
30s
```

---

## 5. owl settings target

目标节点配置管理。

### 使用方法

```bash
# 列出目标配置
owl settings target list

# 添加目标配置
owl settings target add <name> --nodes node1,node2

# 移除目标配置
owl settings target remove <name>
```

### 示例

```bash
# 创建目标配置
owl settings target add prod-web --nodes web-01,web-02 --label env=prod

# 使用目标配置
owl exec run "uptime" --target prod-web
owl file upload app.tar.gz --target prod-web

# 列出配置
owl settings target list
```

---

## 6. 配置文件

配置文件位于 `~/.owl/config.yaml`：

```yaml
ai:
  provider: openai
  model: gpt-4o
  api_key: ${OWL_API_KEY}
  base_url: https://api.openai.com/v1
  timeout: 120
  settings:
    temperature: 0.7
    max_tokens: 4096

ssh:
  port: 22
  timeout: 30s
  keep_alive: true
  strict_host_key_checking: false

log:
  level: info
  format: text
  output: stdout

database:
  type: sqlite3
  path: ~/.owl/owl.db

targets:
  prod-web:
    nodes: [web-01, web-02]
    labels:
      env: prod
  staging-db:
    nodes: [db-01]
    labels:
      env: staging
```

---

## 7. 测试用例

### TC-SETTINGS-001: 显示设置

```bash
# 步骤
$ owl settings show

# 预期结果
# 显示所有配置项
```

### TC-SETTINGS-002: 设置值

```bash
# 步骤
$ owl settings set log.level debug
$ owl settings get log.level

# 预期结果
# debug
```

### TC-SETTINGS-003: 目标配置

```bash
# 步骤
$ owl settings target add test-target --nodes test-01
$ owl settings target list

# 预期结果
# 显示 test-target 配置
```

---

## 8. 常见问题

### Q: 配置文件在哪里？
A: `~/.owl/config.yaml`

### Q: 如何重置所有设置？
A: 删除配置文件，owl 会使用默认配置

### Q: 环境变量优先级？
A: 环境变量 > 配置文件 > 默认值

### Q: 支持多目标配置吗？
A: 支持，使用 `--target` 参数指定
