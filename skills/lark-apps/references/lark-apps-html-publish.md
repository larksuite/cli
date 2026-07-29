# apps +html-publish

把本地 HTML 文件或静态目录发布为妙搭应用访问 URL。运行时命令事实以 `lark-cli apps +html-publish --help` 为准。

## 何时用

用于把已经存在的本地 HTML 文件或静态产物目录发布成妙搭访问 URL。它不负责生成 HTML 内容，也不负责全栈应用代码发布。

## 命令骨架

- 必填：`--app-id`、`--path`。
- `--path` **必须是相对路径**（如 `./dist`、`./index.html`），不支持绝对路径。如果目标文件在其他目录，先 `cd` 到该目录再用相对路径，或用相对于当前目录的路径。
- `--path` 可以是单个文件或目录；入口必须是 `index.html`。
- 可选：`--allow-sensitive`，跳过凭据文件扫描。
- 客户端打包 tar.gz 上传发布。三条硬性大小限制，任一超限即被客户端拒绝、无法发布：单个 `.html` 文件 ≤ 10MB、打包后 tar.gz ≤ 20MB、未压缩候选文件总量 ≤ 200MB。

## 示例

```bash
lark-cli apps +create --name "Demo" --app-type html
lark-cli apps +html-publish --app-id app_xxx --path ./dist
lark-cli apps +html-publish --app-id app_xxx --path ./index.html --dry-run
```

## 输出契约

命令内部完成 tar.gz 打包 → TOS 上传 → 触发发布，返回 `data.release_id`。拿到 `release_id` 后用 `+release-get --app-id <app_id> --release-id <release_id>` 轮询发布状态直到 `finished`，从中读取 `online_url`。

- 业务失败如构建失败、应用不存在通常带 `error.hint`；优先转述 hint。网络/服务端失败则建议稍后重试。

## 链接边界

- 发布态访问链接以 `+release-get` 轮询 `finished` 返回的 `online_url` 为准。
- 重新发布前，`+list` 的 `is_published=true` 只能说明历史上发布过，不代表当前本地产物已经部署。

## 发布前置门（第一步，先于任何其他动作）

收到发布意图后，第一个动作是量三个尺寸，不是读文件内容、不是打包：
1. 单个 `.html` ≤ 10MB / tar.gz ≤ 20MB / 未压缩总量 ≤ 200MB。
2. 任一超限 → 立即 STOP，把超限数字转述给用户，交还决定权。
3. 三项都通过 → 才进入下面的命令骨架。

## 预览与发布边界

- 用户只说“用 HTML 写个 PPT/页面给我看看”时，先生成本地文件或目录，返回路径并问是否发布到妙搭分享；不要默认创建应用或部署。
- 用户明确说“部署出去/发链接/可分享”时，才创建 `html` 应用并用 `+html-publish`。
- 用户要发布但没有 app_id 时，先 `+create --app-type html` 创建应用；应用名可从页面/站点主题生成，不要让用户手动提供 app_id。
- 若产物首页不是 `index.html`，发布前改名或复制为 `index.html`；目录发布时只传干净产物目录，例如 `./dist`。`.git` 目录会被自动排除，不会进入压缩包。
- 重新部署同一个 HTML 应用时复用原 `app_id`，只重新执行 `+html-publish --app-id <id> --path <dir-or-index.html>`。

## 安全规则

默认会拦截 `.env`、`.npmrc`、`.aws/credentials` 等凭据文件。只有用户明确要发布凭据示例文件或教程内容时，才追加 `--allow-sensitive`；追加前先说明将包含哪些敏感候选文件。

## 常见失败

- `--path` 传了绝对路径：`--path` 只接受相对路径，传绝对路径会报 `--path must be a relative path within the current directory`。改用 `cd` + 相对路径，例如 `cd /target/dir && lark-cli apps +html-publish --path .`。
- 缺少 `index.html`：目录根放置 `index.html`，或单文件路径直接指向名为 `index.html` 的文件。
