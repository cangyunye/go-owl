# 命令黑名单功能 Spec

## Why

go-owl 作为运维工具可在远程节点上以任意用户（含 root）执行命令，目前缺乏对危险命令的预防性提醒机制。需要增加命令黑名单，在命令执行前检查危险模式，提醒用户注意风险，防止误操作导致生产事故。

## What Changes

* 新增 `~/.owl/blacklist.yaml` 配置文件，定义按用户分组的危险命令匹配模式

* 新增 `internal/control/blacklist` 包，负责加载配置、匹配命令、生成警告信息

* `owl exec run` 和 `owl exec script` 命令增加 `--force` 参数，跳过黑名单检查

* 在命令执行前进行黑名单检查，命中时输出警告并列出匹配的关键行，等待用户确认

* 提供合理的默认黑名单规则（默认内置，无需用户手动创建配置文件）

## Impact

* Affected specs: 无（新功能）

* Affected code:

  * `cmd/cli/cmd/exec/run.go` — 增加 `--force` flag 和黑名单检查调用

  * `cmd/cli/cmd/exec/script.go` — 增加 `--force` flag 和黑名单检查调用

  * `internal/control/blacklist/` — 新增包

## ADDED Requirements

### Requirement: 黑名单配置加载

系统 SHALL 从 `~/.owl/blacklist.yaml` 加载黑名单配置，若文件不存在则使用内置默认规则。

#### Scenario: 配置文件存在

* **WHEN** 配置文件 `~/.owl/blacklist.yaml` 存在且格式正确

* **THEN** 使用该文件中的规则

#### Scenario: 配置文件不存在

* **WHEN** 配置文件 `~/.owl/blacklist.yaml` 不存在

* **THEN** 使用内置默认规则（root 用户禁止 rm/mv/su/ssh/dd/mkfs/shutdown/reboot 等）

#### Scenario: 配置文件格式错误

* **WHEN** 配置文件存在但 YAML 格式错误

* **THEN** 输出警告信息，回退使用内置默认规则

### Requirement: 按用户分组的规则匹配

系统 SHALL 在命令执行前检查命令是否匹配当前用户对应的黑名单规则。`user: "*"` 规则对所有用户生效。

#### Scenario: root 用户执行 rm 命令

* **WHEN** 目标节点用户为 root，命令包含 `rm`

* **THEN** 命中 root 用户黑名单中的 `rm` 规则

#### Scenario: 普通用户执行 rm 命令

* **WHEN** 目标节点用户为 webuser（非 root），默认配置下执行 `rm`

* **THEN** 不命中黑名单（root 专属规则不适用）

#### Scenario: 任意用户执行 rm -rf / 命令

* **WHEN** 任意用户执行 `rm -rf /`

* **THEN** 命中 `*` 全局黑名单规则

### Requirement: 危险命令警告输出

系统 SHALL 在黑名单命中时，输出警告信息，包含：危险命令的关键行、匹配到的规则、涉及的节点用户信息。

#### Scenario: 单个命令命中多条规则

* **WHEN** 执行 `rm -rf /etc/config && shutdown -h now`

* **THEN** 输出警告，列出 `rm` 和 `shutdown` 两个匹配行

#### Scenario: 多节点执行命中检查

* **WHEN** 对多个节点执行命令，其中部分节点用户为 root

* **THEN** 列出每个 root 节点及匹配的规则行

### Requirement: --force 强制执行

系统 SHALL 提供 `--force` 参数，使用户可以跳过黑名单检查直接执行命令。

#### Scenario: 使用 --force 执行危险命令

* **WHEN** 用户执行 `owl exec run "rm -rf /tmp/test" --nodes server01 --force`

* **THEN** 跳过黑名单检查，直接执行命令

#### Scenario: 不使用 --force 执行危险命令

* **WHEN** 用户执行 `owl exec run "rm -rf /tmp/test" --nodes server01`

* **THEN** 输出警告，等待用户确认是否继续（y/N）

### Requirement: 内置默认危险命令列表

系统 SHALL 提供以下内置默认黑名单规则：

| 用户   | 危险命令模式                | 风险说明       |
| ---- | --------------------- | ---------- |
| root | `rm `                 | 删除文件/目录    |
| root | `mv `                 | 移动/重命名系统文件 |
| root | `su`                  | 切换用户       |
| root | `sudo `               | 提权执行       |
| root | `ssh `                | 远程连接跳转     |
| root | `scp `                | 远程文件传输     |
| root | `dd `                 | 磁盘写入操作     |
| root | `mkfs`                | 格式化文件系统    |
| root | `fdisk `              | 磁盘分区操作     |
| root | `shutdown`            | 关机         |
| root | `reboot`              | 重启         |
| root | `halt`                | 停机         |
| root | `poweroff`            | 断电         |
| root | `init `               | 运行级别切换     |
| root | `chmod `              | 修改文件权限     |
| root | `chown `              | 修改文件所有者    |
| root | `chattr `             | 修改文件属性     |
| root | `iptables `           | 修改防火墙规则    |
| root | `ufw `                | 修改防火墙规则    |
| root | `firewall-cmd `       | 修改防火墙规则    |
| root | `systemctl stop `     | 停止系统服务     |
| root | `systemctl disable `  | 禁用系统服务     |
| root | `systemctl mask `     | 屏蔽系统服务     |
| root | `killall `            | 批量杀进程      |
| root | `pkill `              | 批量杀进程      |
| root | `parted `             | 磁盘分区操作     |
| root | `mkswap`              | 创建交换分区     |
| root | `mount `              | 挂载文件系统     |
| root | `umount `             | 卸载文件系统     |
| \*   | `rm -rf /`            | 递归删除根目录    |
| \*   | `rm -rf /*`           | 递归删除根目录    |
| \*   | `dd if=/dev/`         | 磁盘直接写入     |
| \*   | `mkfs.`               | 格式化文件系统    |
| \*   | `:(){ :\|:& };:`      | fork 炸弹    |
| \*   | `>/dev/sd`            | 覆盖磁盘设备     |
| \*   | `chmod 777 /`         | 开放根目录权限    |

## Configuration File Format

`~/.owl/blacklist.yaml` 格式：

```yaml
rules:
  - user: root
    patterns:
      - "rm "
      - "mv "
      - "su"
      - "ssh "
      # ... more patterns
  - user: "*"
    patterns:
      - "rm -rf /"
      - ":(){ :|:& };:"
      # ... more patterns
```

