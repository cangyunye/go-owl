.PHONY: help test test-unit test-integration test-bash test-all test-clean test-quick

help:
	@echo "go-owl 测试命令"
	@echo ""
	@echo "可用命令:"
	@echo "  make test-all          - 运行所有测试"
	@echo "  make test-unit         - 运行单元测试"
	@echo "  make test-integration  - 运行集成测试"
	@echo "  make test-bash         - 运行 Bash 脚本测试"
	@echo "  make test-quick        - 快速测试（跳过耗时测试）"
	@echo "  make test-clean        - 清理测试环境"
	@echo ""
	@echo "环境变量:"
	@echo "  OWL_TEST_ENABLED       - 启用集成测试（设置为 true）"
	@echo "  OWL_TEST_NODES        - 测试节点列表（默认 self-test-1）"
	@echo ""
	@echo "示例:"
	@echo "  OWL_TEST_ENABLED=true OWL_TEST_NODES=self-test-1 make test-all"

export OWL_TEST_ENABLED ?= false
export OWL_TEST_NODES ?= self-test-1

test-all: test-unit test-integration test-bash

test-unit:
	@echo "========================================="
	@echo "运行单元测试"
	@echo "========================================="
	@go test -v ./unit/...

test-integration:
	@echo "========================================="
	@echo "运行集成测试"
	@echo "========================================="
	@if [ "$(OWL_TEST_ENABLED)" != "true" ]; then \
		echo "警告: 跳过集成测试（需要设置 OWL_TEST_ENABLED=true）"; \
		echo "示例: OWL_TEST_ENABLED=true make test-integration"; \
	else \
		echo "检查测试节点..."; \
		@owl exec run "echo ok" --nodes $(OWL_TEST_NODES) || (echo "错误: 测试节点不可用"; exit 1); \
		echo "运行集成测试..."; \
		@go test -v ./integration/...; \
	fi

test-bash:
	@echo "========================================="
	@echo "运行 Bash 脚本测试"
	@echo "========================================="
	@echo "当前 OWL_TEST_NODES 值: '$(OWL_TEST_NODES)'"
	@chmod +x ./scripts/*.sh
	@OWL_TEST_NODES=$(OWL_TEST_NODES) bash ./scripts/test-exec.sh
	@OWL_TEST_NODES=$(OWL_TEST_NODES) bash ./scripts/test-node.sh
	@OWL_TEST_NODES=$(OWL_TEST_NODES) bash ./scripts/test-history.sh

test-quick:
	@echo "========================================="
	@echo "运行快速测试"
	@echo "========================================="
	@echo "运行单元测试..."
	@go test -v ./unit/...

test-clean:
	@echo "========================================="
	@echo "清理测试环境"
	@echo "========================================="
	@owl exec run "rm -rf /tmp/owl-test" --nodes $(OWL_TEST_NODES) 2>/dev/null || true
	@echo "清理完成"

test-coverage:
	@echo "========================================="
	@echo "生成测试覆盖率报告"
	@echo "========================================="
	@go test -coverprofile=coverage.out ./unit/...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "报告已生成: coverage.html"

test-help:
	@echo "========================================="
	@echo "测试帮助信息"
	@echo "========================================="
	@owl --help
	@owl exec --help
	@owl node --help
	@owl history --help
	@owl settings --help
