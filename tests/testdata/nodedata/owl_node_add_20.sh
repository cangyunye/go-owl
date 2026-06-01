#!/bin/bash
# Owl Node Add 测试数据 - 20条

# Node 1-5: Web服务器组
owl node add node001 --name "web-server-01" --address "10.10.1.101" --port 22 --user root --groups "web,production" --labels "env=prod,appname=owl,owner=张三,region=cn-east"
owl node add node002 --name "web-server-02" --address "10.10.1.102" --port 22 --user root --groups "web,production" --labels "env=prod,appname=owl,owner=李四,region=cn-east"
owl node add node003 --name "web-server-03" --address "10.10.1.103" --port 22 --user root --groups "web,staging" --labels "env=staging,appname=owl,owner=王五,region=cn-east"
owl node add node004 --name "web-server-04" --address "10.10.1.104" --port 22 --user root --groups "web,dev" --labels "env=dev,appname=owl,owner=赵六,region=cn-north"
owl node add node005 --name "web-server-05" --address "10.10.1.105" --port 22 --user root --groups "web,production" --labels "env=prod,appname=owl,owner=张三,region=cn-east"

# Node 6-10: 数据库服务器组
owl node add node006 --name "db-master-01" --address "10.20.1.201" --port 22 --user dbadmin --groups "database,mysql" --labels "env=prod,dbtype=mysql,owner=张三,region=cn-east"
owl node add node007 --name "db-slave-01" --address "10.20.1.202" --port 22 --user dbadmin --groups "database,mysql" --labels "env=prod,dbtype=mysql,owner=张三,region=cn-east"
owl node add node008 --name "db-postgres-01" --address "10.20.1.203" --port 22 --user postgres --groups "database,postgres" --labels "env=staging,dbtype=postgres,owner=李四,region=cn-north"
owl node add node009 --name "db-mongo-01" --address "10.20.1.204" --port 22 --user mongo --groups "database,mongodb" --labels "env=dev,dbtype=mongodb,owner=王五,region=cn-south"
owl node add node010 --name "db-redis-01" --address "10.20.1.205" --port 22 --user redis --groups "database,redis" --labels "env=prod,dbtype=redis,owner=李四,region=cn-east"

# Node 11-15: 应用服务器组
owl node add node011 --name "app-api-01" --address "10.30.1.301" --port 22 --user appuser --groups "app,api" --labels "env=prod,apptype=api,owner=赵六,region=cn-east"
owl node add node012 --name "app-api-02" --address "10.30.1.302" --port 22 --user appuser --groups "app,api" --labels "env=staging,apptype=api,owner=赵六,region=cn-north"
owl node add node013 --name "app-worker-01" --address "10.30.1.303" --port 22 --user appuser --groups "app,worker" --labels "env=prod,apptype=worker,owner=钱七,region=cn-east"
owl node add node014 --name "app-worker-02" --address "10.30.1.304" --port 22 --user appuser --groups "app,worker" --labels "env=dev,apptype=worker,owner=钱七,region=cn-south"
owl node add node015 --name "app-batch-01" --address "10.30.1.305" --port 22 --user appuser --groups "app,batch" --labels "env=prod,apptype=batch,owner=孙八,region=cn-east"

# Node 16-20: 基础设施组
owl node add node016 --name "infra-proxy-01" --address "10.40.1.401" --port 22 --user infra --groups "infrastructure,proxy" --labels "env=prod,role=proxy,owner=周九,region=cn-east"
owl node add node017 --name "infra-lb-01" --address "10.40.1.402" --port 22 --user infra --groups "infrastructure,lb" --labels "env=prod,role=loadbalancer,owner=周九,region=cn-north"
owl node add node018 --name "infra-cache-01" --address "10.40.1.403" --port 22 --user infra --groups "infrastructure,cache" --labels "env=staging,role=cache,owner=吴十,region=cn-south"
owl node add node019 --name "infra-mq-01" --address "10.40.1.404" --port 22 --user infra --groups "infrastructure,mq" --labels "env=prod,role=messagequeue,owner=郑一,region=cn-east"
owl node add node020 --name "infra-log-01" --address "10.40.1.405" --port 22 --user infra --groups "infrastructure,logging" --labels "env=prod,role=logging,owner=郑一,region=cn-north"
