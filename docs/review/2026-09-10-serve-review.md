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

1. **响应约定不统一** ✅ 已修复(动作类,收窄版):C1 收敛错误侧后,顺位 3(评审确认方案 B)确立成功体约定并落 helper——对象 `{data}`(ok)、动作 `{"ok":true,"message"}`(okAction),13 处动作响应归一,`{code:200,...}` 离群清零;history Clean 迁移为标准包裹(前端同批适配)。裸对象/`{items,total}` 与 `{data,meta}` 并存等**既有 GET 形态有意保留**(前端按字段直读,全量改形波及 90+ 前端消费点,收益仅一致性;列为方案 A 前置,暂不做)。
2. **内部错误细节回传客户端** ✅ 已修复:`internalErr(public, err)` 从 monitor 推广到 ai/exec/node_seed/playbook(B7);C1 进一步错误类型化(`businessError`/`respondErr`),400 类也只透传业务消息,原始 error 不再出 handler。仍透传的两处为有意保留:playbook Refresh(管理员刚配置的 library 路径自查信息)与 ai.go 502(provider 诊断)。
3. **服务端 API 文案语言不统一** ✅ 已修复:monitor(B6)、其余 handler(B7)、黑名单提示与终端错误前缀(C2)均已统一英文;`internal/control/blacklist` 的中文文案属 CLI 共享层不在其列。服务端是否接入 i18n 机制列为可选项(见路线图)。
4. **`settings` 接口暴露并允许覆写 `jwt_secret`** ✅ 已修复:新增 `sensitiveSettings` 黑名单,List 过滤、Get/Set 返回 403,并补测试。
5. **无优雅停机与资源释放** ✅ 已修复:`Serve(ctx)` 改用 `http.Server` + `signal.NotifyContext`,停机时停监控、10s drain、依次关闭 monitor/history/serve 三处数据库连接;实测 SIGTERM 退出码由 143 变为 0。
6. **exec 死参数族(上轮路线图第 6 项,本轮确认)** ✅ 已修复(B8):`async*`/`timeout`/`no_color`/`silent` 已从 `execRequest`/`ExecConfig` 移除,前端"异步执行"开关同步删除(执行本就是后台任务 + WS 实时输出);`connect_timeout`/`command_timeout` 已在 B2 接线保留。
7. **用户管理其余缺口** ✅ 已修复:`Delete` 的存在性判断随 P0-4 补齐;改角色/改密码/删除账号后的 token 失效随 B9 解决(见 P1-8)。`Update` 允许 admin 改任意用户角色属预期,未额外限制。
8. **token 无撤销,角色变更有 24h 滞后** ✅ 已修复(B9):新增 `authRevocations`(撤销时刻持久化在 settings),`AuthMiddleware` 比较 `IssuedAt` 即可判定,无需每请求查库;改角色/改密码/删除账号与 `--reset-admin` 都会撤销旧 token。判定从紧——`IssuedAt` 为秒级精度,采用"签发时刻不晚于撤销时刻即失效",不留同秒绕过窗口(代价:撤销当秒内重登可能拿到立即失效的 token,重试即可)。
9. **`/api/v1/ws` 丢弃身份且关闭来源校验** ✅ 已修复(B10 同源校验/未知角色 fail-closed;顺位 1 改一次性票据)。作用域过滤经评估(2026-09-11)**接受现状**:广播的 task_output/task_update 等数据 viewer 经 REST(GET /tasks/:id 含 command+output、history detail、日志下载)本可全部读到,WS 只是主动推送,无 REST 之外的保密边界;按角色收窄零保密增益且破坏 viewer 实时视图,订阅制各页需求拼起来仍等价全量。唯一推翻条件:产品将 viewer 改为"受限自助角色"时 REST+WS 一起重做授权模型。
10. **AI 会话密钥无回收 + 明文回退** ✅ 部分修复(B10):`Server.Serve` 增加 10 分钟周期回收(`Cleanup(1h)`),随 ctx 停止,避免 map 无界增长。`__plain__:` 明文回退**有意保留**——浏览器仅在安全上下文暴露 `crypto.subtle`,局域网内用 `http://<IP>` 访问时该分支是唯一可用路径;移除会让这类部署无法配置 API key。根治需 HTTPS(见路线图)。
11. **playbook 模板名未校验即拼路径** ✅ 已修复(B12):`req.Name` 改为 `^[A-Za-z0-9._-]+$` 白名单并拒绝 `.`/`..`,非法名 400;实测 `../../evil` 被拒、产物仅落在配置的 library 目录内。
12. **任务状态写失败被静默丢弃**:`updateTaskStatus` 重试 5 次后直接 `return`(exec.go:652-660),不记日志、不告知——SQLite 锁竞争下任务可能永久停留在 `running` 而无人知晓。
13. **`format=json` 输出非法 JSON** ✅ 已修复(B12):改用 `encoding/json` 编码,输出含引号/换行时仍为合法 JSON(新增测试断言可被 `json.Unmarshal`)。
14. **owl-serve 帮助文本硬编码中文且无 version 注入**(上轮 P1-3 遗留) ✅ 已修复(C5):Short/Long/flag 描述改用 internal/i18n 的 `serve.*` 键与 CLI 共目录;新增 version/commitID/buildTime 注入点,`--version` 与启动横幅均携带;Makefile 的 `SERVE_LDFLAGS` 接入 build-serve 与 build 的 serve 分支。
15. **登录无防爆破** ✅ 已修复(B13):按"用户名+来源 IP"计数,连续失败 5 次后按 2^n 退避(2s 起、上限 5min),成功登录清零,限流时返回 429 + `Retry-After`;同时 `SetTrustedProxies(nil)` 关闭 gin"信任所有代理"的默认,避免伪造 `X-Forwarded-For` 轮换来源绕过按 IP 限流。
16. **`/nodes/seed` 与批量写落在 editor 组** ✅ 已修复(B11):`/nodes/seed` 收归 admin(实测 editor 403、admin 200)。`/nodes/import`、`/nodes/batch/groups` 经复核**保留 editor**——它们与逐条新建/编辑等价,属编辑角色的预期能力,仅批量形式不同。
17. **staging 上传依赖标准库净化,与 playbook 不一致** ✅ 已修复(B12):改为显式 `filepath.Base`,不再依赖标准库行为;新增"带路径文件名仍落中转站目录"回归测试(注:标准库本就净化,此改动是防御一致性而非修复已存在的穿越)。

