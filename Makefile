# go-owl Makefile
#
# 跨平台编译，支持 DuckDB 和 SQLite3 数据库
# 
# 使用方法：
#   make build              # 编译当前平台（DuckDB）
#   make build/linux       # 编译 Linux 版本
#   make build/windows     # 编译 Windows 版本
#   make build/all         # 编译所有平台
#   make install           # 安装到系统
#   make clean             # 清理构建文件

.PHONY: help build build/linux build/linux-amd64 build/windows build/windows-amd64 build/darwin build/darwin-arm64 build/all build-duckdb build-sqlite3 clean install test test-quick test-unit test-integration test-bash test-all test-clean test-coverage fmt lint all

# 变量定义
BINARY_NAME := owl
DUCKDB_BINARY := owl-duckdb
SQLITE3_BINARY := owl-sqlite3
MAIN_PATH := ./cmd/cli/main.go
BUILD_DIR := build
GO := go

# 编译标志
LDFLAGS := -ldflags "-s -w"

# 颜色定义
BOLD := \033[1m
GREEN := \033[32m
YELLOW := \033[33m
BLUE := \033[34m
NC := \033[0m

# 可执行文件后缀
EXE_SUFFIX := 
ifeq ($(OS),Windows_NT)
    EXE_SUFFIX := .exe
endif

# 默认目标
all: build

# ====================
# 跨平台编译
# ====================

# Linux AMD64 (DuckDB)
build/linux:
	@mkdir -p $(BUILD_DIR)/linux-amd64
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/linux-amd64/$(BINARY_NAME) $(MAIN_PATH)
	@printf "$(BOLD)$(GREEN)✓$(NC) Linux AMD64 (DuckDB) 已编译: $(BUILD_DIR)/linux-amd64/$(BINARY_NAME)\n"

# Linux AMD64 (SQLite3)
build/linux-amd64:
	@mkdir -p $(BUILD_DIR)/linux-amd64-duckdb $(BUILD_DIR)/linux-amd64-sqlite3
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/linux-amd64-duckdb/$(BINARY_NAME) $(MAIN_PATH)
	GOOS=linux GOARCH=amd64 $(GO) build -tags sqlite3 $(LDFLAGS) -o $(BUILD_DIR)/linux-amd64-sqlite3/$(BINARY_NAME) $(MAIN_PATH)
	@printf "$(BOLD)$(GREEN)✓$(NC) Linux AMD64 已编译\n"

# Windows AMD64 (DuckDB)
build/windows:
	@mkdir -p $(BUILD_DIR)/windows-amd64-duckdb $(BUILD_DIR)/windows-amd64-sqlite3
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/windows-amd64-duckdb/$(BINARY_NAME).exe $(MAIN_PATH)
	GOOS=windows GOARCH=amd64 $(GO) build -tags sqlite3 $(LDFLAGS) -o $(BUILD_DIR)/windows-amd64-sqlite3/$(BINARY_NAME).exe $(MAIN_PATH)
	@printf "$(BOLD)$(GREEN)✓$(NC) Windows AMD64 已编译\n"

# Windows AMD64 (单独)
build/windows-amd64:
	@mkdir -p $(BUILD_DIR)/windows-amd64
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/windows-amd64/$(BINARY_NAME).exe $(MAIN_PATH)
	@printf "$(BOLD)$(GREEN)✓$(NC) Windows AMD64 已编译: $(BUILD_DIR)/windows-amd64/$(BINARY_NAME).exe\n"

# macOS AMD64
build/darwin:
	@mkdir -p $(BUILD_DIR)/darwin-amd64-duckdb $(BUILD_DIR)/darwin-amd64-sqlite3
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/darwin-amd64-duckdb/$(BINARY_NAME) $(MAIN_PATH)
	GOOS=darwin GOARCH=amd64 $(GO) build -tags sqlite3 $(LDFLAGS) -o $(BUILD_DIR)/darwin-amd64-sqlite3/$(BINARY_NAME) $(MAIN_PATH)
	@printf "$(BOLD)$(GREEN)✓$(NC) macOS AMD64 已编译\n"

# macOS ARM64 (Apple Silicon)
build/darwin-arm64:
	@mkdir -p $(BUILD_DIR)/darwin-arm64-duckdb $(BUILD_DIR)/darwin-arm64-sqlite3
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/darwin-arm64-duckdb/$(BINARY_NAME) $(MAIN_PATH)
	GOOS=darwin GOARCH=arm64 $(GO) build -tags sqlite3 $(LDFLAGS) -o $(BUILD_DIR)/darwin-arm64-sqlite3/$(BINARY_NAME) $(MAIN_PATH)
	@printf "$(BOLD)$(GREEN)✓$(NC) macOS ARM64 已编译\n"

# 编译所有平台版本
build/all: build/linux build/windows build/darwin build/darwin-arm64
	@printf ""
	@printf "$(BOLD)$(GREEN)✓$(NC) 所有平台版本编译完成\n"
	@find $(BUILD_DIR) -type f -name "$(BINARY_NAME)*" | head -20

# 当前平台编译 (DuckDB)
build:
	@mkdir -p $(BUILD_DIR)/$(shell go env GOOS)-$(shell go env GOARCH)
ifneq ($(shell go env GOOS),windows)
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(shell go env GOOS)-$(shell go env GOARCH)/$(BINARY_NAME) $(MAIN_PATH)
else
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(shell go env GOOS)-$(shell go env GOARCH)/$(BINARY_NAME).exe $(MAIN_PATH)
endif
	@printf "$(BOLD)$(GREEN)✓$(NC) $(shell go env GOOS)-$(shell go env GOARCH) 版本已编译\n"

# ====================
# 数据库版本编译
# ====================

## build-duckdb: 使用 DuckDB 构建（默认）
build-duckdb:
	@printf "$(BOLD)$(BLUE)==>$(NC) 使用 DuckDB 构建...\n"
ifneq ($(shell go env GOOS),windows)
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
else
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME).exe $(MAIN_PATH)
endif
	@printf "$(BOLD)$(GREEN)✓$(NC) DuckDB 版本构建完成\n"

## build-sqlite3: 使用 SQLite3 构建
build-sqlite3:
	@printf "$(BOLD)$(BLUE)==>$(NC) 使用 SQLite3 构建...\n"
ifneq ($(shell go env GOOS),windows)
	$(GO) build -tags sqlite3 $(LDFLAGS) -o $(BUILD_DIR)/$(SQLITE3_BINARY) $(MAIN_PATH)
else
	$(GO) build -tags sqlite3 $(LDFLAGS) -o $(BUILD_DIR)/$(SQLITE3_BINARY).exe $(MAIN_PATH)
endif
	@printf "$(BOLD)$(GREEN)✓$(NC) SQLite3 版本构建完成\n"

## all: 构建所有数据库版本
all: build-duckdb build-sqlite3
	@printf ""
	@printf "$(BOLD)$(GREEN)✓$(NC) 所有版本构建完成！\n"
	@ls -lh $(BUILD_DIR)/$(BINARY_NAME)* 2>/dev/null | awk '{print "  " $$9 ": " $$5}'

# ====================
# Windows 分发包
# ====================

## dist/windows: 创建 Windows 分发包
dist/windows:
	@mkdir -p $(BUILD_DIR)/windows-dist
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BUILD_DIR)/windows-dist/$(BINARY_NAME).exe $(MAIN_PATH)
	
	@echo " Owl CLI 工具" > $(BUILD_DIR)/windows-dist/README.txt
	@echo " ==============" >> $(BUILD_DIR)/windows-dist/README.txt
	@echo "" >> $(BUILD_DIR)/windows-dist/README.txt
	@echo "使用方法：" >> $(BUILD_DIR)/windows-dist/README.txt
	@echo "1. 双击 owl.exe 运行或打开命令行运行" >> $(BUILD_DIR)/windows-dist/README.txt
	@echo "2. 使用 owl --help 查看帮助" >> $(BUILD_DIR)/windows-dist/README.txt
	@echo "" >> $(BUILD_DIR)/windows-dist/README.txt
	@echo "配置：" >> $(BUILD_DIR)/windows-dist/README.txt
	@echo "- 配置文件: %USERPROFILE%\.owl\config.yaml" >> $(BUILD_DIR)/windows-dist/README.txt
	@echo "- 节点配置: %USERPROFILE%\.owl\nodes.json" >> $(BUILD_DIR)/windows-dist/README.txt
	
	@printf "$(BOLD)$(GREEN)✓$(NC) Windows 分发包已创建: $(BUILD_DIR)/windows-dist/\n"
	@ls -lh $(BUILD_DIR)/windows-dist/

# ====================
# 安装
# ====================

## install: 安装到系统路径
install: build-duckdb
ifneq ($(shell go env GOOS),windows)
	@mkdir -p ~/.local/bin
	cp $(BUILD_DIR)/$(BINARY_NAME) ~/.local/bin/
	@printf "$(BOLD)$(GREEN)✓$(NC) 已安装到 ~/.local/bin/$(BINARY_NAME)\n"
	@printf "$(BOLD)$(YELLOW)提示:$(NC) 请确保 ~/.local/bin 在您的 PATH 中\n"
else
	@mkdir -p "C:\Program Files\Owl"
	cp $(BUILD_DIR)/$(BINARY_NAME).exe "C:\Program Files\Owl\"
	@printf "$(BOLD)$(GREEN)✓$(NC) 已安装到 C:\Program Files\Owl\$(BINARY_NAME).exe\n"
	@printf "$(BOLD)$(YELLOW)提示:$(NC) 请添加 C:\Program Files\Owl 到 PATH\n"
endif

## install-duckdb: 安装 DuckDB 版本
install-duckdb: build-duckdb
ifneq ($(shell go env GOOS),windows)
	@mkdir -p ~/.local/bin
	cp $(BUILD_DIR)/$(BINARY_NAME) ~/.local/bin/
	@printf "$(BOLD)$(GREEN)✓$(NC) DuckDB 版本已安装: ~/.local/bin/$(BINARY_NAME)\n"
else
	@mkdir -p "C:\Program Files\Owl"
	cp $(BUILD_DIR)/$(BINARY_NAME).exe "C:\Program Files\Owl\"
	@printf "$(BOLD)$(GREEN)✓$(NC) DuckDB 版本已安装\n"
endif

## install-sqlite3: 安装 SQLite3 版本
install-sqlite3: build-sqlite3
ifneq ($(shell go env GOOS),windows)
	@mkdir -p ~/.local/bin
	cp $(BUILD_DIR)/$(SQLITE3_BINARY) ~/.local/bin/owl
	@printf "$(BOLD)$(GREEN)✓$(NC) SQLite3 版本已安装: ~/.local/bin/owl\n"
else
	@mkdir -p "C:\Program Files\Owl"
	cp $(BUILD_DIR)/$(SQLITE3_BINARY).exe "C:\Program Files\Owl\owl.exe"
	@printf "$(BOLD)$(GREEN)✓$(NC) SQLite3 版本已安装\n"
endif

# ====================
# 清理
# ====================

## clean: 清理构建产物
clean:
	@printf "$(BOLD)$(YELLOW)==>$(NC) 清理构建产物...\n"
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME) $(BINARY_NAME).exe
	@rm -f $(DUCKDB_BINARY) $(DUCKDB_BINARY).exe
	@rm -f $(SQLITE3_BINARY) $(SQLITE3_BINARY).exe
	@printf "$(BOLD)$(GREEN)✓$(NC) 清理完成\n"

# ====================
# 测试
# ====================

## test: 运行测试（所有测试）
test: test-all

## test-all: 运行所有测试
test-all:
	@make -C tests test-all

## test-unit: 运行单元测试
test-unit:
	@make -C tests test-unit

## test-integration: 运行集成测试
test-integration:
	@make -C tests test-integration

## test-bash: 运行 Bash 脚本测试
test-bash:
	@make -C tests test-bash

## test-quick: 快速测试（仅运行单元测试）
test-quick:
	@make -C tests test-quick

## test-clean: 清理测试环境
test-clean:
	@make -C tests test-clean

## test-coverage: 运行测试并生成覆盖率报告
test-coverage:
	@printf "$(BOLD)$(BLUE)==>$(NC) 运行测试（覆盖率）...\n"
	@$(GO) test -v -coverprofile=coverage.out ./...
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@printf "$(BOLD)$(GREEN)✓$(NC) 覆盖率报告: coverage.html\n"

# ====================
# 代码质量
# ====================

## fmt: 格式化代码
fmt:
	@printf "$(BOLD)$(BLUE)==>$(NC) 格式化代码...\n"
	@$(GO) fmt ./...
	@printf "$(BOLD)$(GREEN)✓$(NC) 格式化完成\n"

## lint: 代码检查
lint:
	@printf "$(BOLD)$(BLUE)==>$(NC) 代码检查...\n"
	@which golangci-lint > /dev/null || (printf "$(BOLD)$(YELLOW)警告:$(NC) golangci-lint 未安装，跳过...\n" && exit 0)
	@golangci-lint run ./...

## vet: 代码诊断
vet:
	@printf "$(BOLD)$(BLUE)==>$(NC) 运行 go vet...\n"
	@$(GO) vet ./...

# ====================
# 开发辅助
# ====================

## run: 运行程序
run:
	@$(GO) run $(MAIN_PATH) $(ARGS)

## deps: 下载依赖
deps:
	@printf "$(BOLD)$(BLUE)==>$(NC) 下载依赖...\n"
	@$(GO) mod download
	@$(GO) mod tidy
	@printf "$(BOLD)$(GREEN)✓$(NC) 依赖下载完成\n"

## init: 初始化项目
init: deps
	@printf "$(BOLD)$(BLUE)==>$(NC) 初始化项目...\n"
	@$(GO) generate ./...
	@printf "$(BOLD)$(GREEN)✓$(NC) 项目初始化完成\n"

## build-debug: 调试版本构建
build-debug:
	@printf "$(BOLD)$(BLUE)==>$(NC) 构建调试版本...\n"
ifneq ($(shell go env GOOS),windows)
	$(GO) build -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY_NAME)-debug $(MAIN_PATH)
else
	$(GO) build -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY_NAME)-debug.exe $(MAIN_PATH)
