# Playbook 模板系统设计方案

## 1. 概述

Playbook 模板系统旨在降低用户编写 Playbook 的门槛，提供丰富的预置模板和便捷的创建工具，帮助用户快速生成符合最佳实践的 Playbook。

### 1.1 设计目标

- **降低学习成本**：通过模板引导用户了解 Playbook 结构
- **提高效率**：快速生成常用场景的 Playbook
- **可扩展性**：支持用户自定义模板
- **最佳实践**：内置模板遵循标准化流程

### 1.2 设计原则

- **分层设计**：用户模板优先于系统内置模板
- **零配置优先**：提供合理的默认参数
- **渐进式复杂度**：从简单场景开始，逐步深入
- **向后兼容**：不影响现有 Playbook 功能

---

## 2. 模板存放结构

### 2.1 目录结构

```
~/.owl/
├── templates/                    # 用户自定义模板（优先级高）
│   ├── README.md
│   ├── nginx/
│   │   └── nginx-deploy.yaml
│   ├── docker/
│   │   └── container.yaml
│   └── custom/                  # 用户自定义分组
│       └── my-template.yaml
│
├── builtin-templates/            # 系统内置模板（只读）
│   ├── nginx/
│   │   └── nginx-deploy.yaml
│   ├── nodejs/
│   │   └── nodejs-app.yaml
│   ├── docker/
│   │   └── container.yaml
│   ├── backup/
│   │   └── files.yaml
│   └── healthcheck/
│       └── http.yaml
│
└── playbooks/                  # 用户创建的剧本
    └── my-deploy.yaml
```

### 2.2 模板加载优先级

1. **`~/.owl/templates/`** - 用户自定义模板（优先级最高）
2. **`~/.owl/builtin-templates/`** - 系统内置模板
3. **编译时内置** - 作为备选的默认模板

### 2.3 模板目录初始化

首次使用时自动创建模板目录结构：

```bash
# 初始化用户模板目录
~/.owl/
└── templates/
    └── README.md    # 包含模板编写指南
```

---

## 3. 模板结构规范

### 3.1 模板元数据

每个模板必须包含以下元数据字段：

```yaml
name: nginx-deploy
description: Nginx 部署模板，支持一键部署和配置管理
version: "1.0"
author: "Owl Team"
tags: [web, nginx, deploy]
category: webserver

# 模板参数定义
parameters:
  - name: nginx_version
    description: "Nginx 版本号"
    default: "1.24.0"
    required: false
  
  - name: nginx_port
    description: "HTTP 监听端口"
    default: "80"
    required: false
  
  - name: nginx_host
    description: "目标主机"
    required: true
```

### 3.2 参数定义规范

```yaml
parameters:
  - name: <参数名>                    # 必填，参数标识符
    description: "<描述>"              # 必填，参数用途说明
    default: <默认值>                  # 可选，无默认值时为必填参数
    required: <true|false>            # 可选，默认为 false
    type: <string|number|boolean>     # 可选，默认为 string
    options: [<选项列表>]              # 可选，限制可选值
    pattern: "<正则表达式>"            # 可选，参数验证正则
```

### 3.3 完整模板示例

