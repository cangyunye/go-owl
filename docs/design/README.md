# 设计文档索引

本文档包含 go-owl 项目的所有技术设计文档。

---

## 架构设计

| 文档 | 说明 |
|------|------|
| [ARCHITECTURE.md](../dev/ARCHITECTURE.md) | 整体架构设计 |
| [SESSION_DESIGN.md](../dev/SESSION_DESIGN.md) | 会话管理设计 |

---

## 功能模块设计

| 文档 | 说明 |
|------|------|
| [01_TIMEOUT_SEPARATION.md](01_TIMEOUT_SEPARATION.md) | 超时分离机制设计 |
| [02_RETRY_MECHANISM.md](02_RETRY_MECHANISM.md) | 重试机制设计 |
| [03_ASYNC_EXECUTION.md](03_ASYNC_EXECUTION.md) | 异步执行设计 |
| [04_MONITORING_ALERTING.md](04_MONITORING_ALERTING.md) | 智能监控与告警体系设计 |
| [05_CAPABILITY_MATCHING_SPEC.md](05_CAPABILITY_MATCHING_SPEC.md) | 能力库与 AI 匹配规范（脚本库·剧本库·告警自愈） |
| [PLAYBOOK_ACTION_OPTIONS.md](PLAYBOOK_ACTION_OPTIONS.md) | Playbook 动作选项设计 |
| [PLAYBOOK_TEMPLATE_SYSTEM.md](PLAYBOOK_TEMPLATE_SYSTEM.md) | Playbook 模板系统设计 |

---

## 测试设计

| 文档 | 说明 |
|------|------|
| [TEST_DESIGN.md](../../tests/README.md) | 自动化测试方案设计 |

---

## 参考文档

| 文档 | 说明 |
|------|------|
| [DATABASE.md](../reference/DATABASE.md) | 数据库设计参考 |
| [SSH_USAGE.md](../reference/SSH_USAGE.md) | SSH 使用参考 |
| [API_NODE_SOURCE.md](../reference/API_NODE_SOURCE.md) | API 节点源参考 |

---

## 待开发功能

- [x] 智能监控与告警体系（Phase 1：采集/存储/规则/告警/对策/通知/serve 集成）
- [x] AI 处置闭环设计（Phase 2，监控告警体系的后半环）→ [05_CAPABILITY_MATCHING_SPEC.md](05_CAPABILITY_MATCHING_SPEC.md)（设计评审稿，待分期实施）
- [ ] AI 配置优化设计
- [ ] TUI 交互式界面设计
- [ ] 分布式执行设计
- [ ] Web 管理界面设计

---

*最后更新: 2026-05-22*
