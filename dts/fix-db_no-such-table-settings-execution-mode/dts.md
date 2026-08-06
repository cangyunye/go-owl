---
id: "fix-db_no-such-table-settings-execution-mode"
domain: "fix-db"
slug: "no-such-table-settings-execution-mode"
title: "1. owl-serve 报错 no such table settings;2. 前端节点执行命令提示 has no column named executi"
status: "resolved"
created: "2026-08-06T21:26:28+08:00"
resolved: "2026-08-06T21:43:56+08:00"
commit: "9fa4d59"
branch: "feat/ui-redesign-phase0"
platform: "darwin"
session: "ses_028bf18e5ffeyGS6BPghJKTwdn"
---

# fix-db_no-such-table-settings-execution-mode

## 问题

1. owl-serve 报错 no such table settings;2. 前端节点执行命令提示 has no column named execution_mode

## 环境

| 项 | 值 |
|----|----|
| git commit | 9fa4d59 |
| 分支 | feat/ui-redesign-phase0 |
| 平台 | darwin |
| 建档时间 | 2026-08-06T21:26:28+08:00 |
| 会话 | ses_028bf18e5ffeyGS6BPghJKTwdn |

## 调查过程

- [21:26] 建档
- [21:43] 记录日志 (chat): 两个报错根因定位结论
- [21:43] 新增 E2E 用例: E2E: 旧库(缺 settings 表 + operations 缺列)上 --reset-admin 与前端 exec
- [21:43] 记录日志 (bash): 两个修复的 E2E 关键输出
- [21:43] 记录证据 1 项
- [21:43] 记录终端文本快照
- [21:43] 结案

## 日志与摘录

### [chat] 2026-08-06T21:43:27+08:00 · 两个报错根因定位结论

```
根因（均为存量旧库迁移缺失）:

1. no such table settings
   - cmd/plugins/serve/server.go 的 Server.Init() 会先 initSettings() 建 settings 表,正常启动没问题。
   - 但 ResetAdmin()（--reset-admin 路径）打开库后只初始化 UserStore,直接调 getOrCreateJWTSecret() 查询 settings 表,从未创建该表。
   - 旧版本建出的 ~/.owl/owl.db 没有 settings 表时,--reset-admin 必现 "SQL logic error: no such table: settings"。
   - 复现: Init() 建库后 DROP TABLE settings,再 --reset-admin → 报错。

2. has no column named execution_mode
   - 早期 CLI schema operations 表只有 7 列(id/task_id/op_type/command/targets/status/created_at),execution_mode/playbook_path/current_task_index/current_task_phase 为后续新增,forced 最近新增(已有 EnsureForcedColumn 迁移)。
   - CREATE TABLE IF NOT EXISTS 对存量表不生效;此前只有 forced 有迁移。
   - 前端执行命令走 POST /api/v1/exec → HistoryStore.RecordOperation → INSERT ... execution_mode → 旧库报 "table operations has no column named execution_mode"。
   - serve 与 CLI 共用 ~/.owl/owl.db;internal/history 侧同样只迁移 forced,CLI 执行同样会踩坑。
   - 复现: 把 operations 表替换成 7 列旧 schema,serve 启动后 exec → 报错。
```

### [bash] 2026-08-06T21:43:40+08:00 · 两个修复的 E2E 关键输出

```
$ HOME=$TMPHOME ./build/owl-serve --reset-admin --port 18098
Admin password has been reset.
Username: admin
Password: kxNCb3Xxqerq

# 修复前该命令报: reset admin: jwt secret: SQL logic error: no such table: settings (1)

# 启动后迁移生效
operations cols: ['id','task_id','op_type','command','targets','status','created_at','execution_mode','playbook_path','current_task_index','current_task_phase','forced']
settings exists: True

# exec 落库
('7260618d-...', 'command', 'uptime', '["node-01"]', 'running', '', '', 0, '', 0)

# history 读回
{"operation": {"id": 2, "task_id": "7260618d-...", "op_type": "command", "command": "uptime", "targets": ["node-01"], "status": "running", "execution_mode": "", "playbook_path": "", "current_task_index": 0, "current_task_phase": "", "forced": false, ...}}
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|
| 1 | E2E: 旧库(缺 settings 表 + operations 缺列)上 --reset-admin 与前端 exec | 1. 新 HOME 启动 owl-serve 建库;2. python 删 settings 表,operations 替换为 7 列旧 schema;3. owl-serve --reset-admin;4. owl-serve 正常启动;5. 登录+建节点+POST /api/v1/exec uptime;6. GET /api/v1/history | reset-admin 成功打印新口令;启动后 settings 表重建、operations 补齐 execution_mode 等 5 列;exec 落库无 no column 报错;history 返回含 execution_mode 的完整记录 | pass |

## 证据截图

[文本快照: E2E 终端文本快照: reset-admin 成功 + 迁移列清单 + exec 落库 + history 读回](shots/001-214345.txt)

## 修复方案

两个根因都是"存量旧库迁移缺失",均按 TDD 先写失败测试再修复:

1. --reset-admin 缺 settings 表: cmd/plugins/serve/server.go ResetAdmin() 在 getOrCreateJWTSecret() 前补调 initSettings(ctx, db),确保旧库也先建 settings 表。顺带把 TestServer_ResetAdmin_NoDB 从"期望报错"改为"自愈式建 schema 成功"(cmd/owl-serve main 已用 os.Stat 拦截不存在库,不会走到)。

2. operations 缺 execution_mode 等列: 把"只迁移 forced"泛化为通用列迁移。
   - serve 侧 cmd/plugins/serve/store/history.go: 新增 operationColumnSpecs(execution_mode/playbook_path/current_task_index/current_task_phase/forced)与 ensureOperationColumns(ctx),Init() 改调它;删除旧 ensureForcedColumn/addForcedColumn。
   - CLI 侧 internal/history: 接口方法 EnsureForcedColumn 改名 EnsureOperationColumns;db_sqlite3.go 用同样的 PRAGMA+ALTER 幂等迁移;db_duckdb.go 用 ADD COLUMN IF NOT EXISTS(VARCHAR 变体);并顺手清掉 db_duckdb.go session_commands 表里误粘贴的 4 行 ttttttt 垃圾列。
   - 两处均容忍并发迁移的 "duplicate column name",与既有 forced 迁移容错模式一致。

新增测试: serve store legacy 迁移测试、ResetAdmin legacy 库测试;internal/history sqlite3 legacy 迁移测试(补 !duckdb build tag,因用例用 AUTOINCREMENT 语法)与 duckdb 迁移测试。全部 go test ./... 与 go test -tags duckdb ./internal/history/ 通过。E2E 在旧库(缺 settings + 7 列 operations)上验证 reset-admin、启动迁移、exec 落库、history 读回全通过。

## 复盘

根因: 两个入口(Init 与 ResetAdmin)初始化路径不一致;且 CREATE TABLE IF NOT EXISTS 只对新库生效,存量库缺列只能靠逐列 ALTER。教训: (1) 任何打开同一 ~/.owl/owl.db 的入口都必须先 ensure 自己的表; (2) 共用 DB 的两侧 schema 演进时,新增列必须同时带上幂等迁移,不能只靠 CREATE TABLE IF NOT EXISTS; (3) duckdb 后端测试用例(sqlite3 AUTOINCREMENT 语法)应标 !duckdb,否则 duckdb 构建的测试会失败。
