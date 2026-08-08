# 命令执行日志落盘与下载 — 设计

日期:2026-08-06

## 目标
Web 控制台执行命令时,将每个节点本次执行的输出日志保存到本地磁盘;执行完成后前端可下载(单节点 .log + 整批次 zip);历史详情页也可回看下载。

## 决策(已与用户确认)
- 粒度:**按执行批次(opID)+ 节点**分文件保存
- 下载入口:**命令执行页(完成后)** + **历史详情弹窗**
- 文件内容:**元数据头部 + 完整输出**
- 写入时机:**任务结束后一次性写入**(不实时逐行)
- 下载形式:**zip 包 + 单节点 .log 下载**
- 清理:**跟随历史清理**(DELETE /history?days=N 同步删除过期日志目录)

## 存储布局
```
~/.owl/logs/executions/<opID>/<sanitized-nodeID>.log
~/.owl/logs/executions/<opID>/manifest.json
```
- opID = operation 的 `record_id`(uuid),与历史页天然关联
- nodeID 净化后作为文件名(去 `/`、`..`、空白、控制字符),原始 nodeID 写入文件头与 manifest
- 根目录可被 `OWL_LOG_DIR` 覆盖:`$OWL_LOG_DIR/executions`

## 日志文件格式
```
────────────────────────────────────────────
[2006-01-02 15:04:05] TASK: <taskID>
NODE: <nodeID>
COMMAND: <command>
EXIT CODE: <exit>
DURATION: <duration>
ERROR: <errMsg>          # 仅失败时
OUTPUT:
<完整输出>
────────────────────────────────────────────
```

manifest.json:
```json
{
  "op_id": "<opID>",
  "nodes": {
    "<sanitized-node-id>": {
      "node_id": "<orig>", "file": "<name>.log",
      "task_id": "...", "command": "...",
      "exit_code": 0, "success": true, "created_at": "..."
    }
  }
}
```

## 后端
- `internal/logfile`:新增
  - `ExecutionsDir() string`
  - `SanitizeNodeID(nodeID string) string`
  - `WriteExecutionLog(opID, nodeID, taskID, command string, exitCode int, output, errMsg string, duration time.Duration) (string, error)`
    - 建批次目录,净化文件名,`<name>.log` + manifest 均**原子写**(temp+rename)
    - 同一 opID 并发多节点写入用 per-key mutex 串行化 manifest 合并
- `handler/exec.go`:`ExecHandler` 增加 `LogWriter *logfile.NodeLogWriter`;`executeTask` 成功/失败终态后调 `WriteExecutionLog`(失败保留部分输出+error);写失败仅记日志,不阻塞任务。`NewExecHandler` 默认 nil,由 server 注入
- `handler/logs.go`(新):
  - `GET /executions/:op_id/logs` → 该批次节点日志列表 `{data:[{node_id,file,size,mod_time}]}`
  - `GET /executions/:op_id/logs/archive` → 批次 zip(含 manifest.json)
  - `GET /executions/:op_id/logs/:node_id` → 单节点 .log 下载(Content-Disposition attachment)
  - opID 校验(uuid 字符集)、nodeID 净化,防路径穿越
- `server.go`:注册 reader 组路由;`Init` 中 `s.execHandler.LogWriter = logfile.NewNodeLogWriter("")`
- `handler/history.go Clean`:DB 清理后遍历 `ExecutionsDir()` 删除 mtime 早于 N 天的目录

## 前端
- `api.js`:`executionLogs(opID)`、`executionLogArchive(opID)`、`executionLogDownload(opID, nodeID)`(fetch-blob,复用 export 模式)
- `exec.js`:所有任务结束后,终端下方渲染"下载本次结果(zip)"+ 各节点 .log 下载链接(opID = tasks[0].record_id);新执行时清空
- `history.js`:详情弹窗加"下载 zip"与每节点下载链接(op.task_id + op.targets)

## 边界
- 异步任务未完成时下载 → 列表为空/404,提示执行未完成
- 被 merge 的任务不重复执行 → 不写日志
- 磁盘写失败不阻塞任务
- 净化后 nodeID 碰撞(极小概率)→ 后者覆盖(可接受)

## 测试
- `logfile`:写入格式/建目录/净化/原子写/并发同批次
- `logs handler`:列表/单下载/zip/防穿越/404
- `exec`:成功与失败均写文件
- `history Clean`:日志目录随清理删除
- E2E:真实 SSH 跑一次,验证文件生成、zip 与单节点下载、历史详情可下载