```yaml
name: nginx-deploy
description: |
  Nginx 部署模板，支持一键部署和配置管理。
  功能包括：
  - 自动下载和安装 Nginx
  - 配置文件上传
  - 服务启动和健康检查
version: "1.0"
author: "Owl Team"
tags: [web, nginx, deploy]
category: webserver

parameters:
  - name: nginx_host
    description: "目标主机"
    required: true
  
  - name: nginx_version
    description: "Nginx 版本"
    default: "1.24.0"
    required: false
  
  - name: nginx_port
    description: "监听端口"
    default: "80"
    required: false
  
  - name: enable_ssl
    description: "启用 HTTPS"
    default: false
    required: false
    type: boolean

hosts: ["{{ nginx_host }}"]

vars:
  nginx_version: "{{ nginx_version }}"
  nginx_port: "{{ nginx_port }}"
  enable_ssl: "{{ enable_ssl }}"

pre_tasks:
  - name: 创建工作目录
    action: command
    args:
      cmd: mkdir -p /tmp/nginx-install

tasks:
  - name: 上传 Nginx 压缩包
    action: upload
    args:
      src: ./files/nginx-{{ nginx_version }}.tar.gz
      dest: /tmp/nginx-install/
      overwrite: true

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

post_tasks:
  - name: 下载日志文件
    action: download
    args:
      src: /usr/local/nginx/logs/access.log
      dest: ./logs/
      subdir: true
      name_format: "{node}-nginx-access.log"

  - name: 健康检查
    action: command
    args:
      cmd: curl -f http://localhost:{{ nginx_port }}/health || exit 1

  - name: 清理临时文件
    action: command
    args:
      cmd: rm -rf /tmp/nginx-install
```

---

## 4. 支持的 Action 类型

### 4.1 Action 类型列表

| Action 类型 | 说明 | 主要参数 |
|------------|------|---------|
| `command` / `cmd` / `shell` | 执行 Shell 命令 | `cmd`: 命令内容 |
| `upload` | 上传本地文件到远程节点 | `src`: 本地路径<br>`dest`: 远程路径<br>`overwrite`: 是否覆盖<br>`resume`: 断点续传 |
| `download` | 从远程节点下载文件到本地 | `src`: 远程路径<br>`dest`: 本地路径<br>`subdir`: 按节点创建子目录<br>`name_format`: 文件命名格式 |
| `include` | 包含并执行其他 Playbook | `playbook`: 相对路径 |

### 4.2 Action 参数说明

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

## 5. 命令接口设计

### 5.1 命令列表

```
owl playbook templates            # 列出所有可用模板
owl playbook template-info      # 查看模板详情
owl playbook create             # 使用模板创建 Playbook
owl playbook template-export    # 导出模板到用户目录
```

### 5.2 owl playbook templates

列出所有可用的 Playbook 模板。

```bash
# 列出所有模板
owl playbook templates

# 输出示例：
可用的 Playbook 模板：

📦 内置模板:
  • nginx             - Nginx 部署模板
  • nodejs            - Node.js 应用部署模板
  • docker            - Docker 容器部署模板
  • backup            - 文件备份模板
  • healthcheck       - HTTP 健康检查模板

👤 用户模板:
  • custom/my-template - 我的自定义模板
```

### 5.3 owl playbook template-info

查看指定模板的详细信息。

```bash
# 查看模板详情
owl playbook template-info nginx

# 输出示例：
模板名称: nginx-deploy
描述: Nginx 部署模板，支持一键部署和配置管理
版本: 1.0
作者: Owl Team
标签: web, nginx, deploy
分类: webserver

📋 参数说明:
  • nginx_host    - 目标主机 [必填]
  • nginx_version - Nginx 版本 [默认: 1.24.0]
  • nginx_port   - 监听端口 [默认: 80]
  • enable_ssl   - 启用 HTTPS [默认: false]

📝 任务列表:
  1. 上传 Nginx 压缩包 (upload)
  2. 解压安装 (command)
  3. 上传配置文件 (upload)
  4. 启动 Nginx (command)
  5. 验证安装 (command)

📄 完整模板内容:
  [显示模板 YAML 内容]
```

### 5.4 owl playbook create

使用模板创建 Playbook，支持交互式和参数式两种方式。

```bash
# 交互式创建
owl playbook create --template=nginx

# 输出示例：
🔧 使用模板 'nginx-deploy' 创建 Playbook

请输入以下参数（按 Enter 使用默认值）：

目标主机 (nginx_host): █
  > web-01

Nginx 版本 (nginx_version) [1.24.0]: 
  > 1.25.0

监听端口 (nginx_port) [80]: 
  > 

启用 HTTPS (enable_ssl) [false]: 
  > 

✅ Playbook 已创建: ~/.owl/playbooks/nginx-web01-1.25.0.yaml
```

