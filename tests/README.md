# go-owl 自动化测试方案

## 1. 测试架构概述

### 1.1 测试策略
- **内部功能测试**：使用 Go 标准 `testing` 包
- **命令调用测试**：使用 Bash 脚本
- **测试节点**：用户提供，通过配置文件指定

### 1.2 测试目录结构
```
go-owl/
├── tests/
│   ├── README.md                 # 本文档
│   ├── testdata/                 # 测试数据
│   │   ├── nodes.yaml           # 测试节点配置
│   │   ├── scripts/             # 测试脚本
│   │   │   ├── setup.sh
│   │   │   ├── cleanup.sh
│   │   │   └── test-script.sh
│   │   └── playbooks/           # 测试用 playbook
│   │       ├── test-command.yaml
│   │       └── test-deploy.yaml
│   ├── unit/                    # 单元测试 (Go)
│   │   └── *.go
│   ├── integration/             # 集成测试 (Go)
│   │   └── *.go
│   └── scripts/                 # Bash 测试脚本
│       ├── test-exec.sh
│       ├── test-node.sh
│       ├── test-playbook.sh
│       └── test-history.sh
│
├── Makefile                     # 测试执行脚本
│
└── docs/
    └── design/
        └── TEST_DESIGN.md       # 本文档
```

---

## 2. 测试节点配置

### 2.1 配置文件位置
**用户需要创建**: `~/.owl/test-nodes.yaml`

### 2.2 配置格式
```yaml
# ~/.owl/test-nodes.yaml
nodes:
  - id: self-test-1
    name: "测试节点1"
    address: "192.168.1.100"
    port: 22
    user: "testuser"
    ssh_key: "~/.ssh/id_rsa_test"
    groups: ["test"]
    labels:
      type: "ssh"
      env: "test"
      
  - id: self-test-2
    name: "测试节点2"
    address: "192.168.1.101"
    port: 22
    user: "testuser"
    ssh_key: "~/.ssh/id_rsa_test"
    groups: ["test"]
    labels:
      type: "ssh"
      env: "test"
```

### 2.3 配置说明

| 字段 | 必填 | 说明 | 示例 |
|------|------|------|------|
| `id` | ✅ | 节点唯一标识 | `self-test-1` |
| `name` | ❌ | 节点显示名称 | `"测试节点1"` |
| `address` | ✅ | 节点地址 | `192.168.1.100` 或 `127.0.0.1` |
| `port` | ❌ | SSH 端口，默认 22 | `22` |
| `user` | ✅ | SSH 用户名 | `testuser` |
| `ssh_key` | ❌ | SSH 私钥路径 | `~/.ssh/id_rsa` |
| `groups` | ❌ | 节点分组 | `["test", "web"]` |
| `labels` | ❌ | 节点标签 | `{type: ssh, env: test}` |

### 2.4 测试节点要求

**必需条件**：
- ✅ SSH 连接可用
- ✅ 用户有执行基本命令的权限（`bash`, `echo`, `cat`, `mkdir`, `rm` 等）
- ✅ 能够创建临时目录（`/tmp/` 或其他目录）
- ✅ 网络可达

**建议配置**：
- 关闭 StrictHostKeyChecking（首次连接时避免 prompt）
- 配置 SSH Key 免密登录
- 确保测试目录有写入权限

---

## 3. Go 单元测试 (tests/unit/)

### 3.1 测试文件命名规范
- 文件名以 `_test.go` 结尾
- 示例：`command_test.go`, `executor_test.go`

### 3.2 测试用例结构
```go
// tests/unit/command_test.go
package unit

import (
    "testing"
    "github.com/cangyunye/go-owl/internal/control/command"
)

func TestCommandExecutor(t *testing.T) {
    tests := []struct {
        name     string
        command  string
        wantErr  bool
    }{
        {
            name:    "测试简单命令",
            command: "echo hello",
            wantErr: false,
        },
        {
            name:    "测试带管道的命令",
            command: "echo hello | cat",
            wantErr: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // 测试逻辑
            if tt.wantErr {
                t.Error("预期错误但没有发生")
            }
        })
    }
}
```

### 3.3 常用测试用例

#### 3.3.1 命令解析测试
```go
// tests/unit/command_parser_test.go
func TestParseNodeList(t *testing.T) {
    tests := []struct {
        input    string
        expected []string
    }{
        {input: "node1", expected: []string{"node1"}},
        {input: "node1,node2", expected: []string{"node1", "node2"}},
        {input: "node1, node2, node3", expected: []string{"node1", "node2", "node3"}},
    }
    
    for _, tt := range tests {
        t.Run(tt.input, func(t *testing.T) {
            result := parseNodeList(tt.input)
            if !reflect.DeepEqual(result, tt.expected) {
                t.Errorf("expected %v, got %v", tt.expected, result)
            }
        })
    }
}
```

