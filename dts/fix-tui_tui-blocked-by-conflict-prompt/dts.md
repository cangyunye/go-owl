---
id: "fix-tui_tui-blocked-by-conflict-prompt"
domain: "fix-tui"
slug: "tui-blocked-by-conflict-prompt"
title: "用户在 ~/.owl 有 nodes.json↔db 重复节点时,owl tui 会被启动前的节点冲突交互提示阻塞,吞掉按键、TUI 起不来。需要让 owl t"
status: "resolved"
created: "2026-08-14T18:27:42+08:00"
resolved: "2026-08-14T21:33:31+08:00"
commit: "ec0a588"
branch: "owl-tui"
platform: "darwin"
session: "ses_00424fb02ffevMFHb6Q4Jl47bt"
---

# fix-tui_tui-blocked-by-conflict-prompt

## 问题

用户在 ~/.owl 有 nodes.json↔db 重复节点时,owl tui 会被启动前的节点冲突交互提示阻塞,吞掉按键、TUI 起不来。需要让 owl tui 绕开该交互提示。

## 环境

| 项 | 值 |
|----|----|
| git commit | ec0a588 |
| 分支 | owl-tui |
| 平台 | darwin |
| 建档时间 | 2026-08-14T18:27:42+08:00 |
| 会话 | ses_00424fb02ffevMFHb6Q4Jl47bt |

## 调查过程

- [18:27] 建档
- [21:16] 记录日志 (bash): 复现确认:根因是 NodeStoreDB.List()->ensureConsistent()->resolveNodeConflicts() 在 stdin 为 TTY 时交互式阻塞,非 TUI 专属(owl node list 在 pty 下同样挂死)
- [21:28] 记录日志 (chat): 方案A确认:仅 owl tui 绕过冲突提示,用户拍板,无实质风险
- [21:33] 记录证据 1 项
- [21:33] 记录终端文本快照
- [21:33] 结案

## 日志与摘录

### [bash] 2026-08-14T21:16:10+08:00 · 复现确认:根因是 NodeStoreDB.List()->ensureConsistent()->resolveNodeConflicts() 在 stdin 为 TTY 时交互式阻塞,非 TUI 专属(owl node list 在 pty 下同样挂死)

```
复现证据:
1. `owl node list` 在 pty(script)下挂死,0 输出;非 pty(管道)下秒回 57 节点。
2. `sample` 抓栈:主线程 + 多线程卡 cgo pthread_cond_wait,一线程 read。
3. lsof: fd0/1/2=pty, fd3=~/.owl/owl.db。
4. 喂足输入 `printf '3\n3\n3\n3\n3\n'` 后放行,rc=0,输出冲突横幅:
   - 数据库节点:57 个, nodes.json:1 个, 冲突节点数:4 个
   - 冲突例: [duplicate_name_db] Same name 'db-01' found in db for IDs: node-2, n2
5. 根因: cmd/cli/cmd/common/node_store_db.go List() -> ensureConsistent() (sync.Once) -> resolveNodeConflicts(); term.IsTerminal(stdin)==true 时 fmt.Scanln 交互阻塞。
6. 影响: owl tui 启动即被 4 个冲突逐个阻塞;owl node list / 任何 List() 调用在 TTY 下同样阻塞。
7. owl version(仅 DB 初始化不 List())在 pty 下正常 -> 挂死专属于 List() 路径。
```

### [chat] 2026-08-14T21:28:38+08:00 · 方案A确认:仅 owl tui 绕过冲突提示,用户拍板,无实质风险

```
根因:NodeStoreDB.List()->ensureConsistent()(sync.Once)->resolveNodeConflicts() 在 term.IsTerminal(stdin)==true 时 fmt.Scanln 交互阻塞。复现:owl tui / owl node list 在 pty(script)下均挂死,喂足 4×'3\n' 后放行;冲突为 DB 内 57 节点含种子/测试节点重名(db-01=node-2&n2, web-01=node-1&n1) + nodes.json 的 mac 与 DB mac status 不一致(online vs offline)。

修复方向(用户拍板=方案A):仅 owl tui 绕过交互提示,其余命令行为不变。NodeStoreDB 加 SetConflictPrompt(enabled bool),默认 true;ensureConsistent 在 disabled 时走非交互分支(DetectConflicts+logger.Warn,不弹窗);runTui 在创建 TUI model 前对 common.GetNodeStore() 类型断言为 *NodeStoreDB 后调用 SetConflictPrompt(false)。

风险评估(已与用户确认):数据一致性无风险(DB 主源,JSON 仅供导出);sync.Once 每进程一次,无重复触发;其他命令无回归;TUI 显示 DB 主源数据,分歧仅日志告警;flag 进程内生效无泄漏。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: 修复后 E2E:真实冲突 HOME 下 owl tui 启动渲染并干净退出(此前挂死);node list 在 pty 下仍保留冲突提示](shots/001-213321.txt)

## 修复方案

方案A(用户拍板):NodeStoreDB 新增 SetConflictPrompt(enabled bool)(默认 true 保持原行为),ensureConsistent 在关闭时走非交互分支(DetectConflicts + logger.Warn,不调用 resolveNodeConflicts 的 fmt.Scanln);owl tui 的 runTui 在创建 TUI 前对 common.GetNodeStore() 类型断言为 *NodeStoreDB 后 SetConflictPrompt(false)。新增 2 个单测(SetConflictPrompt 开关、冲突下 List 不阻塞)+ E2E 脚本追加"冲突数据下 owl tui 不被阻塞"场景。验证:真实冲突 HOME 下 owl tui 启动渲染 57 节点并干净退出(此前挂死);owl node list 在 pty 下仍保留冲突提示(其他命令零回归);全量测试无 FAIL。

## 复盘

根因:交互式冲突解决被绑定在 NodeStoreDB.List() 的 ensureConsistent()(sync.Once)上,任何读节点路径在 term.IsTerminal(stdin)==true 时都会被 fmt.Scanln 逐个阻塞,不限于 owl tui。排查关键:用 script(pty)复现、sample 抓栈(主线程卡 cgo cond_wait + read)、lsof 看 fd0/1/2=pty;初期"0 字节转录"是误判信号——实际是 4 个冲突节点只喂了 1 次输入,在第 2 个阻塞。教训:① 交互式 I/O 绝不能放在读路径(读操作不该有副作用/阻塞);② 用户本意是"双源(json 导出交换 + db 主源)+ 程序内对账",方案 A 仅让 TUI 绕过、保留 CLI 命令的显式门禁,符合其设计;③ 复现 pty 挂死时若输出为空,先喂足输入看是否放行,别急着下"无输出=没走到提示"的结论。
