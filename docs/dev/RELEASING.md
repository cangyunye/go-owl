# 发布流程（RELEASING）

go-owl 的版本号不写在代码里：构建时 `VERSION` 由 Taskfile 自动取**最近的 git tag**
（`git describe --tags --abbrev=0`，见 `Taskfile.yml` 顶部 vars，可用 `VERSION=x.y.z` 覆盖）。
因此「发一个版本」= 打一个正确的 tag，其余（产物版本号、版本自识别）自动跟上。

> 这份文档的存在有具体原因：v1.7.0 与 v1.8.0 的功能分支都已合并进 main，
> 但合并后**忘了打 tag**，同时功能分支也没删；直到 2026-09-25 才发现并补打
> （v1.7.0 → `1d57e58`、v1.8.0 → `ad9fc57`）。从此打标签统一走 `task tag`，用清单收尾。

## 1. 版本与分支约定

| 项 | 约定 |
|---|---|
| 版本号 | 语义化 `x.y.z`；小版本（`x.y.0`）= 功能发布，补丁（`x.y.z`）= 修复发布 |
| 功能分支 | `feat/vX.Y.Z-<slug>`（如 `feat/v1.9.0-app-tabs`），从 main 拉出，完成后合并回 main |
| 标签 | 注释标签 `vX.Y.Z`，打在 **main 上的发布提交**（通常是合并该功能分支的 merge commit，或最后一个发布相关提交） |
| 标签注释 | 形如 `vX.Y.Z: <一句话要点>`（历史惯例，如 `v1.6.0: 代码审查优化修复`） |
| 产物 | 版本号来自 tag，无需改代码；`owl-serve --version` / `owl version` 用来自检 |

## 2. 标准流程

```bash
# 0) 功能分支开发完成 → 合并回 main（merge commit 里写清版本要点），推 main
git switch main && git pull --ff-only
git merge --no-ff feat/v1.9.0-app-tabs -m "merge: feat/v1.9.0-app-tabs 内建标签页（v1.9.0）"

# 0.5) 写本版变更记录 docs/releases/vX.Y.Z.md（发布前必须提交，CI 用它当 Release 正文）
git log --no-merges --reverse v1.8.1..HEAD --format='%h %s'   # 素材：真实提交，别凭印象写
git add docs/releases/v1.9.0.md && git commit -m "docs(release): v1.9.0 变更记录"
git push origin main

# 1) 打标签（唯一入口；会校验：工作区干净 / 在 main / 与 origin/main 一致 / 版本号递增）
task tag MESSAGE="内建标签页" -- 1.9.0        # 先看 DRY 预览可加 DRY=1
# VERSION=1.9.0 task tag 等价；注意 vars 要写在 -- 之前

# 2) 构建产物（多平台；只发 Web 控制台可只跑 build-serve）
task build-all WITH=serve
task build-serve PLATFORMS="windows/amd64 linux/amd64 darwin/arm64"

# 3) 验证产物自识别
./build/windows-amd64/owl-serve.exe --version   # 应显示 owl-serve version 1.9.0 (commit ...)

# 4) 收尾：删除已合并的功能分支
git push origin --delete feat/v1.9.0-app-tabs
```

需要严格「产物代码 = 标签代码」时，从标签单独构建（避免带上标签之后的文档提交）：

```bash
git worktree add --detach build/_rel_vX.Y.Z vX.Y.Z
(cd build/_rel_vX.Y.Z && task build-serve PLATFORMS="windows/amd64 linux/amd64 darwin/arm64")
cp build/_rel_vX.Y.Z/build/*/owl-serve* build/            # 按平台目录拷贝
git worktree remove build/_rel_vX.Y.Z --force
```

## 3. 发布清单

- [ ] 功能分支已合并进 main，`git push origin main` 完成
- [ ] `docs/releases/vX.Y.Z.md` 变更记录已写好并提交（`release.yml` 优先用它当 Release 正文；
      缺失时只会有自动生成的一行比较链接）
- [ ] `task tag ... -- x.y.z` 成功（自动校验工作区/分支/远端一致性/版本递增）
- [ ] CI 的 release 工作流跑完：Release 正文正确 + 15 个跨平台资产上传
- [ ] 产物已构建，`--version` 显示新版本号
- [ ] 设计/开发文档与本版一致（`docs/design/README.md` 索引、必要时 dts 档案）
- [ ] 前端有 UI 变更时，`docs/serve-web-ui.md` 截图已按 `.agent/skills/serve-screenshots` 重拍
- [ ] 已合并的功能分支已删除（`git ls-remote --heads origin` 只剩 main 与在建分支）
- [ ] release 要点写进 merge commit 与 tag 注释（便于日后 `git tag -n` 回顾）

## 4. 纠错与补打

| 情况 | 处理 |
|---|---|
| 标签打错提交 | `git tag -d vX.Y.Z && git push origin :refs/tags/vX.Y.Z`，再 `task tag ... -- x.y.z` 重打（打错的标签没人拉过时最安全） |
| 版本号打错（如该是补丁版本） | 同上删除后重打；已推送且有人拉过时，宁可递增一个新版本，不要改历史 |
| 忘了打标签（本次 v1.7.0/v1.8.0 的情形） | 找到 main 上的发布合并提交，补打并注明：`git tag -a vX.Y.Z <sha> -m "vX.Y.Z: <要点>（补打：内容已合并，当时漏打标签）"`；`task tag` 只能给 HEAD 打标签，补历史标签要手动 |
| 标签存在但分支没删 | `git push origin --delete <branch>`；删前用 `git merge-base --is-ancestor origin/<branch> main` 确认已完全合并 |

## 5. 注意点

- **Release 正文来源**：`release.yml` 按 `docs/releases/<tag>.md` 取正文（约定见
  [docs/releases/README.md](../releases/README.md)）。本仓库不走 PR 流程，若缺该文件，
  `--generate-notes` 只会生成一行 `Full Changelog` 链接——v1.5.0–v1.8.1 就是这样发出去的，
  后来才按提交历史补齐；补正文用 `gh release edit vX.Y.Z --notes-file docs/releases/vX.Y.Z.md`。
- `task tag` 的版本号走**位置参数**（`-- 1.9.0`），不用 `VERSION=`：Taskfile 顶层已有自动
  推导的 `VERSION`，任务内引用同名变量会解析到它，导致「没传也非空」而漏掉校验。
- 校验用 git 原生能力（`for-each-ref --sort=-v:refname`）做版本序比较，不依赖 `sort -V` 等外部命令，
  Windows 上 Task 内嵌 shell 也能跑。
- 提交进版本库的只有 tag 与分支动作，`build/` 产物不入库（见 `.gitignore`）。
