#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> 构建 owl"
go build -o build/owl ./cmd/cli

TMP_HOME="$(mktemp -d)"
mkdir -p "$TMP_HOME/.owl"
OUT="$(mktemp)"

cleanup() {
  rm -f "$OUT"
  rm -rf "$TMP_HOME"
}
trap cleanup EXIT

# run_tui <home> <outfile> — 在隔离 HOME 下经 pty 启动 owl tui,喂按键,等退出。
# TERM=dumb:script 下的 pty 没有终端模拟器应答 OSC11/CSI6n 查询,bubbletea 会阻塞
# 等背景色/光标位置响应;TERM=dumb 走 NoTTY 路径,跳过查询。
# watchdog 兜底防挂死(macOS 无 GNU timeout);set -e 下先取 RC 再判(默认 0)。
run_tui() {
  local home="$1" out="$2"
  ( sleep 1.5
    printf 'a'    # 打开新增表单
    sleep 0.6
    printf '\033' # Esc 返回列表
    sleep 0.6
    printf 'q'    # 退出
  ) | TERM=dumb HOME="$home" script -q "$out" ./build/owl tui >/dev/null 2>&1 &
  local pipid=$!
  ( sleep 20 && kill "$pipid" 2>/dev/null ) &
  local timer=$!
  local rc=0
  wait "$pipid" || rc=$?
  kill "$timer" 2>/dev/null || true
  return "$rc"
}

echo "==> E2E 场景 1: 干净数据下 owl tui 渲染列表/开表单/退出"
if ! run_tui "$TMP_HOME" "$OUT"; then
  echo "FAIL: 场景 1 owl tui 未干净退出(可能挂死)"
  cat "$OUT"
  exit 1
fi
echo "PASS: 场景 1 干净退出(rc=0)"
if grep -q "/nodes" "$OUT"; then
  echo "PASS: 场景 1 面包屑 /nodes 渲染"
else
  echo "FAIL: 场景 1 未渲染面包屑 /nodes"
  cat "$OUT"
  exit 1
fi
if grep -q "添加节点" "$OUT"; then
  echo "PASS: 场景 1 新增表单弹出"
else
  echo "FAIL: 场景 1 未渲染新增表单"
  cat "$OUT"
  exit 1
fi

echo "==> E2E 场景 2: nodes.json↔db 冲突数据下 owl tui 不被交互提示阻塞"
# 先经 owl node add 写一个 DB 节点(status=offline),再手写冲突的 nodes.json(status=online)
# -> DetectConflicts 发现 cross_source_id_fields 冲突;若读路径触发交互提示会卡死 TUI。
TERM=dumb HOME="$TMP_HOME" ./build/owl node add mac \
  --name mac-mini-m4 --address 192.168.18.100 --port 22 --user vigil \
  --groups test --labels env=dev >/dev/null 2>&1 || true
cat > "$TMP_HOME/.owl/nodes.json" <<'EOF'
[{"id":"mac","name":"mac-mini-m4","address":"192.168.18.100","port":22,"user":"vigil","password":"","status":"online","groups":["test"],"labels":{"env":"dev"}}]
EOF

if ! run_tui "$TMP_HOME" "$OUT"; then
  echo "FAIL: 场景 2 owl tui 未干净退出(rc=$?,可能仍被冲突提示阻塞)"
  cat "$OUT"
  exit 1
fi
if grep -q "/nodes" "$OUT"; then
  echo "PASS: 场景 2 冲突数据下 owl tui 启动并渲染 /nodes"
else
  echo "FAIL: 场景 2 冲突数据下未渲染 /nodes"
  cat "$OUT"
  exit 1
fi

echo "==> TUI E2E 冒烟通过"
