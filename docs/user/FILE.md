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
| `--resume` | 启用断点续传（rsync 优先），默认 true |

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
🔄 断点续传: 已启用

正在上传...
[node1] rsync 可用，将使用断点续传
✅ [node1] 成功 [rsync, 12.5 MB/s]: /opt/app/app.tar.gz
[node2] 节点使用密码认证，改用 SSH 原生传输
✅ [node2] 成功 [scp]: /opt/app/app.tar.gz

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

节点间扩散传输，将文件从控制节点分批发送到大量目标节点。

### 使用方法

```bash
owl file transfer <local-file> --nodes node1,node2,node3,node4,node5
owl file transfer app.tar.gz --nodes n1,n2,n3,n4,n5 --source-count 2 --fan-out 3
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<file>` | 本地文件路径（必填） |
| `--nodes` | 目标节点 ID（逗号分隔） |
| `--all-nodes` | 选择所有注册节点 |
| `--group` | 按分组选择节点 |
| `--label` | 按标签选择节点 |
| `--dest` | 目标目录，默认 `/tmp` |
| `--source-count` | 源节点数量（前 N 个节点作为源），默认 2 |
| `--fan-out` | 扇出系数（每个源节点最多传给几个子节点），默认 3 |
| `--threshold` | 扩散阈值（低于此数量直接传输），默认 5 |

### 工作原理

```
第一步：扩散树规划
  控制节点将目标节点构建为扩散树，决定传输顺序

第二步：首批发
  控制节点直接 SCP 文件到前几个节点（首批源节点）

第三步：节点分流和接力
  - 密码认证节点 → 由已完成源节点通过 owl-relay.sh 脚本 SCP 接力
  - 密钥认证节点 → 由控制节点直接 SCP（私钥安全考虑）
  - 已完成源节点继续接收新子任务

第四步：全部完成汇总
  打印成功/失败/超时统计
```

### 示例

```bash
# 指定节点列表，扩散传输
owl file transfer app.tar.gz \
  --nodes node1,node2,node3,node4,node5 \
  --dest /opt/app/ --source-count 2

# 发送到所有注册节点
owl file transfer data.zip --all-nodes --dest /data/ --fan-out 3

# 按分组扩散
owl file transfer db.tar.gz --group database --source-count 1

# 少量节点自动走直接传输
owl file transfer app.tar.gz --nodes node1,node2 --dest /opt/app/
```

### 示例输出

```
文件: app.tar.gz (128.00 MB)
目标: /opt/app/app.tar.gz
节点: 5 个
模式: 扩散传输 (fan-out=3, threshold=5)

扩散树结构:
========================
源节点: Node 1, Node 2
  Node 1 -> Node 4, Node 5
  Node 2 -> Node 3

正在传输...
  [node1] rsync 可用，将使用断点续传
  [node1] 成功 [rsync, 12.5 MB/s]
  [node2] 成功 [scp, 10.2 MB/s]
  [node3] 成功 [scp, 11.1 MB/s]
  进度: [========----------] 60% (3/5)
  正在部署中继脚本到 [node1]...
  [node1] 正在向 node4, node5 中继传输...
  [node4] 成功 [relay, 980ms]
  [node5] 成功 [relay, 1250ms]
  进度: [===================] 100% (5/5)

总结: 5 成功, 0 失败
```

中继失败降级示例输出：

```
  [node1] 正在向 node4, node5 中继传输...
  警告: [node1] 中继部分失败: 1/2 个目标失败 (node4)
  [node4] 失败 [relay]: Permission denied, 降级为直接传输
  [node5] 成功 [relay, 1250ms]
  进度: [===============----] 80% (4/5)
  [node1] 1 个节点中继失败，正降级为直接传输: [node4]
  [node4] 降级直传成功 [scp]
  进度: [===================] 100% (5/5)

总结: 5 成功, 0 失败
```

部署脚本失败降级示例输出：

```
  正在部署中继脚本到 [node1]...
  警告: 部署中继脚本到 [node1] 失败: ... , 降级为直接传输
  [node4] 部署失败→降级直传→成功 [scp]
  [node5] 部署失败→降级直传→成功 [scp]
  进度: [===================] 100% (5/5)

总结: 5 成功, 0 失败
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

### Q: 为什么先看到 "rsync 可用" 又看到 "改用 SSH 原生传输"？
A: 如果您配置了密码认证的节点，`CheckRsyncAvailable()` 先检查远程节点是否安装了 rsync（此时不知道认证方式），发现安装了就打印"rsync 可用"。随后 `smartUpload()` 检查到节点使用密码认证，因 rsync CLI 不支持非交互式密码传递，自动切换为 SCP。
从 v2 开始，这一现象已修复——rsync 可用消息仅在**真正使用 rsync** 时打印，密码节点看到的是单一消息"节点使用密码认证，改用 SSH 原生传输"。

### Q: 为什么密码节点不能用 rsync？
A: rsync 的 `--rsh=ssh` 参数只接受 SSH 密钥文件（`-i` 参数），没有接口传递密码。而 SCP 降级使用的是 Go 的 `crypto/ssh` 库，原生支持密码认证。如需 rsync 断点续传，建议用密钥认证配置节点。

### Q: 出现 "中继传输失败" 的警告，但最终统计显示成功？
A: 这是 v2 之前的已知问题。当 `owl-relay.sh` 部分成功（exit=1）时，CSV 结果中既有成功的也有失败的目标，但旧版 `ExecuteRelay()` 在退出码非零时直接丢弃全部结果，并将所有目标降级直传（包括已成功的）。从 v2 开始，`ExecuteRelay()` 先解析 CSV 结果，仅对中继失败的目标降级，成功目标保留中继结果。

### Q: 中继失败后降级为直接传输，最终用的是 relay 还是 scp？
A: 看每个节点的结果行：
- `[node4] 成功 [relay, 980ms]` → 通过 relay 中继成功
- `[node4] 降级直传成功 [scp]` → relay 失败后由控制节点直传成功
- `[node4] 无中继源→降级直传→成功 [scp]` → 无可用中继源，直接由控制节点传
最终总结的计数与方法说明一致。

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
