# owl tui 集成测试文档

## 概述

`owl tui` 是 go-owl 的**原生 TUI 子命令**（bubbletea 实现，非外部二进制转发），位于 `cmd/cli/cmd/tui/`。它通过构建标签门控：

- **纯净构建**（`make build` / `task build`，无标签）：`owl tui` 子命令不注册，且 tui 包因不被 import 而整体不链接（bubbletea/bubbles/lipgloss 依赖一并裁掉）。
- **TUI 构建**（`make build WITH=tui` / `make build-tui` / `task build-tui`，`-tags tui`）：注册并链接 `owl tui` 子命令。

## 构建

```bash
# 纯净（不含 owl tui）
make build
task build

# 带 owl tui 子命令
make build-tui
make build WITH=tui
task build-tui
task build WITH=tui
```

## 测试

### 单元测试

tui 包与 nodes 子包单测（`cmd/cli/cmd/tui/`）：

```bash
# 纯净构建下 root 命令不含 tui 子命令，root_test 断言同样通过
go test ./cmd/cli/cmd/...

# tui 标签下含 tui 子命令，root_test 断言 tui 存在
go test -tags tui ./cmd/cli/...
```

`TestRootCmdHasSubcommands` 通过 `extraRootCommands`（`tui_register.go` / `tui_register_disabled.go` 按标签提供）自适应两种构建。

### pty 端到端冒烟

`scripts/test-tui.sh` 在 pty 下喂真实按键验证：

- 场景 1：干净数据下 `owl tui` 渲染 `/nodes` 面包屑、打开新增表单、`q` 干净退出。
- 场景 2：nodes.json↔db 冲突数据下 `owl tui` 不被读路径冲突交互提示阻塞（`SetConflictPrompt(false)` 绕过）。

```bash
./scripts/test-tui.sh
```

## 构建标签门控实现

- `cmd/cli/cmd/root.go`：调用 `registerTUI(rootCmd)`，不直接 import tui 包。
- `cmd/cli/cmd/tui_register.go`（`//go:build tui`）：注册 `tui.NewTuiCmd()` + `extraRootCommands=["tui"]`。
- `cmd/cli/cmd/tui_register_disabled.go`（`//go:build !tui`）：空实现 + `extraRootCommands=nil`。
