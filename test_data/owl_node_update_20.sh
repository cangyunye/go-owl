#!/bin/bash
# Owl Node Update 测试数据 - 20条（对应上面的node add）

# 更新 owner 标签测试
owl node update node001 --labels "owner=张三,env=prod,appname=owl,region=cn-east,tier=frontend"
owl node update node002 --labels "owner=李四,env=prod,appname=owl,region=cn-east,tier=frontend"
owl node update node003 --labels "owner=王五,env=staging,appname=owl,region=cn-east,tier=frontend"
owl node update node004 --labels "owner=赵六,env=dev,appname=owl,region=cn-north,tier=frontend"
owl node update node005 --labels "owner=张三,env=prod,appname=owl,region=cn-east,tier=frontend"

# 更新数据库节点 owner 测试
owl node update node006 --labels "owner=张三,env=prod,dbtype=mysql,region=cn-east,role=master"
owl node update node007 --labels "owner=张三,env=prod,dbtype=mysql,region=cn-east,role=slave"
owl node update node008 --labels "owner=李四,env=staging,dbtype=postgres,region=cn-north,role=primary"
owl node update node009 --labels "owner=王五,env=dev,dbtype=mongodb,region=cn-south,role=shard"
owl node update node010 --labels "owner=李四,env=prod,dbtype=redis,region=cn-east,role=cluster"

# 更新应用服务器 owner 测试
owl node update node011 --labels "owner=赵六,env=prod,apptype=api,region=cn-east,version=v2.1"
owl node update node012 --labels "owner=赵六,env=staging,apptype=api,region=cn-north,version=v2.2"
owl node update node013 --labels "owner=钱七,env=prod,apptype=worker,region=cn-east,queue=high"
owl node update node014 --labels "owner=钱七,env=dev,apptype=worker,region=cn-south,queue=low"
owl node update node015 --labels "owner=孙八,env=prod,apptype=batch,region=cn-east,schedule=daily"

# 更新基础设施 owner 测试
owl node update node016 --labels "owner=周九,env=prod,role=proxy,region=cn-east,protocol=http"
owl node update node017 --labels "owner=周九,env=prod,role=loadbalancer,region=cn-north,algorithm=roundrobin"
owl node update node018 --labels "owner=吴十,env=staging,role=cache,region=cn-south,type=varnish"
owl node update node019 --labels "owner=郑一,env=prod,role=messagequeue,region=cn-east,broker=rabbitmq"
owl node update node020 --labels "owner=郑一,env=prod,role=logging,region=cn-north,collector=filebeat"
