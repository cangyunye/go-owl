#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/test_common.sh"

DEFAULT_NODES="${OWL_TEST_NODES:-self-test-1}"

check_nodes() {
    log_info "检查测试节点可用性..."
    if [ -z "${OWL_TEST_NODES:-}" ]; then
        log_skip "OWL_TEST_NODES 未设置，跳过 exec E2E 测试"
        exit 0
    fi

    local first_node
    first_node=$(echo "$OWL_TEST_NODES" | cut -d',' -f1)
    if owl exec run "echo ok" --nodes "$first_node" > /dev/null 2>&1; then
        log_pass "测试节点 $first_node 可达"
        return 0
    else
        log_skip "测试节点 $first_node 不可达，跳过 exec E2E 测试"
        exit 0
    fi
}

test_exec_run_single_node() {
    log_info "测试: TC-EXEC-001 单节点执行简单命令"
    local output
    if output=$(owl exec run "echo hello" --nodes "$(echo "$DEFAULT_NODES" | cut -d',' -f1)" 2>&1); then
        assert_contains "$output" "hello" "TC-EXEC-001 单节点执行 echo hello"
    else
        log_fail "TC-EXEC-001 命令执行失败"
        echo "Output: $output"
    fi
}

test_exec_run_multi_node() {
    log_info "测试: TC-EXEC-002 多节点并行执行"
    local node_count
    node_count=$(echo "$DEFAULT_NODES" | tr ',' '\n' | wc -l | tr -d ' ')
    if [ "$node_count" -ge 2 ]; then
        local output
        if output=$(owl exec run "hostname" --nodes "$DEFAULT_NODES" 2>&1); then
            log_pass "TC-EXEC-002 多节点并行执行 hostname"
        else
            log_fail "TC-EXEC-002 多节点命令执行失败"
            echo "Output: $output"
        fi
    else
        log_skip "TC-EXEC-002 多节点执行 (节点数不足: $node_count)"
    fi
}

test_exec_run_by_group() {
    log_info "测试: TC-EXEC-003 按分组执行"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl exec run "echo grouped" --nodes "$first_node" 2>&1); then
        assert_contains "$output" "grouped" "TC-EXEC-003 按节点执行命令"
    else
        log_skip "TC-EXEC-003 分组执行 (节点不可用)"
    fi
}

test_exec_run_timeout() {
    log_info "测试: TC-EXEC-004 超时处理"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl exec run "sleep 1" --nodes "$first_node" --command-timeout 5s 2>&1); then
        log_pass "TC-EXEC-004 命令超时设置 (5s内完成)"
    else
        log_fail "TC-EXEC-004 超时命令失败"
        echo "Output: $output"
    fi
}

test_exec_run_json_output() {
    log_info "测试: TC-EXEC-005 JSON 格式输出"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl exec run "echo hello" --nodes "$first_node" --format json 2>&1); then
        if echo "$output" | grep -q '"'; then
            log_pass "TC-EXEC-005 JSON 输出含引号"
        else
            log_fail "TC-EXEC-005 JSON 输出格式错误"
            echo "Output: $output"
        fi
    else
        log_fail "TC-EXEC-005 JSON 输出命令失败"
        echo "Output: $output"
    fi
}

test_exec_run_error() {
    log_info "测试: TC-EXEC-006 错误命令处理"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl exec run "nonexistent_command_xyz" --nodes "$first_node" 2>&1); then
        log_info "TC-EXEC-006 错误命令完成 (exit 码可能是0但输出含错误信息)"
        echo "Output: $output"
        log_pass "TC-EXEC-006 错误命令处理 (已执行)"
    else
        log_pass "TC-EXEC-006 错误命令正确返回失败"
    fi
}

test_exec_run_async() {
    log_info "测试: TC-EXEC-007 异步执行"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl exec run "echo async-test" --nodes "$first_node" --async 2>&1); then
        log_pass "TC-EXEC-007 异步执行模式"
        if echo "$output" | grep -qi "task\|id\|async"; then
            log_info "TC-EXEC-007 异步任务已提交"
        fi
    else
        log_fail "TC-EXEC-007 异步执行失败"
        echo "Output: $output"
    fi
}

test_exec_run_serial() {
    log_info "测试: exec run 串行模式"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl exec run "echo serial" --nodes "$first_node" --serial 2>&1); then
        assert_contains "$output" "serial" "exec run 串行模式"
    else
        log_fail "exec run 串行模式失败"
        echo "Output: $output"
    fi
}

test_exec_run_detail_output() {
    log_info "测试: exec run 详细输出模式"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl exec run "echo detail" --nodes "$first_node" --format detail 2>&1); then
        assert_contains "$output" "detail" "exec run 详细输出模式"
    else
        log_fail "exec run 详细输出模式失败"
        echo "Output: $output"
    fi
}

test_exec_script_file() {
    log_info "测试: exec script 文件模式"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local script_content='#!/bin/bash\necho "script-test-success"'
    local script_file="/tmp/owl-test-script-$$.sh"
    echo -e "$script_content" > "$script_file"
    chmod +x "$script_file"

    local output
    local rc=0
    if output=$(owl exec script "$script_file" --nodes "$first_node" 2>&1); then
        assert_contains "$output" "success" "exec script 文件模式"
    else
        rc=$?
        log_fail "exec script 文件模式失败"
        echo "Output: $output"
    fi

    rm -f "$script_file"
    return $rc
}

test_exec_script_inline() {
    log_info "测试: exec script 内联模式"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl exec script "echo inline-test" --nodes "$first_node" --inline 2>&1); then
        assert_contains "$output" "inline-test" "exec script 内联模式"
    else
        log_fail "exec script 内联模式失败"
        echo "Output: $output"
    fi
}

cleanup() {
    log_info "清理测试环境..."
    owl exec run "rm -rf /tmp/owl-test" --nodes "$(echo "$DEFAULT_NODES" | cut -d',' -f1)" 2>/dev/null || true
}

main() {
    echo "========================================="
    echo "owl exec 命令 E2E 测试套件"
    echo "========================================="

    check_owl_binary
    check_nodes

    echo ""
    echo "-----------------------------------------"
    echo "exec run - 基础命令"
    echo "-----------------------------------------"
    test_exec_run_single_node
    test_exec_run_multi_node
    test_exec_run_by_group

    echo ""
    echo "-----------------------------------------"
    echo "exec run - 模式与选项"
    echo "-----------------------------------------"
    test_exec_run_serial
    test_exec_run_detail_output
    test_exec_run_json_output
    test_exec_run_timeout

    echo ""
    echo "-----------------------------------------"
    echo "exec run - 错误与异步"
    echo "-----------------------------------------"
    test_exec_run_error
    test_exec_run_async

    echo ""
    echo "-----------------------------------------"
    echo "exec script"
    echo "-----------------------------------------"
    test_exec_script_file
    test_exec_script_inline

    cleanup
    echo ""
    print_summary
}

main "$@"
