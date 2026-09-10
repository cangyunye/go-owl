# owl serve(Web 端)评审报告

- 日期:2026-09-10
- 评审范围:`cmd/plugins/serve` 全部 Go 代码(server/handler/store/service/model/monitor 接线,非测试约 2.1 万行)+ 入口 `cmd/owl-serve` + `owl serve` 包装器;`web/` 前端 JS 仅作调用方核对,**本轮不做前端全量走读**;`internal/monitor` 引擎本体不单独评审(仅评审其 serve 侧接线),但其进程级缺陷因影响面为本轮范围而提前处置(见 P0-1)
- 方法:全量代码走读(34 个非测试源文件)+ grep 交叉验证(339 处响应调用、goroutine/DB 连接/文件路径/命令执行全量盘点)+ **真实 HTTP E2E**(启动、login、seed、角色矩阵、超时与取消实测、SIGTERM 停机)+ **确定性崩溃复现**

## 总体评价

服务端工程质量在同类项目中属中上水平:

- **分层清晰**:`store`(手写 SQL)/`handler`(HTTP)/`service`(JWT)/`monitor`(引擎接线)职责分明,依赖手工注入但接线集中在 `Server.Init`
- **鉴权实现正确**:`RBACMiddleware` 等级比较正确且 fail-closed(未知角色/空角色一律 403);`/session/terminal` 正确要求 operator 且未知角色拒绝;shortcuts 的属主校验下沉到 SQL `WHERE id=? AND user_id=?`,跨用户改不动
- **SQL 全参数化**:store 层未见字符串拼接构造 SQL,无注入面
- **凭据不出读接口**:`NodeResponse` 有意排除 `password/ssh_key`(node.go:64 有注释说明);`/nodes/export` 同样排除
- **执行侧有闸门**:exec 有黑名单检查 + `danger_confirmed` 二次确认;AI 执行器的全部写工具经 `requireOperator()` 把关(viewer 无法借 `/ai/chat` 提权)
- **共享 DB 的并发配置有据可依**:`sqliteDSN` 把 WAL/busy_timeout 注入 DSN 并写明"仅 Exec PRAGMA 只作用于单连接"的原因
- **测试规模可观**:38 个 `_test.go` 约 1.06 万行,覆盖 RBAC/黑名单/迁移竞态等

主要短板集中在:**监控并发缺陷导致进程级崩溃**(与 AGENTS.md 的 seed 流程直接冲突)、**执行语义与前端承诺不符**(超时/取消)、**管理员账号生命周期无防护**、**响应与文案规范不统一**、**无优雅停机**。

## 发现清单

### P0 — 功能缺陷

#### P0-1 owl-serve 稳定崩溃:监控采集并发写 map(`fatal error: concurrent map writes`)

`internal/monitor/AlertManager` 持有可变 map(`failCounts`、`recoverCounts`)与 `seq`,**全结构体无任何互斥保护**(alert_manager.go:25-34);而 `Engine.TickOnce` 以 `Concurrency=10` 起并发 goroutine 逐节点采集(engine.go:124-140),`collectNode` 并发调用 `MarkCollectFail`(engine.go:150 → alert_manager.go:157-159 `m.failCounts[nodeID]++`)/`MarkCollectOK`。并发 map 写被 Go 运行时判定为不可恢复的 fatal error,**`gin.Recovery()` 无法拦截,整个进程退出**。

serve 侧接线使其必然触发:`serveMonitor.Setup` 将节点表全部节点作为采集目标(monitor/monitor.go:29-48),`EngineConfig.Concurrency=10`(monitor/monitor.go:85),`Engine.Run` **启动时立即执行一轮 TickOnce**(engine.go:76-81),`Start` 里 `go Engine.Run(ctx)`(monitor/monitor.go:214)。

**复现(确定性,本项目实测)**:

```
OWL_DB_PATH=.reviewtmp/repro.db ./owl-serve --port 18082 &
# 登录后 POST /api/v1/nodes/seed  → {"created":50,...}
# 等待下一个采集周期
→ T+111s 进程退出,退出码 2,日志:
  fatal error: concurrent map writes
  internal/monitor.(*AlertManager).MarkCollectFail(...) alert_manager.go:159
  internal/monitor.(*Engine).collectNode(...) engine.go:150
  created by internal/monitor.(*Engine).TickOnce engine.go:124
```

**影响面确认(2026-09-10 复核)**:

| 依赖方 | 结论 |
|---|---|
| owl-serve 进程/Web 控制台 | **直接受害者**:节点数≥2 且进入采集轮次即崩(exit 2),REST/WS/终端全部中断 |
| CLI(`owl`) | 不启动监控引擎,不受影响(仅共享 owl.db) |
| 监控链(告警/自愈/通知) | 崩溃点即在共享状态,采集循环随进程中断 |
| **AGENTS.md 的 seed 流程** | **直接触发**:文档要求 seed 50 个不可达 mock 节点,全部走 MarkCollectFail 路径,下一轮采集即崩 |
| 监控单测(engine_test.go 等) | 有 `TickOnce` 覆盖,但未启用 `-race`、并发度低,长期未暴露 |
| internal/monitor(上轮预告的下一评审区域) | 根因所在;因影响面为进程崩溃,提前到本轮必修 |

**处置决定** ✅ 已修复(B1):为 `AlertManager` 与 `RuleEngine` 补齐互斥保护覆盖全部共享状态(含 `SilentUntil` 改为经 `SetSilentUntil` 访问),并新增 `-race` 并发回归测试(16 节点采集全失败、Concurrency=10 连续 5 轮)。

#### P0-2 前端"连接超时/命令超时"控件不生效(超时参数从未接线)

前端执行页确实提供并提交了这两个参数:`web/js/pages/exec.js:835,842` 两个输入框(默认 10/30 秒),`:581-582` 拼入 `connect_timeout`/`command_timeout`。后端也完整接收(`handler/exec.go:99-100`,映射进 `ExecConfig.CommandTimeout/ConnectTimeout`,exec.go:357-358)。

但**没有任何消费方**:`grep 'cfg\.CommandTimeout\|cfg\.ConnectTimeout'` 在 serve 下零命中;连接超时实际是硬编码常量 `sshConnectTimeout = 10 * time.Second`(ssh_executor.go:19,60-70 使用),命令超时则完全无实现。

**实测(单节点,避免 P0-1 干扰)**:请求 `command_timeout=5s`、`connect_timeout=30s`,任务在 **T+11s** 才进入终态(即硬编码 connect 超时先触发),证明命令超时未生效。

**后果**:用户按界面设置超时后,挂起的远端命令(`sleep`、`tail -f`、交互式程序)永不终止——任务长期停留在 `running`,SSH 会话与 goroutine 一并滞留;前端给出的超时承诺是无效承诺。

**处置决定** ✅ 已修复(B2):`streamExecute` 以 `context.WithTimeout` 施加 `command_timeout`,`ExecuteStream` 在 ctx 结束后主动关闭 session/client 解除 `Wait` 阻塞,超时统一上报"命令执行超时（command_timeout=...）";`connect_timeout` 经 `executorFor` 复制执行器下发。

#### P0-3 任务取消不生效:取消状态被后续执行结果覆盖

前端有明确入口:`web/js/pages/task_detail.js:62-67` 的 "Cancel Task" 按钮 → `DELETE /tasks/:id`。后端 `Cancel`(handler/exec.go:525-542)只把 DB 状态写成 `cancelled` 就返回 `{"status":"cancelled"}`;而执行侧 `executeTask`(exec.go:544-650)**从不检查任务是否已被取消**,结束时以无条件的 `UpdateStatus` 写入 `completed`/`failed`(exec.go:622,642;`store/task.go:129-137` 的 UPDATE 无状态前置条件)。运行中的 SSH 会话也不会被关闭。

**实测**:任务创建后 T+1s 调 `DELETE`,状态先变 `cancelled`;T+11s 被执行结果覆盖为 `failed`,`cancelled` 丢失。对照 `playbook_engine` 的 RunCancel 是有中途状态检查的(playbook.go:620,627),exec 侧缺失。

**后果**:管理员点"取消"后远端命令继续执行到结束,且审计上最终呈现的失败/成功掩盖了人的取消意图。

**处置决定** ✅ 已修复(B2):`ExecHandler` 登记任务取消句柄,`Cancel` 真正中断在途 SSH 会话;新增 `TaskStore.UpdateStatusGuarded`,执行进度与终态写入在任务已取消时跳过,取消状态不再被 `failed/completed` 覆盖。

#### P0-4 最后管理员可被删除/降级,且无应用内恢复(可复现失管)

`UserHandler.Update`(user.go:162-210)与 `Delete`(user.go:212-225)既不校验"目标是否是自己",也不校验"删除/降级后是否还剩管理员";`Delete` 连目标是否存在都不判断——对不存在的 id 也返回 `{"code":200,"message":"deleted"}`(user.go:224,store/user.go:149-152)。而 `ensureAdmin` 仅在**整张用户表为空**时补建 admin(server.go:453-460,`if count > 0 { return nil }`)。