### P2 — 关注项(本轮不动)

1. **SSH 凭据明文入库**:`nodes.password`/`ssh_key` 明文存于 `~/.owl/owl.db`(monitor 采集也读同一批凭据),库文件泄露即等于全部节点凭据泄露;读接口已排除凭据,但静态存储未加密(与 CLI 轮 P2-1 同源)。
2. **服务端 SSRF 面**(2026-09-11 评估后**接受现状**):exec 的 `script_url` 由服务端 `http.Get` 拉取(exec.go,operator+);监控通知渠道测试同理由服务端发请求(admin)。从内网制品服务器拉脚本属核心场景,拒绝私网/环回地址对该场景是反向优化,operator 本就持有节点 shell,服务端出网不构成额外授权边界。
3. **query 参数传 token**:`/ws`、`/session/terminal` 以 `?token=` 传 JWT(ws.go:115,terminal.go:45),可能进入反向代理访问日志与浏览器历史;当前服务端无访问日志(见 P2-5),风险主要来自外部反代。
4. **JWT 密钥派生**:`sha256(dbPath+随机盐)` 后存 settings 表(server.go:426-451),拿到库文件即可离线伪造任意用户 token(与 P2-1 同源)。
5. **HTTP 层无请求审计** ✅ 已修复(C3):`accessLog` 中间件记录 方法/URI/状态码/耗时/来源 IP;`/ws`、`/session/terminal` 的 `?token=` 落日志前替换为 `***`。
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
| B7 | P1-2 错误响应收敛:共享 `internalErr`,覆盖 ai/exec/node_seed/playbook 的 8 处拼接 `err.Error()` | `refactor(serve): 收敛其余 handler 的内部错误外泄` |
| B8 | P1-6 exec 死参数:移除 `async*`/`timeout`/`no_color`/`silent` 与前端"异步执行"开关 | `refactor(serve): 清理 exec 无消费方的参数并把异步语义说清` |
| B9 | P1-8 token 撤销:改角色/改密/删除账号即时失效,撤销记录持久化,重启仍生效 | `fix(serve): 改角色/改密/删除账号后旧 token 立即失效` |
| B10 | P1-9/P1-10 恢复 `/ws` 与终端同源校验、未知角色 403;AI 会话密钥定期回收 | `fix(serve): 恢复 WebSocket 同源校验并回收 AI 会话密钥` |
| B11 | P1-16 `/nodes/seed` 收归 admin | `fix(serve): /nodes/seed 收归 admin` |
| B12 | P1-11/P1-13/P1-17 playbook 名白名单、`format=json` 正规编码、staging 显式 basename | `fix(serve): playbook 模板名校验、staging 显式取 basename、format=json 正规编码` |
| B13 | P1-15 登录失败限流(用户名+IP,2^n 退避)+ 关闭信任所有代理头 | `fix(serve): 登录失败限流` |
| C1 | P1-2 余项 错误类型化:`businessError`/`respondErr`,400 类只透传业务消息 | `refactor(serve): 错误类型化,400 类响应不再透传原始 error` |
| C2 | P1-3 余项 服务端自有文案统一英文(黑名单提示、终端错误前缀) | `refactor(serve): 服务端自有文案统一英文` |
| C3 | P2-5 HTTP 访问日志,query 中 token 脱敏 | `fix(serve): 增加 HTTP 访问日志(query 中 token 脱敏)` |
| C5 | P1-14 owl-serve 版本注入 + 帮助文案接入 i18n(上轮遗留) | `fix(serve): owl-serve 版本注入,帮助文案对齐 CLI` |

