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

test_history() {
    local name="$1"
    
    echo ""
    log_info "测试: $name"
    
    if output=$(owl history 2>&1); then
        log_pass "$name"
        log_info "历史记录查询成功"
    else
        log_fail "$name (命令失败)"
        echo "Output: $output"
    fi
}

test_settings_show() {
    local name="$1"
    
    echo ""
    log_info "测试: $name"
    
    if output=$(owl settings show 2>&1); then
        if echo "$output" | grep -q "Output"; then
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
    echo "owl history 和 settings 测试套件"
    echo "========================================="
    
    check_env
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 history"
    echo "-----------------------------------------"
    
    test_history "查询历史记录"
    
    echo ""
    echo "-----------------------------------------"
    echo "测试 settings"
    echo "-----------------------------------------"
    
    test_settings_show "显示设置"
    
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