```bash
# 参数式创建
owl playbook create --template=nginx \
  --var nginx_host=web-01 \
  --var nginx_version=1.25.0 \
  --var nginx_port=8080 \
  --output my-nginx-deploy.yaml

# 输出示例：
✅ Playbook 已创建: ./my-nginx-deploy.yaml
```

### 5.5 owl playbook template-export

导出系统内置模板到用户目录，支持自定义修改。

```bash
# 导出单个模板
owl playbook template-export nginx --to ~/.owl/templates/

# 导出所有模板
owl playbook template-export --all --to ~/.owl/templates/

# 输出示例：
✅ 模板已导出到 ~/.owl/templates/nginx/nginx-deploy.yaml
💡 您可以修改模板内容进行自定义
```

---

## 6. 内置模板库

### 6.1 模板列表

| 模板名称 | 说明 | 包含 Action |
|---------|------|------------|
| `nginx` | Nginx 部署模板 | command, upload, download |
| `nodejs` | Node.js 应用部署 | command, upload, download |
| `docker` | Docker 容器管理 | command, upload, download |
| `backup` | 文件备份模板 | command, upload, download |
| `healthcheck` | HTTP 健康检查 | command |

### 6.2 模板分类

```
webserver/     - Web 服务器相关
application/   - 应用部署相关
database/      - 数据库相关
monitoring/    - 监控相关
utility/       - 工具类模板
```

---

## 7. 实现计划

### 7.1 阶段一：核心功能实现

1. **模板解析器**
   - 定义模板结构体（Template）
   - 实现参数解析和替换逻辑
   - 支持参数验证

2. **模板管理命令**
   - `owl playbook templates` - 列出模板
   - `owl playbook template-info` - 查看详情
   - `owl playbook create --template` - 使用模板创建

3. **内置模板库**
   - 实现 3-5 个常用模板
   - 覆盖所有 Action 类型

### 7.2 阶段二：高级功能

4. **用户自定义支持**
   - `owl playbook template-export` - 导出模板
   - 支持 `~/.owl/templates/` 目录
   - 模板覆盖机制

5. **模板市场**
   - 模板搜索功能
   - 模板评分和评论
   - 在线模板库（未来）

### 7.3 阶段三：增强体验

6. **交互式创建向导**
   - 问答式参数输入
   - 参数验证和提示
   - 自动补全

7. **编辑器集成**
   - VS Code 插件
   - 语法高亮和自动补全
   - 实时预览

---

## 8. 技术细节

### 8.1 模板参数替换

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

### 8.2 默认值处理

```yaml
# 参数定义
parameters:
  - name: timeout
    default: "30s"
    required: false

# 使用默认值（不指定参数时）
owl playbook create --template=app
# timeout 将使用默认值 "30s"

# 覆盖默认值
owl playbook create --template=app --var timeout=60s
```

### 8.3 参数验证

```yaml
parameters:
  - name: port
    type: number
    options: [80, 443, 8080, 9000]
  
  - name: version
    pattern: "^\\d+\\.\\d+\\.\\d+$"
```

---

## 9. 未来扩展

### 9.1 模板市场

- 在线模板库
- 社区分享功能
- 模板版本管理

### 9.2 智能推荐

- 基于历史使用推荐模板
- 场景智能匹配
- 常用组合推荐

### 9.3 模板变量市场

- 变量模板
- 环境配置模板
- 密钥模板

---

## 10. 附录

### 10.1 配置项

```yaml
# ~/.owl/config.yaml
templates:
  builtin_path: "~/.owl/builtin-templates/"
  user_path: "~/.owl/templates/"
  auto_update: true
  cache_ttl: 3600
```

### 10.2 环境变量

```bash
export OWL_TEMPLATE_PATH=~/.owl/templates/
export OWL_TEMPLATE_CACHE=false
```

### 10.3 相关文档

- [PLAYBOOK.md](../user/PLAYBOOK.md) - Playbook 使用文档
- [设计文档索引](./README.md) - 所有设计文档