#### 3.3.2 变量替换测试
```go
// tests/unit/variable_test.go
func TestVariableInterpolation(t *testing.T) {
    tests := []struct {
        name     string
        template string
        vars     map[string]string
        expected string
    }{
        {
            name:     "简单变量",
            template: "echo {{name}}",
            vars:     map[string]string{"name": "world"},
            expected: "echo world",
        },
        {
            name:     "多个变量",
            template: "{{greeting}} {{name}}!",
            vars:     map[string]string{"greeting": "Hello", "name": "World"},
            expected: "Hello World!",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := interpolate(tt.template, tt.vars)
            if result != tt.expected {
                t.Errorf("expected %q, got %q", tt.expected, result)
            }
        })
    }
}
```

---

## 4. Go 集成测试 (tests/integration/)

### 4.1 测试文件结构
```go
// tests/integration/exec_test.go
package integration

import (
    "testing"
    "os/exec"
    "strings"
)

func TestExecRun(t *testing.T) {
    // 跳过测试如果测试节点不可用
    if os.Getenv("OWL_TEST_ENABLED") != "true" {
        t.Skip("跳过集成测试（OWL_TEST_ENABLED != true）")
    }
    
    tests := []struct {
        name     string
        command  string
        args     []string
        wantErr  bool
    }{
        {
            name:    "测试简单命令",
            command: "owl",
            args:    []string{"exec", "run", "echo hello", "--nodes", "self-test-1"},
            wantErr: false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd := exec.Command(tt.command, tt.args...)
            output, err := cmd.CombinedOutput()
            
            if tt.wantErr && err == nil {
                t.Error("预期错误但没有发生")
            }
            if !tt.wantErr && err != nil {
                t.Errorf("执行失败: %v\nOutput: %s", err, output)
            }
        })
    }
}
```

### 4.2 测试用例覆盖

#### 4.2.1 命令执行测试
```go
func TestExecRunCommand(t *testing.T)     // 测试基本命令执行
func TestExecRunMultipleNodes(t *testing.T) // 测试多节点执行
func TestExecRunTimeout(t *testing.T)      // 测试超时处理
func TestExecRunParallel(t *testing.T)     // 测试并行执行
func TestExecRunSerial(t *testing.T)      // 测试串行执行
```

#### 4.2.2 脚本执行测试
```go
func TestScriptFileMode(t *testing.T)     // 测试文件模式
func TestScriptInlineMode(t *testing.T)   // 测试内联模式
func TestScriptWithArgs(t *testing.T)     // 测试带参数
func TestScriptKeepFile(t *testing.T)     // 测试保留文件
```

#### 4.2.3 文件传输测试
```go
func TestUploadFile(t *testing.T)        // 测试上传
func TestDownloadFile(t *testing.T)      // 测试下载
func TestUploadOverwrite(t *testing.T)   // 测试覆盖
func TestDownloadSubdir(t *testing.T)    // 测试子目录
```

#### 4.2.4 历史记录测试
```go
func TestHistoryRecord(t *testing.T)     // 测试记录
func TestHistoryQuery(t *testing.T)      // 测试查询
func TestHistoryClear(t *testing.T)       // 测试清理
```

---

## 5. Bash 测试脚本 (tests/scripts/)

### 5.1 脚本结构规范
```bash
#!/bin/bash
# tests/scripts/test-exec.sh

set -e  # 遇到错误立即退出
set -u  # 使用未定义的变量时报错

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# 测试计数器
PASSED=0
FAILED=0

# 测试函数
test_exec_run() {
    local name="$1"
    local command="$2"
    local expected="$3"
    
    echo -n "Testing: $name ... "
    
    output=$(owl exec run "$command" --nodes self-test-1 2>&1)
    
    if echo "$output" | grep -q "$expected"; then
        echo -e "${GREEN}PASS${NC}"
        ((PASSED++))
    else
        echo -e "${RED}FAIL${NC}"
        echo "Expected: $expected"
        echo "Got: $output"
        ((FAILED++))
    fi
}

# 测试用例
echo "========================================="
echo "owl exec 命令测试"
echo "========================================="

test_exec_run "简单命令" "echo hello" "hello"
test_exec_run "多节点命令" "uptime" "load"
test_exec_run "文件列表" "ls -la /tmp" "/tmp"

# 结果总结
echo "========================================="
echo "测试结果: $PASSED 通过, $FAILED 失败"
echo "========================================="

[ $FAILED -eq 0 ]
```

### 5.2 测试脚本列表

| 脚本 | 用途 | 测试内容 |
|------|------|---------|
| `test-exec.sh` | exec 命令测试 | run, script, upload, download |
| `test-node.sh` | node 命令测试 | list, add, delete |
| `test-playbook.sh` | playbook 测试 | run, validate |
| `test-history.sh` | history 命令测试 | show, query |
| `test-settings.sh` | settings 命令测试 | show, set |

