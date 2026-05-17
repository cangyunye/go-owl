# owl file 命令详解

文件传输模块，支持在本地与节点之间传输文件。

---

## 1. 命令列表

```
owl file - 文件传输
├── owl file upload    - 上传文件到节点
├── owl file download - 从节点下载文件
└── owl file transfer - 节点间扩散传输
```

---

## 2. owl file upload

上传本地文件到远程节点。

### 使用方法

```bash
owl file upload <local-file> --nodes node1,node2
owl file upload app.tar.gz --dest /opt/app/
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<local-file>` | 本地文件路径（必填） |
| `--nodes` | 目标节点 ID（逗号分隔） |
| `--group` | 按分组选择节点 |
| `--label` | 按标签选择节点 |
| `--dest` | 远程目标目录，默认 /tmp |
| `--mode` | 文件权限，默认 0644 |
| `--parallel` | 并行上传，默认 true |
| `--overwrite` | 如果远程文件已存在则覆盖，默认 true |
| `--no-overwrite` | 如果远程文件已存在则跳过，不覆盖 |

### 文件已存在策略

| 选项 | 行为 |
|------|------|
| `--overwrite` (默认) | 覆盖已存在文件（SCP 默认行为） |
| `--no-overwrite` | 跳过已存在文件，返回错误或警告 |

### 示例

```bash
# 基本用法
owl file upload app.tar.gz --nodes web-01

# 上传到多个节点
owl file upload config.yaml --nodes web-01,web-02,web-03

# 按分组上传
owl file upload deploy.sh --group web --dest /opt/scripts/

# 指定目标目录
owl file upload app.tar.gz --nodes web-01 --dest /opt/app/

# 串行上传（不并行）
owl file upload large-file.dat --nodes node1,node2 --parallel=false

# 不覆盖已存在文件
owl file upload backup.tar.gz --nodes web-01 --no-overwrite
```

### 示例输出

```
📤 文件: app.tar.gz
📍 目标: /opt/app/
🎯 节点: 2 个
⚡ 模式: 并行上传
⚡ 覆盖: true

正在上传...
✅ [web-01] 成功: /opt/app/app.tar.gz
✅ [web-02] 成功: /opt/app/app.tar.gz

📊 总结: 2 成功, 0 失败
```

---

## 3. owl file download

从远程节点下载文件到本地。

### 使用方法

```bash
owl file download <remote-file> --node node1 --dest ./downloads/
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<remote-file>` | 远程文件路径（必填） |
| `--node` | 源节点 ID（单个） |
| `--nodes` | 源节点 ID（逗号分隔，仅第一个生效） |
| `--group` | 按分组选择（仅第一个生效） |
| `--label` | 按标签选择（仅第一个生效） |
| `--dest` | 本地目标目录，默认 . |
| `--name-format` | 多节点下载文件名格式，默认 `{name}.{node}` |
| `--subdir` | 多节点下载使用子目录组织 |

### 多节点下载命名策略

#### 方案 1：文件名后缀（默认）

从多个节点下载同名文件时，文件名会自动添加节点后缀：
- 原始文件: `app.log`
- 下载后: `app.log.web-01`, `app.log.web-02`, `app.log.web-03`

```bash
owl file download /var/log/app.log \
  --nodes web-01,web-02,web-03 \
  --dest ./logs/
```

#### 方案 2：子目录组织

使用 `--subdir` 参数，在子目录中保留原始文件名：
```
logs/
├── web-01/
│   └── app.log
├── web-02/
│   └── app.log
└── web-03/
    └── app.log
```

```bash
owl file download /var/log/app.log \
  --nodes web-01,web-02,web-03 \
  --dest ./logs/ \
  --subdir
```

#### 方案 3：自定义格式

使用 `--name-format` 参数自定义格式：
- 可用占位符: `{name}` (文件名), `{node}` (节点ID), `{ext}` (扩展名)
- 示例: `{node}_{name}` → `web-01_app.log`

```bash
owl file download /var/log/app.log \
  --nodes web-01,web-02 \
  --dest ./logs/ \
  --name-format "{node}_{name}"
```

### 示例

```bash
# 基本用法（单节点）
owl file download /var/log/app.log --node web-01 --dest ./logs/

# 下载到当前目录
owl file download /tmp/data.json --node web-01

# 按分组下载
owl file download /var/log/nginx/access.log --group web --dest ./logs/

# 多节点下载，使用子目录
owl file download /var/log/app.log \
  --nodes web-01,web-02,web-03 \
  --dest ./logs/ \
  --subdir
```

### 示例输出

```
📥 源文件: /var/log/app.log
📍 源节点: web-01, web-02, web-03
💾 保存到: ./logs/
📁 组织方式: 子目录

正在下载...
✅ [web-01] 成功: ./logs/web-01/app.log
✅ [web-02] 成功: ./logs/web-02/app.log
✅ [web-03] 成功: ./logs/web-03/app.log
```

---

## 4. owl file transfer