**实测**:admin 把 `PUT /users/1 {"role":"viewer"}` 降级自己后,重新登录得到 viewer 角色,`GET /settings` 返回 403——系统已无任何管理员,重启也不会自愈,只能靠 `--reset-admin` 恢复。

**后果**:一次误操作即可让整个部署失去管理入口(用户/设置/节点删除/监控配置全部不可达)。

**处置决定** ✅ 已修复(B3):降级或删除最后一个 admin 返回 409;`Delete` 补 404(目标不存在不再返回 200)。

### P1 — 规范一致性

1. **响应约定不统一**:全量 339 处 `c.JSON` 中,错误响应基本统一为 `{code,message}`,但成功响应形态各异——`{data:...}`(node/history/playbook)、裸对象(`c.JSON(200, task)`、`SettingResponse`、`DiskInfo`)、`{token,user}`(login)、`{models:...}`(ai)、`{deleted,logs_removed}`(history Clean)、`{status:"deleted"/"cancelled"}`(staging Delete / task Cancel),另有 `user.go:224` 成功也带 `code:200` 而别处成功不带。缺统一的响应封装层。(后续)
2. **内部错误细节回传客户端** ✅ 已修复(监控 handler 部分):`handler/monitor.go` 约 30 处(含 500 类直接回传 `err.Error()`)已收敛为 `internalErr(public, err)`——对外只给短英文描述,细节仅记服务端日志;其余文件仍待处理:`exec.go:253,268,280,301`、`playbook.go:68,140,209,260,265,345`、`transfer.go:103`、`ai.go:176,367,385`、`node_seed.go:202`。
3. **服务端 API 文案语言不统一** ✅ 已修复(监控 handler 部分):`handler/monitor.go` 的校验类与业务类文案已统一为英文;`exec.go:333`、`terminal.go:85-124` 仍为中文,服务端整体仍无 i18n 机制。(后续)
4. **`settings` 接口暴露并允许覆写 `jwt_secret`** ✅ 已修复:新增 `sensitiveSettings` 黑名单,List 过滤、Get/Set 返回 403,并补测试。
5. **无优雅停机与资源释放** ✅ 已修复:`Serve(ctx)` 改用 `http.Server` + `signal.NotifyContext`,停机时停监控、10s drain、依次关闭 monitor/history/serve 三处数据库连接;实测 SIGTERM 退出码由 143 变为 0。
6. **exec 死参数族(上轮路线图第 6 项,本轮确认)**:`async`、`async_max_poll_count`、`async_poll_interval`、`async_remote_dir`、`async_timeout`、`timeout`、`no_color`、`silent` 在 serve 内均无消费方(exec.go:86-101 定义,349-360 部分赋值后无人读取)。前端 `async-toggle` 实际上靠"后端恒为后台执行 + 前端轮询/WS"而"碰巧可用",但参数本身是死代码。(后续;`connect_timeout`/`command_timeout` 已在 B2 接线,不再是死参数)
7. **用户管理其余缺口**:上面 P0-4 之外,`Update` 允许把任意用户改为任意合法角色(admin 改他人角色属预期,未做额外限制);`Delete` 的存在性判断已随 P0-4 补齐 ✅。
8. **token 无撤销,角色变更有 24h 滞后**:`AuthMiddleware` 只校验 JWT 签名与过期(auth.go:84-105),不查库;`Claims.Role` 是签发时快照。**实测**:把自己降级为 viewer 后,旧 token 仍能 `GET /settings`(200)——被降级/被删除的用户在 token 到期(24h)前保留原权限。
9. **`/api/v1/ws` 丢弃身份且关闭来源校验**:`claims` 取到后被 `_ = claims` 丢弃(ws.go:121-126),任意已登录用户(含 viewer)可连;`InsecureSkipVerify: true`(ws.go:129)关闭 Origin 校验;订阅后 `Broadcast` 把全部 `task_output`/`task_update` 下发给所有连接(ws.go:77-111),无任何按用户/按任务的作用域过滤。与"viewer 可读全量任务"的既有设计一致,但缺作用域控制。
10. **AI 会话密钥无回收 + 明文回退**:`KeyManager.Cleanup`(ai_keys.go:98-106)定义后**无调用方**,每个 `GET /ai/session-key` 生成的 2048 位 RSA 私钥常驻内存,map 无界增长;`Decrypt` 保留 `__plain__:` 前缀的明文回退分支(ai_keys.go:68-76),客户端无 WebCrypto 时 API key 以 base64 明文经网络传输。
11. **playbook 模板名未校验即拼路径**:`Create` 用 `filepath.Join(libraryPath, req.Name+".yaml")`(playbook.go:127),`req.Name` 仅校验非空(playbook.go:71-74)——含 `/`、`..` 的名称可写出 library 目录。admin 本可通过 `/playbook/refresh` 改 library 路径,故非提权,但属输入校验缺失;对照 `Upload` 是显式 `filepath.Base` 的(playbook.go:167)。
12. **任务状态写失败被静默丢弃**:`updateTaskStatus` 重试 5 次后直接 `return`(exec.go:652-660),不记日志、不告知——SQLite 锁竞争下任务可能永久停留在 `running` 而无人知晓。
13. **`format=json` 输出非法 JSON**:exec.go:634-636 用 `fmt.Sprintf` 拼 JSON 而不转义,命令或输出含 `"`、换行时返回的不是合法 JSON(前端未用该分支,属潜伏缺陷)。
14. **owl-serve 帮助文本硬编码中文且无 version 注入**(上轮 P1-3 遗留):`cmd/owl-serve/main.go:18-29` 的 `Short/Long` 为中文硬编码,与 CLI 的 i18n 体系割裂;二进制无版本信息。
15. **登录无防爆破**:`Login`(auth.go:28-65)对失败尝试无计数、无延迟、无锁定;虽然错误消息已做用户枚举防护(未知用户与错密码统一 401)。
16. **`/nodes/seed` 与批量写落在 editor 组**:实测 editor 调 `POST /nodes/seed` 返回 200。seed 是测试工具,却允许 editor 在生产注入 50 条 mock 节点并产生审计噪声;`/nodes/import`、`/nodes/batch/groups` 同组同风险。
17. **staging 上传依赖标准库净化,与 playbook 不一致**:staging.go:100 直接用 `header.Filename`(Go 1.17+ 的 `mime/multipart.Part.FileName()` 已做 `filepath.Base`,当前不可穿越),而 playbook Upload 显式 Base。行为依赖 Go 版本,建议显式化以保持防御一致。

