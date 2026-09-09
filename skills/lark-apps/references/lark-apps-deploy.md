# apps +deploy

把本地产物一键发布到它的妙搭应用（产物托管形态）。两种形态，按手上有什么选：

| 手上有什么 | 用哪种形态 |
|---|---|
| 含 `spark.json` 的本地项目（`+init-template` 初始化，或按产物协议改造过） | **项目模式**：`lark-cli apps +deploy`（构建 + 发布） |
| 一个 `.html` 文件，或一个装着静态站点的目录，没有 `spark.json` | **裸 HTML 发布**：`lark-cli apps +deploy --file-path ./report.html` / `--dir ./site`（不构建、不建仓库、不落任何本地文件） |

运行时命令事实以 `lark-cli apps +deploy --help` 为准。源码托管应用走 `+release-create`；`+html-publish` 只服务无 Git 管理的存量旧链路，新产物不要用。

## 项目模式（spark.json）

### 命令骨架

- **必须在项目根目录执行**（项目根须有 `spark.json`，它是唯一的项目声明文件）。同源产物目录取 spark.json 的 `build.output`（缺省 `dist/output`），CDN 产物目录取可选的 `build.output_cdn`（不声明 = 无 CDN 分离），无 `--path` 参数。
- `--app-id` 可选：首次发布传它指定目标（成功后自动写入 `spark.json` 的 app 段，后续免传）；已记录 app id 时可省略；**两者都有且不一致会被拒绝**（防误发错目标），确要切换先更新 spark.json。
- **发布前须启动本地 dev server**：`+deploy` 会验证 `GET localhost:<dev.port>/spark.json` 可达（localhost 双栈解析，dev server 绑 `127.0.0.1` 或 `::1` 均可）且其 `app.id` 与本次部署目标一致（防止把 A 项目的产物发到 B 应用；端点尚未声明 app.id 时放行——首发项目的正常状态）。无头/CI 环境用 `--no-verify` 显式跳过该验证。
- 可选：`--skip-build`（跳过 `build.command`，直接发布已有产物目录）、`--no-verify`（整体跳过本地 dev server 验证：dev.port 声明要求、端点可达性、app 身份比对）。
- 内部流程：读 spark.json → `pre_release` 获取上传地址与 `MIAODA_*` 构建环境变量 → 执行 `build.command`（argv 直接执行不走 shell，自动注入变量；**spark.json 未声明 build.command = buildless，跳过构建直接打包**）→ 校验产物协议 → 归一化打包（`build.output` → zip 内 `output/`，`build.output_cdn` → zip 内 `output_resource/`，流水线不感知项目目录名）→ 上传 → 触发发布。
- 产物协议：`build.output` 目录必须含 ≥1 个 `.html`（SPA 入口须名 `index.html`）与合法的 `routes.json`（**路由枚举数组**，如 `[{"path":"/","file":"index.html"}]`，纯静态站可为空数组；必须与应用真实路由一致）；目录内其余静态文件全部随包上传。**buildless 项目缺 routes.json 时由 CLI 扫描 `.html` 文件树自动生成**（`foo/index.html` → `/foo`），工程自带的 routes.json 永不被覆盖。包体限制：zip ≤ 50MB、未压缩总量 ≤ 200MB。

### 示例

```bash
lark-cli apps +deploy --app-id app_xxx     # 首次发布：指定目标，成功后写入 spark.json
lark-cli apps +deploy                      # 迭代重发：读 spark.json，零参数
lark-cli apps +deploy --skip-build
lark-cli apps +deploy --dry-run
```

### 前置引导

- 未记录 app id 时：先 `lark-cli apps +create --name <name>` 创建应用，然后 `lark-cli apps +deploy --app-id <返回的 app_id>` 发布（成功后自动写入 spark.json，无需手工编辑文件）；应用名可从项目主题生成，不要让用户手动提供 app_id。
- **记录的 app id 不是本会话写入的**（来自历史文件或他人仓库）时，发布前先把目标 app id 告知用户并确认——发布会覆盖该应用的线上内容。

### 安全规则

- 构建环境变量只注入 `pre_release` 下发的 `MIAODA_*` 白名单键；命令会在 stderr 回显实际注入的键名。

### 常见失败

