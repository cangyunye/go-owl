---
id: "fix-ui_dark-theme-menu-text-invisible"
domain: "fix-ui"
slug: "dark-theme-menu-text-invisible"
title: "前端暗色主题下概览、节点列表等菜单左侧文字显示灰色看不见,要求与右侧数据表字体颜色一致改为白色"
status: "resolved"
created: "2026-08-06T22:13:45+08:00"
resolved: "2026-08-06T22:39:52+08:00"
commit: "99b0660"
branch: "feat/ui-redesign-phase0"
platform: "darwin"
session: "ses_02893c943ffelSKZAC9N7xs5Fq"
---

# fix-ui_dark-theme-menu-text-invisible

## 问题

前端暗色主题下概览、节点列表等菜单左侧文字显示灰色看不见,要求与右侧数据表字体颜色一致改为白色

## 环境

| 项 | 值 |
|----|----|
| git commit | 99b0660 |
| 分支 | feat/ui-redesign-phase0 |
| 平台 | darwin |
| 建档时间 | 2026-08-06T22:13:45+08:00 |
| 会话 | ses_02893c943ffelSKZAC9N7xs5Fq |

## 调查过程

- [22:13] 建档
- [22:37] 记录日志 (bash): 暗色主题下左侧面板文字为 muted 灰,数据表为 fg 白
- [22:39] 记录证据 1 项
- [22:39] 记录终端文本快照
- [22:39] 结案

## 日志与摘录

### [bash] 2026-08-06T22:37:07+08:00 · 暗色主题下左侧面板文字为 muted 灰,数据表为 fg 白

```
Playwright computed styles (dark default theme, port 8090):

nodes page:
- #panelTitle span (概览/节点分组 header): color oklch(0.55 0.015 250) = var(--muted) 灰色
- .panel-item (分组列表文字): color oklch(0.55 0.015 250) = var(--muted)
- .nav-item (左侧图标栏): color oklch(0.55 0.015 250) = var(--muted)
- .data-table td (右侧数据表): color oklch(0.92 0.01 250) = var(--fg) 白

dashboard page:
- panelItem active: oklch(0.62 0.18 255) accent
- panelHeader: muted

结论: 左侧面板标题+列表项文字用 var(--muted),暗色下灰度 55% 接近背景不可见;数据表文字用 var(--fg) 白。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: 修复后暗色主题节点列表截图:面板标题与分组文字变为白色与数据表一致](shots/001-223926.txt)

## 修复方案

cmd/plugins/serve/web/css/app.css 中 .panel-header 与 .panel-item 文字颜色由 var(--muted) 改为 var(--fg)。修复后暗色主题下左侧面板标题(概览)与列表项(分组/节点文字)从灰色 oklch(0.55) 变为白色 oklch(0.92),与右侧数据表 td 字体颜色(var(--fg))完全一致;浅色主题同样跟随 var(--fg) 保持与数据表一致。重新 make build-serve 使 go:embed 打包的新 CSS 生效。

## 复盘

根因:左侧面板文字误用 --muted(次要文本色,暗色下 55% 亮度接近背景),而数据表正文用 --fg(主文本色)。次要层级色不应作为可读正文色。教训:侧边栏菜单/列表文字应使用 var(--fg) 而非 var(--muted);改 CSS 后需重新 make build-serve,因为 web/ 通过 go:embed 打进二进制,运行中的服务不会热更新。