### P2 — 关注项(本轮不动)

1. **SSH 凭据明文入库**:`nodes.password`/`ssh_key` 明文存于 `~/.owl/owl.db`(monitor 采集也读同一批凭据),库文件泄露即等于全部节点凭据泄露;读接口已排除凭据,但静态存储未加密(与 CLI 轮 P2-1 同源)。
2. **服务端 SSRF 面**:exec 的 `script_url` 由服务端 `http.Get` 拉取(exec.go:193,operator+);监控通知渠道测试同理由服务端发请求(admin)。operator 本可执行命令,风险有限,但服务端网络位置与操作者不同,值得收敛(如仅允许白名单 scheme/网段)。
3. **query 参数传 token**:`/ws`、`/session/terminal` 以 `?token=` 传 JWT(ws.go:115,terminal.go:45),可能进入反向代理访问日志与浏览器历史;当前服务端无访问日志(见 P2-5),风险主要来自外部反代。
4. **JWT 密钥派生**:`sha256(dbPath+随机盐)` 后存 settings 表(server.go:426-451),拿到库文件即可离线伪造任意用户 token(与 P2-1 同源)。
5. **HTTP 层无请求审计**:`gin.New()` 仅挂 `gin.Recovery()`,无 Logger/审计中间件(server.go:238-239),访问行为不可追溯(操作审计另有 history 表,但 HTTP 层空白)。
6. **共享 owl.db 多连接池并存**:serve、history、monitor 各自 `sql.Open` 同一文件(monitor 已 `SetMaxOpenConns(1)`,serve 侧未限制),CLI 再叠加一个写入方;均有 busy_timeout,但长事务下仍可能互踩(上轮 P2-4 延续)。

## 本轮处置(2026-09-10,已完成)

