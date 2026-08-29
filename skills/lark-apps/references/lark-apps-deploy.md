# apps +deploy

把本地 Web 应用项目一键构建并发布到它的妙搭应用（产物托管形态）。运行时命令事实以 `lark-cli apps +deploy --help` 为准。

## 何时用

用 `+init-template` 初始化（或按产物协议改造）的本地项目要部署/更新到妙搭时使用。它不适用于 html 应用（走 `+html-publish`）或源码托管应用（走 `+release-create`）。

## 命令骨架

- **必须在项目根目录执行**（项目根须有 `spark.json`，它是唯一的项目声明文件）。同源产物目录取 spark.json 的 `build.output`（缺省 `dist/output`），CDN 产物目录取可选的 `build.output_cdn`（不声明 = 无 CDN 分离），无 `--path` 参数。
- `--app-id` 可选：首次发布传它指定目标（成功后自动写入 `spark.json` 的 app 段，后续免传）；已记录 app id 时可省略；**两者都有且不一致会被拒绝**（防误发错目标），确要切换先更新 spark.json。
- 可选：`--skip-build`（跳过 `build.command`，直接发布已有产物目录）、`--allow-sensitive`（跳过凭据文件扫描）。
- 内部流程：读 spark.json → `pre_release` 获取上传地址与 `MIAODA_*` 构建环境变量 → 执行 `build.command`（argv 直接执行不走 shell，自动注入变量；**spark.json 未声明 build.command = buildless，跳过构建直接打包**）→ 校验产物协议 → 归一化打包（`build.output` → zip 内 `output/`，`build.output_cdn` → zip 内 `output_resource/`，流水线不感知项目目录名）→ 上传 → 触发发布。
- 产物协议（详见《妙搭产物托管协议规范》）：`build.output` 目录必须含 ≥1 个 `.html`（SPA 入口须名 `index.html`）与合法的 `routes.json`（**路由枚举数组**，如 `[{"path":"/","file":"index.html"}]`，纯静态站可为空数组；它是安全扫描的输入，必须与真实路由一致）；目录内其余静态文件全部随包上传。**buildless 项目缺 routes.json 时由 CLI 扫描 `.html` 文件树自动生成**（`foo/index.html` → `/foo`），工程自带的 routes.json 永不被覆盖。包体限制：zip ≤ 50MB、未压缩总量 ≤ 200MB。

## 示例

```bash
lark-cli apps +deploy --app-id app_xxx     # 首次发布：指定目标，成功后写入 spark.json
lark-cli apps +deploy                      # 迭代重发：读 spark.json，零参数
lark-cli apps +deploy --skip-build
lark-cli apps +deploy --dry-run
```

## 输出契约

- 发布单受理后**命令立即返回，不原地等待**（agent 运行时不允许长前台等待，轮询由调用方负责）：发布中时返回 `data.release_id` 和 `data.poll_hint`，用 `+release-get --app-id <app_id> --release-id <release_id>` 轮询到 `finished` 后读取 `online_url`（轮询间隔 ≥3s）。
- **在项目根轮询**：`+release-get` 观察到 `finished` 且当前目录 spark.json 记录的正是该 app 时，会自动把 `online_url` 回写进 app 段（`app.online_url`）（无 spark.json 或 id 不匹配时静默跳过）——所以轮询尽量在项目根执行，让状态区保持最新。
- 同步完成（受理响应即 `finished`）时直接返回 `data.online_url` 并随 app 段回写进 `spark.json`。
- **流水线失败 = 发布失败**：exit 非 0，message 含各 step 的 error_logs 摘要，hint 给出复查命令；产物已上传，修复后重新 publish 即可。
- 业务失败通常带 `error.hint`，优先转述 hint；网络/服务端 5xx 失败带 `retryable`，可稍后重试。

## 前置引导

- 未记录 app id 时：先 `lark-cli apps +create --name <name>` 创建应用，然后 `lark-cli apps +deploy --app-id <返回的 app_id>` 发布（成功后自动写入 spark.json，无需手工编辑文件）；应用名可从项目主题生成，不要让用户手动提供 app_id。
- **记录的 app id 不是本会话写入的**（来自历史文件或他人仓库）时，发布前先把目标 app id 告知用户并确认——发布会覆盖该应用的线上内容。

## 安全规则

- 敏感文件扫描命中（`.env`、`.npmrc` 等）时，**不要自动加 `--allow-sensitive` 重试**；把命中的文件列表转述给用户，由用户决定移除还是明确豁免。
- 构建环境变量只注入 `pre_release` 下发的 `MIAODA_*` 白名单键；命令会在 stderr 回显实际注入的键名。

## 常见失败

- `current directory is not a Miaoda app project`：不在项目根执行；`cd` 到含 `spark.json` 的目录。
- `spark.json is missing the required stack field` / `stack ... must end with -webapp or -fullstack`：声明技术栈——官方模板自动写入；自定义项目填 `custom-webapp` / `custom-fullstack`。
- `spark.json is missing the required dev.port field`：声明本地 dev 端口（如 `{"dev":{"port":5173}}`）——托管后平台能力依托本地自描述端点（`GET localhost:<dev.port>/spark.json`），必填。
- `warning: no index.html ...`：不拦截但强烈建议修复——平台 SPA fallback 依赖入口 index.html，缺失时线上路由回退会异常。
- `routes.json is missing` / schema 校验失败：声明了 `build.command` 的项目由构建脚本负责生成合法 routes.json；让用户检查构建配置，不要手工伪造（buildless 项目无此问题，CLI 会自动生成）。
- `build command ... failed`：转述 stderr 摘要让用户修构建错误（构建命令来自 spark.json `build.command`）；用户已手动构建时可用 `--skip-build`。
- `artifact directory ... does not exist`：声明了构建命令时先构建（或去掉 `--skip-build`）；buildless 项目需确认 `build.output` 指向的目录真实存在。
