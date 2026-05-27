> ⚠️ **状态：未实现 — 设计提案**。本文档描述的模板系统（`owl playbook templates` 命令等）尚未在代码中实现，当前 playbook 命令仅支持 `list`、`validate`、`info`、`run` 四个子命令。

# Playbook 模板系统设计方案

## 1. 概述

Playbook 模板系统旨在降低用户编写 Playbook 的门槛，提供丰富的预置模板和便捷的创建工具，帮助用户快速生成符合最佳实践的 Playbook。

### 1.1 核心理念：流水线式执行

不同于 Ansible 式的声明式状态收敛，Owl Playbook 采用**流水线式步骤执行**模型：

- 每个 task 是一个不可分割的步骤（step），按序执行
- 每个 task 执行后产生明确的状态（pending → running → completed / failed）
- 状态**按节点维度**独立追踪——节点 web-01 在第 3 步失败了，不影响 web-02 继续执行
- 失败的任务支持**断点续跑**：用户调查问题后，可以从上次失败的步骤继续，无需重新执行已完成的步骤
- 不提供自动回滚——回滚策略取决于具体业务场景，由用户根据流水线状态人工决策

### 1.2 设计目标

- **降低学习成本**：通过模板引导用户了解 Playbook 结构
- **提高效率**：快速生成常用场景的 Playbook
- **可扩展性**：支持用户自定义模板
- **最佳实践**：内置模板遵循标准化流程
- **安全可控**：dry-run 预览、按节点限流、超时保护

### 1.3 设计原则

- **分层设计**：用户模板优先于系统内置模板
- **零配置优先**：提供合理的默认参数
- **渐进式复杂度**：从简单场景开始，逐步深入
- **向后兼容**：不影响现有 Playbook 功能
- **安全第一**：无 dry-run 不执行，无预览不上生产
- **状态可查**：每个步骤、每个节点的执行状态可追踪、可恢复

---

## 2. 流水线执行模型

### 2.1 任务状态机

每个 task 在生命周期中经历以下状态转换：

```
                    ┌─────────────────────┐
                    │      pending         │  ← 初始状态
                    └─────────┬───────────┘
                              │ 开始执行
                              ▼
                    ┌─────────────────────┐
                    │      running         │
                    └────┬───────────┬────┘
                         │           │ 执行出错
                         │ 成功       ▼
                         │  ┌─────────────────────┐
                         │  │      failed          │
                         │  └──────────┬──────────┘
                         │             │ 用户修复后重试
                         │             ▼
                         │  ┌─────────────────────┐
                         │  │      running         │ ← 从失败步骤继续
                         │  └──────────┬──────────┘
                         │             │ 成功
                         ▼             ▼
                    ┌─────────────────────┐
                    │    completed         │  ← 终态
                    └─────────────────────┘
```

**状态定义**：

| 状态 | 含义 | 可转换到 |
|------|------|---------|
| `pending` | 尚未开始执行 | `running` |
| `running` | 正在执行中 | `completed`, `failed` |
| `completed` | 执行成功（终态） | — |
| `failed` | 执行失败 | `running`（用户重试） |
| `skipped` | 被跳过（条件不满足时） | — |

### 2.2 按节点追踪

状态是**按节点维度**独立存储的。每个节点拥有自己的状态快照：

```
节点: web-01                    节点: web-02
┌──────────────────────┐       ┌──────────────────────┐
│ Step 1: 创建工作目录  │       │ Step 1: 创建工作目录  │
│   status: completed   │       │   status: completed   │
│ Step 2: 上传压缩包    │       │ Step 2: 上传压缩包    │
│   status: completed   │       │   status: completed   │
│ Step 3: 解压安装      │       │ Step 3: 解压安装      │
│   status: failed  ←── │       │   status: completed   │
│ Step 4: 上传配置      │       │ Step 4: 上传配置      │
│   status: pending     │       │   status: running ──→ │
└──────────────────────┘       └──────────────────────┘
```

### 2.3 状态存储

执行状态通过项目已有的 `~/.owl/owl.db`（SQLite3/DuckDB 双引擎）持久化，新增两张表：

```
数据库: ~/.owl/owl.db

已有表（审计日志）:
  └── operations, command_executions, node_communications, file_transfers, ...

新增表（流水线状态）:
  ├── playbook_runs
  │    记录每次 playbook run 的整体执行信息
  │
  └── playbook_step_states
       记录每个节点、每个步骤的执行状态
```

#### playbook_runs 表

```sql
CREATE TABLE playbook_runs (
    id            TEXT PRIMARY KEY,         -- UUID
    playbook_name TEXT NOT NULL,            -- 剧本名称
    playbook_hash TEXT NOT NULL,            -- 剧本内容 SHA256
    nodes         TEXT NOT NULL,            -- 目标节点列表 (JSON array)
    status        TEXT NOT NULL DEFAULT 'running',  -- running/completed/failed
    started_at    DATETIME NOT NULL,
    finished_at   DATETIME,
    total_steps   INTEGER NOT NULL,         -- 总步骤数
    completed_steps INTEGER DEFAULT 0,      -- 已完成步骤数
    failed_steps  INTEGER DEFAULT 0         -- 失败步骤数
);
```

#### playbook_step_states 表

