---
id: "fix-ssh_raspberrypi4-password-auth-failed"
domain: "fix-ssh"
slug: "raspberrypi4-password-auth-failed"
title: "raspberrypi4 节点无法连接（192.168.31.100，kali 账号密码认证）：终端页报 connect failed: SSH 连接失败 .."
status: "resolved"
created: "2026-09-25T19:59:17+08:00"
resolved: "2026-09-25T20:04:13+08:00"
commit: "cc72d95"
branch: "feat/v1.9.0-app-tabs"
platform: "win32"
session: ""
---

# fix-ssh_raspberrypi4-password-auth-failed

## 问题

raspberrypi4 节点无法连接（192.168.31.100，kali 账号密码认证）：终端页报 connect failed: SSH 连接失败 ... ssh: unable to authenticate, attempted methods [none password], no supported methods remain

## 环境

| 项 | 值 |
|----|----|
| git commit | cc72d95 |
| 分支 | feat/v1.9.0-app-tabs |
| 平台 | win32 |
| 建档时间 | 2026-09-25T19:59:17+08:00 |
| 会话 | - |

## 调查过程

- [19:59] 建档
- [20:03] 记录日志 (bash): 根因是节点 user 配成 root（sshd 默认禁止 root 密码登录），密码本身正确；已改为 kali 并验证连通，同时改进认证失败报错
- [20:03] 新增 E2E 用例: 节点 SSH 连通性：raspberrypi4（密码认证）
- [20:04] 结案

## 日志与摘录

### [bash] 2026-09-25T20:03:57+08:00 · 根因是节点 user 配成 root（sshd 默认禁止 root 密码登录），密码本身正确；已改为 kali 并验证连通，同时改进认证失败报错

