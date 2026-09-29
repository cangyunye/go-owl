# go-owl 官网（纯站点分支）

本分支为孤儿分支（orphan branch）：**只有站点静态文件，不含主仓库代码与提交历史**。
单提交、零构建、零依赖，目录本身即可部署。

## 目录结构

```
.
├── index.html          # 单页 9 区块，文案通过 data-i18n 绑定
├── css/style.css       # 视觉令牌取自 frontend-design/ 控制台设计稿（深空蓝暗色主题）
├── js/i18n.js          # 中英双语文案字典
├── js/main.js          # 语言切换 / 版本号拉取 / 终端动画 / 截图 tab / 复制按钮 / 导航
├── assets/favicon.svg  # 品牌 logo（复用产品 favicon）
└── assets/screenshots/ # Web 控制台真实截图（来自主仓库 docs/images/serve/）
```

## 本地预览

```bash
git clone -b website --depth 1 https://github.com/cangyunye/go-owl.git
cd go-owl
python3 -m http.server 8090   # 打开 http://localhost:8090
```

## 发布到 GitHub Pages（无需任何 CI）

1. 仓库 **Settings → Pages → Build and deployment → Source** 选 **Deploy from a branch**。
2. Branch 选 `website` + `/ (root)`，Save。之后每次推送本分支自动重新发布。
3. 绑定自定义域名请同步更新 `index.html` 中的 `og:url` / `og:image` / `canonical` 三处绝对地址。

版本徽章通过 GitHub API 动态获取最新 release tag，接口不可用时回退到 `js/main.js` 中的
`FALLBACK_VERSION`（发版后记得顺手更新）。

## 迁移到其他云厂商

目录是纯静态产物，任何静态托管都能直接用，无需改造：

| 目标环境 | 做法 |
|---|---|
| 阿里云 OSS / 腾讯云 COS | 上传本分支全部内容，开启「静态网站托管」，索引页设为 `index.html`，前端挂 CDN |
| Cloudflare Pages / Netlify / Vercel | 连接仓库，分支选 `website`，构建命令留空，输出目录填根目录 |
| 自有服务器 Nginx | `root /path/to/go-owl;`（本分支检出目录），无需任何额外配置 |

## 合回主仓库

本分支与 main 无共同历史。若站点验证完毕要进主仓库，在 main 上执行：

```bash
git checkout website -- .            # 先临时取回文件
mkdir website && git mv index.html css js assets README.md website/
git commit -m "feat(website): 官网落地页并入主仓库"
```
