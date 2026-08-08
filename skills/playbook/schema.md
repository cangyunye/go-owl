# owl Playbook YAML Schema（生成时的唯一依据）

来源：`docs/user/PLAYBOOK.md` 与 `internal/control/playbook/parser.go`。生成剧本时必须严格遵循本文件。

## 顶层字段

```yaml
name: <剧本名>            # 必填，小写连字符命名，如 deploy-app
description: <描述>       # 可选
version: "1.0"            # 可选，字符串
hosts: []                 # 可选；目标节点 ID 列表，可为空（空时由命令行参数或全部节点决定）
execution_mode: fail_continue  # 可选；fail_continue(默认) / pipeline
default:                  # 可选；CLI 参数显式指定时完全替换对应默认值（不做并集）
  groups: ["web"]         # 默认目标分组
  tags: ["deploy"]        # 默认执行标签
  skip_tags: ["debug"]    # 默认跳过标签
  timeout:                # 默认超时
    connect: 10s
    command: 5m
  retry:                  # 默认重试
    max: 3
    interval: 1s
    max_interval: 30s
vars:                     # 可选；变量定义，供 {{var}} 插值
  version: "1.0.0"
pre_tasks: []             # 可选；前置任务，失败默认终止
tasks: []                 # 必填（与 pre/post 至少一处非空）；主任务
post_tasks: []            # 可选；收尾任务，失败默认终止；pipeline 模式禁用
```

## 任务字段（tasks/pre_tasks/post_tasks 中每项）

```yaml
- name: <任务名>                 # 必填；「动词+对象」格式，如 备份配置 / 停止服务
  action: <动作类型>              # 必填；见下表
  args: { ... }                  # 按动作类型填写，见下表
  when: <表达式>                  # 可选；条件，支持 and/or/not、==、!=、>、>=、<、<=，变量用 {{var}}
  with_items: [...]              # 可选；循环项列表，循环内用 {{item}} 引用当前项
  loop_control: <变量名>          # 可选；with_items 的循环变量名（默认 item）
  ignore_errors: false           # 可选；失败不计入失败（pipeline 模式禁用）
  any_errors_fatal: false        # 可选；fail_continue 下该任务失败即终止（pipeline 模式禁用）
  tags: [<标签>]                  # 可选；配合 --tags / --skip-tags / default.tags 过滤
  register: <变量名>              # 可选；登记执行结果供后续 when 引用
  timeout:                       # 可选；任务级覆盖
    connect: 10s
    command: 5m
  retry:                         # 可选；任务级重试（max>0 生效）
    max: 3
    interval: 1s
    max_interval: 30s
```

## 动作类型与参数

| 动作 | 说明 | args 参数 |
|------|------|-----------|
| `command` / `cmd` / `shell` | 执行命令 | `cmd` 或 `command`：要执行的命令（多行命令用 `\|` 块） |
| `script` | 执行脚本文件 | `script`：脚本路径（本地文件或 URL）；`dest`：远程存放目录（默认 /tmp）；`args`：脚本参数；`inline`：直接发送内容执行不留文件；`keep`：保留远程脚本 |
| `upload` | 上传文件到节点 | `src`：本地源文件（相对路径相对于剧本目录）；`dest`：远程目标路径（以 `/` 结尾自动拼接原文件名）；`overwrite`：覆盖；`resume`：断点续传 |
| `download` | 从节点下载文件 | `src`：远程源文件；`dest`：本地目标路径；`subdir`：按节点建子目录；`name_format`：命名格式（`{node}`、`{file}` 占位符） |
| `include` | 包含其他剧本 | `playbook`：剧本文件路径（相对路径相对于当前剧本目录） |

## 执行模式

| 模式 | 行为 | 适用场景 |
|------|------|---------|
| `fail_continue`（默认） | 所有任务依次执行，失败不阻断，最后汇总 | 批处理、监控检查 |
| `pipeline` | 任一任务失败立即终止后续 | 部署流程、依赖链任务 |

**pipeline 限制**：不允许 post_tasks；不允许任务上设置 ignore_errors / any_errors_fatal。

## 变量插值

- 语法：`{{变量名}}`，变量来自顶层 `vars:` 块；运行时可用 `--vars k=v` 覆盖。
- 特殊变量：
  - `{{PLAYBOOK_DIR}}` / `${PLAYBOOK_DIR}`：剧本文件所在目录（引用同目录脚本/文件）
  - `{{item}}`：with_items 循环的当前项
- 所有相对路径均相对于剧本文件所在目录解析。

## 节点选择优先级（运行时）

1. `--nodes` 命令行节点 ID → 2. `--groups` 命令行分组 → 3. `--label` 命令行标签 → 4. 剧本 `hosts` → 5. 全部可用节点

## 校验命令

```bash
owl playbook validate <剧本文件>
```

- 多文件：`owl playbook validate a.yaml b.yaml`
- 产物失败输出示例：`invalid execution_mode 'invalid_mode': must be 'pipeline' or 'fail_continue'`
