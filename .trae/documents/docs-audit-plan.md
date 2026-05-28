# 文档审计计划 — go-owl docs/ 目录

## 审计结论总览

| 问题类别 | 数量 | 严重程度 |
|----------|------|----------|
| 完全重复的文档 | 7 组 | 🔴 高 |
| 已过期的文档 | 6 个 | 🟡 中 |
| 冗余说明 | 3 处 | 🟢 低 |

---

## 一、完全重复的文档（建议删除副本）

以下文档内容完全相同或高度重复，应只保留一份：

### 1. `docs/SESSION_DESIGN.md` ↔ `docs/dev/SESSION_DESIGN.md`
- **重复程度**: 100% 完全相同
- **建议**: 保留 `docs/dev/SESSION_DESIGN.md`，删除 `docs/SESSION_DESIGN.md`（根目录下的属于无意义的副本）
- **原因**: 这是开发设计文档，放在 `dev/` 目录下更合适

### 2. `docs/SSH_CONFIG.md` ↔ `docs/dev/SSH_CONFIG.md`
- **重复程度**: 100% 完全相同
- **建议**: 保留 `docs/dev/SSH_CONFIG.md`，删除 `docs/SSH_CONFIG.md`
- **原因**: SSH 配置解析属于开发/设计内容

### 3. `docs/DATABASE.md` ↔ `docs/reference/DATABASE.md`
- **重复程度**: 100% 完全相同
- **建议**: 保留 `docs/reference/DATABASE.md`，删除 `docs/DATABASE.md`
- **原因**: 数据库配置是参考类文档，放在 `reference/` 下合适

### 4. `docs/API_NODE_SOURCE.md` ↔ `docs/reference/API_NODE_SOURCE.md`
- **重复程度**: 100% 完全相同（含完整 FastAPI/Flask/Gin 示例）
- **建议**: 保留 `docs/reference/API_NODE_SOURCE.md`，删除 `docs/API_NODE_SOURCE.md`
- **原因**: API 节点源属于参考文档

### 5. `docs/SSH_USAGE.md` ↔ `docs/reference/SSH_USAGE.md`
- **重复程度**: 100% 完全相同
- **建议**: 保留 `docs/reference/SSH_USAGE.md`，删除 `docs/SSH_USAGE.md`

### 6. `docs/AI_OPTIMIZATION_PLAN.md` ↔ `docs/dev/AI_OPTIMIZATION_PLAN.md`
- **重复程度**: 100% 完全相同
- **建议**: 保留 `docs/dev/AI_OPTIMIZATION_PLAN.md`，删除 `docs/AI_OPTIMIZATION_PLAN.md`

### 7. `docs/LOGGING_PLAN.md` ↔ `docs/dev/LOGGING_PLAN.md`
- **重复程度**: 100% 完全相同
- **建议**: 保留 `docs/dev/LOGGING_PLAN.md`，删除 `docs/LOGGING_PLAN.md`

---

## 二、已过期的文档（与当前代码不匹配）

### 1. `docs/implementation_design.md`
- **问题**: 描述基于 gRPC 的 ControlService/AgentService 架构（含完整 .proto 定义）
- **现实**: 代码中没有任何 gRPC 依赖，无 `.proto` 文件，实际是纯 CLI + 本地库调用的单体架构
- **严重程度**: 🔴 严重过时 — 架构描述完全错误
- **建议**: **删除或归档**。当前代码实现的是纯 CLI 架构，此文档是早期设计阶段的产物，容易误导新开发者

### 2. `docs/dev/ARCHITECTURE.md`
- **问题**: 与 `implementation_design.md` 相同，描述 gRPC 架构
- **现实**: 代码中没有 gRPC
- **严重程度**: 🔴 严重过时
- **建议**: **重写或删除**。如果保留，需要彻底更新为符合当前 CLI 单体架构的描述

