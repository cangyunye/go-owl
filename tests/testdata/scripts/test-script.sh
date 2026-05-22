#!/bin/bash
set -e

echo "========================================="
echo "测试脚本执行"
echo "========================================="
echo "开始时间: $(date)"
echo "主机名: $(hostname)"
echo "用户: $(whoami)"
echo ""

echo "创建测试目录..."
mkdir -p /tmp/owl-test
mkdir -p /tmp/owl-test/logs

echo "创建测试文件..."
echo "测试数据 - $(date)" > /tmp/owl-test/test.txt
echo "文件创建成功"
cat /tmp/owl-test/test.txt

echo ""
echo "测试完成!"
echo "结束时间: $(date)"
echo "========================================="

exit 0