```sql
CREATE TABLE playbook_step_states (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id        TEXT NOT NULL,            -- 关联 playbook_runs.id
    node_id       TEXT NOT NULL,            -- 节点 ID
    step_index    INTEGER NOT NULL,         -- 步骤序号 (0-based)
    step_name     TEXT NOT NULL,            -- 步骤名称
    action        TEXT NOT NULL,            -- action 类型
    status        TEXT NOT NULL DEFAULT 'pending',  -- pending/running/completed/failed/skipped
    started_at    DATETIME,
    finished_at   DATETIME,
    duration_ms   INTEGER,
    exit_code     INTEGER,
    stdout        TEXT,                     -- 截断存储（首尾各 4KB）
    stderr        TEXT,
    error         TEXT,                     -- 错误信息
    retry_count   INTEGER DEFAULT 0,
    UNIQUE(run_id, node_id, step_index)    -- 同一 run 同节点同步聚唯一
);
CREATE INDEX idx_step_states_run ON playbook_step_states(run_id);
CREATE INDEX idx_step_states_node ON playbook_step_states(run_id, node_id);
```

#### 状态查询命令

用户无需手动进入配置目录翻找文件，通过以下命令查询状态：

```bash
# 查看某次执行的整体状态
owl playbook state list [--playbook <name>] [--status failed|running]

# 查看某次执行的详细步骤状态
owl playbook state show <run-id> [--node <node>]

# 只显示未完成的步骤（failed + pending）
owl playbook state show <run-id> --status incomplete
```

**输出示例**：

```bash
owl playbook state list
# 输出：
运行历史:
┌──────────────────┬─────────────────┬───────┬───────────┬──────────┬──────────────┐
│ RUN ID           │ 剧本            │ 节点  │ 进度      │ 状态     │ 开始时间     │
├──────────────────┼─────────────────┼───────┼───────────┼──────────┼──────────────┤
│ a1b2c3d4         │ nginx-deploy    │ 2     │ 5/7 (71%) │ failed   │ 05-25 10:00 │
│ e5f6g7h8         │ healthcheck     │ 3     │ 3/3 (100%)│ completed│ 05-25 09:30 │
│ i9j0k1l2         │ backup-files    │ 1     │ 2/4 (50%) │ running  │ 05-25 09:00 │
└──────────────────┴─────────────────┴───────┴───────────┴──────────┴──────────────┘
```

```bash
# 查看失败执行的未完成步骤
owl playbook state show a1b2c3d4 --status incomplete
# 输出：
Run: a1b2c3d4 | 剧本: nginx-deploy
────────────────────────────────────────
节点 web-01:
  ✓ 步骤 1 — 创建工作目录 [completed]
  ✓ 步骤 2 — 上传压缩包   [completed]
  ✗ 步骤 3 — 解压安装     [failed]   exit=1  error="missing libssl"
  ○ 步骤 4 — 上传配置     [pending]
  ○ 步骤 5 — 启动 Nginx   [pending]

节点 web-02:
  ✓ 全部完成 (7/7)
────────────────────────────────────────
💡 执行续跑: owl playbook run deploy.yaml --nodes web-01 --resume
```

### 2.4 断点续跑

```bash
# 首次执行——web-01 在第 3 步失败
owl playbook run deploy.yaml --nodes web-01,web-02
# [1/5] web-01: 创建工作目录      ✓
# [2/5] web-01: 上传压缩包        ✓
# [3/5] web-01: 解压安装          ✗ (exit code 1)
# ...web-02 继续独立执行...

# 用户调查并修复问题后，从失败处继续
owl playbook run deploy.yaml --nodes web-01 --resume
# [3/5] web-01: 解压安装          ✓ (续跑，跳过已完成步骤)
# [4/5] web-01: 上传配置          ✓
# [5/5] web-01: 验证安装          ✓
```

| 选项 | 行为 |
|------|------|
| `--resume` | 从第一个失败或 pending 的步骤继续 |
| `--from-step=N` | 从指定步骤编号开始（含） |
| `--reset` | 清除所有状态，从头执行 |

---

## 3. 模板存放结构

### 3.1 目录结构

```
~/.owl/
├── templates/                          # 用户自定义模板（优先级高）
│   └── README.md                       # 模板编写指南
│   └── webserver/
│       └── nginx.yaml
│   └── application/
│       └── my-app.yaml
│   └── custom/                         # 用户自定义分组
│       └── my-template.yaml
│
├── builtin-templates/                  # 系统内置模板（只读）
│   └── webserver/
│       └── nginx/
│           └── deploy.yaml
│       └── apache/
│           └── deploy.yaml
│   └── application/
│       └── nodejs/
│           └── deploy.yaml
│       └── docker/
│           └── container.yaml
│   └── utility/
│       └── backup/
│           └── files.yaml
│       └── healthcheck/
│           └── http.yaml
│
└── playbooks/                          # 用户创建的剧本
    └── my-deploy.yaml
```

### 3.2 模板加载优先级

1. **`~/.owl/templates/`** - 用户自定义模板（优先级最高）
2. **`~/.owl/builtin-templates/`** - 系统内置模板
3. **编译时内置** - 作为备选的默认模板

### 3.3 模板目录初始化

首次使用时自动创建模板目录结构：

```bash
# 初始化用户模板目录
~/.owl/
└── templates/
    └── README.md    # 包含模板编写指南
```

---

## 4. 模板结构规范

### 4.1 模板元数据

模板的元数据应保持精简，通过**目录结构**和**文件名**表达分类和名称：

```
builtin-templates/
└── webserver/
    └── nginx/
        └── deploy.yaml     # 分类=webserver, 名称=nginx/deploy
```

元数据仅包含必要的描述性信息：

```yaml
description: Nginx 部署模板，支持一键部署和配置管理
tags: [nginx, deploy]

parameters:
  - name: nginx_version
    description: "Nginx 版本号"
    default: "1.24.0"
    required: false

  - name: nginx_port
    description: "HTTP 监听端口"
    default: 80
    required: false
    type: number
```

