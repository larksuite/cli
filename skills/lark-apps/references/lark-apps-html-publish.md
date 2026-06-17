# apps +html-publish

把本地 HTML 文件或静态目录发布为妙搭应用访问 URL。运行时命令事实以 `lark-cli apps +html-publish --help` 为准。

## 何时用

用于把已经存在的本地 HTML 文件或静态产物目录发布成妙搭访问 URL。它不负责生成 HTML 内容，也不负责全栈应用代码发布。

## 命令骨架

- 必填：`--app-id`、`--path`。
- `--path` 可以是单个文件或目录；入口必须是 `index.html`。
- 可选：`--allow-sensitive`，跳过凭据文件扫描。
- 客户端会打包 tar.gz 并上传发布；压缩包上限当前为 20MB，未压缩候选文件总量也有保护上限。

## 示例

```bash
lark-cli apps +create --name "Demo" --app-type html
lark-cli apps +html-publish --app-id app_xxx --path ./dist
lark-cli apps +html-publish --app-id app_xxx --path ./index.html --dry-run
```

## 输出契约

- 成功默认 JSON envelope 只关心 `data.url`；这是本轮 HTML 发布后的发布态访问链接。
- pretty 输出为 `url: <url>`，适合人看；自动化取字段用 JSON 或 `--jq '.data.url'`。
- 业务失败如构建失败、应用不存在通常带 `error.hint`；优先转述 hint。网络/服务端失败则建议稍后重试。

## 链接边界

- 开发态链接可由 `app_id` 拼出：`https://miaoda.feishu.cn/app/{app_id}`，用于进入妙搭编辑/开发态。
- 发布态访问链接以本命令成功返回的 `data.url` 为准。
- 重新发布前，`+list` 的 `is_published=true` 只能说明历史上发布过，不代表当前本地产物已经部署。

## 预览与发布边界

- 用户只说“用 HTML 写个 PPT/页面给我看看”时，先生成本地文件或目录，返回路径并问是否发布到妙搭分享；不要默认创建应用或部署。
- 用户明确说“部署出去/发链接/可分享”时，才创建 `html` 应用并用 `+html-publish`。
- 用户要发布但没有 app_id 时，先 `+create --app-type html` 创建应用；应用名可从页面/站点主题生成，不要让用户手动提供 app_id。
- 若产物首页不是 `index.html`，发布前改名或复制为 `index.html`；目录发布时只传干净产物目录，例如 `./dist`。`.git` 目录会被自动排除，不会进入压缩包；`node_modules`、源码缓存等仍建议手动精简以控制包体。
- 重新部署同一个 HTML 应用时复用原 `app_id`，只重新执行 `+html-publish --app-id <id> --path <dir-or-index.html>`。

## 安全规则

默认会拦截 `.env`、`.npmrc`、`.aws/credentials` 等凭据文件。只有用户明确要发布凭据示例文件或教程内容时，才追加 `--allow-sensitive`；追加前先说明将包含哪些敏感候选文件。

## 常见失败

- 缺少 `index.html`：目录根放置 `index.html`，或单文件路径直接指向名为 `index.html` 的文件。
- 包体过大：让用户精简 `--path`，不要把源码、依赖目录、构建缓存一起发布。
