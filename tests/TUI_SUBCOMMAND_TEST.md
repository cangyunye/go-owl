# owl tui 子命令集成测试文档

## 概述
本文档描述了为 go-owl 添加 `owl tui` 子命令的测试内容，该命令允许用户通过 `owl tui` 直接调用 owl-tui 功能。

## 测试环境
- 操作系统: Linux
- Go 版本: 1.25.0
- 项目路径:
  - go-owl: /workspace/go-owl
  - go-owl-tui: /workspace/go-owl-tui

## 功能说明
新增的 `owl tui` 子命令具有以下功能：
1. 查找已安装的 owl-tui 可执行文件
2. 在 PATH 中搜索 owl-tui
3. 在 owl 可执行文件的同级目录搜索
4. 在相对路径（如与 go-owl 同级的 go-owl-tui 目录）搜索
5. 找到后直接执行 owl-tui 并传递所有参数

## 测试案例

### 测试案例 1: 验证 owl 命令帮助信息包含 tui 子命令
**命令**:
```bash
/tmp/owl-test/owl --help
```

**预期结果**:
输出中应包含以下内容：
```
Available Commands:
  ...
  tui         启动交互式终端用户界面
  ...
```

**测试结果**: ✓ 通过

---

### 测试案例 2: 验证 owl tui --help 显示正确信息
**命令**:
```bash
/tmp/owl-test/owl tui --help
```

**预期结果**:
应显示 tui 子命令的详细帮助信息，包括功能说明和使用示例。

**测试结果**: ✓ 通过

---

### 测试案例 3: 验证 owl 可以找到同目录下的 owl-tui
**前置条件**:
- owl 和 owl-tui 可执行文件在同一个目录
- 目录: /tmp/owl-test/

**测试步骤**:
1. 确认目录内容: `ls -la /tmp/owl-test/`
2. 执行 owl tui 命令: `/tmp/owl-test/owl tui`

**预期结果**:
owl 能够找到并执行同目录下的 owl-tui。

**测试结果**: ✓ 通过

---

### 测试案例 4: 验证参数传递
**测试步骤**:
1. 使用任意参数调用 owl tui，例如: `/tmp/owl-test/owl tui --test`

**预期结果**:
owl-tui 应能接收到所有传递的参数。

**测试结果**: ✓ 通过

---

## 文件变更列表

### 1. /workspace/go-owl/cmd/cli/cmd/tui/tui.go (新建)
- 新建 tui 子命令包
- 实现了 NewTuiCmd() 函数创建 tui 命令
- 实现了 findTuiExecutable() 函数查找 owl-tui
- 实现了 runTui() 函数执行 owl-tui

### 2. /workspace/go-owl/cmd/cli/cmd/root.go (修改)
- 导入了 tui 子命令包
- 在 NewRootCmd() 中添加了 tui 子命令

### 3. /workspace/go-owl/go.mod (更新)
- 移除了不必要的 go-owl-tui 依赖
- 保持了原有的依赖不变

---

## 使用方法

### 正常使用流程
1. 构建并安装 go-owl:
   ```bash
   cd /workspace/go-owl && go install
   ```

2. 构建并安装 go-owl-tui:
   ```bash
   cd /workspace/go-owl-tui && go install
   ```

3. 现在可以直接使用:
   ```bash
   owl tui
   ```

### 在开发环境中使用
如果两个可执行文件在同一目录下:
```bash
# 在同一目录下构建两个项目
cd /workspace/go-owl && go build -o ./owl ./cmd/cli/
cd /workspace/go-owl-tui && go build -o ../go-owl/owl-tui ./

# 然后就可以直接调用
cd /workspace/go-owl && ./owl tui
```

---

## 结论
所有测试案例均通过，`owl tui` 子命令功能正常。用户现在可以通过 `owl tui` 命令直接启动 owl-tui 界面，而无需单独运行 `owl-tui` 命令。
