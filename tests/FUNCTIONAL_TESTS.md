# go-owl 功能测试用例

本文档提供 go-owl 的完整功能测试用例，按操作顺序编排。

## 测试环境准备

```bash
# 编译项目
cd go-owl
make build-duckdb

# 或使用 SQLite3 版本
# make build-sqlite3

# 确认二进制文件
ls -la owl-duckdb
```

## 测试节点信息

| 节点 ID | 名称 | 地址 | 用户 | 端口 | 说明 |
|---------|------|------|------|------|------|
| node1 | Web Server 01 | 192.168.1.10 | root | 22 | 测试节点1 |
| node2 | DB Server 01 | 192.168.1.20 | postgres | 22 | 测试节点2 |
| node3 | Cache Server 01 | 192.168.1.30 | redis | 2222 | 非标准端口 |
| node4 | App Server 01 | 192.168.1.40 | admin | 22 | 多标签节点 |
| bastion | 跳板机 | 192.168.0.1 | ubuntu | 22 | 跳板机 |

---

## 一、节点管理

### 1.1 添加节点

#### 基本添加（密码认证）

```bash
# 添加第一个节点（使用默认端口22）
owl node add node1 \
  --name "Web Server 01" \
  --address 192.168.1.10 \
  --user root \
  --password "your-password"

# 预期输出
✓ 添加节点 node1
```

#### 使用 SSH 密钥认证

```bash
owl node add node2 \
  --name "DB Server 01" \
  --address 192.168.1.20 \
  --user postgres \
  --ssh-key ~/.ssh/id_rsa

# 预期输出
✓ 添加节点 node2
```

#### 非标准端口

```bash
owl node add node3 \
  --name "Cache Server 01" \
  --address 192.168.1.30 \
  --user redis \
  --port 2222 \
  --password "redis-pass"

# 预期输出
✓ 添加节点 node3
```

#### 批量添加标签和分组

```bash
owl node add node4 \
  --name "App Server 01" \
  --address 192.168.1.40 \
  --user admin \
  --password "your-password" \
  --groups web,production \
  --labels env=prod,appname=owl,region=cn-east

# 预期输出
✓ 添加节点 node4
  Groups:  web, production
  Labels:  env=prod, appname=owl, region=cn-east
```

#### 使用跳板机

```bash
owl node add internal-server \
  --name "Internal Server" \
  --address 10.0.0.100 \
  --user admin \
  --password "admin-pass" \
  --proxy-jump bastion.example.com

# 预期输出
✓ 添加节点 internal-server
```

### 1.2 列出节点

#### 查看所有节点

```bash
owl node list

# 预期输出
ID       | Name            | Address          | Port | User      | Status
---------|-----------------|------------------|------|-----------|--------
node1    | Web Server 01   | 192.168.1.10     | 22   | root      | offline
node2    | DB Server 01    | 192.168.1.20     | 22   | postgres  | offline
node3    | Cache Server 01 | 192.168.1.30     | 2222 | redis     | offline
node4    | App Server 01   | 192.168.1.40     | 22   | admin     | offline
```

#### 按分组筛选

```bash
owl node list --group web

# 预期输出
ID       | Name            | Address          | Port | User      | Status
---------|-----------------|------------------|------|-----------|--------
node1    | Web Server 01   | 192.168.1.10     | 22   | root      | offline
node4    | App Server 01   | 192.168.1.40     | 22   | admin     | offline
```

#### 按标签筛选

```bash
owl node list --labels env=prod

# 预期输出
ID       | Name            | Address          | Port | User      | Status
---------|-----------------|------------------|------|-----------|--------
node4    | App Server 01   | 192.168.1.40     | 22   | admin     | offline
```

### 1.3 更新节点

#### 修改端口

```bash
owl node update node3 --port 2223

# 预期输出
✓ 更新节点 node3
```

#### 修改密码

```bash
owl node update node1 --password "new-password"

# 预期输出
✓ 更新节点 node1
```

#### 批量更新

```bash
owl node update node4 \
  --name "App Server Updated" \
  --groups web,production,frontend \
  --labels env=prod,appname=owl,version=v2.0

# 预期输出
✓ 更新节点 node4
```