**设计原则**：
- **`name`**：由文件路径表达，如 `nginx/deploy.yaml` 表示名称为 `nginx/deploy`
- **`category`**：由父目录表达，如 `webserver/nginx/deploy.yaml` 分类为 `webserver`
- **`author`、`version`**：大多数模板是通用模板，不需要版本追踪，移除
- **`hosts`**：属于执行层面配置，模板中不应写死，用户在 `run` 或 `new` 时通过 `--nodes` 指定

### 4.2 参数定义规范

模板参数定义的是**参数的结构约束**，而非具体值。参数分为两类：

| 类别 | 含义 | 来源 |
|------|------|------|
| **业务参数** | 模板自身的可变参数（如版本号、端口） | 模板定义 `parameters` |
| **执行参数** | 运行时的配置（如目标节点、并发数） | 用户通过 `--nodes` / `--serial` 等CLI选项提供 |

业务参数的规范：

```yaml
parameters:
  - name: <参数名>                    # 必填，参数标识符，用于 {{ 模板变量 }}
    description: "<描述>"              # 必填，参数用途说明
    type: <string|number|boolean>     # 可选，默认为 string
    required: <true|false>            # 可选，默认为 false
    default: <默认值>                  # 可选，无默认值时该参数为必填
    options: [<选项列表>]              # 可选，限制可选值
    pattern: "<正则表达式>"            # 可选，参数验证正则
```

**注意**：
- `parameters` 中定义的 `default` 是**提示性默认值**，用于交互式创建时的预填充
- 实际参数值通过 `--var` 或交互式输入提供
- 参数值在 **Playbook 实例化时**被绑定，而非模板定义时
- 主机/节点信息（`hosts`）、超时（`timeout`）、重试（`retries`）等属于执行参数，不走 `parameters`

### 4.3 完整模板示例

文件路径：`builtin-templates/webserver/nginx/deploy.yaml`

```yaml
description: |
  Nginx 部署模板，支持一键部署和配置管理。
  功能包括：
  - 自动下载和安装 Nginx
  - 配置文件上传
  - 服务启动和健康检查
tags: [nginx, webserver, deploy]

parameters:
  - name: nginx_version
    description: "Nginx 版本"
    default: "1.24.0"
    required: false

  - name: nginx_port
    description: "监听端口"
    default: 80
    required: false
    type: number

  - name: enable_ssl
    description: "启用 HTTPS"
    default: false
    required: false
    type: boolean

vars:
  nginx_version: "{{ nginx_version }}"
  nginx_port: "{{ nginx_port }}"
  enable_ssl: "{{ enable_ssl }}"

tasks:
  - name: 创建工作目录
    action: command
    args:
      cmd: mkdir -p /tmp/nginx-install

  - name: 上传 Nginx 压缩包
    action: upload
    args:
      src: ./files/nginx-{{ nginx_version }}.tar.gz
      dest: /tmp/nginx-install/
      overwrite: true
    timeout: 600
    retries: 2

  - name: 解压安装
    action: command
    args:
      cmd: |
        cd /tmp/nginx-install
        tar -xzf nginx-{{ nginx_version }}.tar.gz
        cd nginx-{{ nginx_version }}
        ./configure --prefix=/usr/local/nginx \
                    --with-http_ssl_module \
                    --with-http_gzip_static_module
        make -j$(nproc)
        make install
    timeout: 300

  - name: 上传配置文件
    action: upload
    args:
      src: ./files/nginx.conf
      dest: /usr/local/nginx/conf/nginx.conf
      overwrite: true
      backup: true

  - name: 启动 Nginx
    action: command
    args:
      cmd: /usr/local/nginx/sbin/nginx

  - name: 验证安装
    action: command
    args:
      cmd: curl -s -o /dev/null -w "%{http_code}" http://localhost:{{ nginx_port }}/
    retries: 3
    retry_delay: 5

  - name: 下载日志文件
    action: download
    args:
      src: /usr/local/nginx/logs/access.log
      dest: ./logs/
      subdir: true
      name_format: "{node}-nginx-access.log"

  - name: 清理临时文件
    action: command
    args:
      cmd: rm -rf /tmp/nginx-install
```

**关键变化**：
- 移除 `hosts` 字段——节点信息由 `run` 命令的 `--host` 选项提供
- 移除 `pre_tasks` / `post_tasks`——所有步骤统一为 `tasks`，按序执行
- 增加 `timeout` / `retries` / `retry_delay` 等执行控制字段
- 健康检查合并到 `tasks` 中，通过 `retries` 实现重试

---

## 5. Dry-Run 机制

### 5.1 概述

Dry-run（空运行）是在不实际执行远程命令的前提下，预览 Playbook 将在哪些节点上执行哪些操作。这是安全性的核心保障——**无预览，不执行**。

### 5.2 工作方式

