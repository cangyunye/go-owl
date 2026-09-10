#!/bin/bash
# 共享 owl.db 写并发演练：CLI(无 busy_timeout 池) + serve API(有 busy_timeout DSN)
# + monitor 引擎(独立单连接池) 同时压同一个库,统计 locked 失败
set -u
cd "$(dirname "$0")/.."
PORT=18099
DB=.reviewtmp/drill.db
SL=.reviewtmp/drill-serve.log
D=.reviewtmp/drill
B="http://127.0.0.1:$PORT/api/v1"
DUR=${1:-240}   # 压测秒数

rm -rf "$DB" "$DB-wal" "$DB-shm" "$SL" "$D"; mkdir -p "$D"

OWL_DB_PATH="$DB" .reviewtmp/owl-serve --port $PORT --host 127.0.0.1 > "$SL" 2>&1 &
SRV=$!
READY=0
for i in $(seq 1 60); do
  curl -s --max-time 2 -o /dev/null "$B/health" && READY=1 && break
  grep -q "bind: address already in use" "$SL" 2>/dev/null && break
  sleep 0.3
done
if [ "$READY" != "1" ]; then echo "FATAL: 端口 $PORT 不可用或服务未启动"; grep -i "error\|fatal" "$SL" | head -3; kill $SRV 2>/dev/null; exit 1; fi
for i in $(seq 1 60); do grep -q "^Password:" "$SL" 2>/dev/null && break; sleep 0.3; done
PASS=$(grep -m1 "^Password:" "$SL" | awk '{print $2}')
ATOK=$(curl -s --max-time 5 $B/login -X POST -H 'Content-Type: application/json' -d "{\"username\":\"admin\",\"password\":\"$PASS\"}" | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
A="Authorization: Bearer $ATOK"
curl -s --max-time 10 -X POST -H "$A" $B/nodes/seed > /dev/null
echo "== seed 50 不可达节点完成,monitor 每 60s 一轮采集,压测 ${DUR}s =="

STOP="$D/STOP"

api_writer() {
  local wid=$1 ok=0 fail=0 c id
  while [ ! -f "$STOP" ]; do
    id="api-${wid}-$RANDOM"
    c=$(curl -s --max-time 10 -o /dev/null -w '%{http_code}' -X POST -H "$A" -H 'Content-Type: application/json' \
      -d "{\"id\":\"$id\",\"name\":\"t\",\"address\":\"10.8.0.1\",\"user\":\"root\"}" $B/nodes)
    if [ "$c" = "201" ]; then ok=$((ok+1)); else fail=$((fail+1)); echo "$id create=$c" >> "$D/api-fail.log"; fi
    c=$(curl -s --max-time 10 -o /dev/null -w '%{http_code}' -X PUT -H "$A" -H 'Content-Type: application/json' \
      -d '{"name":"t2"}' $B/nodes/$id)
    if [ "$c" = "200" ]; then ok=$((ok+1)); else fail=$((fail+1)); echo "$id put=$c" >> "$D/api-fail.log"; fi
    c=$(curl -s --max-time 10 -o /dev/null -w '%{http_code}' -X DELETE -H "$A" $B/nodes/$id)
    if [ "$c" = "200" ]; then ok=$((ok+1)); else fail=$((fail+1)); echo "$id del=$c" >> "$D/api-fail.log"; fi
  done
  echo "$ok $fail" > "$D/api-$wid.stat"
}

cli_writer() {
  local wid=$1 ok=0 fail=0 id
  export OWL_DB_PATH="$PWD/$DB"
  while [ ! -f "$STOP" ]; do
    id="cli-${wid}-$RANDOM"
    if .reviewtmp/owl node add "$id" --address 10.9.0.1 --name t >> "$D/cli-$wid.out" 2>> "$D/cli-$wid.err"; then
      ok=$((ok+1))
    else
      fail=$((fail+1)); echo "add $id" >> "$D/cli-fail.log"
    fi
    if .reviewtmp/owl node remove "$id" >> "$D/cli-$wid.out" 2>> "$D/cli-$wid.err"; then
      ok=$((ok+1))
    else
      fail=$((fail+1)); echo "rm $id" >> "$D/cli-fail.log"
    fi
  done
  echo "$ok $fail" > "$D/cli-$wid.stat"
}

reader() {
  local n=0
  while [ ! -f "$STOP" ]; do
    curl -s --max-time 5 -o /dev/null -H "$A" $B/nodes && n=$((n+1))
    sleep 2
  done
  echo "$n" > "$D/reader.stat"
}

for w in 1 2 3 4 5 6; do api_writer $w & done
for w in 1 2 3; do cli_writer $w & done
reader &

sleep $DUR
touch "$STOP"
sleep 3

API_OK=0; API_FAIL=0
for f in "$D"/api-*.stat; do [ -f "$f" ] || continue; read -r o f2 < "$f"; API_OK=$((API_OK+o)); API_FAIL=$((API_FAIL+f2)); done
CLI_OK=0; CLI_FAIL=0
for f in "$D"/cli-*.stat; do [ -f "$f" ] || continue; read -r o f2 < "$f"; CLI_OK=$((CLI_OK+o)); CLI_FAIL=$((CLI_FAIL+f2)); done

echo "== 统计 =="
echo "serve API 写入: ok=$API_OK fail=$API_FAIL"
echo "CLI 写入:       ok=$CLI_OK fail=$CLI_FAIL"
echo "读请求成功:     $(cat "$D/reader.stat" 2>/dev/null)"
echo "serve 日志 locked/busy: $(grep -ci 'database is locked\|SQLITE_BUSY' "$SL")"
echo "CLI 输出含 locked/busy 的文件数: $(grep -ril 'database is locked\|SQLITE_BUSY' "$D" 2>/dev/null | wc -l | tr -d ' ')"
echo "监控轮次告警日志:       $(grep -c '监控采集轮次失败\|监控清理任务失败' "$SL")"
echo "API 失败明细(前5):"; head -5 "$D/api-fail.log" 2>/dev/null
echo "CLI 失败明细(前5):"; head -5 "$D/cli-fail.log" 2>/dev/null
echo "CLI 错误样例:"; grep -h -m2 -i "locked\|busy\|错误\|error" "$D"/cli-*.err 2>/dev/null | head -4
kill $SRV 2>/dev/null
echo "== 演练结束(时长 ${DUR}s) =="