- `current directory is not a Miaoda app project`：不在项目根执行；`cd` 到含 `spark.json` 的目录。
- `spark.json is missing the required dev.port field`：声明本地 dev 端口（如 `{"dev":{"port":5173}}`）——托管后平台能力依托本地自描述端点（`GET localhost:<dev.port>/spark.json`），必填。
- `the local self-description endpoint is unavailable`：先启动 dev server（官方模板已内置 /spark.json 端点；custom 项目须自己伺服项目根 spark.json）；无头/CI 环境用 `--no-verify`——**不要因为端点验证失败就自动加 `--no-verify` 重试**，先确认是环境问题而非发错目录。
- `the dev server ... declares app X, but this deploy targets app Y`：**大概率发错目录**——停下核对当前目录与正在运行的 dev server 是否同一项目，把情况告知用户，不要用 `--no-verify` 绕过。
- `warning: no index.html ...`：不拦截但强烈建议修复——平台 SPA fallback 依赖入口 index.html，缺失时线上路由回退会异常。
- `routes.json is missing` / schema 校验失败：声明了 `build.command` 的项目由构建脚本负责生成合法 routes.json；让用户检查构建配置，不要手工伪造（buildless 项目无此问题，CLI 会自动生成）。
- `build command ... failed`：转述 stderr 摘要让用户修构建错误（构建命令来自 spark.json `build.command`）；用户已手动构建时可用 `--skip-build`。
- `artifact directory ... does not exist`：声明了构建命令时先构建（或去掉 `--skip-build`）；buildless 项目需确认 `build.output` 指向的目录真实存在。

## 裸 HTML 发布（`--file-path` / `--dir`）

不需要 `spark.json`，不构建，不回写任何本地文件：把指定的文件或目录**原样**打包发布成一个 `app_type=html` 的妙搭应用，拿到可分享链接。

### 命令骨架

- 单文件：`lark-cli apps +deploy --file-path ./report.html`。该文件即入口，产物里恒为 `index.html`，路由恒为 `[{"path":"/","file":"index.html"}]`。
- 目录：`lark-cli apps +deploy --dir ./site`。目录下所有普通文件随包上传（跳过 `.git` 子树，不跟随符号链接），入口默认取目录**根**的 `index.html`。
- 指定入口：`lark-cli apps +deploy --dir ./site --entry-file page.html`。
- 非入口的 `.html` 照常发布并自动生成路由（`about.html` → `/about`，`docs/index.html` → `/docs`），不需要自备 routes.json。
- 路径参数只接受**相对当前目录的相对路径**，传绝对路径会被拒；产物不在 cwd 下时先 `cd` 过去（`cd /path/to/site && lark-cli apps +deploy --dir .`）。
- 互斥关系：`--file-path` 与 `--dir` 二选一；`--entry-file` 只在 `--dir` 下有意义；`--skip-build` / `--no-verify` 是项目模式专属，传了直接报参数错。

### `--dir` 的入口判定

