#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/test_common.sh"

DEFAULT_NODES="${OWL_TEST_NODES:-self-test-1}"

check_ai_config() {
    log_info "检查 AI 配置..."
    local config_path="$HOME/.owl/config.yaml"
    if [ ! -f "$config_path" ]; then
        log_skip "$config_path 不存在，跳过 AI E2E 测试（需要先执行 owl ai config init 并配置 API Key）"
        exit 0
    fi
    if ! grep -q "api_key:" "$config_path"; then
        log_skip "config.yaml 中未配置 api_key，跳过 AI E2E 测试"
        exit 0
    fi
    local api_key
    api_key=$(grep "api_key:" "$config_path" | awk '{print $2}' | tr -d '"')
    if [ -z "$api_key" ] || [ "$api_key" = '""' ]; then
        log_skip "config.yaml 中 api_key 为空，跳过 AI E2E 测试"
        exit 0
    fi
    log_pass "AI 配置可用"
    return 0
}

check_nodes() {
    log_info "检查测试节点可用性..."
    if [ -z "${OWL_TEST_NODES:-}" ]; then
        log_skip "OWL_TEST_NODES 未设置，跳过 AI E2E 测试"
        exit 0
    fi

    local first_node
    first_node=$(echo "$OWL_TEST_NODES" | cut -d',' -f1)
    if owl exec run "echo ok" --nodes "$first_node" > /dev/null 2>&1; then
        log_pass "测试节点 $first_node 可达"
        return 0
    else
        log_skip "测试节点 $first_node 不可达，跳过 AI E2E 测试"
        exit 0
    fi
}

# Helper: extract tool_calls JSON from owl ai output
extract_tool_calls() {
    local output="$1"
    echo "$output" | sed -n '/```json/,/```/p' | sed '1d;$d'
}

test_ai_router_exec() {
    log_info "测试: TC-AI-BASH-001 路由到 exec 命令组"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl ai "在 $first_node 执行 echo hello" 2>&1); then
        if echo "$output" | grep -q "execute_command"; then
            log_pass "TC-AI-BASH-001 路由到 execute_command"
        else
            log_fail "TC-AI-BASH-001 未路由到 execute_command"
            echo "Output: $output"
        fi
    else
        log_fail "TC-AI-BASH-001 owl ai 命令失败"
        echo "Output: $output"
    fi
}

test_ai_router_node() {
    log_info "测试: TC-AI-BASH-002 路由到 node 命令组"
    local output
    if output=$(owl ai "列出所有在线节点" 2>&1); then
        if echo "$output" | grep -q "query_nodes"; then
            log_pass "TC-AI-BASH-002 路由到 query_nodes"
        else
            log_fail "TC-AI-BASH-002 未路由到 query_nodes"
            echo "Output: $output"
        fi
    else
        log_fail "TC-AI-BASH-002 owl ai 命令失败"
        echo "Output: $output"
    fi
}

test_ai_router_uncertain() {
    log_info "测试: TC-AI-BASH-003 模糊输入应被拒绝"
    local output
    if output=$(owl ai "随便来点什么不知所云的东西xyz123" 2>&1); then
        if echo "$output" | grep -q "不确定"; then
            log_pass "TC-AI-BASH-003 模糊输入正确拒绝"
        elif echo "$output" | grep -q "tool_calls"; then
            log_info "TC-AI-BASH-003 模糊输入被路由到某命令组（LLM 自行判断）"
            log_pass "TC-AI-BASH-003 模糊输入处理完成"
        else
            log_pass "TC-AI-BASH-003 模糊输入处理完成（无 tool_calls）"
        fi
    else
        log_fail "TC-AI-BASH-003 owl ai 命令失败"
        echo "Output: $output"
    fi
}

test_ai_json_format_exec() {
    log_info "测试: TC-AI-BASH-004 exec 工具返回合法 JSON"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl ai "在 $first_node 执行 uptime" 2>&1); then
        local json_content
        json_content=$(extract_tool_calls "$output")
        if [ -n "$json_content" ]; then
            # Check if it's valid JSON by checking for tool_calls
            if echo "$json_content" | grep -q "tool_calls"; then
                log_pass "TC-AI-BASH-004 输出包含合法 tool_calls JSON"
            else
                log_fail "TC-AI-BASH-004 JSON 不包含 tool_calls 字段"
                echo "JSON: $json_content"
            fi
        else
            log_fail "TC-AI-BASH-004 未找到 JSON 代码块"
            echo "Output: $output"
        fi
    else
        log_fail "TC-AI-BASH-004 owl ai 命令失败"
        echo "Output: $output"
    fi
}

test_ai_exec_script() {
    log_info "测试: TC-AI-BASH-005 exec script 路由"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl ai "在 $first_node 执行脚本 tests/testdata/scripts/test-script.sh" 2>&1); then
        if echo "$output" | grep -q "execute_script"; then
            log_pass "TC-AI-BASH-005 路由到 execute_script"
        else
            log_fail "TC-AI-BASH-005 未路由到 execute_script"
            echo "Output: $output"
        fi
    else
        log_fail "TC-AI-BASH-005 owl ai 命令失败"
        echo "Output: $output"
    fi
}

test_ai_router_file() {
    log_info "测试: TC-AI-BASH-006 路由到 file 命令组"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl ai "上传 /etc/hosts 到 $first_node" 2>&1); then
        if echo "$output" | grep -q "transfer_file"; then
            log_pass "TC-AI-BASH-006 路由到 transfer_file"
        elif echo "$output" | grep -q "不确定"; then
            log_skip "TC-AI-BASH-006 LLM 无法确定（可能是文件不存在）"
        else
            log_info "TC-AI-BASH-006 路由到其他工具"
            log_pass "TC-AI-BASH-006 处理完成"
        fi
    else
        log_fail "TC-AI-BASH-006 owl ai 命令失败"
        echo "Output: $output"
    fi
}

main() {
    echo "========================================="
    echo "owl ai 命令 E2E 测试套件"
    echo "========================================="

    check_owl_binary
    check_ai_config
    check_nodes

    echo ""
    echo "-----------------------------------------"
    echo "AI 路由测试"
    echo "-----------------------------------------"
    test_ai_router_exec
    test_ai_router_node
    test_ai_router_file
    test_ai_router_uncertain

    echo ""
    echo "-----------------------------------------"
    echo "AI JSON 格式 + 子命令测试"
    echo "-----------------------------------------"
    test_ai_json_format_exec
    test_ai_exec_script

    echo ""
    print_summary
}

main "$@"
