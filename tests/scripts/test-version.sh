#!/bin/bash
set -e
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/test_common.sh"

test_version_display() {
    log_info "测试: 显示版本信息"
    local output
    if output=$(owl --version 2>&1); then
        assert_contains "$output" "owl" "版本输出含 owl"
        assert_contains "$output" "1" "版本输出含版本号"
    else
        log_fail "显示版本信息 (--version 命令失败)"
        echo "Output: $output"
        return
    fi
    log_pass "显示版本信息"
}

test_version_help() {
    log_info "测试: 查看帮助"
    local output
    if output=$(owl --help 2>&1); then
        assert_contains "$output" "owl" "帮助输出含 owl"
    else
        log_fail "查看帮助 (--help 命令失败)"
        echo "Output: $output"
        return
    fi
    log_pass "查看帮助"
}

main() {
    echo "========================================="
    echo "owl version 命令测试套件"
    echo "========================================="

    check_owl_binary

    echo ""
    echo "-----------------------------------------"
    echo "测试 version"
    echo "-----------------------------------------"

    test_version_display
    test_version_help

    echo ""
    print_summary
}

main "$@"
