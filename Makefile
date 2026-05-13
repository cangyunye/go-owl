# go-owl Makefile
#
# 提供多种编译方式，支持 DuckDB 和 SQLite3 数据库

.PHONY: help build build-duckdb build-sqlite3 clean install test fmt lint all

# 默认目标
.DEFAULT_GOAL := help

# 变量定义
BINARY_NAME := owl
DUCKDB_BINARY := owl-duckdb
SQLITE3_BINARY := owl-sqlite3
MAIN_PATH := ./cmd/cli/main.go
BUILD_DIR := .
GO := go

# 编译标志
LDFLAGS := -ldflags "-s -w"

# 颜色定义
BOLD := \033[1m
GREEN := \033[32m
YELLOW := \033[33m
BLUE := \033[34m
NC := \033[0m

## help: 显示帮助信息
help:
	@echo ""
	@echo "$(BOLD)$(BLUE)go-owl 构建工具$(NC)"
	@echo ""
	@echo "用法: make $(GREEN)<目标>$(NC)"
	@echo ""
	@echo "$(BOLD)可用目标:$(NC)"
	@echo ""
	@echo "  $(GREEN)build$(NC)           构建默认版本（DuckDB）"
	@echo "  $(GREEN)build-duckdb$(NC)    使用 DuckDB 构建"
	@echo "  $(GREEN)build-sqlite3$(NC)   使用 SQLite3 构建"
	@echo "  $(GREEN)all$(NC)             构建所有版本"
	@echo "  $(GREEN)clean$(NC)          清理构建产物"
	@echo "  $(GREEN)install$(NC)        安装到 ~/.local/bin"
	@echo "  $(GREEN)test$(NC)           运行测试"
	@echo "  $(GREEN)fmt$(NC)            格式化代码"
	@echo "  $(GREEN)lint$(NC)           代码检查"
	@echo ""
	@echo "$(BOLD)示例:$(NC)"
	@echo ""
	@echo "  make build-duckdb      # 构建 DuckDB 版本"
	@echo "  make build-sqlite3     # 构建 SQLite3 版本"
	@echo "  make all               # 构建所有版本"
	@echo "  make clean             # 清理"
	@echo ""

## build: 构建默认版本（DuckDB）
build: build-duckdb

## build-duckdb: 使用 DuckDB 构建（默认）
build-duckdb:
	@echo "$(BOLD)$(BLUE)==>$(NC) 使用 DuckDB 构建..."
	@$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(DUCKDB_BINARY) $(MAIN_PATH)
	@echo "$(BOLD)$(GREEN)✓$(NC) 构建完成: $(DUCKDB_BINARY)"

## build-sqlite3: 使用 SQLite3 构建
build-sqlite3:
	@echo "$(BOLD)$(BLUE)==>$(NC) 使用 SQLite3 构建..."
	@$(GO) build -tags sqlite3 $(LDFLAGS) -o $(BUILD_DIR)/$(SQLITE3_BINARY) $(MAIN_PATH)
	@echo "$(BOLD)$(GREEN)✓$(NC) 构建完成: $(SQLITE3_BINARY)"

## all: 构建所有版本
all: build-duckdb build-sqlite3
	@echo ""
	@echo "$(BOLD)$(GREEN)✓$(NC) 所有版本构建完成！"
	@ls -lh $(DUCKDB_BINARY) $(SQLITE3_BINARY) 2>/dev/null | awk '{print "  " $$9 ": " $$5}'

## clean: 清理构建产物
clean:
	@echo "$(BOLD)$(YELLOW)==>$(NC) 清理构建产物..."
	@rm -f $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(DUCKDB_BINARY) $(BUILD_DIR)/$(SQLITE3_BINARY)
	@echo "$(BOLD)$(GREEN)✓$(NC) 清理完成"

## install: 安装到 ~/.local/bin
install: build-duckdb
	@echo "$(BOLD)$(BLUE)==>$(NC) 安装到 ~/.local/bin..."
	@mkdir -p ~/.local/bin
	@cp $(BUILD_DIR)/$(DUCKDB_BINARY) ~/.local/bin/$(BINARY_NAME)
	@echo "$(BOLD)$(GREEN)✓$(NC) 安装完成: ~/.local/bin/$(BINARY_NAME)"
	@echo "$(BOLD)$(YELLOW)提示:$(NC) 请确保 ~/.local/bin 在您的 PATH 中"

## install-duckdb: 安装 DuckDB 版本
install-duckdb: build-duckdb
	@echo "$(BOLD)$(BLUE)==>$(NC) 安装 DuckDB 版本..."
	@mkdir -p ~/.local/bin
	@cp $(BUILD_DIR)/$(DUCKDB_BINARY) ~/.local/bin/$(BINARY_NAME)
	@echo "$(BOLD)$(GREEN)✓$(NC) 安装完成: ~/.local/bin/$(BINARY_NAME) (DuckDB)"

## install-sqlite3: 安装 SQLite3 版本
install-sqlite3: build-sqlite3
	@echo "$(BOLD)$(BLUE)==>$(NC) 安装 SQLite3 版本..."
	@mkdir -p ~/.local/bin
	@cp $(BUILD_DIR)/$(SQLITE3_BINARY) ~/.local/bin/$(BINARY_NAME)
	@echo "$(BOLD)$(GREEN)✓$(NC) 安装完成: ~/.local/bin/$(BINARY_NAME) (SQLite3)"

## test: 运行测试
test:
	@echo "$(BOLD)$(BLUE)==>$(NC) 运行测试..."
	@$(GO) test -v ./...

## test-cover: 运行测试并生成覆盖率报告
test-cover:
	@echo "$(BOLD)$(BLUE)==>$(NC) 运行测试（覆盖率）..."
	@$(GO) test -v -coverprofile=coverage.out ./...
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "$(BOLD)$(GREEN)✓$(NC) 覆盖率报告: coverage.html"

## fmt: 格式化代码
fmt:
	@echo "$(BOLD)$(BLUE)==>$(NC) 格式化代码..."
	@$(GO) fmt ./...
	@echo "$(BOLD)$(GREEN)✓$(NC) 格式化完成"

## lint: 代码检查
lint:
	@echo "$(BOLD)$(BLUE)==>$(NC) 代码检查..."
	@which golangci-lint > /dev/null || (echo "$(BOLD)$(YELLOW)警告:$(NC) golangci-lint 未安装，跳过..." && exit 0)
	@golangci-lint run ./...

## vet: 代码诊断
vet:
	@echo "$(BOLD)$(BLUE)==>$(NC) 运行 go vet..."
	@$(GO) vet ./...

## run: 运行程序
run:
	@$(GO) run $(MAIN_PATH) $(ARGS)

## deps: 下载依赖
deps:
	@echo "$(BOLD)$(BLUE)==>$(NC) 下载依赖..."
	@$(GO) mod download
	@$(GO) mod tidy
	@echo "$(BOLD)$(GREEN)✓$(NC) 依赖下载完成"

## init: 初始化项目
init: deps
	@echo "$(BOLD)$(BLUE)==>$(NC) 初始化项目..."
	@$(GO) generate ./...
	@echo "$(BOLD)$(GREEN)✓$(NC) 项目初始化完成"

## build-debug: 调试版本构建
build-debug:
	@echo "$(BOLD)$(BLUE)==>$(NC) 构建调试版本..."
	@$(GO) build -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY_NAME)-debug $(MAIN_PATH)
	@echo "$(BOLD)$(GREEN)✓$(NC) 调试版本构建完成: $(BINARY_NAME)-debug"

## version: 显示版本信息
version:
	@$(GO) run $(MAIN_PATH) --version