### 3. `docs/design/01_TIMEOUT_SEPARATION.md`
- **问题**: 文档提到创建 `executor_v2.go` 文件
- **现实**: 实际文件是 `internal/ssh/native_executor.go`，不存在 `executor_v2.go`
- **严重程度**: 🟡 轻微过时 — 核心设计逻辑已实现，文件名不匹配
- **建议**: 更新文档中的文件名引用为 `native_executor.go`

### 4. `docs/design/PLAYBOOK_TEMPLATE_SYSTEM.md`
- **问题**: 描述 `owl playbook templates` 命令及完整模板系统
- **现实**: `cmd/cli/cmd/playbook/` 下仅注册了 `list`、`validate`、`info`、`run` 四个子命令，没有 `templates` 相关代码
- **严重程度**: 🔴 功能未实现 — 纯设计文档，描述的模板系统**完全不存在于代码中**
- **建议**: 文档顶部添加醒目的"未实现"标记，或移至单独的 `proposals/` 目录

### 5. `docs/dev/FIX_PASSWORD_AUTH_CHAIN.md`
- **问题**: 描述的 Key → Password 认证链修复方案（方案 B: crypto/ssh 原生实现）
- **现实**: 修复已完全落地 —— `internal/ssh/native_executor.go` 已实现，`ConnectionInfo` 含 `Password` 字段，`buildAuthMethods()` 实现了密钥优先→密码兜底的认证链
- **严重程度**: 🟡 已完成的修复记录 — 可作为历史参考，但无现实指导意义
- **建议**: 在文档顶部标明 "✅ 已实现"，或归档到历史记录

### 6. `docs/dev/FIX_SSH_ERROR_INFO_LOSS.md`
- **问题**: 描述系统 SSH 命令 exit code 255 错误信息丢失的修复方案
- **现实**: 当前代码已**完全移除系统 SSH 调用**，全部改用 `golang.org/x/crypto/ssh` 原生库。原 bug 场景不再存在。文档描述的修复方式（解析 stderr、判断 exit code 255）与最终采用的架构重构方案不同
- **严重程度**: 🟡 场景已不存在 — 架构变更已从根本上规避了该问题
- **建议**: 标明 "✅ 已通过架构重构解决（替换为 crypto/ssh 原生实现）"，或归档

---

## 三、冗余的说明（内容重叠但非完全重复）

### 1. `docs/USAGE.md` ↔ `docs/user/USAGE.md`
- **重叠程度**: 约 80% — 两者都是 AI 助手使用指南，描述 4 种操作类型、CLI 使用方式、配置方法
- **差异**: `docs/USAGE.md` 更偏设计/规范层面；`docs/user/USAGE.md` 更偏用户操作指引
- **建议**: 合并为一篇，放在 `docs/user/USAGE.md`，删除 `docs/USAGE.md`

### 2. `docs/SESSION_USAGE.md` ↔ `docs/user/SESSION_USAGE.md`
- **重叠程度**: 约 90% — 都描述会话功能使用方式
- **建议**: 合并为一篇，保留 `docs/user/SESSION_USAGE.md`，删除 `docs/SESSION_USAGE.md`

### 3. `docs/implementation_design.md` ↔ `docs/dev/ARCHITECTURE.md` + `docs/dev/FILE_TRANSFER_ARCHITECTURE.md`
- `implementation_design.md` 包含了整体架构、自扩散文件传输方案、数据模型，与 `ARCHITECTURE.md` 和 `FILE_TRANSFER_ARCHITECTURE.md` 的架构内容高度重叠
- **建议**: 由于 `implementation_design.md` 的 gRPC 部分已过时，可用 `FILE_TRANSFER_ARCHITECTURE.md` 中更准确的描述来替代

---

## 四、建议操作汇总

