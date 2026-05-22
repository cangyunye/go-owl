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

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_pass() { echo -e "${GREEN}[PASS]${NC} $1"; ((PASSED++)); }
log_fail() { echo -e "${RED}[FAIL]${NC} $1"; ((FAILED++)); }
log_skip() { echo -e "${YELLOW}[SKIP]${NC} $1"; ((SKIPPED++)); }

check_env() {
    log_info "检查测试环境..."
    
    if ! command -v owl &> /dev/null; then
        log_fail "owl 命令未找到"
        exit 1
    fi
    
    log_pass "owl 命令可用"
}

test_node_list() {
    local name="$1"
    
    echo ""
    log_info "测试: $name"
    
    if output=$(owl node list 2>&1); then
        if echo "$output" | grep -q "ID"; then
            log_pass "$name"
        else
            log_fail "$name (输出格式不正确)"
            echo "Output: $output"
        fi
    else
        log_fail "$name (命令失败)"
        echo "Output: $output"
    fi
}

test_node_show() {
    local name="$1"
    local node="$2"
    
    echo ""
    log_info "测试: $name"
    
    if output=$(owl node show "$node" 2>&1); then
        if echo "$output" | grep -q "Address"; then
            log_pass "$name"
        else
            log_fail "$name (输出格式不正确)"
            echo "Output: $output"
        fi
    else
        log_fail "$name (命令失败)"
        echo "Output: $output"
    fi
}

main() {
    echo "========================================="
    echo "owl node 命令测试套件"
    echo "========================================="
    
    check_env
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 node list"
    echo "-----------------------------------------"
    
    test_node_list "列出所有节点"
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 node show"
    echo "-----------------------------------------"
    
    OWL_TEST_NODES="${OWL_TEST_NODES:-self-test-1}"
    first_node=$(echo "$OWL_TEST_NODES" | cut -d',' -f1)
    test_node_show "显示节点详情" "$first_node"
    
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
        exit 0
    else
        echo -e "${RED}有 $FAILED 个测试失败${NC}"
        exit 1
    fi
}

main "$@"