```bash
# 对所有目标节点执行 dry-run
owl playbook run deploy.yaml --nodes web-01,web-02 --dry-run

# 输出示例
🔍 DRY-RUN 模式 —— 不会执行任何实际操作

目标节点: web-01, web-02
Playbook:  deploy.yaml
模板:     webserver/nginx/deploy

─────────────────────────────────────────────
📋 将执行以下步骤:

步骤 1/7 — 创建工作目录
  ├─ action: command
  ├─ cmd:    mkdir -p /tmp/nginx-install
  └─ 节点:   web-01, web-02

步骤 2/7 — 上传 Nginx 压缩包
  ├─ action:  upload
  ├─ src:     ./files/nginx-1.24.0.tar.gz
  ├─ dest:    /tmp/nginx-install/
  └─ 节点:    web-01, web-02

步骤 3/7 — 解压安装
  ├─ action:  command
  ├─ timeout: 300s
  ├─ cmd:     cd /tmp/nginx-install && tar -xzf ... && make install
  └─ 节点:    web-01, web-02

步骤 4/7 — 上传配置文件
  ├─ action:  upload
  ├─ src:     ./files/nginx.conf
  ├─ dest:    /usr/local/nginx/conf/nginx.conf
  └─ 节点:    web-01, web-02

─────────────────────────────────────────────
✅ Dry-run 完成。共 7 个步骤，影响 2 个节点。
💡 确认无误后，使用以下命令执行:
   owl playbook run deploy.yaml --nodes web-01,web-02
```

### 5.3 实现方式

Dry-run 通过以下机制实现：

| 机制 | 说明 |
|------|------|
| 参数展开 | 模板变量替换为默认值或 `--var` 提供的值 |
| 节点解析 | 展开 `--host` 参数中的主机列表 |
| 步骤展开 | 遍历所有 `tasks`，解析条件判断（`when`） |
| 零副作用 | 不建立 SSH 连接，不执行任何远程命令 |
| 差异标记 | 对比已有状态记录，标记哪些步骤已 `completed` |

### 5.4 与状态的交互

当存在历史执行状态时，dry-run 会显示各节点的当前进度：

```bash
owl playbook run deploy.yaml --nodes web-01,web-02 --dry-run

# 如果 web-01 上次在第 3 步失败：
🔍 DRY-RUN 模式

节点 web-01 (状态: 上次在第 3 步失败)
  ✓ 步骤 1 — 创建工作目录 [completed]
  ✓ 步骤 2 — 上传压缩包   [completed]
  ⚡步骤 3 — 解压安装      [pending → 将从这里执行]
  ○ 步骤 4 — 上传配置文件  [pending]
  ...

节点 web-02 (状态: 全新)
  ○ 步骤 1 — 创建工作目录 [pending]
  ...
```

### 5.5 安全约束

| 规则 | 说明 |
|------|------|
| 生产环境强制确认 | 指定 `--env production` 时，必须先用 `--dry-run` 预览，否则拒绝执行 |
| 节点数量限制 | `--dry-run` 显示受影响节点数，超过 `--max-nodes`（默认 10）时警告 |
| 变更检测 | 对比上次成功的 dry-run 摘要，若步骤或参数变化则高亮差异 |

---

## 6. Action 类型

### 6.1 Action 类型列表

| Action 类型 | 说明 | 主要参数 |
|------------|------|---------|
| `command` / `cmd` / `shell` | 执行 Shell 命令 | `cmd`: 命令内容 |
| `script` | 执行脚本文件 | `script`: 脚本路径<br>`dest`: 远程存放目录<br>`args`: 脚本参数<br>`inline`: 直接发送内容执行<br>`keep`: 是否保留远程文件 |
| `upload` | 上传本地文件到远程节点 | `src`: 本地路径<br>`dest`: 远程路径<br>`overwrite`: 是否覆盖<br>`resume`: 断点续传 |
| `download` | 从远程节点下载文件到本地 | `src`: 远程路径<br>`dest`: 本地路径<br>`subdir`: 按节点创建子目录<br>`name_format`: 文件命名格式 |
| `include` | 包含并执行其他 Playbook | `playbook`: 相对路径 |

### 6.2 通用执行控制参数

以下参数适用于所有 Action 类型，定义在每个 task 顶层：

```yaml
tasks:
  - name: <任务名>
    action: <action 类型>
    args:
      ...
    timeout: <超时秒数>              # 可选，默认 300（5 分钟）
    retries: <重试次数>              # 可选，默认 0（不重试）
    retry_delay: <重试间隔秒数>     # 可选，默认 10
    when: "<条件表达式>"            # 可选，条件执行
    ignore_errors: <true|false>     # 可选，默认 false，设为 true 则失败不阻断后续
```

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `timeout` | int | 300 | 任务最大执行时间（秒），超时则标记为 `failed` |
| `retries` | int | 0 | 失败后的自动重试次数 |
| `retry_delay` | int | 10 | 每次重试前的等待时间（秒） |
| `when` | string | — | 条件表达式，为 false 时跳过该步骤（状态标记为 `skipped`） |
| `ignore_errors` | bool | false | 忽略错误继续执行后续步骤（状态仍标记为 `failed`，但不阻断） |

### 6.3 Action 参数说明

#### script 参数

```yaml
args:
  script: "./scripts/deploy.sh"       # 脚本文件路径（本地文件或 URL）
  dest: "/tmp/"                       # 远程存放目录（默认 /tmp）
  args: "--version {{version}}"       # 传递给脚本的参数
  inline: false                       # 直接发送内容执行，不留文件（可选）
  keep: false                         # 执行后是否保留远程文件（可选）
```

#### upload 参数

```yaml
args:
  src: "./dist/app.tar.gz"           # 本地源文件路径
  dest: "/opt/app/"                   # 远程目标目录
  mode: "0644"                        # 文件权限（可选）
  overwrite: true                     # 是否覆盖已存在文件（可选）
  no_overwrite: false                # 文件存在时跳过（可选）
  resume: true                        # 启用断点续传（可选）
```

#### download 参数

