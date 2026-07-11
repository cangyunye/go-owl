#!/usr/bin/env bash
set -euo pipefail
OWL=owl

echo "==> 添加 50 个测试节点"

# ========== Web 节点 ==========
$OWL node add web-01 --name "web-nginx-01"   --address 10.0.1.10 --user nginx   --groups web,production  --labels env=prod,app=nginx,owner=zhangwei,region=us-east
$OWL node add web-02 --name "web-nginx-02"   --address 10.0.1.11 --user nginx   --groups web,production  --labels env=prod,app=nginx,owner=zhangwei,region=us-east
$OWL node add web-03 --name "web-apache-01"  --address 10.0.1.12 --user apache  --groups web,production  --labels env=prod,app=httpd,owner=wangfang,region=us-west
$OWL node add web-04 --name "web-apache-02"  --address 10.0.1.13 --user apache  --groups web,staging    --labels env=staging,app=httpd,owner=wangfang,region=us-west
$OWL node add web-05 --name "web-nginx-03"   --address 10.0.1.14 --user nginx   --groups web,staging    --labels env=staging,app=nginx,owner=zhaoming,region=ap-east
$OWL node add web-06 --name "web-k8s-ingress" --address 10.0.1.15 --user k8s    --groups web,k8s       --labels env=prod,app=ingress,owner=lisong,region=ap-east
$OWL node add web-07 --name "web-k8s-proxy"  --address 10.0.1.16 --user k8s     --groups web,k8s       --labels env=staging,app=envoy,owner=lisong,region=ap-east
$OWL node add web-08 --name "web-flume-agg"  --address 10.0.1.17 --user flume   --groups web,worker    --labels env=prod,app=flume,owner=chenyu,region=us-east
$OWL node add web-09 --name "web-dev-01"     --address 10.0.1.100 --user nginx  --groups web,dev       --labels env=dev,app=nginx,owner=liuyi,region=dev
$OWL node add web-10 --name "web-dev-02"     --address 10.0.1.101 --user apache --groups web,dev       --labels env=dev,app=httpd,owner=liuyi,region=dev

# ========== DB 节点 ==========
$OWL node add db-01  --name "db-mysql-master" --address 10.0.2.10 --user oracle   --groups db,production   --labels env=prod,role=master,engine=mysql,owner=huangli
$OWL node add db-02  --name "db-mysql-slave"  --address 10.0.2.11 --user oracle   --groups db,production   --labels env=prod,role=slave,engine=mysql,owner=huangli
$OWL node add db-03  --name "db-pg-master"    --address 10.0.2.12 --user oracle   --groups db,production   --labels env=prod,role=master,engine=postgres,owner=zhangwei
$OWL node add db-04  --name "db-pg-slave"     --address 10.0.2.13 --user oracle   --groups db,production   --labels env=prod,role=slave,engine=postgres,owner=zhangwei
$OWL node add db-05  --name "db-redis-01"     --address 10.0.2.14 --user redis    --groups db,cache       --labels env=prod,role=primary,engine=redis,owner=zhaoming
$OWL node add db-06  --name "db-redis-02"     --address 10.0.2.15 --user redis    --groups db,cache       --labels env=prod,role=replica,engine=redis,owner=zhaoming
$OWL node add db-07  --name "db-staging-01"   --address 10.0.2.20 --user oracle   --groups db,staging     --labels env=staging,engine=mysql,owner=chenyu
$OWL node add db-08  --name "db-staging-02"   --address 10.0.2.21 --user oracle   --groups db,staging     --labels env=staging,engine=postgres,owner=chenyu
$OWL node add db-09  --name "db-dev-01"       --address 10.0.2.100 --user oracle  --groups db,dev        --labels env=dev,engine=mysql,owner=liuyi
$OWL node add db-10  --name "db-dev-02"       --address 10.0.2.101 --user oracle  --groups db,dev        --labels env=dev,engine=postgres,owner=liuyi

# ========== Cache 节点 ==========
$OWL node add cache-01 --name "cache-redis-01" --address 10.0.3.10 --user redis    --groups cache,production --labels env=prod,role=main,engine=redis,owner=wangfang
$OWL node add cache-02 --name "cache-redis-02" --address 10.0.3.11 --user redis    --groups cache,production --labels env=prod,role=main,engine=redis,owner=wangfang
$OWL node add cache-03 --name "cache-mc-01"    --address 10.0.3.12 --user ttadm  --groups cache,production --labels env=prod,role=main,engine=memcache,owner=lisong
$OWL node add cache-04 --name "cache-kafka-01" --address 10.0.3.13 --user kafka   --groups cache,kafka     --labels env=prod,role=broker,engine=kafka,owner=huangli
$OWL node add cache-05 --name "cache-kafka-02" --address 10.0.3.14 --user kafka   --groups cache,kafka     --labels env=prod,role=broker,engine=kafka,owner=huangli
$OWL node add cache-06 --name "cache-dev-01"   --address 10.0.3.100 --user redis  --groups cache,dev      --labels env=dev,role=main,engine=redis,owner=zhaoming

