#!/bin/bash

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASSED=0
FAILED=0
SKIPPED=0

log_info()  { echo -e "${BLUE}[INFO]${NC} $1"; }
log_pass()  { echo -e "${GREEN}[PASS]${NC} $1"; PASSED=$((PASSED + 1)); }
log_fail()  { echo -e "${RED}[FAIL]${NC} $1"; FAILED=$((FAILED + 1)); }
log_skip()  { echo -e "${YELLOW}[SKIP]${NC} $1"; SKIPPED=$((SKIPPED + 1)); }

check_owl_binary() {
    log_info "检查 owl 命令..."
    if ! command -v owl &> /dev/null; then
        log_fail "owl 命令未找到，请先构建项目: go build -o owl ./cmd/cli && mv owl /usr/local/bin/"
        exit 1
    fi
    log_pass "owl 命令可用"
}

assert_contains() {
    local output="$1"
    local expected="$2"
    local test_name="$3"

    if echo "$output" | grep -q "$expected"; then
        log_pass "$test_name"
        return 0
    else
        log_fail "$test_name (期望包含 '$expected')"
        echo "实际输出: $output"
        return 1
    fi
}

assert_not_contains() {
    local output="$1"
    local unexpected="$2"
    local test_name="$3"

    if echo "$output" | grep -q "$unexpected"; then
        log_fail "$test_name (不应包含 '$unexpected')"
        echo "实际输出: $output"
        return 1
    else
        log_pass "$test_name"
        return 0
    fi
}

assert_exit_code() {
    local actual=$1
    local expected=$2
    local test_name="$3"

    if [ "$actual" -eq "$expected" ]; then
        log_pass "$test_name"
        return 0
    else
        log_fail "$test_name (退出码: 期望 $expected, 实际 $actual)"
        return 1
    fi
}

run_owl_cmd() {
    local test_name="$1"
    shift

    log_info "测试: $test_name"
    local output
    if output=$(owl "$@" 2>&1); then
        log_pass "$test_name"
        echo "$output"
        return 0
    else
        local rc=$?
        log_fail "$test_name (命令失败, 退出码: $rc)"
        echo "$output"
        return $rc
    fi
}

run_owl_cmd_expect_fail() {
    local test_name="$1"
    shift

    log_info "测试: $test_name (预期失败)"
    local output
    if output=$(owl "$@" 2>&1); then
        log_fail "$test_name (预期失败但命令成功了)"
        echo "$output"
        return 1
    else
        log_pass "$test_name (正确失败)"
        echo "$output"
        return 0
    fi
}

print_summary() {
    echo ""
    echo "========================================="
    echo "测试结果总结"
    echo "========================================="
    echo -e "通过: ${GREEN}$PASSED${NC}"
    echo -e "失败: ${RED}$FAILED${NC}"
    echo -e "跳过: ${YELLOW}$SKIPPED${NC}"
    echo "========================================="

    if [ $FAILED -eq 0 ]; then
        echo -e "${GREEN}所有测试通过！${NC}"
        return 0
    else
        echo -e "${RED}有 $FAILED 个测试失败${NC}"
        return 1
    fi
}