```yaml
args:
  src: "/var/log/app.log"            # 远程源文件路径
  dest: "./logs/"                     # 本地目标目录
  subdir: true                        # 为每个节点创建子目录（可选）
  name_format: "{node}-{file}"       # 文件命名格式（可选）
  resume: true                        # 启用断点续传（可选）
```

#### include 参数

```yaml
args:
  playbook: "./common/healthcheck.yaml"  # 相对路径
```

---

## 7. 可观测性与执行控制

### 7.1 实时输出

执行时每个步骤的 stdout/stderr 实时流式回显到终端，按节点分组显示：

```bash
owl playbook run deploy.yaml --nodes web-01,web-02

# 输出示例：
═══════════════════════════════════════════
🚀 owl playbook run — deploy.yaml
   目标: web-01, web-02 | 步骤: 1/7
═══════════════════════════════════════════

▸ web-01 — 步骤 1/7: 创建工作目录
  web-01 | mkdir: created directory '/tmp/nginx-install'
  ✓ 完成 (1.0s)

▸ web-02 — 步骤 1/7: 创建工作目录
  web-02 | mkdir: created directory '/tmp/nginx-install'
  ✓ 完成 (0.8s)

▸ web-01 — 步骤 2/7: 上传 Nginx 压缩包
  web-01 | [████████████████████] 100%  45.2 MB  12.3 MB/s
  ✓ 完成 (3.7s)

▸ web-02 — 步骤 2/7: 上传 Nginx 压缩包
  web-02 | [████████████████████] 100%  45.2 MB  8.1 MB/s
  ✓ 完成 (5.6s)

▸ web-01 — 步骤 3/7: 解压安装
  web-01 | checking for OS
  web-01 |  + Linux 5.15.0 x86_64
  web-01 | checking for C compiler ... found
  web-01 | ...
  web-01 | make[1]: Leaving directory '/tmp/nginx-install/nginx-1.24.0'
  ✗ 失败 — exit code 1
  web-01 | /usr/bin/ld: cannot find -lssl

▸ web-02 — 步骤 3/7: 解压安装
  web-02 | checking for OS
  web-02 |  + Linux 5.15.0 x86_64
  ...
  ✓ 完成 (45.2s)
```

### 7.2 进度指示

整体进度条实时更新，展示每个节点的执行状态：

```
进度:  ████████░░░░░░░░░░░░  40%  (4/10 节点完成)

web-01  ✓ 步骤 3/7  解压安装       [████████████████]  45.2s
web-02  ✓ 步骤 7/7  验证安装       [████████████████]  0.3s
web-03  ✗ 步骤 2/7  上传压缩包      [██████░░░░░░░░░░]  失败 (retry 1/3)
web-04  ▶ 步骤 5/7  启动 Nginx     [████████░░░░░░░░]  运行中...
web-05  ○ 步骤 1/7  创建工作目录     [░░░░░░░░░░░░░░░░]  等待中
```

进度条左侧显示节点状态图标：

| 图标 | 含义 |
|------|------|
| `✓` | 当前步骤完成 |
| `✗` | 失败（正在重试或已停止） |
| `▶` | 正在执行 |
| `○` | 等待中（串行模式下） |
| `⏭` | 跳过（条件不满足） |

### 7.3 执行摘要

Playbook 执行完毕后输出结构化摘要：

```bash
═══════════════════════════════════════════
📊 执行摘要 — deploy.yaml
═══════════════════════════════════════════
   开始时间:  2026-05-25 10:00:00
   结束时间:  2026-05-25 10:05:42
   总耗时:    5m 42s

   目标节点:  2
   ├─ 成功:   1 (web-02)
   └─ 失败:   1 (web-01)

   执行步骤:  7
   ├─ 成功:   14 (2 节点 × 7 步骤)
   ├─ 失败:   1  (web-01 步骤 3)
   └─ 跳过:   0

   数据传输:  90.4 MB (45.2 MB × 2 节点)
═══════════════════════════════════════════
```

### 7.4 超时与重试

#### 超时控制

```yaml
tasks:
  - name: 解压安装
    action: command
    args:
      cmd: make -j$(nproc) && make install
    timeout: 600        # 10 分钟超时
```

超时后行为：
- 进程被发送 `SIGTERM`，等待 5 秒后 `SIGKILL`
- 状态标记为 `failed`，错误信息包含 "timed out after Ns"
- 若配置了 `retries`，自动进入重试流程

#### 重试机制

```yaml
tasks:
  - name: 验证安装
    action: command
    args:
      cmd: curl -s -o /dev/null -w "%{http_code}" http://localhost:{{ nginx_port }}/
    retries: 5
    retry_delay: 10     # 每次重试间隔 10 秒
```

重试时的终端显示：

```
web-01 — 步骤 5/7: 验证安装
  web-01 | 000 (connection refused)
  ✗ 失败 — 将在 10s 后重试 (1/5)
  web-01 | 000 (connection refused)
  ✗ 失败 — 将在 10s 后重试 (2/5)
  web-01 | 200
  ✓ 完成 (25.3s, 重试 2 次)
```

### 7.5 输出模式

| 模式 | 选项 | 适用场景 |
|------|------|---------|
| 交互模式 | 默认（TTY 时） | 人工操作，彩色输出，进度条，实时回显 |
| 纯文本模式 | `--plain` | CI/CD 流水线，日志归档 |
| JSON 模式 | `--output json` | 程序化消费，对接监控系统 |
| 静默模式 | `--quiet` | 只输出错误，适合 cron |

```bash
# CI/CD 中使用纯文本模式
owl playbook run deploy.yaml --nodes web-01 --plain --dry-run

# JSON 输出用于外部系统集成
owl playbook run deploy.yaml --nodes web-01 --output json
```

