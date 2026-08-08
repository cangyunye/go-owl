# playbook 生成技能

根据用户描述的使用步骤，生成符合 owl playbook 模板的 YAML 剧本（写入 `playbooks/` 并自动 `owl playbook validate` 校验）。

本目录是**可分发副本**：把整个 `playbook/` 目录复制到你所用 AI 代理的项目级技能目录即可启用。

## 安装（按所用工具选择目标目录）

| 工具 | 复制到 |
|------|--------|
| opencode | 项目根 `.opencode/skills/playbook/` |
| Claude Code | 项目根 `.claude/skills/playbook/` |
| 其他（agent 兼容） | 项目根 `.agents/skills/playbook/` |

复制后需重启/新开会话，技能才会被自动发现。

## 使用

直接说出操作步骤即可触发，例如：

- "生成一个部署 nginx 的剧本：1. 安装 nginx 2. 上传配置文件 3. 重启服务 4. 检查状态"
- "帮我编排一个数据库备份的 playbook"
- "创建 playbook：每天早上对 db 分组执行全量备份"

触发词：`生成剧本`、`创建 playbook`、`部署流程`、`自动化运维步骤`、`编排任务`。

## 技能行为

1. **澄清**：节点/分组、执行模式（pipeline / fail_continue）、是否涉及文件传输，缺失时一次问完
2. **拆解**：每步映射为一个任务（command / script / upload / download / include），按 pre_tasks / tasks / post_tasks 划分阶段
3. **推荐参数**：命令、路径、端口按常识给出可执行的默认值；**描述不清的部分用 `{{变量}}` 占位**，统一收进 `vars:` 块
4. **校验**：写入 `playbooks/<name>.yaml` 后运行 `owl playbook validate`，失败自动修复直到通过
5. **交付**：YAML 全文 + 待填变量清单 + 建议执行命令 `owl playbook run <name> --vars k=v`

## 文件说明

| 文件 | 说明 |
|------|------|
| `SKILL.md` | 技能主指令（工作流与占位策略） |
| `schema.md` | playbook YAML 完整 schema 参考（生成唯一依据） |
| `examples/deploy-app.yaml` | 完整示例剧本 |