### 5.3 测试脚本示例：test-exec.sh

```bash
#!/bin/bash
# tests/scripts/test-exec.sh
# 测试 owl exec 命令

set -e
set -u

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASSED=0
FAILED=0
SKIPPED=0

log_info() { echo -e "${YELLOW}[INFO]${NC} $1"; }
log_pass() { echo -e "${GREEN}[PASS]${NC} $1"; ((PASSED++)); }
log_fail() { echo -e "${RED}[FAIL]${NC} $1"; ((FAILED++)); }
log_skip() { echo -e "${YELLOW}[SKIP]${NC} $1"; ((SKIPPED++)); }

cleanup() {
    log_info "清理测试环境..."
    owl exec run "rm -rf /tmp/owl-test" --nodes self-test-1,self-test-2 2>/dev/null || true
}

# 检查测试环境
check_env() {
    log_info "检查测试环境..."
    
    # 检查 owl 命令
    if ! command -v owl &> /dev/null; then
        log_fail "owl 命令未找到，请先构建项目"
        exit 1
    fi
    
    # 检查测试节点
    if ! owl exec run "echo ok" --nodes self-test-1 &> /dev/null; then
        log_fail "测试节点 self-test-1 不可达"
        exit 1
    fi
    
    log_pass "测试环境检查通过"
}

# 测试 exec run
test_exec_run() {
    local name="$1"
    local command="$2"
    local nodes="${3:-self-test-1}"
    local expected="$4"
    
    echo ""
    log_info "测试: $name"
    
    if output=$(owl exec run "$command" --nodes "$nodes" 2>&1); then
        if echo "$output" | grep -q "$expected"; then
            log_pass "$name"
        else
            log_fail "$name (输出不匹配)"
            echo "Expected: $expected"
            echo "Got: $output"
        fi
    else
        log_fail "$name (命令失败)"
        echo "Output: $output"
    fi
}

# 测试 exec script
test_exec_script() {
    local name="$1"
    local script="$2"
    local nodes="${3:-self-test-1}"
    local inline="${4:-false}"
    
    echo ""
    log_info "测试: $name"
    
    local cmd="owl exec script $script --nodes $nodes"
    if [ "$inline" = "true" ]; then
        cmd="$cmd --inline"
    fi
    
    if output=$(eval "$cmd" 2>&1); then
        log_pass "$name"
    else
        log_fail "$name"
        echo "Output: $output"
    fi
}

# 主测试流程
main() {
    echo "========================================="
    echo "owl exec 命令测试套件"
    echo "========================================="
    
    cleanup
    check_env
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 exec run"
    echo "-----------------------------------------"
    
    test_exec_run "简单命令" "echo hello" "self-test-1" "hello"
    test_exec_run "多节点命令" "uptime" "self-test-1" "load"
    test_exec_run "创建目录" "mkdir -p /tmp/owl-test" "self-test-1" ""
    test_exec_run "串行模式" "echo serial" "self-test-1" "serial"
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 exec script"
    echo "-----------------------------------------"
    
    test_exec_script "脚本执行(文件模式)" "./tests/testdata/scripts/test-script.sh"
    test_exec_script "脚本执行(内联模式)" "./tests/testdata/scripts/test-script.sh" "self-test-1" "true"
    
    echo ""
    echo "========================================="
    echo "测试结果总结"
    echo "========================================="
    echo -e "通过: ${GREEN}$PASSED${NC}"
    echo -e "失败: ${RED}$FAILED${NC}"
    echo -e "跳过: ${YELLOW}$SKIPPED${NC}"
    echo "========================================="
    
    cleanup
    
    if [ $FAILED -eq 0 ]; then
        echo -e "${GREEN}所有测试通过！${NC}"
        exit 0
    else
        echo -e "${RED}有 $FAILED 个测试失败${NC}"
        exit 1
    fi
}

main "$@"
```

---

## 6. Makefile 测试脚本