| 批次 | 内容 | 提交 |
|---|---|---|
| — | 评审报告 | `docs(serve): 新增 Web 端评审报告` |
| B1 | P0-1 监控采集并发写 map 崩溃:AlertManager/RuleEngine 补互斥,`SilentUntil` 经锁访问,新增 `-race` 并发回归测试 | `fix(serve): 修复监控采集并发写 map 导致的进程崩溃` |
| B2 | P0-2/P0-3 执行语义:`command_timeout`/`connect_timeout` 接线生效;取消登记可取消句柄、中断在途 SSH 会话、终态不覆盖取消 | `fix(serve): 命令超时接线并让任务取消真正生效` |
| B3 | P0-4 管理员生命周期:禁止删除/降级最后一个 admin;Delete 补 404 | `fix(serve): 增加最后管理员保护` |
| B4 | P1-4 敏感键:settings 屏蔽 `jwt_secret`(List 过滤、Get/Set 403) | `fix(serve): settings 接口屏蔽 jwt_secret` |
| B5 | P1-5 优雅停机:http.Server + 信号处理,drain 后关闭 monitor/history/serve 三处数据库连接 | `fix(serve): 增加优雅停机并释放数据库连接` |
| B6 | P1-2/P1-3(监控部分) 错误响应统一:`internalErr` 收敛 500 类细节外泄,校验/业务文案改英文 | `refactor(serve): 统一监控接口错误响应` |

**结果**:

- P0 四项全部修复;imports 与 `internal/monitor` 相关的根因(并发写 map)一并消除
- 新增 12 个行为/回归测试:B1 并发竞态 ×1、B2 超时/取消/连接超时 ×3、B3 最后管理员 ×4、B4 敏感键 ×1、B5 优雅停机 ×1、B6 错误响应 ×2;`go test -race ./...` 两个模块全绿
- E2E 实证(每项均为修复前后对照):
  - **崩溃**:按 AGENTS.md 的 seed 50 节点流程,进程在 111s 后 `fatal error: concurrent map writes`、退出码 2 → 修复后跨采集轮次**存活**,`fatal error` 计数 0
  - **超时**:`command_timeout=5s` 修复前 ~10s 才结束(硬编码 connect 超时)、无超时语义 → 修复后 **5s 准时终止**并返回"命令执行超时"
  - **取消**:`DELETE /tasks/:id` 修复前 21s 后被 `failed` 覆盖 → 修复后 21s **仍为 cancelled**,且执行上下文被中断
  - **停机**:SIGTERM 修复前退出码 143、日志无停机记录 → 修复后**退出码 0**、日志记录 draining
  - **敏感键**:`GET/PUT /settings/jwt_secret` 修复前 200 返回密钥原文 → 修复后 **403**,List 不含该键
  - **管理员**:修复前降级/删除唯一 admin 均成功(需 `--reset-admin` 救回) → 修复后 **409**,系统仍可用;`DELETE` 不存在用户由 200 变 **404**
  - 鉴权矩阵:无 token 401、viewer 读 200 / 写 403

## 后续改进路线图(按优先级)

1. **通用错误响应收敛**:把 B6 的 `internalErr` 模式推广到 exec/playbook/transfer/ai/node_seed(约 15 处 `err.Error()` 外泄),并抽公共响应封装统一成功体形态(P1-1/P1-2)
2. **token 撤销机制**:引入 token 版本号/`jti` 或短 TTL + refresh,使降级/删除/改密即时生效(P1-8)
3. **exec 死参数清理**:移除 `async*`/`timeout`/`no_color`/`silent`,并明确前端 `async-toggle` 的语义(或直接移除该开关)(P1-6)
4. **AI 会话密钥生命周期**:接定时 `Cleanup` 或改为按需短周期密钥;评估移除 `__plain__:` 明文回退(P1-10)
5. **批量写接口权限复核**:`/nodes/seed` 收归 admin 或改为仅 dev 模式注册;`/nodes/import`、`/nodes/batch/groups` 同并复核(P1-16)
6. **登录防爆破**:失败计数 + 指数退避或锁定(P1-15)
7. **playbook 模板名白名单校验**(P1-11)
8. **`format=json` 用 `encoding/json` 正确编码**(P1-13)
9. **owl-serve 版本注入与文案对齐 CLI**(P1-14,上轮路线图第 4 项)
10. **`/ws` 作用域与来源校验**:按用户/任务过滤广播,恢复 Origin 校验(P1-9)
11. **staging 上传显式 `filepath.Base`**,与 playbook Upload 保持防御一致(P1-17)
12. **凭据静态加密 / OS keychain**(P2-1,与 CLI 轮合并考虑);**HTTP 访问日志**(P2-5)

## 下一评审区域建议

按既定节奏,建议后续:① `web/` 前端 JS(1.3 万行原生 SPA:token 存储、`marked` 渲染 XSS 面、API client 权限联动、`crypto.js` 明文回退);② `internal/monitor` 引擎本体(告警状态机、自愈闸门与 LLM 超时、通知重试;本次已在 P0-1 处置其并发缺陷,但失败语义与审批链仍未走读);③ 共享 owl.db 的并发写策略与 CLI/Web 双写演练。
