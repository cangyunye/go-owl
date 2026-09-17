# owl 能力元数据 Schema（录入时的唯一依据）

来源：`docs/design/05_CAPABILITY_MATCHING_SPEC.md`（§2 / §5 / §8）。为 owl 录入自愈处置能力（脚本/剧本）时必须严格遵循本文件。

## 什么是「能力」

一段可被告警自愈匹配管线调度的处置单元 = **脚本/剧本本体 + 元数据**。
AI 匹配靠元数据决定「这个告警该不该用它」——**元数据质量 = 匹配质量**。

## 脚本元数据字段

```yaml
name: 清理磁盘空间             # 必填；简短名称
description: >-               # 必填；一句话说清「做什么 + 对什么问题有效」（AI 语义匹配主锚点）
  清理 journal 日志与包缓存，释放磁盘空间，适用于磁盘使用率过高告警
language: bash                # 必填；bash|python3|perl|ruby|go|expect（必须与 shebang 一致）
category: disk                # 必填；取值见下方分类表
alert_types:                  # 强烈建议；内置告警类型多绑定（零成本确定性命中）
  - OWL-DSK-001
  - OWL-DSK-002
symptoms:                     # 强烈建议；≤3 条自然语言症状（写法细则见下文）
  - 磁盘使用率超过阈值
  - 根分区剩余空间不足
parameters: []                # 可选；参数 schema，见下文 TemplateParameter
risk: low                     # 必填；low|medium|high（如实标注！分级后果见下表）
rollback: ""                  # 建议；回滚脚本路径或一句话描述
timeout: 300                  # 可选；执行超时秒数，默认 300
requires_root: false          # 可选；是否需要 root
idempotent: true              # 建议；是否可安全重试/重复执行
destructive: false            # 可选；是否含破坏性操作（删数据、格式化、重启节点等）
tags: []                      # 可选；自由标签
```

## 分类 category

对齐 `internal/monitor/alert_type.go` 内置告警分类：

| category | 覆盖问题 | 对应告警类型 |
|----------|----------|--------------|
| `disk` | 磁盘空间/inode | OWL-DSK-001/002 |
| `mem` | 内存/swap | OWL-MEM-001/002 |
| `cpu` | 负载/卡顿 | OWL-CPU-001 |
| `net` | 网卡流量/TCP 连接 | OWL-NET-001/002 |
| `svc` | 服务停止/反复重启 | OWL-SVC-001/002 |
| `err` | 日志错误/OOM | OWL-ERR-001/002 |
| `avail` | 节点失联 | OWL-OSS-001 |
| `generic` | 以上都不对口的自定义能力 | （无，靠 symptoms 语义匹配） |

## 内置告警类型（alert_types 取值）

| ID | 分类 | 名称 |
|----|------|------|
| OWL-DSK-001 | disk | 磁盘使用率过高 |
| OWL-DSK-002 | disk | inode 使用率过高 |
| OWL-MEM-001 | mem | 内存使用率过高 |
| OWL-MEM-002 | mem | swap 使用过高 |
| OWL-CPU-001 | cpu | 负载持续过高（卡顿） |
| OWL-NET-001 | net | 网卡流量异常 |
| OWL-NET-002 | net | TCP 连接堆积 |
| OWL-SVC-001 | svc | 关键服务停止 |
| OWL-SVC-002 | svc | 服务反复重启 |
| OWL-ERR-001 | err | 日志高频错误 |
| OWL-ERR-002 | err | OOM 事件 |
| OWL-OSS-001 | avail | 节点失联 |

> 对不上任何内置类型时**不要编造 OWL-ID**，category 兜底为 `generic`，靠 symptoms 语义匹配。

## 症状 symptoms 写法（决定语义匹配成败）

- 用**告警现场会出现的自然语言短语**（运维看到告警/现象时会想到的词）
- ≤3 条，每条一句话
- 好例子：`磁盘使用率超过阈值`、`根分区剩余空间不足`、`服务反复崩溃重启`
- 坏例子：`disk_full_v2 处理`（内部代号）、`日常巡检脚本`（不是症状）

## 参数 parameters（TemplateParameter）

```yaml
- name: keep_days             # 参数名（脚本内 $1 / getopts / argparse 对应）
  description: 日志保留天数    # 语义描述（AI 据此自动装配取值）
  type: int                   # string|int|float|bool
  required: false             # 慎用 true：必填参数 AI 无法自动装配时会转人工
  default: 3                  # 有默认值必给默认
  options: []                 # 可选；枚举合法值
  pattern: ""                 # 可选；正则校验
```

## 风险分级 risk（如实标注，审批矩阵据此执行）

| 级别 | 定义 | 审批矩阵后果 |
|------|------|--------------|
| `low` | 只读或可逆清理（vacuum、清理缓存） | 类型+对策双放行时可自动执行 |
| `medium` | 重启服务、删除文件、变更配置 | 永远转人工审批 |
| `high` | 破坏性操作（格式化、dd 写盘、重启节点） | 永远禁止自动执行，仅人工 |

## 静态分析信号（生成元数据草稿时自检）

| 信号 | 提取 |
|------|------|
| shebang `#!/usr/bin/env python3` | `language` |
| `getopts` / `getopt` / `argparse` / `$1..$9` / `usage()` | `parameters` 草稿 |
| `rm -rf` / `mkfs` / `dd` 写盘 / `reboot` / `shutdown` / `kill -9` | `risk=high` + `destructive=true` |
| `systemctl restart/stop` | `risk=medium` 起步 + side_effects 说明 |
| 纯只读命令（df/free/status/du/journalctl） | `risk=low` 草稿 |

## 安全红线（脚本本体）

- 幂等、有超时意识、优先非破坏性命令
- **禁止** `rm -rf /`、`mkfs`、`fdisk`、`dd` 写盘等破坏性命令（会被自愈管线黑名单闸门直接拦截）
- 破坏性操作必须前置条件判断（阈值、白名单、确认标记）
- shebang 与元数据 `language` 必须一致（执行器按 language 选解释器）

## 剧本能力的 meta 扩展

剧本（多步编排）在 YAML 头部声明同语义字段：`category` / `alert_types` / `symptoms` /
`risk` / `rollback` / `timeout` / `applicable_os`；`parameters` 段与脚本通用
（详见设计规范 §4）。任务 schema 见 `skills/playbook/schema.md`。

## 落库位置

- **脚本**：Web「脚本库」上传，存 `~/.owl/scripts/<category>/`，元数据入 `scripts` 表
- **剧本**：剧本库目录（`~/.owl/playbooks/<category>/`），meta 写在 YAML 头部
- **AI 草稿必须人工确认后以 `reviewed=true` 入库；未审核的能力不参与自动匹配**