### 1.4 节点分组管理

#### 添加分组

```bash
owl node group add production --nodes node1,node2

# 预期输出
✓ 添加分组 production，包含节点: node1, node2
```

#### 列出所有分组

```bash
owl node groups

# 预期输出
分组名称         | 包含节点数
----------------|-----------
web             | 1
production      | 2
database        | 1
```

### 1.5 节点标签管理

#### 设置标签

```bash
owl node labels set node1 env=production region=us-west version=2.0

# 预期输出
Labels updated for node 'node1'
env: production
region: us-west
version: 2.0
```

#### 显示所有标签

```bash
owl node labels show node1

# 预期输出
Labels for node 'node1':
  env: production
  region: us-west
  version: 2.0
  env: prod
```

#### 显示指定标签

```bash
owl node labels show node1 env

# 预期输出
env=production
```

#### 删除标签

```bash
owl node labels remove node1 version

# 预期输出
Label 'version' removed from node 'node1'
```

### 1.6 节点导入导出

#### 生成导入模板

```bash
owl node import --template > nodes.yaml

# 预期输出（YAML格式）
version: "1.0"
nodes:
  - id: web-server-01
    ...
```

#### 导出节点

```bash
owl node export -f nodes.yaml

# 预期输出
已导出 3 个节点到 nodes.yaml
```

#### 按分组导出

```bash
owl node export --groups web,production -f prod-nodes.yaml

# 预期输出
已导出 2 个节点到 prod-nodes.yaml
```

#### 按标签导出

```bash
owl node export --labels env=prod -f prod-nodes.yaml

# 预期输出
已导出 1 个节点到 prod-nodes.yaml
```

#### 导入节点

```bash
owl node import -f nodes.yaml

# 预期输出
✓ 添加节点 node1
✓ 添加节点 node2
✓ 添加节点 node3

结果: 添加/更新 3, 跳过 0, 失败 0
```

#### 导入并覆盖

```bash
owl node import -f nodes.yaml --overwrite

# 预期输出
✓ 更新节点 node1
✓ 更新节点 node2
✓ 更新节点 node3

结果: 添加/更新 3, 跳过 0, 失败 0
```

#### 导入预览

```bash
owl node import -f nodes.yaml --dry-run

# 预期输出
[预览] node1 -> Web Server 01 (192.168.1.10:22)
[预览] node2 -> DB Server 01 (192.168.1.20:22)
[预览] node3 -> Cache Server 01 (192.168.1.30:2222)

结果: 添加/更新 3, 跳过 0, 失败 0
```

### 1.7 删除节点

```bash
owl node remove node3

# 预期输出
✓ 删除节点 node3
```

---

## 二、交互式会话

### 2.1 连接单节点

#### 基本连接

```bash
owl session attach node1

# 预期输出
正在连接到 1 个节点...
[连接成功]
Welcome to Ubuntu 22.04 LTS
root@node1:~#
```

#### 指定用户连接

```bash
owl session attach root@192.168.1.10

# 预期输出
正在连接到 1 个节点...
[连接成功]
```

#### 指定密钥连接

```bash
owl session attach node2 --key ~/.ssh/db_key.pem

# 预期输出
正在连接到 1 个节点...
[连接成功]
```

### 2.2 连接多节点

```bash
owl session attach --nodes node1,node2

# 预期输出
正在连接到 2 个节点...
[node1] 已连接
[node2] 已连接

[多节点会话模式]
输入命令后，结果将分别显示在各个节点的终端中
输入 exit 退出多节点会话
```

### 2.3 查看会话历史

```bash
owl session history

# 预期输出
会话 ID              | 模式     | 节点                    | 状态   | 创建时间
--------------------|----------|------------------------|--------|----------
sess-abc123         | single   | node1                   | closed | 2024-01-15 10:00
sess-def456         | multiple | node1, node2            | closed | 2024-01-15 11:00
```

#### 查看特定会话详情

```bash
owl session history --session-id sess-abc123

# 预期输出
会话 ID: sess-abc123
模式: single
节点: node1
状态: closed
创建时间: 2024-01-15 10:00:00
命令数量: 15
成功率: 93%
```

### 2.4 列出活动会话

