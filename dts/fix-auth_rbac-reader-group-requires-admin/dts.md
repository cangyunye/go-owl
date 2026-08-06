---
id: "fix-auth_rbac-reader-group-requires-admin"
domain: "fix-auth"
slug: "rbac-reader-group-requires-admin"
title: "RBACMiddleware 取 allowedRoles 的最大等级而非最小，导致 reader(writer/operator) 分组实际要求 admin "
status: "resolved"
created: "2026-08-07T00:05:06+08:00"
resolved: "2026-08-07T00:09:15+08:00"
commit: "5a04a04"
branch: "feat/ui-redesign-phase0"
platform: "darwin"
session: "ses_0284acb95ffeHLcLs8jFL1x3Kl"
---

# fix-auth_rbac-reader-group-requires-admin

## 问题

RBACMiddleware 取 allowedRoles 的最大等级而非最小，导致 reader(writer/operator) 分组实际要求 admin 才能访问：GET /nodes 对 editor/viewer 返回 403。用户要求另开任务修复。

## 环境

| 项 | 值 |
|----|----|
| git commit | 5a04a04 |
| 分支 | feat/ui-redesign-phase0 |
| 平台 | darwin |
| 建档时间 | 2026-08-07T00:05:06+08:00 |
| 会话 | ses_0284acb95ffeHLcLs8jFL1x3Kl |

## 调查过程

- [00:05] 建档
- [00:05] 记录日志 (bash): 复现：editor/viewer 访问 GET /nodes 得到 403
- [00:09] 记录日志 (bash): 修复后角色矩阵 E2E 全部符合预期
- [00:09] 记录证据 1 项
- [00:09] 记录终端文本快照
- [00:09] 结案

## 日志与摘录

### [bash] 2026-08-07T00:05:12+08:00 · 复现：editor/viewer 访问 GET /nodes 得到 403

```
# 复现（E2E 时发现）
editor1 登录后 GET /api/v1/nodes 返回 {"code":403,"message":"insufficient permissions"}
viewer1 同理 403；admin 正常返回列表。

# 根因代码 cmd/plugins/serve/handler/auth.go:113-129
func (h *AuthHandler) RBACMiddleware(allowedRoles ...model.Role) gin.HandlerFunc {
	minLevel := 0
	for _, r := range allowedRoles {
		if lvl, ok := roleHierarchy[string(r)]; ok && lvl > minLevel {
			minLevel = lvl   // BUG: 取最大值
		}
	}
	...
}
roleHierarchy = {viewer:0, editor:1, operator:2, admin:3}

# 调用点 cmd/plugins/serve/server.go
reader  : RBACMiddleware(viewer, editor, operator, admin)  -> minLevel=3(admin) 应=0(viewer)
writer  : RBACMiddleware(editor, operator, admin)          -> minLevel=3(admin) 应=1(editor)
operator: RBACMiddleware(operator, admin)                  -> minLevel=3(admin) 应=2(operator)
admin   : RBACMiddleware(admin)                            -> minLevel=3 正确
```

### [bash] 2026-08-07T00:09:03+08:00 · 修复后角色矩阵 E2E 全部符合预期

```
修复：cmd/plugins/serve/handler/auth.go RBACMiddleware 改为取 allowedRoles 最低等级
（minLevel 初值 -1 作为哨兵，避免与 viewer=0 冲突；空列表回退 0）。
新增 rbac_test.go 三个分组语义测试（reader/writer/operator 多角色）。
全量 go test ./... 与 go vet 通过。

E2E（真实服务 + 重建二进制）角色矩阵验证：
viewer  GET /nodes          -> 200
editor  GET /nodes          -> 200
op      GET /nodes          -> 200
viewer  GET /nodes/stats    -> 200
viewer  POST /nodes         -> 403
editor  POST /nodes         -> 201
op      POST /exec          -> 202
viewer  POST /exec          -> 403
viewer  DELETE /nodes/:id   -> 403
admin   DELETE /nodes/:id   -> 200
浏览器：viewer 行仅「展开详情」按钮、editor 行 = Ping/SSH检查/编辑、admin 全量。
```

## 测试场景与 E2E 用例

| # | 用例 | 步骤 | 预期 | 结果 |
|---|------|------|------|------|

## 证据截图

[文本快照: E2E 角色访问矩阵输出](shots/001-000908.txt)

## 修复方案

RBACMiddleware 改为求 allowedRoles 的最小等级（minLevel 初值 -1 哨兵，防 viewer=0 冲突；空列表回退 0）。分组语义恢复正确：reader→viewer、writer→editor、operator→operator、admin→admin。新增 rbac_test.go 三个多角色分组测试覆盖该语义。

## 复盘

根因：取 allowedRoles 的最大等级当作门槛，使 reader/writer/operator 分组全部退化为仅 admin 可访问（变量名 minLevel 与实际实现取 max 相反）。旧测试只覆盖单角色调用，未暴露。教训：多角色分组（RBACMiddleware 传多个角色）应有专门用例覆盖最低角色放行；另外 E2E 时发现端口上有旧二进制进程占着 IPv6 *:8080，导致 curl localhost 打到旧服务、误以为修复无效——排查权限类问题务必先确认命中进程是刚编译的新版本。