```json
{
  "playbook": "deploy.yaml",
  "status": "completed",
  "nodes": {
    "web-01": {
      "status": "completed",
      "steps": [
        {"index": 0, "name": "创建工作目录", "status": "completed", "duration_ms": 1023},
        {"index": 1, "name": "解压安装", "status": "completed", "duration_ms": 45200}
      ]
    }
  },
  "summary": {
    "total_steps": 7,
    "completed": 7,
    "failed": 0,
    "skipped": 0,
    "duration_ms": 342000
  }
}
```

---

## 8. 命令接口设计

### 8.1 命令列表

```
owl playbook template list                    # 列出所有可用模板
owl playbook template info <name>             # 查看模板详情
owl playbook template export <name>           # 导出模板到用户目录
owl playbook new --from=<template> [--var ...] # 从模板创建 Playbook 实例
owl playbook scaffold [--type=basic]          # 生成 Playbook 骨架
owl playbook run <playbook>                   # 执行 Playbook（含 dry-run/限流/续跑）
owl playbook state list|show                  # 查看执行状态（运行历史、步骤状态）
```

### 8.2 owl playbook template list

```bash
# 列出所有模板
owl playbook template list

# 输出示例：
可用的 Playbook 模板：

📦 内置模板:
  webserver/
    • nginx/deploy      - Nginx 部署模板
    • apache/deploy     - Apache 部署模板
  application/
    • nodejs/deploy     - Node.js 应用部署模板
    • docker/container  - Docker 容器部署模板
  utility/
    • backup/files      - 文件备份模板
    • healthcheck/http  - HTTP 健康检查模板

👤 用户模板:
  • custom/my-template - 我的自定义模板
```

### 8.3 owl playbook template info

```bash
# 查看模板详情
owl playbook template info nginx/deploy

# 输出示例：
模板路径: webserver/nginx/deploy
描述: Nginx 部署模板，支持一键部署和配置管理
标签: nginx, webserver, deploy

📋 参数说明:
  • nginx_version - Nginx 版本 [默认: 1.24.0]
  • nginx_port   - 监听端口 [默认: 80]
  • enable_ssl   - 启用 HTTPS [默认: false]

📝 任务列表 (共 7 步):
  1. 创建工作目录      (command, timeout: —)
  2. 上传 Nginx 压缩包 (upload, timeout: 600s, retries: 2)
  3. 解压安装          (command, timeout: 300s)
  4. 上传配置文件      (upload)
  5. 启动 Nginx        (command)
  6. 验证安装          (command, retries: 3)
  7. 下载日志文件      (download)

📄 完整模板内容:
  [显示模板 YAML 内容]
```

### 8.4 owl playbook new

从模板创建 Playbook 实例。

```bash
# 交互式创建
owl playbook new --from=nginx/deploy

# 输出示例：
🔧 从模板 'nginx/deploy' 创建 Playbook

请输入以下参数（按 Enter 使用默认值）：

Nginx 版本 (nginx_version) [1.24.0]:
  > 1.25.0

监听端口 (nginx_port) [80]:
  >

启用 HTTPS (enable_ssl) [false]:
  >

✅ Playbook 已创建: ~/.owl/playbooks/nginx-1.25.0.yaml
💡 执行命令:
   owl playbook run ~/.owl/playbooks/nginx-1.25.0.yaml --nodes <节点> --dry-run
```

```bash
# 参数式创建
owl playbook new --from=nginx/deploy \
  --var nginx_version=1.25.0 \
  --var nginx_port=8080 \
  --output my-nginx-deploy.yaml

# 输出示例：
✅ Playbook 已创建: ./my-nginx-deploy.yaml
```

### 8.5 owl playbook scaffold

生成带注释的 Playbook 骨架，供用户手动填充。

```bash
owl playbook scaffold --type=basic > my-playbook.yaml

# 生成的文件内容：
# description: "TODO: 描述此 Playbook 的用途"
# tags: []
#
# parameters:
#   # - name: app_version
#   #   description: "应用版本号"
#   #   default: "latest"
#
# tasks:
#   - name: "TODO: 步骤名称"
#     action: command
#     args:
#       cmd: echo "hello"
#     # timeout: 300
#     # retries: 3
```

### 8.6 owl playbook template export

导出系统内置模板到用户目录，支持自定义修改。

```bash
# 导出单个模板
owl playbook template export nginx/deploy --to ~/.owl/templates/

# 导出整个分类
owl playbook template export webserver --to ~/.owl/templates/

# 导出所有模板
owl playbook template export --all --to ~/.owl/templates/

# 输出示例：
✅ 模板已导出到 ~/.owl/templates/webserver/nginx/deploy.yaml
💡 您可以修改模板内容进行自定义
```

### 8.7 owl playbook run

执行 Playbook，集成了 dry-run、限流、续跑等机制。

```bash
owl playbook run <playbook> [options]
```