```bash
owl session list

# 预期输出
会话 ID              | 模式     | 节点                    | 状态   | 活动时间
--------------------|----------|------------------------|--------|----------
sess-xyz789         | single   | node1                   | active | 2024-01-15 12:00
```

### 2.5 退出会话

```bash
# 在交互式会话中输入
exit

# 或按 Ctrl+D
```

---

## 三、命令执行

### 3.1 单节点命令执行

```bash
owl exec --nodes node1 --command "uptime"

# 预期输出
[node1] 执行: uptime
[node1] 结果:
 12:00:00 up 100 days,  1:30,  2 users,  load average: 0.15, 0.10, 0.05
```

### 3.2 多节点命令执行

```bash
owl exec --nodes node1,node2 --command "df -h"

# 预期输出
[node1] 结果:
Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1       100G   50G   50G  50% /

[node2] 结果:
Filesystem      Size  Used Avail Use% /dev/sda1       200G   80G  120G  40% /
```

### 3.3 按分组执行

```bash
owl exec --group web --command "systemctl status nginx"

# 预期输出
[node1] 执行: systemctl status nginx
[node1] 结果:
● nginx.service - A high performance web server
   Loaded: loaded (/lib/systemd/system/nginx.service; enabled)
   Active: active (running)
```

### 3.4 执行脚本

```bash
owl exec --nodes node1 --script ./deploy.sh

# 预期输出
[node1] 上传脚本: ./deploy.sh
[node1] 执行: bash /tmp/deploy.sh
[node1] 结果:
部署开始...
[SUCCESS] 应用部署完成
```

---

## 四、剧本管理

### 4.1 创建剧本

创建文件 `deploy.yml`：

```yaml
version: "1.0"
name: 部署应用到 Web 节点
hosts:
  - web
become: true
tasks:
  - name: 检查环境
    command: echo "环境检查"

  - name: 拉取代码
    shell: |
      cd /opt/app
      git pull origin main

  - name: 安装依赖
    shell: npm install

  - name: 重启服务
    systemd:
      name: myapp
      state: restarted
```

### 4.2 验证剧本

```bash
owl playbook validate -f deploy.yml

# 预期输出
✓ 剧本验证通过
```

### 4.3 列出剧本

```bash
owl playbook list

# 预期输出
剧本名称                    | 文件路径              | 任务数
---------------------------|----------------------|--------
部署应用到 Web 节点         | ./deploy.yml         | 4
```

### 4.4 执行剧本

```bash
owl playbook run deploy.yml

# 预期输出
[1/4] 检查环境
[node1] ✓ 完成

[2/4] 拉取代码
[node1] ✓ 完成

[3/4] 安装依赖
[node1] ✓ 完成

[4/4] 重启服务
[node1] ✓ 完成

剧本执行完成: 4/4 成功
```

---

## 五、文件传输

### 5.1 上传文件

```bash
owl file upload app.tar.gz --nodes node1 --dest /opt/app/

# 预期输出
[node1] 上传: app.tar.gz -> /opt/app/app.tar.gz
[node1] ✓ 上传成功
```

### 5.2 下载文件

```bash
owl file download --nodes node1 --source /var/log/app.log --dest ./logs/

# 预期输出
[node1] 下载: /var/log/app.log -> ./logs/node1/app.log
[node1] ✓ 下载成功
```

### 5.3 自扩散传输（多节点）

```bash
owl file transfer app.tar.gz --nodes node1,node2,node3,node4,node5 --dest /opt/app/

# 预期输出
[node1] 作为种子节点
[扩散树] 层级 1: node1 -> node2, node3
[扩散树] 层级 2: node2 -> node4, node3 -> node5
✓ 传输完成
```

---

## 六、设置管理

### 6.1 查看当前设置

```bash
owl settings show

# 预期输出
AI 配置:
  Provider: openai
  Model: gpt-4o
  API Key: ************

其他设置:
  默认超时: 30m
  日志级别: info
```

### 6.2 更新设置

```bash
owl settings set ai.model gpt-4-turbo

# 预期输出
✓ 设置已更新: ai.model = gpt-4-turbo
```

### 6.3 重置设置

```bash
owl settings reset

# 预期输出
✓ 设置已重置为默认值
```

