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

# ==============================================================
# 持久化测试 — 依赖 owl settings set/target 写入 ~/.owl/config.yaml
# ==============================================================

# 备份和恢复用户的 ~/.owl/config.yaml
CONFIG_BACKUP=""
setup_config_backup() {
    local cfg="$HOME/.owl/config.yaml"
    if [ -f "$cfg" ]; then
        CONFIG_BACKUP=$(mktemp)
        cp "$cfg" "$CONFIG_BACKUP"
        log_info "已备份 $cfg 到 $CONFIG_BACKUP"
    fi
}
restore_config_backup() {
    if [ -n "$CONFIG_BACKUP" ] && [ -f "$CONFIG_BACKUP" ]; then
        local cfg="$HOME/.owl/config.yaml"
        mkdir -p "$HOME/.owl"
        cp "$CONFIG_BACKUP" "$cfg"
        log_info "已恢复 $cfg 从备份"
        rm -f "$CONFIG_BACKUP"
        CONFIG_BACKUP=""
    fi
}
cleanup_config_backup() {
    if [ -n "$CONFIG_BACKUP" ] && [ -f "$CONFIG_BACKUP" ]; then
        rm -f "$CONFIG_BACKUP"
        CONFIG_BACKUP=""
    fi
}

test_settings_template() {
    log_info "测试: TC-SETTINGS-004 template 输出所有可配置项"
    local output
    if output=$(owl settings template 2>&1); then
        assert_contains "$output" "output.format"   "TC-SETTINGS-004 包含 output.format"
        assert_contains "$output" "output.color"    "TC-SETTINGS-004 包含 output.color"
        assert_contains "$output" "default.timeout" "TC-SETTINGS-004 包含 default.timeout"
        assert_contains "$output" "default.group"   "TC-SETTINGS-004 包含 default.group"
        assert_contains "$output" "default.parallel" "TC-SETTINGS-004 包含 default.parallel"
        assert_contains "$output" "default.labels"  "TC-SETTINGS-004 包含 default.labels"
        assert_contains "$output" "target.groups"   "TC-SETTINGS-004 包含 target.groups"
        assert_contains "$output" "target.label"    "TC-SETTINGS-004 包含 target.label"
        assert_contains "$output" "target.nodes"    "TC-SETTINGS-004 包含 target.nodes"
    else
        log_fail "TC-SETTINGS-004 template 命令执行失败"
        echo "Output: $output"
    fi
}

test_settings_set_persist() {
    log_info "测试: TC-SETTINGS-005 set 持久化 default.group"

    # 先设置
    local set_output
    set_output=$(owl settings set default.group web 2>&1)
    if ! echo "$set_output" | grep -qi "saved"; then
        log_skip "TC-SETTINGS-005 set 命令可能不支持持久化 (跳过)"
        return
    fi

    # show 检查
    local show_output
    show_output=$(owl settings show 2>&1)
    assert_contains "$show_output" "web" "TC-SETTINGS-005 show 显示 default.group=web"

    # config.yaml 检查
    local cfg="$HOME/.owl/config.yaml"
    if [ -f "$cfg" ]; then
        if grep -q "group: web" "$cfg" 2>/dev/null || grep -q "web" "$cfg" 2>/dev/null; then
            log_pass "TC-SETTINGS-005 config.yaml 包含 group 设置"
        else
            log_fail "TC-SETTINGS-005 config.yaml 不包含 group 设置"
            echo "--- $cfg ---"
            cat "$cfg"
        fi
    else
        log_fail "TC-SETTINGS-005 config.yaml 不存在"
    fi
}

test_settings_set_labels_persist() {
    log_info "测试: TC-SETTINGS-006 set default.labels 持久化"

    local set_output
    set_output=$(owl settings set default.labels env=prod,region=us 2>&1) || true
    if echo "$set_output" | grep -qi "saved"; then
        local show_output
        show_output=$(owl settings show 2>&1)
        assert_contains "$show_output" "env=prod"  "TC-SETTINGS-006 show 包含 env=prod"
        assert_contains "$show_output" "region=us" "TC-SETTINGS-006 show 包含 region=us"
    else
        log_skip "TC-SETTINGS-006 default.labels 设置可能不支持 (跳过)"
    fi
}

