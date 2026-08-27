# apps +app-dev-publish

把本地 Web 应用项目一键构建并发布到它的妙搭应用（产物托管形态）。运行时命令事实以 `lark-cli apps +app-dev-publish --help` 为准。

## 何时用

用 `+app-dev-init-template` 初始化（或按产物协议改造）的本地项目要部署/更新到妙搭时使用。它不适用于 html 应用（走 `+html-publish`）或源码托管应用（走 `+release-create`）。

## 命令骨架

- **必须在项目根目录执行**（项目根须有 `miaoda.json`；旧项目回退读 `.spark/meta.json`）。产物目录取 miaoda.json 的 `build.output`（缺省 `dist`），无 `--path` 参数。
- `--app-id` 可选：首次发布传它指定目标（成功后自动写入 `miaoda.json` 的 app 段，后续免传）；已记录 app id 时可省略；**两者都有且不一致会被拒绝**（防误发错目标），确要切换先更新 miaoda.json。
- 可选：`--skip-build`（跳过 `npm run build`，直接发布已有 `./dist`）、`--allow-sensitive`（跳过凭据文件扫描）。
- 内部流程：读 miaoda.json → `pre_release` 获取上传地址与 `MIAODA_*` 构建环境变量 → 执行 `build.command`（缺省 `npm run build`，argv 直接执行不走 shell，自动注入变量）→ 校验产物协议 → zip 上传 → 触发发布。
- 产物协议（详见《妙搭产物托管协议规范》）：`output/` 必须含 ≥1 个 `.html`（SPA 入口须名 `index.html`）与合法的 `routes.json`（`{"version":1,"type":"<stack>","fallback":"index.html"}`）；`output_resource/`、`output_capabilities/` 可选；顶层不允许其他条目。包体限制：zip ≤ 50MB、未压缩总量 ≤ 200MB。

## 示例

```bash
lark-cli apps +app-dev-publish --app-id app_xxx     # 首次发布：指定目标，成功后写入 meta.json
lark-cli apps +app-dev-publish                      # 迭代重发：读 meta.json，零参数
lark-cli apps +app-dev-publish --skip-build
lark-cli apps +app-dev-publish --dry-run
```

## 输出契约

- 同步完成：`data.online_url` 直接可访问，同时随 app 段回写进 `miaoda.json`。
- 异步发布：返回 `data.release_id` 和 `data.poll_hint`；用 `+release-get --app-id <app_id> --release-id <release_id>` 轮询到 `finished` 后读取 `online_url`。
- 业务失败通常带 `error.hint`，优先转述 hint；网络/服务端 5xx 失败带 `retryable`，可稍后重试。

## 前置引导

- 未记录 app id 时：先 `lark-cli apps +create --name <name>` 创建应用，然后 `lark-cli apps +app-dev-publish --app-id <返回的 app_id>` 发布（成功后自动写入 miaoda.json，无需手工编辑文件）；应用名可从项目主题生成，不要让用户手动提供 app_id。
- **记录的 app id 不是本会话写入的**（来自历史文件或他人仓库）时，发布前先把目标 app id 告知用户并确认——发布会覆盖该应用的线上内容。

## 安全规则

- 敏感文件扫描命中（`.env`、`.npmrc` 等）时，**不要自动加 `--allow-sensitive` 重试**；把命中的文件列表转述给用户，由用户决定移除还是明确豁免。
- 构建环境变量只注入 `pre_release` 下发的 `MIAODA_*` 白名单键；命令会在 stderr 回显实际注入的键名。

## 常见失败

- `current directory is not a Miaoda app project`：不在项目根执行；`cd` 到含 `miaoda.json` 的目录。
- `output/routes.json is missing` / schema 校验失败：模板构建脚本负责生成合法 routes.json；让用户检查构建配置，不要手工伪造。
- `build command ... failed`：转述 stderr 摘要让用户修构建错误（构建命令来自 miaoda.json `build.command`）；用户已手动构建时可用 `--skip-build`。
- `--skip-build is set but ./dist does not exist`：先构建或去掉 `--skip-build`。
