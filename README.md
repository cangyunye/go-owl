# go-owl - 智能分布式运维工具

## 🦉 项目简介

**go-owl**（分布式运维工具箱，为你带来智能运维而生。

## ✨ 特性

- 🚀 **节点管理**: 注册、监控和管理多节点，支持分组和标签
- 💻 **批量命令执行**: 在多个节点上同时执行 Shell 命令
- 📜 **脚本传输执行**: 批量上传和执行自定义脚本
- 📋 **Ansible-like 剧本**: 支持 Ansible 风格的 YAML 剧本编排
- 📁 **文件传输**: 支持直接和自扩散传输（P2P 模式）
- 🤖 **AI 助手**: 自然语言驱动的智能运维操作
- 🔒 **安全设计**: 内置安全检查和危险命令识别
- 🖥️ **交互式会话**: 支持单节点实时交互和多节点批量管理
- 📊 **会话历史**: 完整的会话和命令记录，可以随时查看
- 🔑 **SSH 配置集成**: 自动检测和使用 `~/.ssh/config`

## 📦 安装

### 从源码构建

```bash
git clone https://github.com/cangyunye/go-owl.git
cd go-owl
go build -o owl ./cmd/cli/main.go
```

或者直接运行：

```bash
go install github.com/cangyunye/go-owl/cmd/cli@latest
```

## 🎉 快速开始

### 🐣 1. 节点管理

```bash
# 添加节点
owl node add --name web1 --address 192.168.1.10 --port 22
owl node add --name db1 --address 192.168.1.20 --port 22

# 查看所有节点
owl node list

# 添加分组和标签
owl node group add web1 --nodes web1,web2
```

### 📊 2. 批量执行命令

```bash
# 在所有节点执行命令
owl exec --command "uptime"

# 在指定节点执行
owl exec --nodes web1,web2 --command "df -h"

# 按分组执行
owl exec --group web --command "systemctl status nginx"
```

### 📜 3. 剧本执行

编写一个 YAML 剧本：

```yaml
# deploy.yml
- name: 部署应用
  hosts: web
  become: yes
  tasks:
    - name: 安装依赖
      shell: apt-get install -y nginx
    - name: 启动服务
      systemd:
        name: nginx
        state: started
```

执行剧本：

```bash
owl playbook run deploy.yml
```

### 📁 4. 文件传输

```bash
# 简单上传
owl file upload app.tar.gz --nodes web1,web2 --dest /opt/app/

# 自扩散传输（多节点时自动使用）
owl file transfer app.tar.gz --nodes web1,web2,web3,web4,web5 --dest /opt/app/
```

### 🤖 5. AI 助手

```bash
# 交互式模式
owl ai

# 单次查询
owl ai "在所有 web 节点上执行 uptime"
```

### 🖥️ 6. 交互式会话

```bash
# 单节点实时交互
owl session attach root@192.168.1.10

# 多节点批量管理
owl session attach --nodes web1,web2,web3

# 查看会话历史
owl session history
```

#### AI 助手优化说明

最近 AI 助手进行了重要优化，确保自然语言解析严格映射到 4 种操作：

1. **查询节点信息** - 查看节点状态、分组、标签
2. **执行命令** - 在指定节点上运行 shell 命令  
3. **生成并执行剧本** - 自动化部署操作
4. **传输文件** - 向节点分发文件

#### 智能意图识别

- 无法确定意图时，提供友好帮助
- 关键词精准识别
- 参数自动提取
- 严格参数验证

更多 AI 助手详细使用方法请参考 [docs/USAGE.md](docs/USAGE.md)

更多会话功能详细使用方法请参考 [docs/SESSION_USAGE.md](docs/SESSION_USAGE.md)

## 数据库配置

go-owl 支持 DuckDB 和 SQLite3 两种嵌入式数据库，通过编译时构建标签选择。详见 [docs/DATABASE.md](docs/DATABASE.md)。

### 💡 LLM 实现说明

本项目的 AI 模块采用**自定义轻量级实现**，未使用第三方 AI 框架（如 cloudwego/eino），原因如下：

**为什么不使用 cloudwego/eino？**

- **依赖冲突**：eino 库与其他依赖项存在版本兼容性问题
- **构建复杂性**：引入额外的框架会增加项目构建和依赖管理的复杂度
- **功能匹配**：项目仅需基础的 LLM 调用功能，无需完整的 AI Agent 框架能力

**自定义实现的优势：**

- ✅ **零外部依赖**：避免版本冲突，确保项目稳定构建
- ✅ **功能精简**：只实现必要的 LLM 调用接口
- ✅ **易于维护**：代码结构清晰，调试和扩展简单
- ✅ **完全可控**：无隐藏依赖，便于排查问题

**支持的 LLM 提供商：**

- OpenAI (GPT 系列)
- Anthropic (Claude 系列)
- Qwen (阿里通义千问)
- DeepSeek

所有提供商均通过统一的接口调用，支持流式输出和上下文管理。

## 🔧 使用示例

### 配置文件

配置文件默认位置：`~/.owl/config.yml`

#### 1. OpenAI
```yaml
ai:
  provider: openai
  model: gpt-4o
  api-key: your-openai-api-key
  base-url: https://api.openai.com/v1
  timeout: 120
```

#### 2. Anthropic
```yaml
ai:
  provider: anthropic
  model: claude-3-opus-20240229
  api-key: your-anthropic-api-key
  timeout: 120
```

#### 3. Qwen (阿里通义千问)
```yaml
ai:
  provider: qwen
  model: qwen-turbo
  api-key: your-dashscope-api-key
  base-url: https://dashscope.aliyuncs.com/compatible-mode/v1
  timeout: 120
```

#### 4. DeepSeek
```yaml
ai:
  provider: deepseek
  model: deepseek-chat
  api-key: your-deepseek-api-key
  base-url: https://api.deepseek.com
  timeout: 120
```

**或使用环境变量**：
```bash
export OWL_API_KEY=your-api-key
export OWL_BASE_URL=https://your-proxy-endpoint
owl ai --provider openai --model gpt-4o
```

## 架构图 (O
## 📚 详细文档

- [节点管理]
- [命令执行]
- [剧本编写]
- [文件传输]
- [AI 助手]

## 🤝 贡献

欢迎任何形式的贡献！

## 📄 许可证

本项目采用 **MIT License** 开源许可证，详见 [LICENSE](LICENSE) 文件。