### 6.1 Makefile 内容
```makefile
.PHONY: help test test-unit test-integration test-e2e test-all test-clean test-quick

# 帮助信息
help:
	@echo "go-owl 测试命令"
	@echo ""
	@echo "可用命令:"
	@echo "  make test-all          - 运行所有测试"
	@echo "  make test-unit         - 运行单元测试"
	@echo "  make test-integration  - 运行集成测试"
	@echo "  make test-e2e         - 运行端到端测试"
	@echo "  make test-quick       - 快速测试（跳过耗时测试）"
	@echo "  make test-clean       - 清理测试环境"
	@echo ""

# 设置环境变量
export OWL_TEST_ENABLED := true
export OWL_TEST_NODES := self-test-1,self-test-2

# 运行所有测试
test-all: test-unit test-integration test-e2e

# 单元测试
test-unit:
	@echo "运行单元测试..."
	@go test -v ./tests/unit/...

# 集成测试
test-integration:
	@echo "检查测试节点..."
	@owl exec run "echo ok" --nodes $(OWL_TEST_NODES) || (echo "测试节点不可用，请检查配置"; exit 1)
	@echo "运行集成测试..."
	@go test -v ./tests/integration/...

# 端到端测试
test-e2e:
	@echo "运行 Bash 脚本测试..."
	@bash tests/scripts/test-exec.sh
	@bash tests/scripts/test-node.sh
	@bash tests/scripts/test-playbook.sh
	@bash tests/scripts/test-history.sh

# 快速测试
test-quick:
	@echo "运行快速测试..."
	@OWL_TEST_QUICK=true go test -v ./tests/unit/...
	@bash tests/scripts/test-exec.sh

# 清理测试环境
test-clean:
	@echo "清理测试环境..."
	@owl exec run "rm -rf /tmp/owl-test" --nodes $(OWL_TEST_NODES) 2>/dev/null || true
	@echo "清理完成"

# 测试覆盖率
test-coverage:
	@echo "生成测试覆盖率报告..."
	@go test -coverprofile=coverage.out ./tests/unit/...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "报告已生成: coverage.html"
```

---

## 7. 用户配置指南

### 7.1 快速开始

**步骤 1：创建测试节点配置**
```bash
mkdir -p ~/.owl
vim ~/.owl/test-nodes.yaml
```

**步骤 2：写入配置内容**
```yaml
nodes:
  - id: self-test-1
    name: "测试节点1"
    address: "你的节点地址"
    port: 22
    user: "你的用户名"
    ssh_key: "~/.ssh/id_rsa"
```

**步骤 3：测试连接**
```bash
owl exec run "echo hello" --nodes self-test-1
```

**步骤 4：运行测试**
```bash
cd go-owl
make test-unit        # 单元测试
make test-integration # 集成测试
make test-e2e        # 端到端测试
```

### 7.2 常见问题

**Q: 测试节点需要什么权限？**
A: 基本执行权限即可，不需要 root。

**Q: 可以使用 localhost 吗？**
A: 可以，只要 SSH 服务运行且配置了免密登录。

**Q: 如何跳过某些测试？**
A: 设置环境变量 `OWL_TEST_QUICK=true`。

**Q: 测试失败了怎么办？**
A: 使用 `--debug` 选项查看详细错误信息。

---

## 8. 持续集成（可选）

### 8.1 CI 的作用

| 场景 | 没有 CI | 有 CI |
|------|--------|------|
| 代码提交 | 手动运行测试 | 自动运行测试 |
| PR 合并 | 人工检查 | 自动检查 |
| 问题发现 | 上线后发现 | 提交时发现 |
| 团队协作 | 难以保证代码质量 | 自动保证 |

### 8.2 CI 配置示例

如果您决定使用 CI，可以在 `.github/workflows/test.yml` 中配置：

```yaml
name: Tests

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
      - uses: actions/checkout@v3
      
      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Build
        run: go build -o owl ./cmd/cli
      
      - name: Setup SSH
        run: |
          eval $(ssh-agent -s)
          echo "${{ secrets.SSH_KEY }}" | tr -d '\r' | ssh-add -
          
      - name: Configure Test Nodes
        run: |
          mkdir -p ~/.owl
          echo "${{ secrets.TEST_NODES_CONFIG }}" > ~/.owl/test-nodes.yaml
          
      - name: Run Tests
        run: |
          export OWL_TEST_ENABLED=true
          export OWL_TEST_NODES=self-test-1,self-test-2
          make test-all
```

**注意**：CI 配置是可选的，对于个人项目来说不是必需的。

---

## 9. 测试最佳实践

### 9.1 测试命名规范
- 测试函数：`Test` 开头
- 测试用例：描述性名称，如 `TestExecRunMultipleNodes`
- 使用 Go 惯例：`func TestSomething(t *testing.T)`

### 9.2 测试组织
- 每个功能模块一个测试文件
- 相关测试放在同一个 package
- 使用 table-driven tests 减少重复代码

### 9.3 测试隔离
- 每个测试前后清理环境
- 使用唯一的测试数据（如带时间戳）
- 不依赖测试执行顺序

### 9.4 错误处理
- 预期错误要明确标记
- 提供有意义的错误信息
- 包含实际值和期望值

---

## 10. 下一步

确认此方案后，我将创建：

1. ✅ 完整的测试目录结构
2. ✅ Go 单元测试示例（command, variable）
3. ✅ Go 集成测试示例（exec, history）
4. ✅ Bash 测试脚本（test-exec.sh）
5. ✅ Makefile 测试脚本
6. ✅ 测试数据文件

您确认可以开始实现了吗？
