# 发布变更记录（docs/releases）

每个版本一个文件 `vX.Y.Z.md`，内容就是该版本的变更记录（也就是 GitHub Release 的正文）。

## 约定

- 文件在**发布前**提交到 main：`release.yml` 在推 tag 时按 `docs/releases/<tag>.md` 取正文，
  文件不存在则退化为只有一行 Full Changelog 链接的自动正文（并打 warning）。
- 写法沿用 v1.4.1 起的格式：标题 → 一句话概述 + 提交数 → 按 `✨ 功能增强 / 🐛 问题修复 /
  ⚡ 性能 / 🧪 测试 / 📝 文档` 分节，末尾保留 `**Full Changelog**: .../compare/vPREV...vX.Y.Z` 链接。
- 正文来自真实提交（`git log --no-merges --reverse vPREV..vX.Y.Z`），不写未落地的内容；
  重要条目带短 hash，便于回溯。
- 版本补齐：因 `--generate-notes` 在本仓库（不用 PR 流程）只会生成比较链接，
  v1.5.0–v1.8.1 的正文是事后按提交历史补齐的；v1.4.1 为当时手写，已镜像到本目录。

## 相关

- 发布流程与清单见 [docs/dev/RELEASING.md](../dev/RELEASING.md)
- Release 正文的手工修正：`gh release edit vX.Y.Z --notes-file docs/releases/vX.Y.Z.md`
