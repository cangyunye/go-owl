# go-owl Makefile —— 组件化跨平台构建
#
# 组件: owl CLI（核心，必编）+ 可选追加 serve / metrics / tui / gscp
# 全部组件均为纯 Go（CGO_ENABLED=0），任意平台可交叉编译。
#
# 常用:
#   make build                                   # 当前平台 owl CLI
#   make build WITH=serve,metrics,tui,gscp       # 追加组件
#   make build PLATFORMS="linux/amd64 windows/amd64"
#   make build/all                               # 全平台 × 全组件
#   make install                                 # 安装到 ~/.local/bin

SHELL       := /bin/sh
GO          ?= go
BUILD_DIR   := build
VERSION     ?= 0.16.0

CLI_MAIN    := ./cmd/cli
SERVE_MAIN  := ./cmd/owl-serve
PKG         := github.com/cangyunye/go-owl/cmd/cli/cmd
COMMIT_ID   := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME  := $(shell date '+%Y-%m-%d %H:%M:%S')
LDFLAGS     := -ldflags "-s -w -X '$(PKG).version=$(VERSION)' -X '$(PKG).commitID=$(COMMIT_ID)' -X '$(PKG).buildTime=$(BUILD_TIME)'"

ALL_PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
HOST_PLATFORM := $(shell $(GO) env GOOS)/$(shell $(GO) env GOARCH)
HOST_PLATDIR  := $(subst /,-,$(HOST_PLATFORM))
PLATFORMS     ?= $(HOST_PLATFORM)

WITH ?=
comma := ,
COMPS := $(subst $(comma), ,$(WITH))

# 外部源码: gscp（自动克隆）/ tui（优先本地兄弟目录）
GSCP_REPO ?= https://github.com/cangyunye/gscp.git
GSCP_REF  ?= main
GSCP_SRC  := $(BUILD_DIR)/.gscp-src
TUI_SRC   ?= ../go-owl-tui
TUI_REPO  ?= https://github.com/cangyunye/go-owl-tui.git
TUI_CLONE := $(BUILD_DIR)/.tui-src

.PHONY: build build/all build-serve build-metrics build-tui build-gscp build/local-gscp \
	install install-gscp clean test test-unit test-integration test-quick test-coverage \
	fmt lint vet help

# 通用跨平台编译: $(1)=附加编译参数 $(2)=包路径 $(3)=二进制名
define cross_build
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		mkdir -p $(BUILD_DIR)/$$os-$$arch; \
		printf '==> %-10s %s\n' '$(3)' "$$p"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build $(1) -o $(BUILD_DIR)/$$os-$$arch/$(3)$$ext $(2); \
	done
endef

build: ## 编译 owl CLI；WITH=serve,metrics,tui,gscp 追加组件
ifneq (,$(filter metrics,$(COMPS)))
	$(call cross_build,-tags metrics $(LDFLAGS),$(CLI_MAIN),owl)
else
	$(call cross_build,$(LDFLAGS),$(CLI_MAIN),owl)
endif
ifneq (,$(filter serve,$(COMPS)))
	$(call cross_build,,$(SERVE_MAIN),owl-serve)
endif
ifneq (,$(filter tui,$(COMPS)))
	@$(MAKE) --no-print-directory build-tui PLATFORMS='$(PLATFORMS)'
endif
ifneq (,$(filter gscp,$(COMPS)))
	@$(MAKE) --no-print-directory build-gscp PLATFORMS='$(PLATFORMS)'
endif

build/all: ## 全部平台 × 全部组件
	@$(MAKE) --no-print-directory build PLATFORMS='$(ALL_PLATFORMS)' WITH=serve,metrics,tui,gscp

build-serve: ## 编译 Web 控制台 owl-serve（纯 Go，可交叉编译）
	$(call cross_build,,$(SERVE_MAIN),owl-serve)

build-metrics: ## 编译带 metrics 功能的 owl CLI（需兄弟目录 ../go-owl-metrics）
	$(call cross_build,-tags metrics $(LDFLAGS),$(CLI_MAIN),owl)

