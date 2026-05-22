# owl playbook 命令详解

剧本管理模块，支持预定义、可复用的任务流程。

---

## 1. 命令列表

```
owl playbook - 剧本管理
├── owl playbook list     - 列出剧本
├── owl playbook run     - 执行剧本
├── owl playbook info    - 查看剧本详情
├── owl playbook validate - 验证剧本
└── owl playbook create  - 创建剧本（未来版本）
```

---

## 2. owl playbook list

列出所有可用的剧本。

### 使用方法

```bash
owl playbook list
owl playbook list --group web
owl playbook list --format json
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `--group` | 按分组筛选 |
| `--format` | 输出格式 |

### 示例输出

```
  名称          分组    步骤数  描述
 ─────────────────────────────────────────────────────────
  deploy-app    web     5      应用部署
  restart-nginx web     2      重启 Nginx
  backup-db     db      3      数据库备份
  health-check  common  4      健康检查
```

---

## 3. owl playbook run

执行剧本。

### 使用方法

```bash
owl playbook run <playbook-name>
owl playbook run <playbook-name> --nodes node1,node2
owl playbook run deploy-app --vars version=v1.2.0
```

### 参数说明

| 参数 | 说明 |
|------|------|
| `<playbook-name>` | 剧本名称（必填） |
| `--nodes` | 目标节点 |
| `--limit` | 限制执行的节点 |
| `--vars` | 传递变量 |
| `--tags` | 只执行指定标签的步骤 |
| `--check` | 检查模式（不实际执行） |

### 示例

```bash
# 执行剧本
owl playbook run deploy-app

# 指定节点
owl playbook run deploy-app --nodes web-01,web-02

# 传递变量
owl playbook run deploy-app --vars version=v1.2.0,env=prod

# 检查模式
owl playbook run deploy-app --check

# 只执行特定步骤
owl playbook run deploy-app --tags pre-deploy
```

### 示例输出

```
📜 剧本: deploy-app
🎯 节点: 2 个
📦 变量: version=v1.2.0, env=prod

正在执行...
[1/5] ✓ [web-01] 备份配置
[1/5] ✓ [web-02] 备份配置
[2/5] ✓ [web-01] 停止服务
[2/5] ✓ [web-02] 停止服务
[3/5] ✓ [web-01] 部署应用
[3/5] ✓ [web-02] 部署应用
[4/5] ✓ [web-01] 启动服务
[4/5] ✓ [web-02] 启动服务
[5/5] ✓ [web-01] 健康检查
[5/5] ✓ [web-02] 健康检查

📊 总结: 10/10 成功, 0 失败
总耗时: 45s
```

---

## 4. owl playbook info

查看剧本详细信息。

### 使用方法

```bash
owl playbook info <playbook-name>
```

### 示例输出

```
剧本: deploy-app
────────────────────────────────────────

描述: 完整的应用部署流程

变量:
  version  - 应用版本 (必填)
  env     - 环境名称 (默认: prod)

步骤:
  1. [pre-deploy] 备份配置
     命令: tar -czf /backup/app-$(date +%Y%m%d).tar.gz /opt/app/

  2. [pre-deploy] 停止服务
     命令: systemctl stop myapp

  3. [deploy] 部署应用
     命令: |
       curl -O http://repo/app-{{version}}.tar.gz
       tar -xzf app-{{version}}.tar.gz -C /opt/
       mv /opt/app-{{version}} /opt/app

  4. [post-deploy] 启动服务
     命令: systemctl start myapp

  5. [post-deploy] 健康检查
     命令: curl -f http://localhost:8080/health
```

---

## 5. owl playbook validate

验证剧本语法。

### 使用方法

```bash
owl playbook validate <playbook-file>
```

### 示例输出

```
✅ 剧本语法正确
✅ 变量定义完整
✅ 命令语法正确
```

---

## 6. 剧本格式

剧本使用 YAML 格式定义，支持多种动作类型：

```yaml
name: deploy-app
description: 应用部署流程
version: "1.0"

hosts: ["web-01", "web-02"]

vars:
  version: "1.0.0"
  env: "prod"

pre_tasks: []

