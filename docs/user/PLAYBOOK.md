# owl playbook 命令详解

剧本管理模块，支持预定义、可复用的任务流程。

---

## 1. 命令列表

```
owl playbook - 剧本管理
├── owl playbook list     - 列出剧本
├── owl playbook run     - 执行剧本
├── owl playbook validate - 验证剧本
└── owl playbook template - 创建剧本
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

### 目标节点选择优先级

执行剧本时，目标节点的选择按以下优先级：

1. **命令行 `--nodes` 参数** - 最高优先级，指定具体的节点 ID
2. **命令行 `--group` 参数** - 按分组选择节点
3. **命令行 `--label` 参数** - 按标签选择节点
4. **剧本中的 `hosts` 配置** - 如果指定了 hosts，使用其中的节点
5. **所有可用节点** - 如果都没有指定，则对所有节点执行

> **注意**：`hosts` 可以为空数组或省略，此时会使用命令行参数或全部节点。

### 参数说明

| 参数 | 说明 |
|------|------|
| `<playbook-file>` | 剧本文件路径（必填） |
| `--nodes` | 目标节点 ID（逗号分隔） |
| `--group` | 按分组选择节点 |
| `--label` | 按标签选择节点 |
| `--vars` | 传递变量 |
| `--tags` | 只执行指定标签的步骤 |
| `--skip-tags` | 跳过指定标签的步骤 |
| `--check` | 检查模式（不实际执行） |
| `--default-connect-timeout` | SSH 连接超时（默认 10s） |
| `--default-command-timeout` | 命令执行超时（默认 5m） |
| `--default-retry` | 最大重试次数 |
| `--default-retry-interval` | 初始重试间隔（默认 1s） |
| `--default-retry-max-interval` | 最大重试间隔（默认 30s） |
| `--resume` | 从上次失败处断点续跑（仅 pipeline 模式） |

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

## 4. owl playbook validate

验证剧本语法。

### 使用方法

```bash
owl playbook validate <playbook-file>
owl playbook validate ./playbooks/*.yml
owl playbook validate a.yml b.yml
```

### 示例输出

```
  ✅ site.yml: 有效
  ✅ playbooks/app.yaml: 有效
  ❌ playbooks/bad.yaml: invalid execution_mode 'invalid_mode': must be 'pipeline' or 'fail_continue'
```

---

## 6. 剧本格式

剧本使用 YAML 格式定义，支持多种动作类型：

```yaml
name: deploy-app
description: 应用部署流程
version: "1.0"

# hosts 可以为空，默认为所有节点
# hosts: ["web-01", "web-02"]
hosts: []

# 执行模式: fail_continue(默认)/pipeline
# pipeline 模式下任一任务失败立即终止，且不允许 post_tasks
execution_mode: fail_continue

# 默认配置（可选，可被 CLI 参数覆盖）
# CLI 参数显式指定时完全替换默认值（不做并集）
default:
  groups: ["web"]        # 默认目标分组，可被 --group 覆盖
  tags: ["deploy"]       # 默认执行标签，可被 --tags 覆盖
  # skip_tags: ["debug"] # 默认跳过标签，可被 --skip-tags 覆盖
  # timeout:             # 默认超时，可被 --default-*-timeout 覆盖
  #   connect: 10s
  #   command: 5m
  # retry:                # 默认重试，可被 --default-retry-* 覆盖
  #   max: 3
  #   interval: 1s
  #   max_interval: 30s

vars:
  version: "1.0.0"
  env: "prod"

pre_tasks: []

tasks:
  - name: 包含基础设置
    action: include
    args:
      playbook: ./common/setup.yaml

  - name: 上传应用包（使用 PLAYBOOK_DIR 变量）
    action: upload
    args:
      src: "${PLAYBOOK_DIR}/dist/app-{{version}}.tar.gz"
      dest: /opt/app/
      overwrite: true
      resume: true

  - name: 上传脚本（dest 以 / 结尾会自动拼接文件名）
    action: upload
    args:
      src: ./scripts/deploy.sh
      dest: /tmp/
      overwrite: true

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

### 执行模式

Playbook 支持两种执行模式，通过 `execution_mode` 字段配置：

| 模式 | 名称 | 行为 | 适用场景 |
|------|------|------|---------|
| `fail_continue` | 失败继续模式（默认） | 所有任务依次执行，失败不阻断 | 批处理、监控检查 |
| `pipeline` | 流水线模式 | 任一任务失败立即终止后续任务 | 部署流程、依赖链任务 |

**pipeline 模式限制：**
- 不允许使用 `post_tasks`
- 不允许在任务上设置 `ignore_errors` 或 `any_errors_fatal`
- 任务如果被 `when` 条件跳过，不计为失败

**示例：**
```yaml
# pipeline 模式：部署流水线，任一环节失败即终止
execution_mode: pipeline
tasks:
  - name: 备份配置
    action: shell
    args:
      cmd: backup.sh
  - name: 部署应用
    action: shell
    args:
      cmd: deploy.sh
  - name: 启动服务
    action: shell
    args:
      cmd: start.sh
```

```yaml
# fail_continue 模式：监控检查，继续收集所有节点数据
execution_mode: fail_continue
tasks:
  - name: 检查节点1
    action: shell
    args:
      cmd: check.sh
  - name: 检查节点2
    action: shell
    args:
      cmd: check.sh
```

### 默认配置块（`default`）

Playbook 可以包含可选的 `default` 块，用于提供节点选择和任务过滤的默认值。CLI 参数显式指定时完全替换对应默认值（不做并集）：

```yaml
default:
  groups: ["web", "db"]    # 默认目标分组，可被 --group 覆盖。支持多个分组，节点自动去重
  tags: ["deploy"]          # 默认执行标签，可被 --tags 覆盖
  skip_tags: ["debug"]      # 默认跳过标签，可被 --skip-tags 覆盖
  timeout:                  # 默认超时配置，可被 --default-*-timeout 覆盖
    connect: 10s
    command: 5m
  retry:                    # 默认重试配置，可被 --default-retry-* 覆盖
    max: 3
    interval: 1s
    max_interval: 30s
```

**优先级（从高到低）：**

1. CLI 参数（`--group` / `--tags` / `--skip-tags` / `--default-*-timeout` / `--default-retry-*`）
2. YAML `default` 块
3. 程序内置默认值

**注意：** `groups` 支持指定多个分组，节点来自所有分组的并集，重复节点会自动去重。

### 支持的动作类型

| 动作类型 | 说明 | 参数 |
|---------|------|------|
| `command` / `cmd` / `shell` | 执行命令 | `cmd` 或 `command` - 要执行的命令 |
| `script` | 执行脚本文件 | `script` - 脚本文件路径（本地文件或 URL）<br>`dest` - 远程存放目录（默认 /tmp）<br>`args` - 传递给脚本的参数<br>`inline` - 是否直接发送内容执行（不留文件）<br>`keep` - 是否保留远程脚本文件 |
| `upload` | 上传文件到节点 | `src` - 本地源文件（支持相对路径）<br>`dest` - 远程目标路径<br>`overwrite` - 是否覆盖<br>`resume` - 是否断点续传<br>`**dest 以 / 结尾会自动拼接原文件名**` |
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

### 特殊变量

系统提供以下特殊变量：

| 变量 | 说明 | 示例 |
|------|------|------|
| `{{PLAYBOOK_DIR}}` 或 `${PLAYBOOK_DIR}` | 剧本文件所在目录 | `{{PLAYBOOK_DIR}}/scripts/deploy.sh` |
| `{{item}}` | 循环任务中的当前项 | `{{item}}` |

```yaml
tasks:
  # 使用 PLAYBOOK_DIR 引用剧本同目录下的文件
  - name: 上传脚本
    action: upload
    args:
      src: "{{PLAYBOOK_DIR}}/scripts/deploy.sh"
      dest: /tmp/

  # 使用相对路径（相对于剧本目录）
  - name: 上传配置
    action: upload
    args:
      src: ./config/app.conf
      dest: /etc/app/
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

### TC-PLAY-002: 执行剧本（无 hosts 配置）

```bash
# 步骤
$ owl playbook run deploy-app

# 预期结果
# 对所有可用节点执行剧本
```

### TC-PLAY-003: 执行剧本（指定节点）

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

### TC-PLAY-006: 测试文件上传（dest 以 / 结尾）

```bash
# 剧本配置
# - name: 上传脚本
#   action: upload
#   args:
#     src: ./scripts/deploy.sh
#     dest: /tmp/  # 会自动拼接为 /tmp/deploy.sh

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

### TC-PLAY-008: 使用 PLAYBOOK_DIR 变量

```bash
# 剧本配置
# - name: 上传脚本
#   action: upload
#   args:
#     src: "${PLAYBOOK_DIR}/scripts/deploy.sh"
#     dest: /tmp/

# 预期结果
# 使用剧本所在目录作为基准上传脚本
```

### TC-PLAY-009: 创建剧本模板

```bash
# 步骤
$ owl playbook template

# 预期结果
# 进入交互式创建剧本向导
```

---

## 8. 常见问题

### Q: 如何创建自定义剧本？
A: 使用 `owl playbook template` 进入交互式创建向导，或在 `~/.owl/playbooks/` 目录下创建 YAML 文件

### Q: 剧本的 hosts 可以为空吗？
A: 可以。如果 hosts 为空或省略，会使用命令行参数（`--nodes`、`--group`、`--label`）或对所有可用节点执行

### Q: 支持变量插值吗？
A: 支持，使用 `{{variable}}` 语法。还支持特殊变量 `{{PLAYBOOK_DIR}}` 或 `${PLAYBOOK_DIR}`

### Q: 上传文件时 dest 路径如何处理？
A: 
- 如果 `dest` 以 `/` 结尾，会自动拼接原文件名，例如 `dest: /tmp/` + `src: ./a.sh` = `/tmp/a.sh`
- 如果 `dest` 不是以 `/` 结尾，则视为完整的目标路径

### Q: relative paths are resolved relative to which directory?
A: All relative paths are resolved relative to the directory where the playbook file is located. You can also use `{{PLAYBOOK_DIR}}` or `${PLAYBOOK_DIR}` to explicitly reference the playbook's directory.

### Q: 任务失败时会怎样？
A: 取决于执行模式:
- `fail_continue`（默认）: 失败任务的后续任务仍会继续执行，最后汇总失败状态
- `pipeline`: 任一任务失败立即终止后续所有任务

可通过 `--resume` 从上次失败处断点续跑。

### Q: How to retry failed steps?
A: Use `retry: {max: N}` to define the maximum number of retries

### Q: How to reuse playbook fragments?
A: Use the `include` action to include other playbook files

### Q: How are include paths resolved?
A: Relative to the directory of the main playbook file
