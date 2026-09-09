# owl CLI 命令行端评审报告

- 日期:2026-09-09
- 评审范围:`cmd/cli/`(owl CLI)及其直接依赖(`internal/control/async`、`internal/ai` 工具层);`owl-serve` 仅作对照
- 方法:全量代码走读(95 个源文件)+ 引用链追踪(grep 交叉验证)+ 与 web 端(`cmd/plugins/serve`)的耦合确认

## 总体评价

CLI 整体工程质量在同类项目中属扎实水平:

- **i18n 体系完整**:en-US/zh-CN 两份目录(internal/i18n/locale/*.json,各 900+ 键),help/flag 文本全量走 `i18n.T`
- **cobra 结构清晰**:11 个顶层命令分组合理,testutil 提供 9 个命令测试助手
- **输出格式化能力**:node/history 支持 table/json/yaml + 自定义列,`--no-color` 统一控制
- **测试规模可观**:58 个 `_test.go` 约 1.2 万行,含 transfer e2e/relay 等真实验证
- **构建裁剪设计**:TUI 走 `tui` build tag,纯净构建不连 bubbletea/lipgloss

主要短板集中在:**存在长期失效却未暴露的功能缺陷(async)**、**错误处理绕过 cobra 机制**、**部分文案与输出流违反自家规范**。

## 发现清单

### P0 — 功能缺陷

#### P0-1 `owl async` 子命令整体不可用(跨进程内存态)

`cmd/cli/cmd/async/async.go` 的 5 个子命令(list/status/wait/cancel/cleanup)每次执行都各自调用 `async.NewAsyncTaskManager(nil)` 新建**纯内存** manager(`internal/control/async/manager.go` 的 `tasks map`,无任何持久化);而 `owl exec --async` 的任务记录同样只存在于启动进程的内存中,进程退出即消失。

结果:在另一个进程执行 `owl async list` 永远为空,`status/wait/cancel` 永远 not found。该子命令自引入起从未真正可用。

**影响面确认(2026-09-09 复核)**:

| 依赖方 | 结论 |
|---|---|
| web 服务端(cmd/plugins/serve、owl-serve) | **不引用** `internal/control/async`,零命中 |
| web exec API 的 `Async bool` 字段(handler/exec.go:86) | 赋值后无消费方,疑似死参数;web 执行本身走 `store.Task` 持久化 + 后台 goroutine + 任务中心 |
| web AI 执行器(handler/aiexecutor.go:743-753) | 显式存根,返回"请在任务中心操作",刻意不支持 |
| **CLI AI 助手**(internal/ai/agent.go:376-378 注册 async_list/status/cancel 工具) | **真实依赖**:CLIExecutor 经子进程调用 `owl async list/status/cancel`(internal/ai/tools.go:569-585) |
| AI `execute_command mode=async`(tools.go:73) | 转化为 `owl exec run --async`,同进程轮询,**不受影响** |

**处置决定**:移除 `owl async` 子命令;CLI AI 三个 async 工具同步降级为优雅文案(与 web 端存根风格一致);`owl exec --async` 与 `internal/control/async` 保留(同进程模式仍有效)。跨进程任务持久化列为后续路线图(见文末)。

#### P0-2 每次运行(含 `--help`/`--version`)都打开 SQLite 且从不 Close

`cmd/cli/cmd/root.go:34-40` 在 `Execute()` 中无条件初始化历史库并迁移/初始化 NodeStore。`owl --help`、`owl --version` 也要付出一次 SQLite 打开 + 迁移检查的代价;失败时降级内存存储(尚可),成功后从不关闭。DB 初始化应下沉到真实执行子命令时(PersistentPreRun),并在退出前 Close。

### P1 — 规范一致性

1. **129 处 `os.Exit`**(非测试代码)散布在各命令 `Run` 中,绕过 cobra 错误机制、跳过 defer,导致错误路径不可测试。应分批迁移到 `RunE` + 返回 error。
2. **错误信息打到 stdout**:`cmd/ai/config.go:87,95,100,115,149` 等用 `fmt.Printf` 输出错误;node/ai/async/file/history 多处直接 `fmt.Print*` 而非 `cmd.ErrOrStderr()`(playbook 系列是正确范本)。
3. **硬编码用户可见文案,违反 CONTEXT.md i18n 规则**(本轮 B3 已修复 history/settings;余下见各项):
   - `cmd/history/history.go`(clean 确认流、错误信息全为硬编码英文)✅ 已修复
   - `cmd/settings/target.go`(target 展示/保存文案)✅ 已修复
   - `cmd/settings/show.go`(show 展示文案,实施中发现)✅ 已修复
   - `owl-serve`(cmd/owl-serve/main.go)帮助文本硬编码中文,且无 version 注入,与 CLI 体系割裂(后续)
4. **测试断言依赖具体中文文案**:`cmd/serve/serve_test.go:16-18` 断言 `Short == "启动 OWL Web 管理控制台"`,依赖 i18n 源语言为中文的巧合;应改为与 `i18n.T("serve.cmd.short")` 比较。
5. **flag 冗余**:`exec run` 的 `--parallel`(默认 true)与 `--serial`(默认 false)为同一概念两个人口;`--group` 废弃别名的"注册 + MarkHidden"模式在 exec/file/playbook/settings 等 5 处重复,应收敛为公共 helper。

### P2 — 关注项(本轮不动)

1. **SSH 凭据安全**:`owl node add -P <password>` 密码出现在命令行参数(进程列表可见)并明文存入 owl.db。至少应:交互式输入密码、文档标注风险;可选 OS keychain。
2. **`owl serve` 与 owl-serve 的隐式协议**:CLI 端 5 个 flag 与二进制靠字符串拼接同步,无版本协商;`findServeBinary` 向上 8 层找 `build/owl-serve` 的逻辑较脆弱。
3. **CI 只有 release workflow**,无测试 workflow;Makefile 的 test 目标未接入 CI。
4. **共享 DB 双写风险**:CLI 与 web 控制台直接共享 `~/.owl/owl.db` 的 nodes 表,SQLite 并发写依赖 busy timeout,长事务场景可能互踩(监控体系上线后 web 写入频率上升,值得观察)。

## 本轮处置(2026-09-09,已完成)

| 批次 | 内容 | 提交 |
|---|---|---|
| B1 | 移除 `owl async` 子命令(含 root 注册、root_test、文档同步、locale async.* 键清理);CLI AI async_list/status/cancel 工具降级为优雅文案;web 端确认不受影响 | `fix(cli): 移除跨进程不可见的 owl async 子命令` |
| B2 | DB 生命周期:初始化移至 PersistentPreRun(`--help`/`--version` 不再开库),Execute 返回前 Close | `fix(cli): 历史库按需初始化` |
| B3 | i18n 违规清理:history clean/run、settings target/show(28 个新 locale 键)并迁移 cmd 输出流;serve_test 断言去中文依赖 | `refactor(cli): 修复 settings/history 违反 i18n 规则的硬编码文案` |
| B4 | ai config init/show/setup 迁移 RunE + SilenceUsage,错误改走 stderr | `refactor(cli): ai config 子命令迁移 RunE` |
| B5 | history clean/run 迁移 RunE 去 os.Exit,新增运行期行为测试 | `refactor(cli): history clean/run 迁移 RunE` |

**结果**:CLI 非测试代码 `os.Exit` 从 129 处降至 117 处;新增 4 个行为测试文件(root 生命周期 ×2、settings i18n ×2、ai config 流 ×2、history clean 运行期 ×2);E2E 冒烟通过(help/version 不建库、async 报 unknown command、clean 校验错误走 stderr 且 exit 1、settings 双语言输出)。

## 后续改进路线图(按优先级)

1. **全量 os.Exit → RunE 迁移**(129 处,建议按命令域分 3-4 批:node → file/exec → ai/settings → 其余),迁移时同步把 `fmt.Print*` 收敛到 `cmd.OutOrStdout()/ErrOrStderr()`
2. **跨进程异步任务**(若确有需求):任务状态持久化到 owl.db(复用 internal/history 的库),`owl async` 以只读视图重建;否则保持移除,`owl exec --async` 文档明确"同进程内轮询"语义
3. **flag 收敛**:`--parallel/--serial` 合一(保留 `--serial`,`--parallel` 标记废弃);`--group` 废弃别名抽公共 helper
4. **owl-serve 版本注入与 i18n**(与 CLI 对齐)
5. **CI 接入测试 workflow**(unit + 短集成)
6. **web exec API 的 Async 死参数**:清理或接线(建议清理)
7. **SSH 密码安全加固**(交互式输入 + 文档风险标注起步)

## 下一评审区域建议

按"先命令行端"的既定节奏,建议下一轮评审:① `cmd/plugins/serve` handler 层(错误码一致性、鉴权中间件覆盖);② `internal/monitor`(P2 新增的 AI 处置/审批流,注意 LLM 超时与闸门链的失败语义);③ 共享 DB 的并发写策略。
