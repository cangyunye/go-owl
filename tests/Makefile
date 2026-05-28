.PHONY: help test test-unit test-integration test-bash test-all test-clean test-quick \
        test-bash-exec test-bash-node test-bash-history test-bash-version \
        test-bash-file test-bash-playbook test-bash-settings test-bash-ai

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
	@echo ""
	@echo "运行 CLI 命令结构性测试 (cmd/cli/cmd/)"
	@cd .. && go test ./cmd/cli/cmd/...
	@echo ""
	@echo "运行 internal 层逻辑测试"
	@cd .. && go test ./internal/...

test-integration:
	@echo "========================================="
	@echo "运行集成测试"
	@echo "========================================="
	@if [ "$(OWL_TEST_ENABLED)" != "true" ]; then \
		echo "警告: 跳过集成测试（需要设置 OWL_TEST_ENABLED=true）"; \
		echo "示例: OWL_TEST_ENABLED=true make test-integration"; \
	else \
		echo "检查测试节点..."; \
		owl exec run "echo ok" --nodes $(OWL_TEST_NODES) || (echo "错误: 测试节点不可用"; exit 1); \
		echo "运行集成测试..."; \
		go test -v ./integration/...; \
	fi

test-bash: test-bash-version test-bash-node test-bash-exec test-bash-file test-bash-playbook test-bash-history test-bash-settings test-bash-ai
	@echo ""
	@echo "全部 bash 测试完成!"

test-bash-exec:
	@echo "========================================="
	@echo "bash 测试: exec"
	@echo "========================================="
	@chmod +x ./scripts/*.sh
	@OWL_TEST_NODES=$(OWL_TEST_NODES) ./scripts/test-exec.sh

test-bash-node:
	@echo ""
	@echo "========================================="
	@echo "bash 测试: node"
	@echo "========================================="
	@OWL_TEST_NODES=$(OWL_TEST_NODES) ./scripts/test-node.sh

test-bash-history:
	@echo ""
	@echo "========================================="
	@echo "bash 测试: history 和 settings"
	@echo "========================================="
	@./scripts/test-history.sh

test-bash-version:
	@echo ""
	@echo "========================================="
	@echo "bash 测试: version"
	@echo "========================================="
	@./scripts/test-version.sh

test-bash-file:
	@echo ""
	@echo "========================================="
	@echo "bash 测试: file"
	@echo "========================================="
	@chmod +x ./scripts/*.sh
	@OWL_TEST_NODES=$(OWL_TEST_NODES) ./scripts/test-file.sh

test-bash-playbook:
	@echo ""
	@echo "========================================="
	@echo "bash 测试: playbook"
	@echo "========================================="
	@OWL_TEST_NODES=$(OWL_TEST_NODES) ./scripts/test-playbook.sh

test-bash-settings:
	@echo ""
	@echo "========================================="
	@echo "bash 测试: settings"
	@echo "========================================="
	@./scripts/test-settings.sh

test-bash-ai:
	@echo ""
	@echo "========================================="
	@echo "bash 测试: ai"
	@echo "========================================="
	@chmod +x ./scripts/*.sh
	@OWL_TEST_NODES=$(OWL_TEST_NODES) ./scripts/test-ai.sh

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
	@cd .. && go test -coverprofile=tests/coverage.out ./cmd/cli/cmd/... ./internal/... ./tests/unit/...
	@cd .. && go tool cover -html=tests/coverage.out -o tests/coverage.html
	@echo "覆盖率报告已生成: coverage.html"
	@cd .. && go tool cover -func=tests/coverage.out | tail -1

test-help:
	@echo "========================================="
	@echo "测试帮助信息"
	@echo "========================================="
	@owl --help
	@owl exec --help
	@owl node --help
	@owl history --help
	@owl settings --help
