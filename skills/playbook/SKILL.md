---
name: playbook
description: 根据用户描述的使用步骤生成 owl 运维 playbook YAML 剧本。用户只需提供操作步骤，AI 自动拆解为符合本项目 playbook 模板的任务序列，推荐命令与参数（不必精确，用户可自行修改），描述不清的命令、路径、端口等一律用 {{变量}} 占位并集中收进 vars 块。触发词：生成剧本、创建 playbook、部署流程、自动化运维步骤、编排任务。
---

# playbook 生成技能

根据用户描述的使用步骤，生成符合 owl playbook 模板的 YAML 剧本，并写入 `playbooks/` 目录，最后通过 `owl playbook validate` 校验。

生成时必须先阅读本项目 `.opencode/skills/playbook/schema.md`（playbook YAML 唯一 schema 依据），严格按其字段与动作参数表生成。

## 工作流

### 1. 澄清（仅当信息缺失，一次问完）

按需向用户确认：

- **目标节点/分组**：执行在哪些节点上？（决定 hosts 与 default.groups）
- **执行模式**：部署依赖链 → pipeline；巡检/批量 → fail_continue
- **是否涉及文件**：有脚本/配置要传 → upload/script；要拉取日志 → download

用户未说明且不影响生成的，不要追问，直接按常识给出默认值或占位。

### 2. 拆解步骤

把用户的使用步骤逐条映射为任务：

- 每个步骤对应一个任务（动作类型分类见 schema.md 动作表）
- 任务命名：「动词+对象」，如 `备份配置`、`停止服务`、`上传应用包`、`健康检查`
- 阶段划分：
  - 准备类（备份、安装依赖、上传文件/脚本）→ `pre_tasks` 或 `tasks`
  - 核心操作（部署、启动、迁移）→ `tasks`
  - 收尾类（健康检查、清理、日志收集）→ `post_tasks`（pipeline 模式禁用）
- 有依赖关系的步骤放 `tasks` 并用 `execution_mode: pipeline`；无依赖的巡检类用默认 fail_continue
- 多个同构步骤（如逐个检查 3 个服务）用 `with_items` + `{{item}}` 合并为一个任务

### 3. 推荐命令与参数（重要）

- 命令、路径、端口等按常识给出**合理可执行的默认值**（如 `systemctl restart nginx`、`/opt/app/`），允许用户事后修改
- **用户描述不清楚的部分**（具体命令、版本号、路径、端口、文件名、分组名、脚本名）一律用 `{{变量名}}` 占位，变量名取语义化英文，并在顶层 `vars:` 块集中列出，方便用户一次填完
- 引用剧本同目录文件用 `{{PLAYBOOK_DIR}}/xxx`（如 `{{PLAYBOOK_DIR}}/scripts/deploy.sh`）
- 不用猜测不存在的具体文件名当真实路径；占位符优先级高于瞎猜

### 4. 生成 YAML

- 严格按 schema.md 字段生成，**只用 schema 中列出的字段**，不得发明新字段
- pipeline 模式不得出现 post_tasks、ignore_errors、any_errors_fatal
- 生成的剧本中占位变量（`{{xxx}}`）必须在 `vars:` 块有对应键
- 命令建议用 `command` 动作（`shell`/`cmd` 等价）

### 5. 写入与校验（强制）

1. 写入 `playbooks/<name>.yaml`
2. 运行校验：`owl playbook validate playbooks/<name>.yaml`（若 `owl` 不在 PATH，用 `go run ./cmd/cli playbook validate ...` 或构建产物）
3. 校验失败 → 按错误信息修复 → 重新校验，直到通过

### 6. 交付

向用户展示：

1. 生成的 YAML 全文
2. **待填变量清单**（表格：变量名 → 含义 → 当前占位值），提示运行时可 `--vars k=v` 覆盖
3. 建议执行命令，如：
   ```bash
   owl playbook run <name> --nodes node-01 --vars version=v1.0.0
   ```

## 示例参考

完整示例见 `.opencode/skills/playbook/examples/deploy-app.yaml`。

## 边界

- 只负责 playbook 的生成与语法校验，不实际执行剧本（如需执行由用户自行运行 `owl playbook run`）
- 目标节点未提供时 hosts 保持空数组，不要编造节点 ID
