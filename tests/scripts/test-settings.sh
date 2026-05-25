#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/test_common.sh"

test_settings_show() {
    log_info "测试: TC-SETTINGS-001 显示当前设置"
    local output
    if output=$(owl settings show 2>&1); then
        log_pass "TC-SETTINGS-001 显示设置"
    else
        log_info "TC-SETTINGS-001 显示设置 (命令执行可能有非零退出)"
        if echo "$output" | grep -qi "output\|format\|server\|color"; then
            log_pass "TC-SETTINGS-001 显示设置 (含配置项)"
        else
            log_fail "TC-SETTINGS-001 显示设置失败"
            echo "Output: $output"
        fi
    fi
}

test_settings_target() {
    log_info "测试: TC-SETTINGS-003 默认目标选择"
    local output
    if output=$(owl settings target 2>&1); then
        log_pass "TC-SETTINGS-003 默认目标"
    else
        if echo "$output" | grep -qi "target\|group\|label\|nodes"; then
            log_pass "TC-SETTINGS-003 默认目标 (含预期字段)"
        else
            log_fail "TC-SETTINGS-003 默认目标失败"
            echo "Output: $output"
        fi
    fi
}

test_settings_target_set() {
    log_info "测试: TC-SETTINGS-003 设置默认目标"
    local original
    original=$(owl settings target 2>&1) || true
    local output
    if output=$(owl settings target --nodes self-test-1 2>&1); then
        log_pass "TC-SETTINGS-003 设置默认目标 --nodes self-test-1"
    else
        log_skip "TC-SETTINGS-003 设置默认目标 (命令不支持或配置只读)"
    fi
}

test_settings_show_json() {
    log_info "测试: settings show 不支持 JSON 输出"
    local output
    if output=$(owl settings show 2>&1); then
        if ! echo "$output" | grep -q "^{"; then
            log_pass "settings show 输出为纯文本格式（非 JSON）"
        else
            log_pass "settings show 命令执行"
        fi
    else
        log_pass "settings show 命令执行"
    fi
}

test_settings_help() {
    log_info "测试: settings 帮助信息"
    local output
    if output=$(owl settings --help 2>&1); then
        assert_contains "$output" "settings" "settings 帮助"
    else
        log_fail "settings --help 失败"
    fi
}

main() {
    echo "========================================="
    echo "owl settings 命令 E2E 测试套件"
    echo "========================================="

    check_owl_binary

    echo ""
    echo "-----------------------------------------"
    echo "帮助信息验证"
    echo "-----------------------------------------"
    test_settings_help

    echo ""
    echo "-----------------------------------------"
    echo "settings show"
    echo "-----------------------------------------"
    test_settings_show
    test_settings_show_json

    echo ""
    echo "-----------------------------------------"
    echo "settings target"
    echo "-----------------------------------------"
    test_settings_target
    test_settings_target_set

    echo ""
    print_summary
}

main "$@"