---

## 七、AI 助手

### 7.1 交互式模式

```bash
owl ai

# 预期输出
🦉 欢迎使用 AI 助手！
请输入您的运维指令...

> 在所有 web 节点上执行 uptime

正在执行: owl exec --nodes node1,node2 --command "uptime"
[node1] 结果: 12:00:00 up 100 days
[node2] 结果: 12:00:00 up 50 days
```

### 7.2 单次查询

```bash
owl ai "查看所有 web 节点的磁盘使用情况"

# 预期输出
正在执行: owl exec --nodes node1,node2 --command "df -h"
...
```

### 7.3 指定模型

```bash
owl ai --provider anthropic --model claude-3 "检查 node1 的服务状态"
```

---

## 八、会话历史

### 8.1 查看历史记录

```bash
owl history

# 预期输出
时间                | 类型     | 详情                           | 状态
--------------------|----------|--------------------------------|--------
2024-01-15 10:00   | exec     | node1: uptime                  | ✓
2024-01-15 10:05   | session  | node1 attach                   | ✓
2024-01-15 11:00   | exec     | node1,node2: df -h             | ✓
```

---

## 九、清理测试数据

### 9.1 退出会话

```bash
# 在会话中执行
exit

# 或在新终端
owl session list
# 找到需要关闭的会话 ID
owl session detach <session-id>
```

### 9.2 删除节点

```bash
owl node remove node1
owl node remove node2
owl node remove node3
owl node remove node4

# 预期输出
✓ 删除节点 node1
✓ 删除节点 node2
✓ 删除节点 node3
✓ 删除节点 node4
```

### 9.3 确认清理

```bash
owl node list

# 预期输出
ID | Name | Address | Port | User | Status
```

---

## 测试用例执行清单

| 序号 | 功能模块 | 测试项 | 状态 |
|------|----------|--------|------|
| 1 | 节点管理 | 添加节点（密码认证） | [ ] |
| 2 | 节点管理 | 添加节点（密钥认证） | [ ] |
| 3 | 节点管理 | 添加节点（非标准端口） | [ ] |
| 4 | 节点管理 | 添加节点（跳板机） | [ ] |
| 5 | 节点管理 | 列出节点 | [ ] |
| 6 | 节点管理 | 按分组/标签筛选 | [ ] |
| 7 | 节点管理 | 更新节点 | [ ] |
| 8 | 节点管理 | 分组管理 | [ ] |
| 9 | 节点管理 | 标签管理 | [ ] |
| 10 | 节点管理 | 导入导出 | [ ] |
| 11 | 节点管理 | 删除节点 | [ ] |
| 12 | 会话管理 | 单节点连接 | [ ] |
| 13 | 会话管理 | 多节点连接 | [ ] |
| 14 | 会话管理 | 会话历史 | [ ] |
| 15 | 命令执行 | 单节点命令 | [ ] |
| 16 | 命令执行 | 多节点命令 | [ ] |
| 17 | 命令执行 | 按分组执行 | [ ] |
| 18 | 剧本管理 | 验证剧本 | [ ] |
| 19 | 剧本管理 | 执行剧本 | [ ] |
| 20 | 文件传输 | 上传下载 | [ ] |
| 21 | 设置管理 | 查看/更新/重置 | [ ] |
| 22 | AI 助手 | 交互模式 | [ ] |
| 23 | 历史记录 | 查看历史 | [ ] |

---

## 注意事项

1. **测试前准备**：确保有可用的 SSH 测试节点，或使用本地 Docker 容器模拟
2. **敏感信息**：测试完成后删除节点和会话历史
3. **并行测试**：多节点测试需要至少 2 个可用节点
4. **网络要求**：确保测试环境网络畅通

## 故障排查

| 问题 | 可能原因 | 解决方案 |
|------|----------|----------|
| 连接失败 | SSH 密钥权限不对 | `chmod 600 ~/.ssh/id_rsa` |
| 连接失败 | 端口不正确 | 检查节点配置 `owl node list` |
| 命令执行超时 | 节点无响应 | 检查网络和节点状态 |
| 剧本执行失败 | YAML 格式错误 | 使用 `owl playbook validate` 验证 |
