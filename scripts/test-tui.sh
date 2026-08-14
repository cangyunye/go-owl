#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> 构建 owl"
go build -o build/owl ./cmd/cli

OUT="$(mktemp)"
TMP_HOME="$(mktemp -d)"
mkdir -p "$TMP_HOME/.owl"

cleanup() {
  rm -f "$OUT"
  rm -rf "$TMP_HOME"
}
trap cleanup EXIT

echo "==> E2E: 启动 owl tui,进入 /nodes 渲染列表,开表单,返回,退出"
# TERM=dumb:script 下的 pty 没有终端模拟器应答 OSC11/CSI6n 查询,bubbletea 会阻塞
# 等背景色/光标位置响应;TERM=dumb 走 NoTTY 路径,跳过查询。
# HOME 隔离:避免 ~/.owl 里真实数据触发 nodes.json↔db 冲突交互提示,阻塞 TUI 启动。
( sleep 1.5
  printf 'a'    # 打开新增表单
  sleep 0.6
  printf '\033' # Esc 返回列表
  sleep 0.6
  printf 'q'    # 退出
) | TERM=dumb HOME="$TMP_HOME" script -q "$OUT" ./build/owl tui >/dev/null 2>&1 || true

echo "==> 校验输出"
if grep -q "/nodes" "$OUT"; then
  echo "PASS: 面包屑 /nodes 渲染"
else
  echo "FAIL: 未渲染面包屑 /nodes"
  cat "$OUT"
  exit 1
fi

if grep -q "添加节点" "$OUT"; then
  echo "PASS: 新增表单弹出"
else
  echo "FAIL: 未渲染新增表单"
  cat "$OUT"
  exit 1
fi

echo "==> TUI E2E 冒烟通过"