tasks:
  - name: 包含基础设置
    action: include
    args:
      playbook: ./common/setup.yaml

  - name: 上传应用包
    action: upload
    args:
      src: ./dist/app-{{version}}.tar.gz
      dest: /opt/app/
      overwrite: true
      resume: true

  - name: 执行部署脚本
    action: script
    args:
      script: ./scripts/deploy.sh
      dest: /tmp/
      args: "--version {{version}}"

  - name: 执行安全检查脚本（不留文件）
    action: script
    args:
      script: ./scripts/security-check.sh
      inline: true

  - name: 解压并部署
    action: command
    args:
      cmd: |
        cd /opt/app
        tar -xzf app-{{version}}.tar.gz
        systemctl restart myapp

  - name: 下载日志文件
    action: download
    args:
      src: /var/log/myapp/app.log
      dest: ./logs/
      subdir: true
      name_format: "{node}-app.log"

post_tasks: []
```

### 支持的动作类型

| 动作类型 | 说明 | 参数 |
|---------|------|------|
| `command` / `cmd` / `shell` | 执行命令 | `cmd` 或 `command` - 要执行的命令 |
| `script` | 执行脚本文件 | `script` - 脚本文件路径（本地文件或 URL）<br>`dest` - 远程存放目录（默认 /tmp）<br>`args` - 传递给脚本的参数<br>`inline` - 是否直接发送内容执行（不留文件）<br>`keep` - 是否保留远程脚本文件 |
| `upload` | 上传文件到节点 | `src` - 本地源文件<br>`dest` - 远程目标路径<br>`overwrite` - 是否覆盖<br>`resume` - 是否断点续传 |
| `download` | 从节点下载文件 | `src` - 远程源文件<br>`dest` - 本地目标路径<br>`subdir` - 是否按节点创建子目录<br>`name_format` - 文件命名格式（支持 `{node}` 和 `{file}` 占位符） |
| `include` | 包含其他剧本 | `playbook` - 要包含的剧本文件路径（支持相对路径） |

### 变量插值

支持使用 `{{variable}}` 语法进行变量插值，例如：

```yaml
vars:
  version: "1.0.0"

tasks:
  - name: 上传应用
    action: upload
    args:
      src: ./dist/app-{{version}}.tar.gz
      dest: /opt/app/
```

### 模块化与包含

使用 `include` 动作可以实现剧本的模块化复用：

```yaml
# main.yaml
name: 完整部署
hosts: ["web-01"]

tasks:
  - name: 基础设置
    action: include
    args:
      playbook: ./common/setup.yaml

  - name: 应用部署
    action: include
    args:
      playbook: ./deploy/app.yaml

  - name: 健康检查
    action: include
    args:
      playbook: ./common/healthcheck.yaml
```

包含的剧本可以嵌套包含，但会检测循环包含防止死循环。

---

## 7. 测试用例

### TC-PLAY-001: 列出剧本

```bash
# 步骤
$ owl playbook list

# 预期结果
# 显示所有可用剧本
```

### TC-PLAY-002: 查看剧本信息

```bash
# 步骤
$ owl playbook info deploy-app

# 预期结果
# 显示剧本详细信息和步骤
```

### TC-PLAY-003: 执行剧本

```bash
# 步骤
$ owl playbook run health-check --nodes test-01

# 预期结果
# 按步骤执行健康检查
```

### TC-PLAY-004: 传递变量

```bash
# 步骤
$ owl playbook run deploy-app --vars version=v1.0.0 --nodes test-01

# 预期结果
# 使用变量值执行剧本
```

### TC-PLAY-005: 验证剧本

```bash
# 步骤
$ owl playbook validate ./my-playbook.yaml

# 预期结果
# ✅ 验证通过 或 显示错误
```

### TC-PLAY-006: 测试文件上传

```bash
# 步骤
$ owl playbook run upload-test --nodes test-01

# 预期结果
# 成功上传文件到目标节点
```

### TC-PLAY-007: 测试剧本包含

```bash
# 步骤
$ owl playbook run include-test --nodes test-01

# 预期结果
# 成功执行包含的剧本
```

---

## 8. 常见问题

### Q: 如何创建自定义剧本？
A: 在 `~/.owl/playbooks/` 目录下创建 YAML 文件

### Q: 支持变量插值吗？
A: 支持，使用 `{{variable}}` 语法

### Q: 步骤失败会怎样？
A: 默认停止执行，使用 `--continue-on-error` 继续

### Q: 如何重试失败的步骤？
A: 使用 `retry: N` 定义重试次数

### Q: 如何复用剧本片段？
A: 使用 `include` 动作包含其他剧本文件

### Q: include 的路径如何解析？
A: 相对于主剧本所在目录解析相对路径
