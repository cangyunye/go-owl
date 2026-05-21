# owl settings 命令详解

系统设置模块，用于配置和管理 owl 的各种选项。

---

## 1. 命令列表

```
owl settings - 设置管理
├── owl settings show    - 显示当前设置
├── owl settings set    - 设置配置值
└── owl settings target - 默认目标配置
```

---

## 2. owl settings show

显示当前所有配置。

### 使用方法

```bash
owl settings show
```

### 示例输出

```
Current Settings:
=================

Server:
  Address: localhost:8080
  Timeout: 30s

Output:
  Format: table
  Color:  true

Diffusion:
  Fan-out:      3
  Max depth:    10
  Source count: 2

Defaults:
  Timeout: 60s
  Groups:  (none)
  Labels:  (none)
```

---

## 3. owl settings set

设置配置项的值。

### 使用方法

```bash
owl settings set <key> <value>
```

### 支持的配置项

| 配置项 | 说明 |
|--------|------|
| `server.address` | 服务器地址 |
| `server.timeout` | 超时时间 |
| `output.format` | 输出格式 (table, json, yaml) |
| `output.color` | 启用颜色 (true, false) |
| `diffusion.fan-out` | 扇出系数 |
| `diffusion.source-count` | 源节点数量 |
| `defaults.timeout` | 默认超时时间 |

### 示例

```bash
# 设置输出格式
owl settings set output.format json

# 设置颜色输出
owl settings set output.color false

# 设置扇出系数
owl settings set diffusion.fan-out 5
```

---

## 4. owl settings target

设置默认的目标节点选择条件。

### 使用方法

```bash
owl settings target --group web
owl settings target --label env=prod
owl settings target --nodes node1,node2
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--group` | 默认分组 |
| `--label` | 默认标签 |
| `--nodes` | 默认节点列表 |

### 示例

```bash
# 设置默认分组
owl settings target --group web

# 设置默认标签
owl settings target --label env=prod --label app=nginx

# 设置默认节点
owl settings target --nodes web-01,web-02
```

---

## 5. 测试用例

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
$ owl settings set output.format json
$ owl settings show

# 预期结果
# Output.Format 显示为 json
```

### TC-SETTINGS-003: 默认目标配置

```bash
# 步骤
$ owl settings target --group web

# 预期结果
# 显示 Group: web
```

---

## 6. 常见问题

### Q: 配置文件在哪里？
A: 默认配置在 `~/.owl/config.yaml`

### Q: 如何重置所有设置？
A: 删除配置文件，owl 会使用默认配置

### Q: 环境变量优先级？
A: 环境变量 > 配置文件 > 默认值

### Q: 设置会持久化吗？
A: 当前版本设置不持久化，重启后会重置