endif
	@printf "$(BOLD)$(GREEN)✓$(NC) 调试版本构建完成\n"

## version: 显示版本信息
version:
	@$(GO) run $(MAIN_PATH) --version

# ====================
# 帮助信息
# ====================

## help: 显示帮助信息
help:
	@printf ""
	@printf "$(BOLD)$(BLUE)go-owl 跨平台构建工具$(NC)\n"
	@printf ""
	@printf "$(BOLD)用法:$(NC) make $(GREEN)<目标>$(NC)\n"
	@printf ""
	@printf "$(BOLD)跨平台编译:$(NC)\n"
	@printf "  $(GREEN)build$(NC)              编译当前平台（DuckDB）\n"
	@printf "  $(GREEN)build/linux$(NC)         编译 Linux AMD64\n"
	@printf "  $(GREEN)build/windows$(NC)       编译 Windows AMD64\n"
	@printf "  $(GREEN)build/darwin$(NC)        编译 macOS AMD64\n"
	@printf "  $(GREEN)build/darwin-arm64$(NC)  编译 macOS ARM64 (Apple Silicon)\n"
	@printf "  $(GREEN)build/all$(NC)           编译所有平台\n"
	@printf ""
	@printf "$(BOLD)数据库版本:$(NC)\n"
	@printf "  $(GREEN)build-duckdb$(NC)        使用 DuckDB 构建（默认）\n"
	@printf "  $(GREEN)build-sqlite3$(NC)       使用 SQLite3 构建\n"
	@printf ""
	@printf "$(BOLD)分发包:$(NC)\n"
	@printf "  $(GREEN)dist/windows$(NC)        创建 Windows 分发包\n"
	@printf ""
	@printf "$(BOLD)安装:$(NC)\n"
	@printf "  $(GREEN)install$(NC)             安装到系统\n"
	@printf "  $(GREEN)install-duckdb$(NC)      安装 DuckDB 版本\n"
	@printf "  $(GREEN)install-sqlite3$(NC)     安装 SQLite3 版本\n"
	@printf ""
	@printf "$(BOLD)其他:$(NC)\n"
	@printf "  $(GREEN)clean$(NC)               清理构建文件\n"
	@printf "  $(GREEN)test$(NC)                运行测试\n"
	@printf "  $(GREEN)test-cover$(NC)          运行测试（覆盖率）\n"
	@printf "  $(GREEN)fmt$(NC)                 格式化代码\n"
	@printf "  $(GREEN)lint$(NC)                代码检查\n"
	@printf "  $(GREEN)deps$(NC)                下载依赖\n"
	@printf "  $(GREEN)run$(NC)                 运行程序\n"
	@printf ""
	@printf "$(BOLD)当前平台:$(NC) $(GREEN)$(shell go env GOOS) $(shell go env GOARCH)$(NC)\n"
	@printf ""
