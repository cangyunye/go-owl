# SSH 配置集成

go-owl 支持智能 SSH 连接，可以自动读取 `~/.ssh/config` 文件来查找密钥和连接配置。

## 功能特性

- ✅ 自动解析 `~/.ssh/config` 文件
- ✅ 根据主机名/IP 地址智能匹配 SSH 配置
- ✅ 优先使用密钥认证
- ✅ 支持自定义 SSH config 路径
- ✅ 支持节点指定 SSH 用户

## 连接优先级

当连接到一个节点时，系统按以下优先级选择连接方式：

1. **节点配置的用户** - 如果添加节点时指定了 `--user`，优先使用
2. **SSH config 中的配置** - 查找匹配的 Host 条目
   - 按节点 ID 匹配
   - 按节点地址匹配
   - 按 HostName 匹配
3. **当前用户名** - 使用运行程序的用户

## SSH Config 示例

### 基本密钥认证

```bash
# ~/.ssh/config

Host web-server-1
    HostName 192.168.1.10
    User ubuntu
    IdentityFile ~/.ssh/id_rsa_web
    Port 22

Host db-server-1
    HostName 192.168.1.20
    User admin
    IdentityFile ~/.ssh/id_rsa_db
    Port 22
```

### 使用跳板机

```bash
Host web-server
    HostName 192.168.1.10
    User deploy
    IdentityFile ~/.ssh/id_rsa
    ProxyCommand ssh -W %h:%p jumphost.example.com
```

### 批量主机别名

```bash
Host web-*
    User ubuntu
    IdentityFile ~/.ssh/id_rsa_production
    StrictHostKeyChecking no

Host 192.168.1.*
    User admin
    IdentityFile ~/.ssh/id_rsa_internal
```

## 使用方法

### 添加节点时指定用户

```bash
owl node add web1 --name web-server-1 --address 192.168.1.10 --user ubuntu
```

### 不指定用户（自动查找 SSH config）

```bash
owl node add web1 --name web-server-1 --address 192.168.1.10
# 系统会自动在 ~/.ssh/config 中查找 192.168.1.10 或 web1 的配置
```

### 使用自定义 SSH config

```bash
owl node add web1 --name web-server-1 --address 192.168.1.10 \
    --ssh-config /path/to/custom_config
```

## 实现细节

### 核心组件

- [internal/ssh/ssh_config.go](internal/ssh/ssh_config.go) - SSH config 解析器
- [internal/ssh/connection_manager.go](internal/ssh/connection_manager.go) - 连接信息管理器
- [internal/ssh/executor_factory.go](internal/ssh/executor_factory.go) - 执行器工厂

### 支持的配置项

| SSH Config 项 | 支持 |
|--------------|------|
| Host | ✅ |
| HostName | ✅ |
| User | ✅ |
| Port | ✅ |
| IdentityFile | ✅ |
| ProxyCommand | ✅ |
| ForwardAgent | ✅ |

### 本地节点识别

以下地址被认为是本地节点，直接使用本地 shell 执行：

- `127.0.0.1`
- `localhost`
- `::1`
- `0.0.0.0`

## 故障排除

### 密钥权限问题

确保 SSH 私钥权限正确：

```bash
chmod 600 ~/.ssh/id_rsa
chmod 700 ~/.ssh
```

### 主机密钥检查

首次连接时，系统会自动接受主机密钥。如果遇到问题，检查：

```bash
# 查看 SSH 配置
ssh -G 192.168.1.10

# 手动测试连接
ssh -v 192.168.1.10
```