节点间扩散传输（P2P 模式），将文件从一个节点扩散到其他节点。

### 使用方法

```bash
owl file transfer <file> --source node1 --targets node2,node3,node4
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<file>` | 要传输的文件路径 |
| `--source` | 源节点 ID |
| `--targets` | 目标节点 ID（逗号分隔） |
| `--fan-out` | 同时连接数，默认 3 |
| `--threshold` | 阈值，低于此数量自动扩散 |

### 工作原理

```
1. 从源节点获取文件
2. 同时发送给 N 个节点（fan-out）
3. 已收到的节点继续扩散给其他节点
4. 直到所有节点都收到文件
```

### 示例

```bash
# 基本用法
owl file transfer /tmp/large-file.tar.gz \
  --source web-01 \
  --targets web-02,web-03,web-04,web-05

# 高并发扩散
owl file transfer /opt/app.tar.gz \
  --source web-01 \
  --targets web-02,web-03,web-04,web-05,web-06 \
  --fan-out 5

# 自动阈值扩散
owl file transfer /tmp/data.zip \
  --source web-01 \
  --targets node1,node2,node3,node4,node5 \
  --threshold 2
```

### 示例输出

```
📦 扩散传输
📁 文件: /tmp/large-file.tar.gz (500MB)
📍 源节点: web-01
🎯 目标: 5 个节点
⚡ Fan-out: 3

正在扩散...
[10:30:00] web-01 → web-02, web-03, web-04 ✓
[10:30:05] web-02 → web-05 ✓
[10:30:10] 扩散完成

📊 总结: 5 成功, 0 失败
总耗时: 10s
```

---

## 5. 测试用例

### TC-FILE-001: 单节点上传

```bash
# 步骤
$ echo "test content" > /tmp/test.txt
$ owl file upload /tmp/test.txt --nodes test-01 --dest /tmp/

# 预期结果
# ✅ [test-01] 成功: /tmp/test.txt
```

### TC-FILE-002: 多节点并行上传

```bash
# 步骤
$ owl file upload /tmp/test.txt --nodes test-01,test-02

# 预期结果
# ✅ [test-01] 成功
# ✅ [test-02] 成功
# 📊 总结: 2 成功, 0 失败
```

### TC-FILE-003: 分组上传

```bash
# 步骤
$ owl node groups add test-group --nodes test-01,test-02
$ owl file upload /tmp/test.txt --group test-group

# 预期结果
# ✅ [test-01] 成功
# ✅ [test-02] 成功
```

### TC-FILE-004: 单节点下载

```bash
# 步骤
$ owl file download /etc/hostname --node test-01 --dest /tmp/

# 预期结果
# ✅ 下载成功: /tmp/hostname
```

### TC-FILE-005: 上传覆盖已存在文件

```bash
# 步骤
$ echo "old content" > /tmp/test.txt
$ owl file upload /tmp/test.txt --nodes test-01 --dest /tmp/
$ echo "new content" > /tmp/test.txt
$ owl file upload /tmp/test.txt --nodes test-01 --dest /tmp/ --overwrite

# 预期结果
# ✅ 覆盖成功
```

### TC-FILE-006: 上传不覆盖已存在文件

```bash
# 步骤
$ echo "old content" > /tmp/test.txt
$ owl file upload /tmp/test.txt --nodes test-01 --dest /tmp/
$ echo "new content" > /tmp/test.txt
$ owl file upload /tmp/test.txt --nodes test-01 --dest /tmp/ --no-overwrite

# 预期结果
# ⚠️  [test-01] 跳过: 文件已存在
```

### TC-FILE-007: 多节点下载，后缀命名

```bash
# 步骤
$ owl file download /var/log/app.log \
  --nodes web-01,web-02 \
  --dest ./logs/

# 预期结果
# ./logs/app.log.web-01
# ./logs/app.log.web-02
```

### TC-FILE-008: 多节点下载，子目录组织

```bash
# 步骤
$ owl file download /var/log/app.log \
  --nodes web-01,web-02 \
  --dest ./logs/ \
  --subdir

# 预期结果
# ./logs/web-01/app.log
# ./logs/web-02/app.log
```

### TC-FILE-009: 文件不存在

```bash
# 步骤
$ owl file upload /nonexistent/file.txt --nodes test-01

# 预期结果
# 错误: 本地文件不存在: /nonexistent/file.txt
# exit code: 1
```

---

## 6. 常见问题

### Q: 上传大文件很慢？
A: 使用并行模式（默认），或增加 `--fan-out` 参数

### Q: 传输中断怎么办？
A: 重新执行命令，会覆盖已有文件

### Q: 如何保留文件权限？
A: 使用 `--mode` 参数指定权限，如 `--mode 0755`

### Q: 支持通配符吗？
A: 暂不支持，建议使用脚本预处理

### Q: 节点离线会怎样？
A: 跳过离线节点，最终显示失败统计

### Q: 多节点下载同名文件如何区分来源？
A: 使用自动后缀命名或子目录组织，详见"多节点下载命名策略"部分