# ========== Worker 节点 ==========
$OWL node add worker-01 --name "worker-job-01" --address 10.0.4.10 --user ttadm --groups worker,production --labels env=prod,task=job,owner=zhangwei
$OWL node add worker-02 --name "worker-job-02" --address 10.0.4.11 --user ttadm --groups worker,production --labels env=prod,task=job,owner=zhangwei
$OWL node add worker-03 --name "worker-etl-01" --address 10.0.4.12 --user ttadm --groups worker,production --labels env=prod,task=etl,owner=chenyu
$OWL node add worker-04 --name "worker-flume-01" --address 10.0.4.13 --user flume --groups worker,flume   --labels env=prod,app=flume,owner=wangfang
$OWL node add worker-05 --name "worker-k8s-node-01" --address 10.0.4.14 --user k8s --groups worker,k8s   --labels env=prod,role=node,owner=lisong
$OWL node add worker-06 --name "worker-k8s-node-02" --address 10.0.4.15 --user k8s --groups worker,k8s   --labels env=prod,role=node,owner=lisong
$OWL node add worker-07 --name "worker-dev-01" --address 10.0.4.100 --user ttadm --groups worker,dev      --labels env=dev,task=job,owner=liuyi

# ========== Monitor 节点 ==========
$OWL node add monitor-01 --name "monitor-prom-01" --address 10.0.5.10 --user ttadm  --groups monitor,production --labels env=prod,app=prometheus,owner=zhaoming
$OWL node add monitor-02 --name "monitor-grafana" --address 10.0.5.11 --user ttadm  --groups monitor,production --labels env=prod,app=grafana,owner=zhaoming
$OWL node add monitor-03 --name "monitor-es-01"   --address 10.0.5.12 --user oracle --groups monitor,production --labels env=prod,app=elasticsearch,owner=huangli
$OWL node add monitor-04 --name "monitor-es-02"   --address 10.0.5.13 --user oracle --groups monitor,production --labels env=prod,app=elasticsearch,owner=huangli
$OWL node add monitor-05 --name "monitor-zookeeper" --address 10.0.5.14 --user zookeeper --groups monitor,zookeeper --labels env=prod,app=zookeeper,owner=zhangwei
$OWL node add monitor-06 --name "monitor-alert"    --address 10.0.5.15 --user ttadm --groups monitor,staging  --labels env=staging,app=alertmanager,owner=chenyu
$OWL node add monitor-07 --name "monitor-dev-01"   --address 10.0.5.100 --user ttadm --groups monitor,dev    --labels env=dev,app=prometheus,owner=liuyi

# ========== Gateway 节点 ==========
$OWL node add gateway-01 --name "gw-haproxy-01"  --address 10.0.6.10 --user ttadm  --groups gateway,production --labels env=prod,app=haproxy,owner=wangfang
$OWL node add gateway-02 --name "gw-haproxy-02"  --address 10.0.6.11 --user ttadm  --groups gateway,production --labels env=prod,app=haproxy,owner=wangfang
$OWL node add gateway-03 --name "gw-nginx-01"    --address 10.0.6.12 --user nginx   --groups gateway,production --labels env=prod,app=nginx,owner=zhangwei
$OWL node add gateway-04 --name "gw-k8s-lb"      --address 10.0.6.13 --user k8s    --groups gateway,k8s        --labels env=prod,app=metallb,owner=lisong
$OWL node add gateway-05 --name "gw-staging-01"  --address 10.0.6.20 --user nginx  --groups gateway,staging    --labels env=staging,app=nginx,owner=chenyu
$OWL node add gateway-06 --name "gw-dev-01"      --address 10.0.6.100 --user nginx --groups gateway,dev        --labels env=dev,app=nginx,owner=liuyi
$OWL node add gateway-07 --name "gw-dev-02"      --address 10.0.6.101 --user apache --groups gateway,dev       --labels env=dev,app=httpd,owner=liuyi

# ========== Kafka / ZK 专用节点 ==========
$OWL node add kafka-01 --name "kafka-broker-01" --address 10.0.7.10 --user kafka   --groups kafka,production --labels env=prod,role=broker,owner=huangli
$OWL node add kafka-02 --name "kafka-broker-02" --address 10.0.7.11 --user kafka   --groups kafka,production --labels env=prod,role=broker,owner=huangli
$OWL node add kafka-03 --name "kafka-broker-03" --address 10.0.7.12 --user kafka   --groups kafka,production --labels env=prod,role=broker,owner=huangli
$OWL node add zk-01    --name "zookeeper-01"    --address 10.0.7.20 --user zookeeper --groups zookeeper,production --labels env=prod,role=leader,owner=zhangwei
$OWL node add zk-02    --name "zookeeper-02"    --address 10.0.7.21 --user zookeeper --groups zookeeper,production --labels env=prod,role=follower,owner=zhangwei
$OWL node add zk-03    --name "zookeeper-03"    --address 10.0.7.22 --user zookeeper --groups zookeeper,production --labels env=prod,role=follower,owner=zhangwei

echo ""
echo "==> 全部节点:"
$OWL node list
