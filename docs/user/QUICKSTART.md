# 快速入门

本指南帮助您快速上手 go-owl。

---

## 1. 安装

```bash
# 下载二进制
curl -L https://github.com/cangyunye/go-owl/releases/latest/download/owl-linux-amd64 -o owl
chmod +x owl
sudo mv owl /usr/local/bin/

# 验证安装
owl version
```

---

## 2. 添加节点

```bash
# 添加单个节点
owl node add web-01 \
  --name "Web Server 1" \
  --address 192.168.1.10 \
  --user root

# 带分组和标签
owl node add web-02 \
  --address 192.168.1.11 \
  --user root \
  --groups web,production \
  --labels env=prod
```

---

## 3. 检查节点

```bash
# 查看节点列表
owl node list

# Ping 检查可达性
owl node ping web-01

# SSH 连接检查并更新状态
owl node check --all --update
```

---

## 4. 执行命令

```bash
# 在单个节点执行
owl exec run "uptime" --nodes web-01

# 在分组执行
owl exec run "df -h" --group web

# 多个节点并行执行
owl exec run "systemctl status nginx" --nodes web-01,web-02,web-03
```

---

## 5. 文件传输

```bash
# 上传文件
owl file upload ./app.tar.gz --nodes web-01 --dest /opt/

# 下载文件
owl file download /var/log/app.log --node web-01 --dest ./logs/
```

---

## 6. Playbook 剧本

```yaml
# deploy.yml
name: Deploy Application
hosts:
  - web

tasks:
  - name: Stop service
    action: shell systemctl stop myapp

  - name: Deploy files
    action: copy
      src: ./app/
      dest: /opt/myapp/

  - name: Start service
    action: shell systemctl start myapp
```

```bash
# 执行剧本
owl playbook run deploy.yml --nodes web-01
```

---

## 7. AI 助手

```bash
# 交互式对话
owl ai

# 单次请求
owl ai "在所有 web 节点上执行 df -h"
```

---

## 8. 常用命令速查

| 操作 | 命令 |
|------|------|
| 添加节点 | `owl node add <id> --address <ip>` |
| 列出会话 | `owl node list` |
| 执行命令 | `owl exec run "<cmd>" --nodes <id>` |
| 上传文件 | `owl file upload <file> --nodes <id>` |
| 下载文件 | `owl file download <path> --node <id>` |
| 执行剧本 | `owl playbook run <file>` |
| AI 对话 | `owl ai` |

---

## 下一步

- 查看 [NODE.md](NODE.md) 了解节点管理详情
- 查看 [EXEC.md](EXEC.md) 了解命令执行详情
- 查看 [PLAYBOOK.md](PLAYBOOK.md) 了解剧本功能
- 查看 [FILE.md](FILE.md) 了解文件传输
