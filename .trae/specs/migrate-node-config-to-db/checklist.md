# Checklist

## 数据库 Schema
- [x] SQLite3 版本的 `nodes` 表 DDL 正确创建（含所有字段和索引）
- [x] DuckDB 版本的 `nodes` 表 DDL 正确创建（使用 SEQUENCE + JSON 类型）
- [x] 两个版本 `InitSchema()` 执行不报错

## NodeStoreDB CRUD
- [x] `Add()` 正确 INSERT 节点数据，`groups`/`labels` 序列化为 JSON
- [x] `Get()` 按 ID 查询并正确反序列化 JSON 字段
- [x] `List()` 返回所有节点
- [x] `Update()` 更新节点并刷新 `updated_at`
- [x] `Remove()` 删除节点成功
- [x] `Save()`/`Load()` 空操作保持接口兼容（不崩溃）

## LocalSource 数据库读取 + nodes.json 覆盖
- [x] 数据库有节点时，`GetNode()` 返回数据库中的节点
- [x] `nodes.json` 中存在同名节点时，覆盖数据库中的配置
- [x] `nodes.json` 不存在时静默降级，不影响其他功能
- [x] `nodes.json` JSON 格式错误时输出警告，使用数据库数据

## 初始化顺序
- [x] `root.go Execute()` 先初始化数据库连接，再初始化节点存储
- [x] 空数据库首次启动不报错
- [x] 全局 DB 连接被节点存储正确复用

## CLI 子命令
- [x] `owl node add` 节点写入数据库
- [x] `owl node remove` 从数据库删除
- [x] `owl node list` 从数据库查询
- [x] `owl node update` 更新数据库记录
- [x] `owl node export` 从数据库导出
- [x] `owl node import` 导入到数据库
- [x] `owl node ping/check` 通过 resolver 解析节点正常工作
- [x] `owl exec run/script` 通过 resolver 解析节点正常工作
- [x] `owl file upload/download/transfer` 通过 resolver 解析节点正常工作
- [x] `owl playbook run` 通过 resolver 解析节点正常工作
- [x] `owl session attach` 通过 resolver 解析节点正常工作

## 自动迁移
- [x] 数据库为空 + nodes.json 存在时，自动导入节点
- [x] 数据库非空时跳过迁移
- [x] 迁移后不删除原 nodes.json 文件

## 测试
- [x] `go test ./internal/history/...` 通过
- [x] `go test ./cmd/cli/cmd/common/...` 通过
- [x] `go test ./internal/node/...` 通过
- [x] `go test ./cmd/cli/cmd/node/...` 通过
- [x] `go test ./...` 全部通过
- [x] 集成测试 `tests/scripts/test-node.sh` 通过