`--entry-file` **只接受 `--dir` 的直接子文件名**：不能含 `/` 或 `\`，必须是 `.html`。入口在子目录里时，把 `--dir` 直接指到那一层，不要在 `--entry-file` 里写路径。

| 给了 `--entry-file` | 目录根有 `index.html` | 结果 |
|---|---|---|
| 否 | 是 | `index.html` 即入口，直接发布 |
| 否 | 否 | 报错 `no entry file`：加 `--entry-file`，或把入口改名为 `index.html` |
| 是 | 否 | 发布，该文件在产物里被改名为 `index.html` |
| 是 | 是 | 报错 `entry conflict`：去掉 `--entry-file`，或把 `index.html` 移出 `--dir` |

改名只发生在上传的产物里，**磁盘上的原始文件不受影响**——不要发布后去本地找 `index.html`，也不要为了迎合它先在本地改名。

### 复发布同一份产物

- **带上一次返回的 `--app-id` 是常态主路径**：`lark-cli apps +deploy --dir ./site --app-id app_xxx` 跳过幂等查询，直接更新同一个应用。首次发布返回的 `app_id` 必须记住并复用。
- 不带 `--app-id` 时，**入口文件的绝对路径**（按当前用户隔离）就是幂等标识：同一路径复发布落回同一个应用；文件改名、目录搬家、或换个 cwd 导致绝对路径变了，会**新建一个应用**。要继续更新已有应用就带 `--app-id`。
- 查不到已有应用时自动创建：应用名取入口文件名去扩展名（`report.html` → `report`）；入口是 `index.html` 时取父目录名；都取不到用 `html-app`。要自定义名字就先 `lark-cli apps +create --name <name> --app-type html`，再用返回的 app_id 发布。
- 发布时 stderr 回显 `publishing to app <app_id> (from <来源>)`，来源为 `--app-id` / `has_html_app_created` / `+create`——据此确认这次发到了哪个应用。发布会覆盖该应用线上内容，`app_id` 不是本会话拿到的（来自历史记录或他人）时先与用户确认。

### 凭证扫描与体积上限

- 默认扫描凭证类文件，命中即拒绝发布并列出命中项，大小写不敏感。`--dry-run` 命中同样非零退出，不能用它绕过。两类匹配：
  - **按文件名**：`.env` 及 `.env.*`、`.npmrc`、`.netrc`、`.pypirc`、`.git-credentials`、`id_rsa` 等私钥、`*.pem` / `*.p12` / `*.pfx` / `*.keystore`、`credentials`、`service-account.json`
  - **按父目录锚定**（这些文件名太通用，只看文件名会漏）：`.aws/credentials`、`.aws/config`、`.docker/config.json`、`.kube/config`，以及 `.ssh/` 与 `.gnupg/` 目录下的任何文件
- 命中后的默认动作是**把这些文件移出发布范围**（删掉、或把 `--dir` 收窄到只含站点的那一层）。只有确认它们本就是要公开的页面内容时才加 `--allow-sensitive`：它跳过整道扫描并在 stderr 列出被放行的文件，而产物发布后是公网可分享链接，误放行等于把凭证发到公网。
- 体积上限：单个 `.html` ≤ 20 MiB、打包前原始总量 ≤ 200 MiB、打包后 zip ≤ 50 MiB。超限直接失败并给出当前体积，只能收窄产物范围，没有放开的参数。

### 示例

```bash
lark-cli apps +deploy --file-path ./report.html                    # 首发单文件，自动建应用
lark-cli apps +deploy --file-path ./report.html --app-id app_xxx   # 复发布到同一应用
lark-cli apps +deploy --dir ./site                                 # 目录，入口为 ./site/index.html
lark-cli apps +deploy --dir ./site --entry-file page.html          # 目录里没有 index.html 时指定入口
lark-cli apps +deploy --dir ./site --dry-run                       # 只看计划，不发起任何写请求
```

`--dry-run` 会原样打出实际外发的 `idempotent_key`（本机绝对路径，含用户名与目录结构）；需要判断这个外发是否可接受时先跑它。

### 常见失败

| 报错关键字 | 处理 |
|---|---|
| `mutually exclusive` | `--file-path` 与 `--dir` 同时给了；发单文件用前者，发目录用后者 |
| `--entry-file only applies together with --dir` | 单文件模式下不要传 `--entry-file` |
| `only applies to the spark.json project mode` | `--skip-build` / `--no-verify` 属于项目模式，裸 HTML 发布下去掉 |
| `must point at an .html file` / `must be an .html file` | `--file-path` 与 `--entry-file` 都只接受 `.html` |
| `must be a file name directly under --dir, not a path` | `--entry-file` 不接受路径；把 `--dir` 指到入口所在的那一层 |
| `not found directly under --dir` | 入口文件名拼错，或它其实在子目录里 |
| `no entry file` / `entry conflict` | 见上方入口判定表，不要靠改本地文件名试错 |
| `credential file(s) that should not be published` | 先把凭证文件移出发布范围；确认是公开页面内容才加 `--allow-sensitive` |
| `exceeds the ... limit` / `packed zip is ... bytes` | 体积超限，收窄 `--dir` 或删减产物 |
| 路径不存在 / 越出 cwd | 路径只接受 cwd 下的相对路径，先 `cd` 到产物所在目录 |

## 输出契约（两种形态共用）

- 发布单受理后**命令立即返回，不原地等待**（agent 运行时不允许长前台等待，轮询由调用方负责）：发布中时返回 `data.release_id` 和 `data.poll_hint`，用 `+release-get --app-id <app_id> --release-id <release_id>` 轮询到 `finished` 后读取 `online_url`（轮询间隔 ≥3s）。
- **在项目根轮询**：`+release-get` 观察到 `finished` 且当前目录 spark.json 记录的正是该 app 时，会自动把 `online_url` 回写进 app 段（`app.online_url`）（无 spark.json 或 id 不匹配时静默跳过）——所以轮询尽量在项目根执行，让状态区保持最新。
- 同步完成（受理响应即 `finished`）时直接返回 `data.online_url` 并随 app 段回写进 `spark.json`。
- 裸 HTML 发布不读写 `spark.json`：`data.built` 恒为 `false`，`online_url` 不回写任何本地文件，`poll_hint` 在任意目录都可直接执行。
- **流水线失败 = 发布失败**：exit 非 0，message 含各 step 的 error_logs 摘要，hint 给出复查命令；产物已上传，修复后重新 publish 即可。
- 业务失败通常带 `error.hint`，优先转述 hint；网络/服务端 5xx 失败带 `retryable`，可稍后重试。
