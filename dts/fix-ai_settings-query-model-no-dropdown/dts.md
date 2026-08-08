---
id: "fix-ai_settings-query-model-no-dropdown"
domain: "fix-ai"
slug: "settings-query-model-no-dropdown"
title: "AI设置里的查询模型，为什么查询成功，但是没有下拉框提示可选择模型"
status: "open"
created: "2026-08-07T13:14:38+08:00"
resolved: ""
commit: "ae59de5"
branch: "feat/ui-redesign-phase0"
platform: "darwin"
session: "ses_0258828dbffe79RyrztZw3jdc2"
---

# fix-ai_settings-query-model-no-dropdown

## 问题

AI设置里的查询模型，为什么查询成功，但是没有下拉框提示可选择模型

## 环境

| 项 | 值 |
|----|----|
| git commit | ae59de5 |
| 分支 | feat/ui-redesign-phase0 |
| 平台 | darwin |
| 建档时间 | 2026-08-07T13:14:38+08:00 |
| 会话 | ses_0258828dbffe79RyrztZw3jdc2 |

## 调查过程

- [13:14] 建档
- [13:19] 记录日志 (bash): 根因是 custom provider 模型字段为 input,查询成功分支只提示手动输入不渲染下拉框
- [13:19] 记录证据 1 项
- [13:19] 记录终端文本快照

## 日志与摘录

### [bash] 2026-08-07T13:19:32+08:00 · 根因是 custom provider 模型字段为 input,查询成功分支只提示手动输入不渲染下拉框

```
复现(Playwright + 本地 mock OpenAI /v1/models):
- custom provider(输入框): 查询成功 toast「获取到 3 个模型（请手动输入）」,字段仍是 <input>,无下拉框
- deepseek(select): 查询成功,下拉框被填充 model-alpha/beta/gamma

根因: settings.js fetch-models 分支,modelInput.tagName !== 'SELECT' 时(仅 custom provider 的模型字段是 input)只弹「请手动输入」并 return,不渲染下拉框。

修复: 该分支改为把 input 原地替换为 <select class="ai-model-select" data-provider=...> 并填入获取到的模型选项(保留同 class/data-provider,保存/加载逻辑不受影响)。

修复后 E2E: custom → tag=SELECT, options=['— 选择模型 —','model-alpha (mock)',...], toast「获取到 3 个模型」, 无 JS 错误。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: 修复后 custom provider 查询模型出现下拉框](shots/001-131938.txt)

## 修复方案

## 复盘
