#!/usr/bin/env bash
set -euo pipefail
OWL=./build/darwin-arm64/owl

echo "==> 添加 5 个测试节点"

$OWL node add web-01 \
  --name "web-server-01" \
  --address 10.0.1.10 \
  --groups web,production \
  --labels env=prod,app=nginx,region=us-east

$OWL node add db-01 \
  --name "db-master-01" \
  --address 10.0.2.10 \
  --groups db,production \
  --labels env=prod,role=primary,engine=postgres

$OWL node add cache-01 \
  --name "redis-cache-01" \
  --address 10.0.3.10 \
  --groups cache,production \
  --labels env=prod,role=main,engine=redis

$OWL node add monitor-01 \
  --name "monitor-server" \
  --address 10.0.4.10 \
  --groups monitor,staging \
  --labels env=staging,tier=observability

$OWL node add dev-box \
  --name "dev-sandbox" \
  --address 10.0.100.10 \
  --groups dev \
  --labels env=dev,owner=team-a,app=sandbox

echo ""
echo "==> 完成"
$OWL node list
