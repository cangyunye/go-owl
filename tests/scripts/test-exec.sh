#!/bin/bash
set -e
set -u

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASSED=0
FAILED=0
SKIPPED=0

log_info() { echo "[INFO] $1"; }
log_pass() { echo "[PASS] $1"; ((PASSED++)); }
log_fail() { echo "[FAIL] $1"; ((FAILED++)); }
log_skip() { echo "[SKIP] $1"; ((SKIPPED++)); }

cleanup() {
    log_info "清理测试环境..."
    owl exec run "rm -rf /tmp/owl-test" --nodes "${OWL_TEST_NODES}" 2>/dev/null || true
}

check_env() {
    log_info "检查测试环境..."
    
    if ! command -v owl &> /dev/null; then
        log_fail "owl 命令未找到，请先构建项目: go build -o owl ./cmd/cli"
        exit 1
    fi
    
    log_pass "owl 命令可用"
    
    log_info "步骤1: 检查 OWL_TEST_NODES 变量"
    if [ -z "${OWL_TEST_NODES:-}" ]; then
        log_info "OWL_TEST_NODES 为空，使用默认值"
        OWL_TEST_NODES="self-test-1,self-test-2"
    else
        log_info "OWL_TEST_NODES 已设置为: '$OWL_TEST_NODES'"
    fi
    export OWL_TEST_NODES
    log_info "测试节点: '$OWL_TEST_NODES'"
    
    log_info "步骤2: 开始节点检查"
    for node in $(echo "$OWL_TEST_NODES" | tr ',' ' '); do
        log_info "检查节点: $node"
        log_info "执行命令: owl exec run \"echo ok\" --nodes \"$node\""
        if owl exec run "echo ok" --nodes "$node"; then
            log_pass "节点 $node 可达"
        else
            log_fail "节点 $node 不可达"
            exit 1
        fi
    done
    
    cleanup
}

test_exec_run() {
    local name="$1"
    local command="$2"
    local nodes="${3:-$OWL_TEST_NODES}"
    local expected="$4"
    local flags="${5:-}"
    
    echo ""
    log_info "测试: $name"
    
    local cmd="owl exec run $command --nodes $nodes"
    if [ -n "$flags" ]; then
        cmd="$cmd $flags"
    fi
    
    if output=$($cmd 2>&1); then
        if echo "$output" | grep -q "$expected"; then
            log_pass "$name"
        else
            log_fail "$name (输出不匹配)"
            echo "Expected: $expected"
            echo "Got: $output"
        fi
    else
        log_fail "$name (命令失败，退出码: $?)"
        echo "Output: $output"
    fi
}

test_exec_script() {
    local name="$1"
    local script="$2"
    local nodes="${3:-$OWL_TEST_NODES}"
    local inline="${4:-false}"
    
    echo ""
    log_info "测试: $name"
    
    local cmd="owl exec script $script --nodes $nodes"
    if [ "$inline" = "true" ]; then
        cmd="$cmd --inline"
    fi
    
    if output=$($cmd 2>&1); then
        log_pass "$name"
    else
        log_fail "$name (执行失败)"
        echo "Output: $output"
    fi
}

main() {
    echo "========================================="
    echo "owl exec 命令测试套件"
    echo "========================================="
    
    check_env
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 exec run - 基础命令"
    echo "-----------------------------------------"
    
    test_exec_run "简单命令-echo" "echo hello" "self-test-1" "hello"
    test_exec_run "uptime 命令" "uptime" "self-test-1" "load"
    test_exec_run "日期命令" "date" "self-test-1" "20"
    test_exec_run "创建目录" "mkdir -p /tmp/owl-test" "self-test-1" ""
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 exec run - 多节点"
    echo "-----------------------------------------"
    
    if [ $(echo "$OWL_TEST_NODES" | tr ',' '\n' | wc -l) -ge 2 ]; then
        test_exec_run "多节点命令" "uptime" "$OWL_TEST_NODES" "load"
    else
        log_skip "多节点测试 (节点数不足)"
    fi
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 exec run - 串行模式"
    echo "-----------------------------------------"
    
    test_exec_run "串行模式" "echo serial" "self-test-1" "serial" "--serial"
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 exec run - 超时"
    echo "-----------------------------------------"
    
    test_exec_run "命令超时" "sleep 1" "self-test-1" "成功" "--command-timeout 5s"
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 exec run - 输出格式"
    echo "-----------------------------------------"
    
    test_exec_run "JSON 输出" "echo hello" "self-test-1" "hello" "--output json"
    test_exec_run "详细输出" "echo hello" "self-test-1" "hello" "--output detail"
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 exec script"
    echo "-----------------------------------------"
    
    test_script_path="../testdata/scripts/test-script.sh"
    if [ -f "$test_script_path" ]; then
        test_exec_script "脚本执行(文件模式)" "$test_script_path"
        test_exec_script "脚本执行(内联模式)" "$test_script_path" "self-test-1" "true"
    else
        log_skip "测试脚本不存在: $test_script_path"
    fi
    
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
