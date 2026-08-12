---
id: "feat-user_role-select-pagination-search"
domain: "feat-user"
slug: "role-select-pagination-search"
title: "用户管理界面的用户角色没有加载，添加的用户很多，无法下拉，需要增加翻页和搜索"
status: "resolved"
created: "2026-08-12T18:56:44+08:00"
resolved: "2026-08-12T19:19:30+08:00"
commit: "98004dc"
branch: "main"
platform: "darwin"
session: "ses_00a620158ffe0J0rk1kIXk3JpZ"
---

# feat-user_role-select-pagination-search

## 问题

用户管理界面的用户角色没有加载，添加的用户很多，无法下拉，需要增加翻页和搜索

## 环境

| 项 | 值 |
|----|----|
| git commit | 98004dc |
| 分支 | main |
| 平台 | darwin |
| 建档时间 | 2026-08-12T18:56:44+08:00 |
| 会话 | ses_00a620158ffe0J0rk1kIXk3JpZ |

## 调查过程

- [18:56] 建档
- [19:19] 记录证据 1 项
- [19:19] 记录终端文本快照
- [19:19] 结案

## 日志与摘录

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: E2E 验证输出:后端分页/搜索 + 浏览器端断言](shots/001-191920.txt)

## 修复方案

GET /users 从一次性返回全部用户改为分页+搜索:store 新增 UserStore.ListPaged(ctx, keyword, page, pageSize),COUNT 总数 + keyword 对 username/display_name 模糊匹配 + LIMIT/OFFSET;handler List 解析 page/page_size/q 并返回 {data, meta:{total,page,page_size}};前端 api.users(params) 透传查询参数,users.js 加搜索框(防抖 100ms)+ 上/下一页控件 + "共 N 条 · 第 X/Y 页",新建用户后重置回第 1 页。E2E 验证 36 用户默认返回 20 条、翻页正常、q=user3 命中 7 条、无匹配返回 0。

## 复盘

根因:用户管理页无分页/搜索,后端全量返回、前端全量渲染,用户多时列表过长无法滚动查看,角色列数据也随之"加载不出来"。教训:任何管理列表接口都应默认支持分页(页大小上限)+ 搜索,避免全量返回;改动内嵌 web 资源后必须重建 build/owl-serve(go:embed 旧二进制不会生效)。另发现 handler 包有未完成 WIP(playbook_test.go 引用尚不存在的 Category 字段)导致包无法编译,需提醒用户完成该改动。