```
【1. 复现与根因定位】

用户截图报错（Web 终端页）:
  connect failed: SSH 连接失败 on node 192.168.31.100:22: ssh: handshake failed:
  ssh: unable to authenticate, attempted methods [none password], no supported methods remain

DB 里的节点定义（owl node list）:
  rasberrypi4  192.168.31.100:22  root  offline  groups=dev  labels=app=kali,env=dev

隔离实验（build/_sshprobe：直接用 x/crypto/ssh 的 Password 认证）:
  kali + hwx1515661 → 连接成功 ✓  输出="raspberrypi-kali\nkali"
  root + hwx1515661 → 认证失败: ssh: unable to authenticate, attempted methods [none password],
                      no supported methods remain   ← 与截图报错逐字一致

树莓派 sshd 策略（经 kali 登录后读取）:
  KbdInteractiveAuthentication no
  （sshd_config 未显式设置 PermitRootLogin → 走默认 prohibit-password：root 禁止密码登录）
  kali 账号: passwd -S kali → P（已设密码，可登录）

凭据形态探针（build/_nodeprobe，只输出指纹不输出密钥）:
  rasberrypi4  password 前缀=(明文) 长度=10 解密err=nil 指纹=9c2a171a
  期望密码 hwx1515661 指纹=9c2a171a  → 存储的密码与用户提供的密码一致
  ⇒ 密码、加密、解密链路都正常；唯一错的是节点里的 user 字段（root 应为 kali）

【2. 修复】

$ owl node update rasberrypi4 --user kali
  User: kali   Password: [已设置]   Groups: dev   Labels: app=kali, env=dev

【3. 验证（owl 自己的认证链）】

$ owl node check rasberrypi4
  ✓ rasberrypi4 (192.168.31.100:22) - 在线 [密码认证] [状态已更新]
  总结: 1 在线, 0 离线, 共 1

serve 路径复刻（build/_nodeprobe PROBE_DIAL=1，与 sshExecutor.dialNode 同款 DialOptions）:
  rasberrypi4  user=kali
    serve 路径拨号: ✓ 192.168.31.100:22 成功, 远端返回 "kali\nraspberrypi-kali"

【4. 报错信息改进（本次一并修）】

修复前: SSH 连接失败 on node 192.168.31.100:22: ssh: ... no supported methods remain
        （看不出用的哪个账号、也没有排查方向，用户只能来问）
修复后: SSH 连接失败 on node 192.168.31.100:22 (user=root): ssh: ... no supported methods remain
        —— 认证失败：检查该节点的用户名/密码/密钥，确认该用户被 sshd 允许登录（root 通常默认禁止密码登录）
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|
| 1 | 节点 SSH 连通性：raspberrypi4（密码认证） | 1. owl node list 确认节点 user 字段 2. owl node update rasberrypi4 --user kali 修正账号 3. owl node check rasberrypi4 走 owl 自身认证链检查 4. build/_nodeprobe PROBE_DIAL=1 复刻 serve 的 sshExecutor.dialNode 拨号（Web 终端/执行同链路） | 节点在线，密码认证成功，远端返回 kali@raspberrypi-kali | pass |

## 证据截图

## 修复方案

问题不在 owl 的连接代码，而在节点定义：rasberrypi4 的 user 字段是 root，而树莓派 sshd 默认禁止 root 密码登录（PermitRootLogin 默认 prohibit-password，且 KbdInteractiveAuthentication no），所以无论密码对不对都认证失败。

三步修复：
1. 修正节点数据：`owl node update rasberrypi4 --user kali`（该节点标签就是 app=kali，本就该用 kali 账号）。验证：`owl node check rasberrypi4` → ✓ 在线 [密码认证]；并用 build/_nodeprobe 的 PROBE_DIAL=1 复刻 serve 的 sshExecutor.dialNode（同款 DialOptions + DB 读凭据 + secrets.Decrypt），拨号成功返回 "kali@raspberrypi-kali"，即 Web 终端/命令执行同链路也已通。
2. 改进报错可诊断性（commit eaa32a4）：ConnectionError 增加 User 字段并渲染 (user=xxx)；认证类错误追加「认证失败：检查该节点的用户名/密码/密钥，确认该用户被 sshd 允许登录（root 通常默认禁止密码登录）」提示，连接类错误不带该提示。user 从 Dial 一路透传到 connErr/newSSHClient，跳板路径用 firstNonEmpty(jumpUser, opts.User)。
3. 回归用例：internal/ssh/dial_err_test.go 三个用例锁住「认证错误带账号与提示 / 连接错误不带提示 / 无用户名不渲染 user=」。

排查手段沉淀（都在 build/ 下，已 gitignore，不进版本库）：_sshprobe（x/crypto/ssh 字面凭据直连，用于区分凭据问题与 owl 侧问题）、_nodeprobe（读 DB 打印凭据「形态+指纹」而非密钥本身，可复刻 serve 拨号）、_dialprobe（复现 owl 拨号并打印其报错）。

## 复盘

根因：节点 user 配错（root ≠ 可密码登录的账号）。密码、加密存储、解密、认证链、拨号代码全部正常 —— 报错里的 "attempted methods [none password]" 恰好说明「密码认证被尝试过并被服务端拒绝」，指向凭据/账号而非 owl 代码。

教训：
1. 「连不上」先分清三层：TCP 可达 → 认证 → 授权。`attempted methods [none password]` 属于认证层被拒；`connection refused` 才是连接层。owl 之前把两者都渲染成「SSH 连接失败 on node <addr>」，信息量太低。
2. 报错必须回答「用了什么身份、下一步查什么」。修复后附带 user 与「检查用户名/密码/密钥、该用户是否允许登录」的提示，同类问题不必再靠外部探针。
3. 排查凭据类问题不要打印密钥：用「前缀 + 长度 + sha256 前 4 字节指纹」比对，既能确认是不是同一个值，又不把密码写进日志。
4. root 登录是常见陷阱：多数发行版 sshd 默认 prohibit-password，节点里配 root+密码必然失败。产品侧值得在创建节点时给出该提示（后续可做）。
