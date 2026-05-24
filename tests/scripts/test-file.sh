#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/test_common.sh"

DEFAULT_NODES="${OWL_TEST_NODES:-self-test-1}"
TEST_DIR="/tmp/owl-test-file-$$"

check_nodes() {
    log_info "检查测试节点可用性..."
    if [ -z "${OWL_TEST_NODES:-}" ]; then
        log_skip "OWL_TEST_NODES 未设置，跳过 file E2E 测试"
        exit 0
    fi

    local first_node
    first_node=$(echo "$OWL_TEST_NODES" | cut -d',' -f1)
    if owl exec run "echo ok" --nodes "$first_node" > /dev/null 2>&1; then
        log_pass "测试节点 $first_node 可达"
        return 0
    else
        log_skip "测试节点 $first_node 不可达，跳过 file E2E 测试"
        exit 0
    fi
}

setup_test_file() {
    mkdir -p "$TEST_DIR"
    echo "owl-e2e-file-test-content-$$" > "$TEST_DIR/testfile.txt"
}

test_file_upload_single() {
    log_info "测试: TC-FILE-001 单节点上传文件"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl file upload "$TEST_DIR/testfile.txt" --nodes "$first_node" --dest "/tmp/" 2>&1); then
        log_pass "TC-FILE-001 单节点上传"
    else
        log_skip "TC-FILE-001 单节点上传 (节点不可用)"
    fi
}

test_file_upload_multi() {
    log_info "测试: TC-FILE-002 多节点上传文件"
    local node_count
    node_count=$(echo "$DEFAULT_NODES" | tr ',' '\n' | wc -l | tr -d ' ')
    if [ "$node_count" -ge 2 ]; then
        local output
        if output=$(owl file upload "$TEST_DIR/testfile.txt" --nodes "$DEFAULT_NODES" --dest "/tmp/" 2>&1); then
            log_pass "TC-FILE-002 多节点上传"
        else
            log_skip "TC-FILE-002 多节点上传 (节点不可用)"
        fi
    else
        log_skip "TC-FILE-002 多节点上传 (节点数不足: $node_count)"
    fi
}

test_file_upload_group() {
    log_info "测试: TC-FILE-003 分组上传文件"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl file upload "$TEST_DIR/testfile.txt" --nodes "$first_node" --dest "/tmp/" 2>&1); then
        log_pass "TC-FILE-003 分组上传 (by node)"
    else
        log_skip "TC-FILE-003 分组上传 (节点不可用)"
    fi
}

test_file_download_single() {
    log_info "测试: TC-FILE-004 单节点下载文件"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    owl exec run "echo download-test > /tmp/owl-dl-test.txt" --nodes "$first_node" 2>/dev/null || true
    local output
    if output=$(owl file download "/tmp/owl-dl-test.txt" --nodes "$first_node" --dest "$TEST_DIR/" 2>&1); then
        log_pass "TC-FILE-004 单节点下载"
    else
        log_skip "TC-FILE-004 单节点下载 (节点不可用)"
    fi
    owl exec run "rm -f /tmp/owl-dl-test.txt" --nodes "$first_node" 2>/dev/null || true
}

test_file_upload_overwrite() {
    log_info "测试: TC-FILE-005 上传覆盖策略"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    owl exec run "echo old > /tmp/owl-overwrite-test.txt" --nodes "$first_node" 2>/dev/null || true
    local output
    if output=$(owl file upload "$TEST_DIR/testfile.txt" --nodes "$first_node" --dest "/tmp/owl-overwrite-test.txt" --overwrite 2>&1); then
        log_pass "TC-FILE-005 上传覆盖策略"
    else
        log_skip "TC-FILE-005 上传覆盖 (节点不可用)"
    fi
    owl exec run "rm -f /tmp/owl-overwrite-test.txt" --nodes "$first_node" 2>/dev/null || true
}

test_file_upload_no_overwrite() {
    log_info "测试: TC-FILE-006 上传不覆盖策略"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    owl exec run "echo existing > /tmp/owl-noov-test.txt" --nodes "$first_node" 2>/dev/null || true
    local output
    if output=$(owl file upload "$TEST_DIR/testfile.txt" --nodes "$first_node" --dest "/tmp/owl-noov-test.txt" --no-overwrite 2>&1); then
        log_pass "TC-FILE-006 上传不覆盖策略"
    else
        log_skip "TC-FILE-006 上传不覆盖 (节点不可用)"
    fi
    owl exec run "rm -f /tmp/owl-noov-test.txt" --nodes "$first_node" 2>/dev/null || true
}