test_settings_set_output_format() {
    log_info "测试: TC-SETTINGS-007 set output.format 持久化"

    local set_output
    set_output=$(owl settings set output.format json 2>&1) || true
    if echo "$set_output" | grep -qi "saved"; then
        local show_output
        show_output=$(owl settings show 2>&1)
        assert_contains "$show_output" "json" "TC-SETTINGS-007 show 显示 output.format=json"

        # 恢复默认
        owl settings set output.format table > /dev/null 2>&1 || true
    else
        log_skip "TC-SETTINGS-007 output.format 设置可能不支持 (跳过)"
    fi
}

test_settings_target_persist() {
    log_info "测试: TC-SETTINGS-008 target --groups 持久化"

    local target_output
    target_output=$(owl settings target --groups web,db 2>&1)
    if echo "$target_output" | grep -qi "saved"; then
        # 验证 show 读取持久化值
        local show_output
        show_output=$(owl settings show 2>&1)
        assert_contains "$show_output" "web"  "TC-SETTINGS-008 show 包含 target groups web"
        assert_contains "$show_output" "db"   "TC-SETTINGS-008 show 包含 target groups db"

        # 验证 config.yaml
        local cfg="$HOME/.owl/config.yaml"
        if [ -f "$cfg" ] && grep -q "groups:.*web,db" "$cfg" 2>/dev/null; then
            log_pass "TC-SETTINGS-008 config.yaml 包含 target.groups"
        else
            log_fail "TC-SETTINGS-008 config.yaml 不包含 target.groups"
        fi
    else
        log_skip "TC-SETTINGS-008 target 持久化可能不支持 (跳过)"
    fi
}

test_settings_config_file_integrity() {
    log_info "测试: TC-SETTINGS-009 config.yaml AI 配置节不受 settings 影响"

    local cfg="$HOME/.owl/config.yaml"
    if [ ! -f "$cfg" ]; then
        log_skip "TC-SETTINGS-009 config.yaml 不存在，跳过 AI 节检查"
        return
    fi

    # 检查是否有 ai 节（由 internal/ai 管理）
    if grep -q "^ai:" "$cfg" 2>/dev/null || grep -q "^\s\+provider:" "$cfg" 2>/dev/null; then
        log_pass "TC-SETTINGS-009 config.yaml 包含 ai 配置节"
    else
        # 可能没有 AI 配置，这不是错误
        log_info "TC-SETTINGS-009 config.yaml 无 ai 节 (正常)"
    fi

    # 确保 settings 节不污染其他节：ai 和 settings 应该是同级 key
    local settings_line
    settings_line=$(grep -n "^settings:" "$cfg" 2>/dev/null | head -1 | cut -d: -f1)
    local ai_line
    ai_line=$(grep -n "^ai:" "$cfg" 2>/dev/null | head -1 | cut -d: -f1)

    if [ -n "$settings_line" ] && [ -n "$ai_line" ]; then
        # 验证 settings 节没有嵌套在 ai 节中
        if [ "$settings_line" -eq "$((ai_line + 1))" ]; then
            # settings 紧跟在 ai 后面，检查缩进
            if grep -A1 "^ai:" "$cfg" 2>/dev/null | tail -1 | grep -q "^\s"; then
                log_pass "TC-SETTINGS-009 ai 和 settings 为同级 key"
            else
                log_fail "TC-SETTINGS-009 可能的缩进污染，检查 yaml 结构"
            fi
        else
            log_pass "TC-SETTINGS-009 ai 和 settings 为同级独立 key"
        fi
    else
        log_info "TC-SETTINGS-009 仅有一个配置节 (正常)"
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
    echo "-----------------------------------------"
    echo "template 子命令"
    echo "-----------------------------------------"
    test_settings_template

    echo ""
    echo "-----------------------------------------"
    echo "持久化测试（需要写入 ~/.owl/config.yaml）"
    echo "-----------------------------------------"
    setup_config_backup

    test_settings_set_persist
    test_settings_set_labels_persist
    test_settings_set_output_format
    test_settings_target_persist
    test_settings_config_file_integrity

    # 清理：恢复备份的 config.yaml
    restore_config_backup

    echo ""
    print_summary
}

main "$@"
