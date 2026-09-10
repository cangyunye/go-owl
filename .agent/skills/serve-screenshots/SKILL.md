---
name: serve-screenshots
description: owl-serve Web 控制台截图重拍工作流。当前端 UI 变更后需要更新 docs/serve-web-ui.md 的 26 张截图、或新增页面需要补截图时使用。基于 Playwright 对运行中的 owl-serve 实拍，包含演示数据准备、采集脚本与验收清单。
---

# owl-serve 截图工作流（serve-screenshots）

维护 [docs/serve-web-ui.md](../../../docs/serve-web-ui.md) 的截图（`docs/images/serve/01-login.png` ... `26-theme-warm.png`）。
配套脚本位于 `scripts/serve-screenshots/`：`seed_demo_data.py`（准备演示数据）、`capture.py`（采集 26 张图）。

## 使用时机

- 前端页面/交互变更，导致现有截图与实际 UI 不一致
- 新增页面或功能，需要补充截图条目
- 文档要求的分辨率、主题等截图口径调整

## 工作流

1. **构建并启动服务**（前端资源是编译期嵌入的，改过 web/ 必须重新构建）：
   ```bash
   task build-serve
   ./build/darwin-arm64/owl-serve --reset-admin --port 8080 > .reasonix-tmp/serve.log 2>&1 &
   # 从 .reasonix-tmp/serve.log 读取输出的 admin 密码
   ```
   注意：**绝不删除 `~/.owl/owl.db`** 来找回密码，用 `--reset-admin`（见 AGENTS.md）。

2. **准备演示数据**（幂等，可重复执行）：
   ```bash
   python3 scripts/serve-screenshots/seed_demo_data.py --password <密码>
   ```
   注入：50 个种子节点、演示用户、中转站文件、演示剧本、3 条分组执行任务。
   建议等约 60s 让任务进入 failed 终态（历史/详情截图更真实）。

3. **采集截图**：
   ```bash
   python3 scripts/serve-screenshots/capture.py --password <密码>
   ```
   输出 26 张 PNG 到 `docs/images/serve/`，脚本自校验数量与文件大小（<10KB 视为异常）。

4. **人工抽查关键图**（每类至少一张）：仪表盘、节点列表、剧本列表、任务详情弹窗、
   设置页、主题图。重点核对：空表格/「加载中」卡死、弹窗未打开、页面报错。
   「加载中」卡死通常是前端挂载回调抛错（历史案例：api.js 引用未定义的 wsTicket，
   提交 67e8c75），用 Playwright 的 `pageerror` 事件定位。

5. **更新文档**：若新增/改名了截图文件，同步 `docs/serve-web-ui.md` 的图片引用
   （文档按 13 个功能小节组织，新页面应新增小节并保持编号连续）。

6. **提交**：截图与文档一个原子提交（`docs(serve): ...`）；若过程中修复了前端 bug，
   bug 修复单独一个 `fix(serve): ...` 提交在前。

## 依赖与口径

- Python3 + Playwright（`python3 -c 'import playwright'` 验证；chromium headless）
- 口径：视口 1440×900、deviceScaleFactor=2、zh-CN、默认「深空」主题（主题图为另外两套）
- 模拟节点 SSH 不可达：任务失败/部分失败属预期，不要为了「全部成功」的截图造假数据
- 端口被占用时换 `--port`，并给脚本传 `--base-url`