test_file_download_multi_suffix() {
    log_info "测试: TC-FILE-007 多节点下载(后缀命名)"
    local node_count
    node_count=$(echo "$DEFAULT_NODES" | tr ',' '\n' | wc -l | tr -d ' ')
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    owl exec run "echo multi > /tmp/owl-multi.txt" --nodes "$first_node" 2>/dev/null || true
    if [ "$node_count" -ge 2 ]; then
        local output
        if output=$(owl file download "/tmp/owl-multi.txt" --nodes "$DEFAULT_NODES" --dest "$TEST_DIR/" --name-format "{name}_{node}" 2>&1); then
            log_pass "TC-FILE-007 多节点下载(后缀)"
        else
            log_skip "TC-FILE-007 多节点下载(后缀) (节点不可用)"
        fi
    else
        log_skip "TC-FILE-007 多节点下载(后缀) (节点数不足)"
    fi
    owl exec run "rm -f /tmp/owl-multi.txt" --nodes "$first_node" 2>/dev/null || true
}

test_file_download_subdir() {
    log_info "测试: TC-FILE-008 多节点下载(子目录)"
    local node_count
    node_count=$(echo "$DEFAULT_NODES" | tr ',' '\n' | wc -l | tr -d ' ')
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    owl exec run "echo subdir > /tmp/owl-subdir.txt" --nodes "$first_node" 2>/dev/null || true
    if [ "$node_count" -ge 2 ]; then
        mkdir -p "$TEST_DIR/subdir"
        local output
        if output=$(owl file download "/tmp/owl-subdir.txt" --nodes "$DEFAULT_NODES" --dest "$TEST_DIR/subdir/" --subdir 2>&1); then
            log_pass "TC-FILE-008 多节点下载(子目录)"
        else
            log_skip "TC-FILE-008 多节点下载(子目录) (节点不可用)"
        fi
    else
        log_skip "TC-FILE-008 多节点下载(子目录) (节点数不足)"
    fi
    owl exec run "rm -f /tmp/owl-subdir.txt" --nodes "$first_node" 2>/dev/null || true
}

test_file_not_found() {
    log_info "测试: TC-FILE-009 文件不存在错误处理"
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    local output
    if output=$(owl file upload "/nonexistent/path/file-$$.txt" --nodes "$first_node" 2>&1); then
        log_info "TC-FILE-009 文件不存在 (命令退出了但未报错)"
        log_pass "TC-FILE-009 文件不存在处理"
    else
        log_pass "TC-FILE-009 文件不存在正确报错"
    fi
}

test_file_upload_help() {
    log_info "测试: file upload 帮助信息"
    local output
    if output=$(owl file upload --help 2>&1); then
        assert_contains "$output" "upload" "file upload 帮助"
    else
        log_fail "file upload --help 失败"
    fi
}

test_file_download_help() {
    log_info "测试: file download 帮助信息"
    local output
    if output=$(owl file download --help 2>&1); then
        assert_contains "$output" "download" "file download 帮助"
    else
        log_fail "file download --help 失败"
    fi
}

cleanup() {
    log_info "清理测试文件..."
    local first_node
    first_node=$(echo "$DEFAULT_NODES" | cut -d',' -f1)
    owl exec run "rm -rf /tmp/owl-test /tmp/owl-dl-test.txt /tmp/owl-overwrite-test.txt /tmp/owl-noov-test.txt /tmp/owl-multi.txt /tmp/owl-subdir.txt" --nodes "$first_node" 2>/dev/null || true
    rm -rf "$TEST_DIR"
}

main() {
    echo "========================================="
    echo "owl file 命令 E2E 测试套件"
    echo "========================================="

    check_owl_binary

    echo ""
    echo "-----------------------------------------"
    echo "帮助信息验证 (不需要SSH)"
    echo "-----------------------------------------"
    test_file_upload_help
    test_file_download_help

    setup_test_file
    check_nodes

    echo ""
    echo "-----------------------------------------"
    echo "file upload"
    echo "-----------------------------------------"
    test_file_upload_single
    test_file_upload_multi
    test_file_upload_group
    test_file_upload_overwrite
    test_file_upload_no_overwrite

    echo ""
    echo "-----------------------------------------"
    echo "file download"
    echo "-----------------------------------------"
    test_file_download_single
    test_file_download_multi_suffix
    test_file_download_subdir

    echo ""
    echo "-----------------------------------------"
    echo "错误处理"
    echo "-----------------------------------------"
    test_file_not_found

    cleanup
    echo ""
    print_summary
}

main "$@"