**结果**:

- P0 四项全部修复;其中 P0-1 的根因在 `internal/monitor`(并发写 map),已一并消除
- 新增 12 个行为/回归测试:B1 并发竞态 ×1、B2 超时/取消/连接超时 ×3、B3 最后管理员 ×4、B4 敏感键 ×1、B5 优雅停机 ×1、B6 错误响应 ×2;`go test -race ./...` 两个模块全绿
- E2E 实证(每项均为修复前后对照):
  - **崩溃**:按 AGENTS.md 的 seed 50 节点流程,进程在 111s 后 `fatal error: concurrent map writes`、退出码 2 → 修复后跨采集轮次**存活**,`fatal error` 计数 0
  - **超时**:`command_timeout=5s` 修复前 ~10s 才结束(硬编码 connect 超时)、无超时语义 → 修复后 **5s 准时终止**并返回"命令执行超时"
  - **取消**:`DELETE /tasks/:id` 修复前 21s 后被 `failed` 覆盖 → 修复后 21s **仍为 cancelled**,且执行上下文被中断
  - **停机**:SIGTERM 修复前退出码 143、日志无停机记录 → 修复后**退出码 0**、日志记录 draining
  - **敏感键**:`GET/PUT /settings/jwt_secret` 修复前 200 返回密钥原文 → 修复后 **403**,List 不含该键
  - **管理员**:修复前降级/删除唯一 admin 均成功(需 `--reset-admin` 救回) → 修复后 **409**,系统仍可用;`DELETE` 不存在用户由 200 变 **404**
  - 鉴权矩阵:无 token 401、viewer 读 200 / 写 403

追加批次(B7–B13)结果:

- 新增 25 个测试:错误收敛 ×1、死参数兼容 ×1、token 撤销 ×8、WS/终端来源与角色 ×5、密钥回收 ×1、seed 权限 ×2、playbook/JSON/staging ×3、登录限流 ×4
- 覆盖的实证:降级即时生效(editor 降级后旧 token 401,重启后仍 401,重登按新角色 403/200)、跨源握手 101→403、`/nodes/seed` editor 403、非法 playbook 名 400 且无越界产物、5 次错密码后 429 + `Retry-After`
- 两个模块 `go test -race ./...` 全绿

第三批(C1–C5)结果:

- 新增 9 个测试:C1 错误类型化 ×2(业务消息、关库不泄露)、C3 访问日志 ×3、C5 版本/文案/flag 对齐 ×3;另更新黑名单与节点不存在两处断言
- 覆盖的实证:400 类内部错误不再含 DB 细节;日志中 token 显示为 `token=%2A%2A%2A`;`owl-serve --version` 输出注入的版本/提交/构建时间,横幅带版本
- 有意保留:`__plain__:` 明文回退(局域网 HTTP 场景唯一路径)、ai.go 502 与 playbook Refresh 的透传(管理员自查诊断)、`/nodes/import|batch/groups` 保持 editor

## 后续改进路线图(按优先级)

已完成/已处置:B1–B13、C1–C3、C5,以及:一次性票据(顺位 1)、共享 owl.db 并发演练与 CLI DSN 修复(顺位 2)、
动作类响应收敛(顺位 3)、SSRF 接受现状(顺位 4)、/ws 作用域接受现状(顺位 5)。

剩余:

1. **凭据静态加密**(P2-1,设计中):`nodes.password`/`nodes.ssh_key` 经 AES-256-GCM 加密,
   `OWL_ENC_KEY` 环境变量开启,`enc:v1:` 前缀兼容存量明文,CLI/serve 同钥互读;详见设计记录
2. **`/ws` 作用域过滤**:评估后接受现状,仅当 viewer 角色语义变更时随 REST 授权模型一起重做(P1-9 余项)
3. **SSRF 收敛**:评估后接受现状(P2-2)
4. **AI API key 传输与 `__plain__:` 回退**:保留;根治需 HTTPS/反代(P1-10 余项)
5. **服务端 i18n 机制(可选)**:文案已统一英文,多语言需求出现再接 internal/i18n(P1-3 余项)
6. **notify 渠道配置的密码字段加密**(凭据加密二期)
7. **HTTP 访问日志保留策略与脱敏扩展**(如 ticket 也脱敏)(P2-5 后续)

## 下一评审区域建议

按既定节奏,建议后续:① `web/` 前端 JS(1.3 万行原生 SPA:token 存储、`marked` 渲染 XSS 面、API client 权限联动、`crypto.js` 明文回退);② `internal/monitor` 引擎本体(告警状态机、自愈闸门与 LLM 超时、通知重试;本次已在 P0-1 处置其并发缺陷,但失败语义与审批链仍未走读);③ 共享 owl.db 的并发写策略与 CLI/Web 双写演练。