### 立即删除（7 个纯副本）
| 文件 | 理由 |
|------|------|
| `docs/SESSION_DESIGN.md` | 与 `docs/dev/SESSION_DESIGN.md` 完全相同 |
| `docs/SSH_CONFIG.md` | 与 `docs/dev/SSH_CONFIG.md` 完全相同 |
| `docs/DATABASE.md` | 与 `docs/reference/DATABASE.md` 完全相同 |
| `docs/API_NODE_SOURCE.md` | 与 `docs/reference/API_NODE_SOURCE.md` 完全相同 |
| `docs/SSH_USAGE.md` | 与 `docs/reference/SSH_USAGE.md` 完全相同 |
| `docs/AI_OPTIMIZATION_PLAN.md` | 与 `docs/dev/AI_OPTIMIZATION_PLAN.md` 完全相同 |
| `docs/LOGGING_PLAN.md` | 与 `docs/dev/LOGGING_PLAN.md` 完全相同 |

### 合并后删除（2 个）
| 文件 | 合并目标 |
|------|----------|
| `docs/USAGE.md` | 合并到 `docs/user/USAGE.md` 后删除 |
| `docs/SESSION_USAGE.md` | 合并到 `docs/user/SESSION_USAGE.md` 后删除 |

### 标记/归档（3 个）
| 文件 | 处理方式 |
|------|----------|
| `docs/dev/FIX_PASSWORD_AUTH_CHAIN.md` | 顶部添加 "✅ 已实现" 标记 |
| `docs/dev/FIX_SSH_ERROR_INFO_LOSS.md` | 顶部添加 "✅ 已通过架构重构解决" 标记 |
| `docs/design/PLAYBOOK_TEMPLATE_SYSTEM.md` | 顶部添加 "⚠️ 未实现 — 设计提案" 标记 |

### 重写/更新（3 个）
| 文件 | 需要修改 |
|------|----------|
| `docs/implementation_design.md` | 删除（gRPC 架构已完全不存在） |
| `docs/dev/ARCHITECTURE.md` | 重写为当前 CLI 单体架构 |
| `docs/design/01_TIMEOUT_SEPARATION.md` | 更新 `executor_v2.go` → `native_executor.go` |

### 目录结构调整后
```
docs/
├── README.md
├── design/
│   ├── README.md
│   ├── 01_TIMEOUT_SEPARATION.md    (更新文件名引用)
│   ├── 02_RETRY_MECHANISM.md
│   ├── 03_ASYNC_EXECUTION.md
│   ├── PLAYBOOK_ACTION_OPTIONS.md
│   └── PLAYBOOK_TEMPLATE_SYSTEM.md (标记未实现)
├── dev/
│   ├── README.md
│   ├── ARCHITECTURE.md             (需重写)
│   ├── SESSION_DESIGN.md
│   ├── SSH_CONFIG.md
│   ├── FILE_TRANSFER_ARCHITECTURE.md
│   ├── AI_CONFIG_DESIGN.md
│   ├── AI_OPTIMIZATION_PLAN.md
│   ├── LOGGING_PLAN.md
│   ├── TEST_IMPLEMENTATION_REPORT.md
│   ├── FIX_PASSWORD_AUTH_CHAIN.md   (标记已实现)
│   └── FIX_SSH_ERROR_INFO_LOSS.md   (标记已解决)
├── reference/
│   ├── README.md
│   ├── SSH_USAGE.md
│   ├── DATABASE.md
│   └── API_NODE_SOURCE.md
└── user/
    ├── README.md
    ├── QUICKSTART.md
    ├── USAGE.md                     (合并后)
    ├── NODE.md
    ├── EXEC.md
    ├── PLAYBOOK.md
    ├── FILE.md
    ├── SESSION.md
    ├── SESSION_USAGE.md             (合并后)
    ├── AI.md
    ├── HISTORY.md
    └── SETTINGS.md
```

---

## 五、实施顺序

1. 删除 7 个完全重复的根目录文档
2. 合并 `USAGE.md` 和 `SESSION_USAGE.md` 到 user/ 对应文件
3. 删除 `docs/implementation_design.md`（gRPC 架构已不存在）
4. 为已完成的修复文档添加状态标记
5. 为未实现的设计文档添加警告标记
6. 更新 `01_TIMEOUT_SEPARATION.md` 中的文件名引用
7. 更新 `docs/README.md` 中的索引导航，确保链接指向正确的文件
