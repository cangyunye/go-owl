---
id: "feat-script_category-crud-missing-write"
domain: "feat-script"
slug: "category-crud-missing-write"
title: "owl剧本管理菜单缺少对\"分类\"编辑和添加时的写入"
status: "resolved"
created: "2026-08-12T18:57:17+08:00"
resolved: "2026-08-12T19:13:26+08:00"
commit: "98004dc"
branch: "main"
platform: "darwin"
session: "ses_00a617fbfffeLWZqCrhhvj97Bz"
---

# feat-script_category-crud-missing-write

## 问题

owl剧本管理菜单缺少对"分类"编辑和添加时的写入

## 环境

| 项 | 值 |
|----|----|
| git commit | 98004dc |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-12T18:57:17+08:00 |
| 会话 | ses_00a617fbfffeLWZqCrhhvj97Bz |

## 调查过程

- [18:57] 建档
- [19:12] 记录证据 1 项
- [19:12] 记录终端文本快照
- [19:12] 新增 E2E 用例: 剧本管理:创建/编辑剧本时写入分类
- [19:12] 记录日志 (chat): 根因与方案:分类此前仅由路径派生,向导写文件始终为空
- [19:13] 结案

## 日志与摘录

### [chat] 2026-08-12T19:12:59+08:00 · 根因与方案:分类此前仅由路径派生,向导写文件始终为空

```
根因:分类(category)在系统中由 SyncFromDir 按库目录子目录路径派生;而"新建/编辑"向导始终把 YAML 写到库根目录(handler.Create 写 libraryPath/{name}.yaml),且向导 UI 与 createTemplateRequest 都没有分类字段 → 分类永远是空,无法写入。

方案:分类内嵌进 playbook YAML 顶层 category 字段(YAML 成为源)。
- pkg/playbook/template.go TemplatePlaybook 增加 Category,渲染进 YAML
- store.readPlaybookMeta 从 YAML 读 category;子目录文件的路径派生分类优先(现有行为不变)
- handler createTemplateRequest 增加 Category;Create 写入 tpl.Category;Edit 返回 DB 行的 pb.Category(兼顾路径派生分类的剧本)
- 前端 wizard Step1 增加"分类"输入框,编辑回填,保存携带 category

不采用"按分类写子目录"方案,因其会改文件路径→ID=hash(path)变化→破坏运行历史引用与 Sync 去重,风险大。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|
| 1 | 剧本管理:创建/编辑剧本时写入分类 | 1. 登录 → 剧本管理页 → 新建 → Step1 填名称+分类 web → 确认保存 2. 列表查看分类列显示 web 3. 点该行"编辑" → Step1 分类框回填 web → 改为 ui-cat2 保存 4. 列表分类列显示 ui-cat2;刷新(playbook/refresh)后分类仍保留 | 创建与编辑时分类均可填写并持久化(写入 YAML 与 DB),刷新不丢失 | pass |

## 证据截图

[文本快照: E2E 结果(Playwright 控制台输出):创建/编辑分类均写入成功](shots/001-191252.txt)

## 修复方案

分类内嵌进 playbook YAML 顶层 category 字段,打通"新建/编辑向导→后端→库→前端"全链路:
1. pkg/playbook/template.go:TemplatePlaybook 增加 Category(yaml:category,omitempty),RenderTemplateYAML 渲染进文件
2. store/playbook.go:playbookYAMLMeta/readPlaybookMeta 从 YAML 读 category,刷新(SyncFromDir)时根目录文件保留内嵌分类、子目录文件仍以路径派生优先(原行为不变)
3. handler/playbook.go:createTemplateRequest 增加 Category;Create 写入 tpl.Category;Edit 返回 DB 行 pb.Category(覆盖路径派生分类的剧本),宽松解析分支同样回填
4. 前端 playbooks.js:wizard Step1 增加"分类"输入框,resetCpModal 清空,编辑回填 data.category,保存携带 category,确认页与 YAML 预览展示

验证:go test(store/handler/pkg-playbook 全绿)+ Playwright 浏览器 E2E(创建 ui-cat → 列表显示;编辑回填 ui-cat → 改为 ui-cat2 → 列表更新);API E2E:分类写入 YAML 与 DB,playbook/refresh 后分类保留,编辑改分类 ID 保持稳定。

## 复盘

根因:分类此前唯一来源是 SyncFromDir 按库目录子目录路径派生,而向导始终把文件写进库根目录且无分类字段,导致永远为空。踩坑点:曾考虑"按分类写子目录",但文件路径变更会改变 playbook ID(hash of abs path),破坏运行历史引用与 Sync 去重,风险大;最终选"分类内嵌 YAML"这一低风险方案。下次遇到同类"元数据写不进"的问题,先确认该元数据的既有来源与 ID 稳定性约束,再选落点(路径 vs 文件内嵌)。