| 选项 | 说明 |
|------|------|
| `--nodes <节点列表>` | 目标节点（逗号分隔），必填 |
| `--group <分组名>` | 按分组筛选节点 |
| `--label key=value` / `-l` | 按标签筛选节点（可多次指定） |
| `--vars key=value` / `--extra-vars` | 传递变量（`{{ }}` 模板变量替换值） |
| `--tags <标签>` | 只执行匹配标签的步骤 |
| `--skip-tags <标签>` | 跳过匹配标签的步骤 |
| `--dry-run` / `--check` | 空运行预览，不实际执行（`--check` 为向后兼容别名） |
| `--diff` | 显示变更差异 |
| `--limit <N>` | 限制同时执行的节点数，默认 10 |
| `--serial <N>` | 分批执行，每批 N 个节点，一批完成后才执行下一批 |
| `--resume` | 从上次失败的步骤继续执行 |
| `--from-step <N>` | 从指定步骤开始执行 |
| `--reset` | 清除已有状态，从头开始 |
| `--default-command-timeout <dur>` | 命令默认超时（可被 task 级 `timeout` 覆盖） |
| `--default-retry <N>` | 默认重试次数（可被 task 级 `retries` 覆盖） |
| `--plain` | 纯文本输出（适用于 CI/CD） |
| `--output <json\|text>` | 输出格式 |
| `--env <production\|staging>` | 环境标记，production 下强制 dry-run 确认 |

```bash
# 典型使用流程
# 1. 先预览
owl playbook run deploy.yaml --nodes web-01,web-02 --dry-run

# 2. 确认后分批执行（每批1个节点，安全滚动更新）
owl playbook run deploy.yaml --nodes web-01,web-02 --serial 1

# 3. 某节点失败后修复问题，续跑
owl playbook run deploy.yaml --nodes web-01 --resume

# 4. CI/CD 中使用
owl playbook run deploy.yaml --nodes web-01 --plain --output json
```

---

## 9. 内置模板库

### 9.1 模板列表

| 模板路径 | 说明 | 包含 Action |
|---------|------|------------|
| `webserver/nginx/deploy` | Nginx 部署模板 | command, upload, download |
| `webserver/apache/deploy` | Apache 部署模板 | command, upload, download |
| `application/nodejs/deploy` | Node.js 应用部署 | command, upload, download |
| `application/docker/container` | Docker 容器管理 | command, upload, download |
| `utility/backup/files` | 文件备份模板 | command, upload, download |
| `utility/healthcheck/http` | HTTP 健康检查 | command |

### 9.2 模板分类

| 分类 | 说明 |
|------|------|
| `webserver/` | Web 服务器相关（nginx, apache） |
| `application/` | 应用部署相关（nodejs, docker） |
| `database/` | 数据库相关（预留） |
| `monitoring/` | 监控相关（预留） |
| `utility/` | 工具类模板（backup, healthcheck） |

---

## 10. 实现计划

### 10.1 阶段一：核心功能实现

1. **流水线引擎**
   - 定义步骤状态机（pending → running → completed / failed）
   - 在 `owl.db` 中新增 `playbook_runs` 和 `playbook_step_states` 两张表
   - 实现按节点维度的状态写入与查询
   - 支持断点续跑（`--resume`、`--from-step`）

2. **状态查询命令**
   - `owl playbook state list` — 列出历史运行记录，支持按剧本名、状态筛选
   - `owl playbook state show <run-id>` — 查看某次执行的各节点详细步骤状态
   - `owl playbook state show <run-id> --status incomplete` — 只显示未完成步骤

2. **模板解析器**
   - 定义模板结构体（Template）
   - 实现参数解析和 `{{ }}` 替换逻辑
   - 支持参数验证（type / options / pattern）

3. **Dry-Run 机制**
   - 实现零副作用的步骤展开
   - 查询 `playbook_step_states` 表标记已完成步骤
   - 差异检测（对比上次 dry-run）

4. **模板管理命令**
   - `owl playbook template list` — 列出模板
   - `owl playbook template info` — 查看详情
   - `owl playbook template export` — 导出模板
   - `owl playbook new --from` — 使用模板创建
   - `owl playbook scaffold` — 生成骨架

5. **内置模板库**
   - 实现 3-5 个常用模板
   - 覆盖所有 Action 类型

### 10.2 阶段二：可观测性

6. **实时输出引擎**
   - 按节点分组的 stdout/stderr 流式回显
   - 进度条显示
   - 彩色状态图标

7. **超时与重试**
   - `timeout` 超时控制 + SIGTERM/SIGKILL
   - `retries` + `retry_delay` 重试逻辑
   - `when` 条件执行

8. **多输出模式**
   - 交互模式（TTY）
   - `--plain` 纯文本模式
   - `--output json` JSON 模式

### 10.3 阶段三：增强体验

9. **安全控制**
   - `--limit` / `--serial` 节点限流
   - `--env production` 生产环境强制 dry-run
   - 执行确认提示

10. **交互式创建向导**
    - 问答式参数输入
    - 参数验证和提示
    - 自动补全

11. **编辑器集成**
    - VS Code 插件
    - 语法高亮和自动补全
    - 实时预览

---

## 11. 技术细节

### 11.1 模板与实例的概念分离

| 概念 | 说明 | 生命周期 |
|------|------|---------|
| **模板 (Template)** | 定义参数结构和任务流程 | 只读，可复用 |
| **实例 (Instance)** | 绑定具体参数值的 Playbook YAML | 保存为文件，可编辑 |
| **执行状态 (State)** | 一次 `run` 的各节点、各步骤状态快照 | 与 playbook hash 关联 |

```bash
# 从模板生成实例
owl playbook new --from=nginx/deploy --var nginx_version=1.25.0 --output my-nginx.yaml

# 实例不包含 hosts——执行时指定
owl playbook run my-nginx.yaml --nodes web-01,web-02

# 执行状态与实例 hash 绑定存储在 ~/.owl/state/<hash>/
```

### 11.2 模板参数替换

使用 `{{ 参数名 }}` 语法进行参数替换：

