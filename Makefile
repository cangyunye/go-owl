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

# 颜色定义 - 使用 printf 避免 macOS echo -e 问题
BOLD := \033[1m
GREEN := \033[32m
YELLOW := \033[33m
BLUE := \033[34m
NC := \033[0m

# 跨平台打印函数
print = @printf "$(1)\n"

## help: 显示帮助信息
help:
	@printf ""
	@printf "$(BOLD)$(BLUE)go-owl 构建工具$(NC)\n"
	@printf ""
	@printf "用法: make $(GREEN)<目标>$(NC)\n"
	@printf ""
	@printf "$(BOLD)可用目标:$(NC)\n"
	@printf ""
	@printf "  $(GREEN)build$(NC)           构建默认版本（DuckDB）\n"
	@printf "  $(GREEN)build-duckdb$(NC)    使用 DuckDB 构建\n"
	@printf "  $(GREEN)build-sqlite3$(NC)   使用 SQLite3 构建\n"
	@printf "  $(GREEN)all$(NC)             构建所有版本\n"
	@printf "  $(GREEN)clean$(NC)          清理构建产物\n"
	@printf "  $(GREEN)install$(NC)        安装到 ~/.local/bin\n"
	@printf "  $(GREEN)test$(NC)           运行测试\n"
	@printf "  $(GREEN)fmt$(NC)            格式化代码\n"
	@printf "  $(GREEN)lint$(NC)           代码检查\n"
	@printf ""
	@printf "$(BOLD)示例:$(NC)\n"
	@printf ""
	@printf "  make build-duckdb      # 构建 DuckDB 版本\n"
	@printf "  make build-sqlite3     # 构建 SQLite3 版本\n"
	@printf "  make all               # 构建所有版本\n"
	@printf "  make clean             # 清理\n"
	@printf ""

## build: 构建默认版本（DuckDB）
build: build-duckdb

## build-duckdb: 使用 DuckDB 构建（默认）
build-duckdb:
	@printf "$(BOLD)$(BLUE)==>$(NC) 使用 DuckDB 构建...\n"
	@$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(DUCKDB_BINARY) $(MAIN_PATH)
	@printf "$(BOLD)$(GREEN)✓$(NC) 构建完成: $(DUCKDB_BINARY)\n"

## build-sqlite3: 使用 SQLite3 构建
build-sqlite3:
	@printf "$(BOLD)$(BLUE)==>$(NC) 使用 SQLite3 构建...\n"
	@$(GO) build -tags sqlite3 $(LDFLAGS) -o $(BUILD_DIR)/$(SQLITE3_BINARY) $(MAIN_PATH)
	@printf "$(BOLD)$(GREEN)✓$(NC) 构建完成: $(SQLITE3_BINARY)\n"

## all: 构建所有版本
all: build-duckdb build-sqlite3
	@printf ""
	@printf "$(BOLD)$(GREEN)✓$(NC) 所有版本构建完成！\n"
	@ls -lh $(DUCKDB_BINARY) $(SQLITE3_BINARY) 2>/dev/null | awk '{print "  " $$9 ": " $$5}'

## clean: 清理构建产物
clean:
	@printf "$(BOLD)$(YELLOW)==>$(NC) 清理构建产物...\n"
	@rm -f $(BUILD_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(DUCKDB_BINARY) $(BUILD_DIR)/$(SQLITE3_BINARY)
	@printf "$(BOLD)$(GREEN)✓$(NC) 清理完成\n"

## install: 安装到 ~/.local/bin
install: build-duckdb
	@printf "$(BOLD)$(BLUE)==>$(NC) 安装到 ~/.local/bin...\n"
	@mkdir -p ~/.local/bin
	@cp $(BUILD_DIR)/$(DUCKDB_BINARY) ~/.local/bin/$(BINARY_NAME)
	@printf "$(BOLD)$(GREEN)✓$(NC) 安装完成: ~/.local/bin/$(BINARY_NAME)\n"
	@printf "$(BOLD)$(YELLOW)提示:$(NC) 请确保 ~/.local/bin 在您的 PATH 中\n"

## install-duckdb: 安装 DuckDB 版本
install-duckdb: build-duckdb
	@printf "$(BOLD)$(BLUE)==>$(NC) 安装 DuckDB 版本...\n"
	@mkdir -p ~/.local/bin
	@cp $(BUILD_DIR)/$(DUCKDB_BINARY) ~/.local/bin/$(BINARY_NAME)
	@printf "$(BOLD)$(GREEN)✓$(NC) 安装完成: ~/.local/bin/$(BINARY_NAME) (DuckDB)\n"

## install-sqlite3: 安装 SQLite3 版本
install-sqlite3: build-sqlite3
	@printf "$(BOLD)$(BLUE)==>$(NC) 安装 SQLite3 版本...\n"
	@mkdir -p ~/.local/bin
	@cp $(BUILD_DIR)/$(SQLITE3_BINARY) ~/.local/bin/$(BINARY_NAME)
	@printf "$(BOLD)$(GREEN)✓$(NC) 安装完成: ~/.local/bin/$(BINARY_NAME) (SQLite3)\n"

## test: 运行测试
test:
	@printf "$(BOLD)$(BLUE)==>$(NC) 运行测试...\n"
	@$(GO) test -v ./...

## test-cover: 运行测试并生成覆盖率报告
test-cover:
	@printf "$(BOLD)$(BLUE)==>$(NC) 运行测试（覆盖率）...\n"
	@$(GO) test -v -coverprofile=coverage.out ./...
	@$(GO) tool cover -html=coverage.out -o coverage.html
	@printf "$(BOLD)$(GREEN)✓$(NC) 覆盖率报告: coverage.html\n"

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
	@$(GO) build -gcflags="all=-N -l" -o $(BUILD_DIR)/$(BINARY_NAME)-debug $(MAIN_PATH)
	@printf "$(BOLD)$(GREEN)✓$(NC) 调试版本构建完成: $(BINARY_NAME)-debug\n"

## version: 显示版本信息
version:
	@$(GO) run $(MAIN_PATH) --version
