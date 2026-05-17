# SSH 连接使用示例

## 场景 1: 基本密钥认证

### 1.1 准备 SSH 配置

```bash
# ~/.ssh/config
Host web1
    HostName 192.168.1.10
    User ubuntu
    IdentityFile ~/.ssh/id_rsa_web
```

### 1.2 添加节点（不指定用户，自动查找）

```bash
owl node add web1 \
    --name "Web Server 1" \
    --address 192.168.1.10 \
    --port 22
```

### 1.3 执行命令

```bash
owl exec --nodes web1 --command "uptime"
# 系统会自动使用 ~/.ssh/config 中的配置
# 即: ssh -l ubuntu -i ~/.ssh/id_rsa_web 192.168.1.10 uptime
```

## 场景 2: 覆盖 SSH config 配置

### 2.1 SSH config 配置了一个用户

```bash
# ~/.ssh/config
Host db1
    HostName 192.168.1.20
    User admin
    IdentityFile ~/.ssh/id_rsa_db
```

### 2.2 添加节点时指定不同用户

```bash
owl node add db1 \
    --name "DB Server 1" \
    --address 192.168.1.20 \
    --user deploy
```

### 2.3 执行命令

```bash
owl exec --nodes db1 --command "df -h"
# 实际执行: ssh -l deploy 192.168.1.20 df -h
# 忽略了 ~/.ssh/config 中的 admin 用户
```

## 场景 3: 通过 IP 地址连接

### 3.1 只有 IP 地址，没有别名

```bash
# ~/.ssh/config 中没有 web1，但有针对 IP 的配置
Host 192.168.1.*
    User ubuntu
    IdentityFile ~/.ssh/id_rsa_prod
```

### 3.2 添加节点

```bash
owl node add web-prod-1 \
    --name "Production Web 1" \
    --address 192.168.1.100 \
    --port 22
```

### 3.3 执行命令

```bash
owl exec --nodes web-prod-1 --command "nginx -v"
# 系统会匹配 192.168.1.* 通配符配置
# 使用: ssh -l ubuntu -i ~/.ssh/id_rsa_prod 192.168.1.100
```

## 场景 4: 使用跳板机

### 4.1 SSH config 配置跳板

```bash
Host prod-web-1
    HostName 10.0.1.10
    User ubuntu
    IdentityFile ~/.ssh/id_rsa_prod
    ProxyCommand ssh -W %h:%p bastion.example.com
```

### 4.2 添加节点

```bash
owl node add prod-web-1 \
    --name "Production Web 1" \
    --address 10.0.1.10
```

### 4.3 执行命令

```bash
owl exec --nodes prod-web-1 --command "systemctl status nginx"
# 系统会使用 ProxyCommand 配置通过跳板机连接
```

## 场景 5: 本地节点

### 5.1 添加本地测试节点

```bash
owl node add local-test \
    --name "Local Test" \
    --address 127.0.0.1 \
    --port 22
```

### 5.2 执行命令

```bash
owl exec --nodes local-test --command "docker ps"
# 识别为本地节点，直接执行: /bin/sh -c "docker ps"
# 不会使用 SSH 连接
```

## 场景 6: 不同环境使用不同密钥

### 6.1 配置多环境 SSH

```bash
# ~/.ssh/config

# 开发环境
Host dev-*
    User developer
    IdentityFile ~/.ssh/id_rsa_dev
    StrictHostKeyChecking no

# 生产环境
Host prod-*
    User ubuntu
    IdentityFile ~/.ssh/id_rsa_prod
    StrictHostKeyChecking no
```

### 6.2 添加不同环境的节点

```bash
# 开发环境
owl node add dev-web-1 --name "Dev Web" --address 10.10.1.10
owl node add dev-db-1 --name "Dev DB" --address 10.10.1.20

# 生产环境
owl node add prod-web-1 --name "Prod Web" --address 192.168.1.10
owl node add prod-db-1 --name "Prod DB" --address 192.168.1.20
```

### 6.3 批量执行

```bash
# 只在开发环境执行
owl exec --nodes dev-web-1,dev-db-1 --command "uptime"

# 只在生产环境执行
owl exec --nodes prod-web-1,prod-db-1 --command "df -h"
```

## 场景 7: 调试连接问题

### 7.1 查看节点将使用什么配置

```go
// 使用代码查看
factory := ssh.NewNodeExecutorFactory()
config, found := factory.GetSSHConfigForNode("web1", "192.168.1.10")

if found {
    fmt.Printf("找到配置: User=%s, Key=%s\n", config.User, config.IdentityFile)
} else {
    fmt.Println("未找到 SSH config 配置")
}
```

### 7.2 模拟连接参数

```go
connInfo, err := ssh.ResolveConnection(
    "web1",           // nodeID
    "192.168.1.10",   // nodeAddress
    22,               // nodePort
    "",               // nodeUser (空表示使用 SSH config)
    "",               // sshConfigPath (空表示 ~/.ssh/config)
)

if err == nil {
    args := connInfo.BuildSSHCommand("echo test")
    fmt.Println("SSH 命令:", "ssh", strings.Join(args, " "))
}
```

### 7.3 手动测试

```bash
# 从 SSH config 查看配置
ssh -G web1

# 模拟连接
ssh -v -l ubuntu -i ~/.ssh/id_rsa_web 192.168.1.10
```

## 最佳实践

### 1. 使用 Host 别名

在 `~/.ssh/config` 中使用有意义的别名：

```bash
Host web-prod-1
    HostName 192.168.1.10
    User ubuntu
    IdentityFile ~/.ssh/id_rsa_prod
```

### 2. 使用通配符简化配置

```bash
# 所有生产服务器
Host prod-*
    User ubuntu
    IdentityFile ~/.ssh/id_rsa_prod

# 所有内网 IP
Host 10.0.*.*
    User admin
    IdentityFile ~/.ssh/id_rsa_internal
```

### 3. 设置合理的 SSH 超时

在连接时设置适当的超时时间：

```bash
owl exec --nodes web1 --command "uptime" --timeout 30s
```

### 4. 分离不同环境的密钥

```bash
# 生产环境
~/.ssh/id_rsa_prod (600)
~/.ssh/id_rsa_prod.pub

# 开发环境
~/.ssh/id_rsa_dev (600)
~/.ssh/id_rsa_dev.pub
```

### 5. 验证配置加载

如果连接失败，先验证 SSH config 是否正确加载：

```bash
# 检查配置是否被正确解析
cat ~/.ssh/config | grep -A 5 "Host web1"
```