build-tui: ## 编译 owl-tui（默认源码 ../go-owl-tui，可 TUI_SRC= 覆盖；缺失时克隆 TUI_REPO）
	@set -e; src=$(TUI_SRC); \
	if [ ! -d "$$src" ]; then \
		printf '==> clone owl-tui: $(TUI_REPO)\n'; \
		rm -rf $(TUI_CLONE); git clone --depth 1 $(TUI_REPO) $(TUI_CLONE); src=./$(TUI_CLONE); \
	fi; \
	for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		out=$(CURDIR)/$(BUILD_DIR)/$$os-$$arch; mkdir -p $$out; \
		printf '==> %-10s %s\n' owl-tui "$$p"; \
		(cd "$$src" && GOWORK=off CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -o "$$out/owl-tui$$ext" .); \
	done

build-gscp: ## 克隆并跨平台编译 gscp
	@if [ ! -d '$(GSCP_SRC)/.git' ]; then git clone --depth 1 '$(GSCP_REPO)' '$(GSCP_SRC)'; fi
	@cd '$(GSCP_SRC)' && git fetch -q origin '$(GSCP_REF)' && git checkout -q FETCH_HEAD
	$(call cross_build_ext,$(GSCP_SRC),gscp)

build/local-gscp: ## 用本地源码编 gscp: make build/local-gscp GSCP_LOCAL=../gscp
	@if [ -z '$(GSCP_LOCAL)' ] || [ ! -d '$(GSCP_LOCAL)' ]; then echo '用法: make build/local-gscp GSCP_LOCAL=../gscp（路径需存在）'; exit 1; fi
	$(call cross_build_ext,$(GSCP_LOCAL),gscp)

# 外部模块跨平台编译: $(1)=源码目录 $(2)=二进制名
define cross_build_ext
	@set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; \
		if [ "$$os" = windows ]; then ext=.exe; fi; \
		out=$(CURDIR)/$(BUILD_DIR)/$$os-$$arch; mkdir -p $$out; \
		printf '==> %-10s %s\n' '$(2)' "$$p"; \
		(cd '$(1)' && GOWORK=off CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -ldflags '-w -s' -o "$$out/$(2)$$ext" .); \
	done
endef

install: ## 安装当前平台产物（owl 及已追加组件）到 ~/.local/bin
	@$(MAKE) --no-print-directory build PLATFORMS='$(HOST_PLATFORM)'
	@mkdir -p ~/.local/bin
	@for b in owl owl-serve owl-tui; do \
		f=$(BUILD_DIR)/$(HOST_PLATDIR)/$$b; \
		if [ -f "$$f" ]; then cp "$$f" ~/.local/bin/; printf 'installed %s\n' "$$b"; fi; \
	done

install-gscp: ## 安装 gscp（linux/darwin）到 ~/.owl/gscp/，中继传输自动发现
	@$(MAKE) --no-print-directory build-gscp PLATFORMS='linux/amd64 linux/arm64 darwin/amd64 darwin/arm64'
	@for p in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
		os=$${p%/*}; arch=$${p#*/}; plat=$$os-$$arch; \
		if [ -f $(BUILD_DIR)/$$plat/gscp ]; then \
			mkdir -p ~/.owl/gscp/$$plat; cp $(BUILD_DIR)/$$plat/gscp ~/.owl/gscp/$$plat/gscp; \
			printf 'installed gscp (%s)\n' "$$plat"; \
		fi; \
	done

clean: ## 清理构建产物
	rm -rf $(BUILD_DIR) coverage.out coverage.html
	rm -f owl owl.exe cli cli.exe owl-serve owl-serve.exe owl-tui owl-tui.exe owl-duckdb owl-sqlite3

test: ## 运行全部测试（委托 tests/）
	@$(MAKE) --no-print-directory -C tests test-all

test-unit: ## 运行单元测试
	@$(MAKE) --no-print-directory -C tests test-unit

test-integration: ## 运行集成测试
	@$(MAKE) --no-print-directory -C tests test-integration

test-quick: ## 快速测试（跳过耗时项）
	@$(MAKE) --no-print-directory -C tests test-quick

test-coverage: ## 测试并生成覆盖率报告 coverage.html
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

fmt: ## 格式化代码
	$(GO) fmt ./...

lint: ## golangci-lint 检查（未安装则跳过）
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo 'golangci-lint 未安装，跳过'

vet: ## go vet 静态检查
	$(GO) vet ./...

help: ## 显示本帮助
	@awk 'BEGIN{FS=":.*?## "} /^[a-zA-Z0-9\/_.-]+:.*?## /{printf "  \033[32m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