```yaml
# 模板定义
vars:
  app_name: "{{ app_name }}"
  app_version: "{{ app_version }}"

# 参数传递
--var app_name=myapp --var app_version=1.0.0

# 替换结果
vars:
  app_name: "myapp"
  app_version: "1.0.0"
```

### 11.3 参数默认值处理

模板中 `parameters` 下的 `default` 是**提示性默认值**：

```yaml
parameters:
  - name: timeout
    description: "超时时间"
    default: "30s"
    required: false
```

| 场景 | 行为 |
|------|------|
| 交互式创建时不输入 | 使用默认值 `"30s"` |
| 使用 `--var timeout=60s` | 覆盖默认值，使用 `"60s"` |
| 参数 `required: true` 且无 default | 必须提供，否则报错 |

### 11.4 参数验证

```yaml
parameters:
  - name: port
    description: "监听端口"
    type: number
    options: [80, 443, 8080, 9000]

  - name: version
    description: "版本号"
    pattern: "^\\d+\\.\\d+\\.\\d+$"
```

### 11.5 状态与 Playbook 的关联

状态通过 `playbook_runs.playbook_hash` 与 Playbook 内容关联，存储在已有的 `~/.owl/owl.db` 中：

```
playbook_hash = sha256(playbook_content + nodes_list)
```

查询时通过 `run_id` 关联 `playbook_runs` 和 `playbook_step_states`：

```sql
-- 查找某剧本的最近一次失败运行
SELECT id, started_at, completed_steps, failed_steps
FROM playbook_runs
WHERE playbook_name = 'nginx-deploy' AND status = 'failed'
ORDER BY started_at DESC LIMIT 1;

-- 查看该运行的未完成步骤
SELECT node_id, step_index, step_name, status, error
FROM playbook_step_states
WHERE run_id = '<run_id>' AND status IN ('failed', 'pending')
ORDER BY node_id, step_index;
```

这意味着：
- 修改 Playbook 内容后，旧状态自动失效（hash 变化）
- 同一 Playbook 在不同时间对相同节点执行，状态可复用（`--resume`）
- 不同节点列表产生不同的 hash 和运行记录
- 所有状态可通过 `owl playbook state` 命令查询，无需手动操作数据库

---

## 12. 未来扩展

### 12.1 模板市场

- 在线模板库
- 社区分享功能
- 模板贡献和评分

### 12.2 智能推荐

- 基于历史使用推荐模板
- 场景智能匹配
- 常用组合推荐

### 12.3 变量管理

- 变量模板（预设常用变量组合）
- 环境配置模板（dev/staging/prod）
- 密钥模板（敏感信息管理）

### 12.4 增量步骤执行

- 基于文件变化检测跳过不必要的步骤
- 时间戳比对优化（"如果配置文件未变则跳过上传"）

---

## 13. 附录

### 13.1 配置项

```yaml
# ~/.owl/config.yaml
templates:
  builtin_path: "~/.owl/builtin-templates/"
  user_path: "~/.owl/templates/"
  auto_update: true
  cache_ttl: 3600

execution:
  default_timeout: 300
  max_retries: 5
  max_nodes: 10
  require_dry_run_for_production: true

output:
  color: true
  progress_bars: true
```

### 13.2 环境变量

```bash
export OWL_TEMPLATE_PATH=~/.owl/templates/
export OWL_TEMPLATE_CACHE=false
export OWL_DEFAULT_TIMEOUT=300
```

### 13.3 与现有功能的兼容性

本设计方案与当前已实现的 [PLAYBOOK.md](../user/PLAYBOOK.md) 功能存在以下需要协调的差异：

| 功能点 | 当前手册 | 本设计提案 | 兼容策略 |
|--------|---------|-----------|---------|
| 节点参数名 | `--nodes` | `--nodes` | 一致，无需变更 |
| 检查模式 | `--check` | `--dry-run` | `--check` 作为别名保留向后兼容 |
| Playbook 结构 | `pre_tasks` / `tasks` / `post_tasks` | 统一 `tasks` | 解析器同时支持两种格式，`pre_tasks` 和 `post_tasks` 按序合并到 `tasks` |
| `hosts` 字段 | 写在 Playbook YAML 中 | 仅通过 `--nodes` CLI 提供 | `--nodes` 优先级高于 YAML 中的 `hosts`，YAML 中的 `hosts` 作为回退默认值 |
| 创建命令名 | `owl playbook create`（未来版本） | `owl playbook new --from=` | `create` 作为 `new` 的别名保留 |
| Action 类型 | 包含 `script`（带 `inline` / `keep`） | 仅列 `command`/`upload`/`download`/`include` | 补充 `script` 类型，参数对齐 |
| 超时参数 | `--command-timeout`（全局） | `timeout`（per-task） | 两者共存：task 级覆盖全局默认 |
| 重试参数 | `retry: N`（FAQ 提及） | `retries` + `retry_delay`（per-task） | 字段名从 `retry` 改为 `retries`，新增 `retry_delay` |
| 状态查询 | 无 | `owl playbook state list|show` | 新增，不影响现有功能 |
| 剧本骨架 | 无 | `owl playbook scaffold` | 新增 |
| 执行限流 | `--limit`（无默认值） | `--limit <N>`（默认 10）、`--serial <N>` | `--serial` 新增；`--limit` 语义不变 |
| `name`/`version` 字段 | 有（剧本格式中） | 移除（由路径/文件名表达） | 解析器忽略这些字段，不报错 |

### 13.4 相关文档

- [PLAYBOOK.md](../user/PLAYBOOK.md) - Playbook 使用文档
- [设计文档索引](./README.md) - 所有设计文档
