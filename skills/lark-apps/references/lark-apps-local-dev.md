# lark-apps 本地开发

适用：用户要把妙搭应用（full_stack、frontend 或 html）源码拉到本地，用本地 code agent/IDE 开发、再发布。其中调试数据库仅 full_stack 适用（frontend / html 无数据库）。

## 新建 vs 已有应用

新建还是修改已有，由上方入口（SKILL.md「选择开发路径」）判定；进到本地流程后按分支走：

- **新建**：从 `+create` 开始走下面的端到端流程。
- **已有应用，本地还没有源码**：跳过 `+create`，先按下方「存量应用入口」拿 `app_id`，再 `+init`（或 `+git-credential-init` + `git clone`）把它拉到本地，然后照常开发。
- **已有应用，本地已经有项目目录**（用户自己 clone 的、或上次会话留下的）：不要 `+create`、也不要另起新目录，先做下方「接管已有仓库前先核对事实」，再按「改完代码后部署上线」继续。

## 端到端流程（新建应用）

### full_stack

`+create(full_stack)` -> `+init`（或手动 `+git-credential-init` + `git clone`）-> 读仓库 Skill -> `npm install && npm run dev` -> 按需 `+db-*` 调库 -> 非自动化改动按本页 commit/push/release；包含自动化 handler 时，在任何 release 前转到 [automation SOP](lark-apps-automation.md)，由它接管状态门禁和完整发布。

```bash
# 新建 full_stack 应用
lark-cli apps +create --as user --name "审批系统" --app-type full_stack \
  --description "支持登录、提交申请、多级审批、状态查询"

# 初始化本地仓库（--dir 取值见下方「领域规则」，勿照抄此处示例值）
lark-cli apps +init --as user --app-id app_xxx --dir ./approval-app

# 进入仓库后按项目脚手架启动
cd ./approval-app
npm install
npm run dev

# 开发完成后：提交本次改动 -> git push origin sprint/default -> +release-create。
# +release-create 部署的是远端 sprint/default 上已 push 的代码，不是本地工作区——没 commit + push 的改动不会进入发布。
git add <本次开发的文件>          # 提交粒度见下方「改完代码后部署上线」
git commit -m "feat: ..."
git push origin sprint/default
lark-cli apps +release-create --as user --app-id app_xxx --branch sprint/default
```

### frontend

纯前端应用（vite-react，无数据库）。流程与 full_stack 基本一致——`+init` 装依赖、`npm run dev`、commit/push/release——差别是无 `+db-*` 调库步骤。后续需要数据库/后端能力时不在本地升级，按 SKILL.md「类型升级」引导到云端会话。

```bash
# 新建 frontend 应用
lark-cli apps +create --as user --name "JSON 格式化工具" --app-type frontend \
  --description "纯前端交互工具，无需数据库"

# 初始化本地仓库（--dir 取值见下方「领域规则」，勿照抄此处示例值）
lark-cli apps +init --as user --app-id app_xxx --dir ./json-tool

# 进入仓库后按项目脚手架启动（vite-react）
cd ./json-tool
npm install
npm run dev

# 开发完成后：提交本次改动 -> git push origin sprint/default -> +release-create
git add <本次开发的文件>
git commit -m "feat: ..."
git push origin sprint/default
lark-cli apps +release-create --as user --app-id app_xxx --branch sprint/default
# 发布是异步的：用 +release-get 轮询到 status=finished 才算部署完成、拿到 online_url
lark-cli apps +release-get --as user --app-id app_xxx --release-id <上一步返回的 release_id>
```

### html

#### 首次开发（无 app，无代码）

`+create(html)` → `+init` → 加载 [`creative-design`](../creative-design/creative-design.md) skill 在 repo 根目录产出文件 → `git add .` + `git commit` → `git push origin sprint/default` → `+release-create` → `+release-get`。

```bash
lark-cli apps +create --name "活动页" --app-type html --as user

lark-cli apps +init --app-id app_xxx --dir ./my-page

cd ./my-page
# html 类型无需 npm install，+init 已跳过依赖安装
# 加载 creative-design skill，在 repo 根目录产出 HTML 及关联文件（JSX 组件、starter components 等）

git add .
git commit -m "feat: ..."
git push origin sprint/default
lark-cli apps +release-create --app-id app_xxx
```

#### 已有 app，二次开发/迭代

`+init`（拉取远程代码）→ 加载 creative-design skill 在 repo 根目录迭代 → `git add .` + `git commit` → `git push origin sprint/default` → `+release-create` → `+release-get`。

#### creative-design 已提前生成文件，需要 init 后迁入

`+create(html)` → `+init` → 先 `ls` 查看 repo 根目录模板结构（创意模式模板无 `src/` 目录，文件直接放根目录）→ 将已生成的所有产出文件（HTML、JSX 组件、starter components 等）拷贝到 repo 根目录 → `git add .` + `git commit` → `git push origin sprint/default` → `+release-create` → `+release-get`。

`+init` 是推荐便捷入口；想逐步手动控制时，先 `+git-credential-init` 拿 `repository_url`，再用原生 `git clone` / `git checkout sprint/default`。

**`+init` 完成后必须执行**（前提是已按下方「`+init` 耗时、超时与成功门禁」确认成功）：先 `ls <project-path>/.agents/skills/` 看有哪些项目 guide；`coding-guide/SKILL.md` 是项目编码规范，写任何代码前先读；`cat <project-path>/.agents/skills/plugin-guide/SKILL.md` 读取仓库插件指引，该文件包含插件目录、实例配置规则和调用代码生成方式——不读就无法正确集成插件能力。其余 guide 按任务读取。目录或文件不存在则跳过（html 与部分 frontend 脚手架不带这套 guide）。

## `+init` 耗时、超时与成功门禁

`+init` 是长耗时命令，内部依次做：签发 Git 凭证 → `git clone` → 切到 `sprint/default` → 生成项目代码（含拉取模板与安装依赖）→ 提交并推送 → 拉取本地环境变量。"生成项目代码"占绝大部分时间且完全依赖网络；命令没有内部超时，耗时上限由网络决定。

实测参考（macOS、公司内网、npm 缓存已预热）：full_stack 新建约 40-50 秒，frontend 约 20 秒，html 约 10 秒，已有 full_stack 应用全新 clone 约 50 秒；冷 npm 缓存（需下载约 160 MB 模板与依赖）实测约 100 秒，外网或弱网环境会再明显拉长。下文「执行方式」要求的 10 分钟，是按冷缓存耗时留出数倍余量的保守值。

### 执行方式

- `+init` 单独一次调用，不与其他命令串在同一条 shell 里；给它的工具超时至少 **10 分钟**（html 至少 5 分钟）。
- 工具超时上限达不到 10 分钟时，改为后台执行并把 stdout/stderr 重定向到文件，然后在**同一轮里主动轮询**：每 15-30 秒 `sleep` 后检查进程是否退出、输出文件里有没有 envelope，直到拿到结果再继续。不要把等待交给宿主的"后台任务完成通知"然后结束回合——无头运行或没有这种机制的 agent 不会再被唤醒；会话结束后那个初始化进程既可能被连带终止，也可能变成无人看管的孤儿进程继续往目录写文件并提交推送（见「中断后的处置顺序」），两种结果都拿不到 envelope。进程仍在运行就继续等；"生成项目代码"阶段不会打印细粒度进度，stderr 长时间没有新行不等于卡死。
- 同一目录、同一 app 同时只允许一次 `+init`。上一次被中断后不要立刻重跑，先按下方「中断后的处置顺序」确认没有残留进程。
- stderr 的 `→` 进度行是正常输出，结果只看 stdout 的 JSON envelope。

### 成功判定

只有同时满足以下三点才算成功：退出码为 0；stdout 是 `ok: true` 的 JSON envelope；`data.scaffold` 为 `init`（新建空仓库）、`upgrade`（仓库已有代码）或 `already_initialized`。新建应用时 `committed` 与 `pushed` 应同为 `true`。

没有拿到完整 envelope（超时被 kill、只看到进度行、进程被中断）一律按**未完成**处理，不是"可能成功了"。

### 成功前禁止

门禁未通过时，不要：写业务代码或手工创建脚手架文件；执行 `npm install` / `npm run dev`；`git add` / `git commit` / `git push`；`+release-create`；改用 `+git-credential-init` + `git clone` 的手动路径替代——新建应用的仓库只有一个 seed README，手动 clone 拿不到脚手架和 `.spark/meta.json`，后续开发与发布都会失去平台契约。

### 通过后、写代码前核对

脚手架内部的依赖安装是软失败：装不上也会正常退出，`+init` 仍报成功且不会转述安装错误。所以门禁通过后仍要核对：

1. `<dir>/.spark/meta.json` 存在，且 `app_id` 与目标应用一致。
2. `git log --oneline -3` 能看到初始化提交；`git status --porcelain` 为空；`git rev-parse HEAD` 与 `git rev-parse origin/sprint/default` 一致。
3. full_stack / frontend：`node_modules/` 存在且非空，缺失则先 `npm install` 再继续；html 无此步。
4. full_stack / frontend：`.env.local` 存在。缺失或 envelope 里 `env_pulled=false` 时先执行 `lark-cli apps +env-pull --app-id <app_id> --project-path <dir>`；html 应用 `env_pull_skipped=true` 是正常的。
5. 按上文「`+init` 完成后必须执行」读取项目 guide。

### 中断后的处置顺序

工具超时、进程被 kill、只看到进度行而没有 envelope，都按这个顺序处理，先查进程再动目录：

1. `pgrep -fl <app_id>` 列出仍在运行的进程，只认命令行里是 `lark-cli`、`npm` 或 Node 脚手架的那些，排除你自己包着 app_id 的 shell。外层 shell 或启动 shim 被 kill 不代表真正的 CLI 和依赖安装停了，它们会继续运行几分钟并最终提交推送。
2. 有进程在跑：每 15-30 秒查一次，最多再等 5 分钟，期间不要碰目录。它自己退出后按「通过后、写代码前核对」逐项判定，全过即视为成功，不必重跑。
3. 超过 5 分钟仍在跑，或核对不过需要重跑：**必须先清进程再动目录**。残留进程会继续往同一路径写文件、抢着提交推送，把新一次初始化污染成半成品，所以 `pgrep` 不为空时不允许删目录或重跑：

```bash
# 对步骤 1 认定的每个 pid：先杀它拉起的子进程，再杀它自己
pkill -P <pid>; kill <pid>
# 必须确认输出为空，仍有残留就对残留 pid 重复上一行
pgrep -fl <app_id>
```

4. 再看目录：没有 `.spark/meta.json`（最常见，脚手架未写完），或有 meta 但 `git status` 不干净、`git log` 没有初始化提交，都算半成品，删除**本次 `+init` 新建的**目录后重跑。只删本次新建的目录，不动用户原有目录。
5. 重跑最多 2 次；仍失败就停止，报告 app_id、`--dir`、退出码、`error.hint` 和目录残留状态。

### 失败与中断的处置

| 现象 | 判定 | 动作 |
|---|---|---|
| 工具超时 / 进程被 kill / 没有 envelope；或重跑报 `--dir` 已存在且非空 | 未完成，目录里是上次的残留 | 按上面「中断后的处置顺序」执行：先查进程、等待或清理，再判定目录、删除本次新建的目录后重跑。不要把代码写进这个目录。 |
| 本轮刚建的目录却返回 `scaffold=already_initialized` | 疑似半成品被短路 | 按「通过后、写代码前核对」逐项核对，有一项不过就删目录重跑。 |
| 退出非 0，错误含 `git push failed` | 脚手架已提交、未推送 | 不要重跑 `+init`（会被短路）。先看 `error.message` 里的 git 输出：non-fast-forward（远端有新提交）→ `git pull --rebase origin sprint/default` 后 `git push origin sprint/default`；认证失败 → 先 `lark-cli apps +git-credential-init --app-id <app_id> --as user` 再 push。然后按核对清单继续。 |
| 退出 0 但 `env_pulled=false` 且有 `env_pull_error` | 初始化成功、环境变量未拉到 | 不阻塞开发；启动前执行 `+env-pull`。 |
| `failed_precondition`：`git` 或 `npx`（随 Node.js 安装）不在 PATH | 环境缺失 | 安装后重跑原命令，不改走其他路径。 |
| 未登录、缺 scope（`missing_scope`）、凭证签发失败 | 认证问题 | 按 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 处理后重跑原命令。`+init` 是 write 风险、没有确认门禁，不会返回 exit 10。 |
| `--dir` 报 `.spark/meta.json` 缺 `app_id` 或不可读 | 上次 `+init` 中断的残留 | 按「中断后的处置顺序」先查进程，再删除**本次新建的**目录重跑；这也是命令 hint 给的方向。 |
| `--dir` 校验错误：软链、非目录、已属于另一个 app | 目录选择错误 | 换目录；绝不删用户目录来腾位置。 |
| 生成项目代码阶段报错（网络、镜像源、模板或依赖拉取失败） | 外部工具失败 | 检查网络后删目录重跑一次；不要自己拼脚手架。 |

任何一种失败都不要在未初始化的目录里继续写代码；停止时把 app_id、`--dir`、退出码、`error.hint` 和目录残留状态一起报告，等用户决定。

## Trigger guide 的项目边界

涉及自动化业务代码时，先查看工作区 `.agents/skills/`，读取与自动化任务匹配的 `trigger-guide`。它定义业务 handler 的实现与接入约束；Apps 触发器配置细节见 [automation SOP](lark-apps-automation.md)。

文件缺失或不能覆盖当前任务时，报告项目缺少可用的领域 guide；不要在本 lark-cli reference 中猜测安装命令、版本或包内目录。由项目维护方通过其受支持的初始化或同步流程补齐后，再继续代码闭环；`+init` 只负责准备本地项目，不能替代领域 guide。

## 改完代码后部署上线

已拉到本地、改完代码，用户说"推上去""部署""上线""发布到云端"时，按此序列。

若本次改动包含自动化 handler，在执行本节通用 commit/push/release 序列前就转到 [automation SOP](lark-apps-automation.md) 的匹配路径，由该 SOP 负责完整的状态门禁、commit/push、release 和可选 enable/test；不要先按本节发布再补 trigger 状态检查。下列通用序列只用于不含自动化 handler 的改动。

> `+release-create` 部署的是远端 `sprint/default` 上**已 push** 的代码，不是你本地工作区——未 commit / 未 push 的改动不会进入这次发布。所以发布前务必先把本次改动提交并推送。

1. 先按项目 `package.json` 里的脚本跑一遍 type:check / lint / build（脚本名以项目为准；html 应用没有构建步骤则跳过），失败先修：`+release-create` 在远端对同一份代码构建，本地过不了的代码推上去大概率也会 `failed`。然后 `git status` 看本次改动；`git add <本次相关文件>` 暂存后 `git commit` 提交。只提交本次任务相关的改动即可，无关的零散文件不必强求清空——发布门禁是「**本次相关改动已提交并推送**」，不是「工作区绝对干净」。
2. `git push origin sprint/default` 把工作分支推到云端（遇非 fast-forward：先 `git pull --rebase origin sprint/default` 解决冲突再推，绝不 force-push；遇 Git 认证失败 / 401 / 403 / credential helper 缺失 / token 过期：先执行 `lark-cli apps +git-credential-init --app-id <app_id> --as user` 刷新本地 Git 凭证，再重试原 git 命令；刷新凭证也失败时，停止并向用户报告错误，不要换路）。
3. `lark-cli apps +release-create --as user --app-id <app_id> --branch sprint/default` 发起部署上线，记下返回的 `release_id`。
4. `lark-cli apps +release-get --as user --app-id <app_id> --release-id <release_id>` 轮询：`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟；超时仍未完成时停止本轮轮询、报告 `release_id` 和当前 status。`finished` 后，若返回 `commit_id`，与你 push 的那个 `git rev-parse HEAD` 比对；不一致说明这次发布的不是你刚推的版本（常见于云端会话或其他协作者在你之后又推了提交），要如实报告，不能把 `online_url` 说成本次改动已上线；未返回 `commit_id` 时不对「线上是哪个版本」下结论。`finished` 成功时，若返回 `online_url`，可直接使用；未返回时不要编造链接。交付线上访问链接给他人前，注意 `online_url` 默认仅创建者可见，需先告知当前仅本人可见、按需用 `+access-scope-set` 放开可见范围；放开后用 `+access-scope-get` 回读确认，交付前让目标用户实际打开一次——发布成功和目标用户能打开是两个独立的验收项。无需再调 `+list`；`failed` 时若返回非空 `error_logs`，据此给出失败原因；否则只报告 `release_id` 和当前 status，不要编造原因（`+list` 仅作独立查询入口）。

用户只要求启用已有 trigger 时，转到 [automation SOP 的「仅启用已有 disabled trigger」路径](lark-apps-automation.md#仅启用已有-disabled-trigger)；不得因 enable 反向修改 handler、commit/push 或 release。

## 领域规则

- 代码读写走原生 `git`；CLI 负责凭证、初始化、发布和数据库调试。不存在 `apps +pull` / `apps +push` / `apps code +read` 这类代码读写 shortcut，不要臆造。
- 工作环境没有 `git` 时，先引导安装 Git（macOS 可用 `xcode-select --install` 或 `brew install git`；Linux 按发行版包管理器安装），安装后重试原 `+init` / git 命令；不要因此改走其他发布链路。
- `+init` 会编排 `+git-credential-init`、`git clone`、切到 `sprint/default`、运行脚手架，并在有变更时提交/推送。 它是长耗时、无内部超时的命令，成功与否只看 stdout envelope，退出 0 也不代表依赖已装好；执行方式、超时和核对见「`+init` 耗时、超时与成功门禁」。
- `+init --dir` 选目录：用户已预授权或表达"不要询问"（见 SKILL.md「预授权判定」）→ 按应用名派生 `./<app-name>` 直接传 `--dir`、不停问；否则先问用户用哪个目录再传。目标已存在/非空时回问换目录。
- `sprint/default` 是工作分支；`main` 是发布态快照，由 `+release-create` 成功后服务端 fast-forward 推进；服务端护栏禁直推 `main`、拒 force-push、要求 `sprint/default` fast-forward。
- 已拉到本地后，pull/push/diff/log 都用原生 git；云端 `sprint/default` 比本地新时，先 `git pull --rebase origin sprint/default`，解决冲突后再 push 和 publish。
- `git clone` / `git pull` / `git push` 如果报认证失败、401/403、credential helper 缺失或 token 过期，优先重新执行 `lark-cli apps +git-credential-init --app-id <app_id> --as user` 更新本地 Git 凭证，然后重试原 git 命令；刷新凭证也失败时，停止并向用户报告错误，不要换路；不要手动复制 token、不要把 token 拼进 remote URL。
- 环境变量由脚手架在本地启动时处理；需要手动刷新时用 `+env-pull`。
- full_stack / frontend 本地只用仓库的 `npm run dev` 启动（它会自动拉取本地环境变量，见 [`lark-apps-env-pull.md`](lark-apps-env-pull.md)），不要绕过它直接起子进程或直连后端端口——那样会丢掉运行时注入的用户身份，表现为未登录、接口 401 或首页 404。具体机制见项目 `.agents/skills/` 下的 coding-guide。
- 资源型文件（图片、字体、音视频等）不要直接引用本地路径，也不要提交到 git 仓库或以 base64 内联到代码中。先通过 `lark-cli apps +file-upload --app-id <app_id> --file <local_path>` 上传到应用文件存储，拿到返回的远端 URL 后在代码中引用该 URL。详情读 [`lark-apps-file.md`](lark-apps-file.md)。上传返回的链接按 app 隔离，不同应用必须各自重新上传，不能跨应用复用同一链接。
- DB 调试用 `+db-table-list` / `+db-table-get` / `+db-execute`；不要裸连数据库或自行拼连接串。改了表结构或写了数据之后，用 `+db-table-get` 或只读 SELECT 独立回读一次；页面提示成功、接口返回 200 都不能代替回读。
- DB 分 `dev` / `online`；使用 `--environment dev|online`，不要使用旧的 `--env`。只有确认应用已开启多环境时才引导 `--environment dev`；单环境应用省略 `--environment`（服务端选 online）或显式传 `--environment online`。在 dev 写入不能证明线上 handler 已验证。dev 的库结构变更要上线时，仍按应用发布链路走 `+release-create`，不要另造“数据库发布”步骤。
- 存量单库应用需要 dev/online 多环境时，用 `+db-env-create --environment dev`。这是不可逆 high-risk 操作。
- 只从 `+list` 看到 `is_published=true`，不能证明本地刚推送的代码已经部署；必须有本轮 `+release-get finished`。
- 发布 `finished` 但线上行为回退或报错时，先用 `+release-list` 记下上一次 `finished` 的 `release_id` 与 `commit_id`，连同当前现象报告用户。回退也是一次 `+release-create`，属高影响动作，未经用户确认不要自动发起。

## 存量应用入口

已有项目目录先读 `.spark/meta.json` 取 `app_id`；没有本地项目但知道应用名时用：

```bash
lark-cli apps +list --keyword "应用名"
```

拿到 `app_id` 后再 `+init` 或 `+git-credential-init`。

### 接管已有仓库前先核对事实

进入一个已存在的本地项目目录（用户自己 clone 的、或上次会话留下的）改代码之前，先把下面几项看一遍，再决定怎么动：

1. `git status --porcelain`：有用户未提交或未跟踪的文件时，先摘要给用户并问怎么处理；不要为了"干净"自动 `git stash`、`git checkout -- .` 或覆盖这些改动。
2. `git fetch origin` 后比较 `git rev-parse HEAD`、`origin/sprint/default`、`origin/main`：`origin/sprint/default` 领先本地，说明云端会话或其他协作者推过代码，先 `git log HEAD..origin/sprint/default --oneline` 看清是谁改了什么，再 `git pull --rebase origin sprint/default`；本地领先，说明有未推送的提交，发布前必须先 push；`origin/main` 是最近一次发布成功的快照，它与 `origin/sprint/default` 的差就是"已开发、未上线"的部分。
3. `lark-cli apps +release-list --app-id <app_id> --status finished --page-size 1 --as user` 取最近一次**成功**发布的 `release_id`，再 `lark-cli apps +release-get --app-id <app_id> --release-id <release_id> --as user` 读它的 `commit_id`，和 `origin/main` 对照，确认线上跑的是哪个 commit；`commit_id` 未返回时只说明「线上版本未能确认」，不要猜。
4. `.spark/meta.json` 的 `app_id` 与用户指认的应用一致；不一致说明目录属于另一个 app，停下确认，不要在里面继续。

核对出分歧（用户有改动、远端领先、release 与分支对不上）时先报告再动手。沿用原应用的 app_id、数据、访问范围和发布历史；只有用户明确要求独立 Demo、重建或对照实验时才新建应用。

## 与云端会话并存

同一个 app 可以同时被本地开发和云端会话（[`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md)）修改：云端的改动由云端自己提交到该应用的远端仓库，你的 push 也进同一个远端，而 lark-cli 不提供任何互斥或冲突提示，改动何时到达远端只能以 `git fetch` 的结果为准。同一时段只让一方写代码：

- 本地开发期间不要对同一个 app 发起 `+chat`；用户要切到云端生成时，先把本地改动 commit 并 push，再开始云端轮次。
- 云端有轮次在跑（`+session-get` 的 `is_streaming=true` 或 `latest_turn.status=running`）时不要 push；等它 `completed` 后 `git fetch origin`，用 `git log HEAD..origin/sprint/default --oneline` 看清云端改了什么，再 `git pull --rebase origin sprint/default`。
- 不确定云端有没有在改：`+session-list --app-id <app_id>` 找活跃会话，再用 `+session-get` 看状态；`git fetch` 后远端出现你没见过的提交，也一律按云端改动处理，不要覆盖。
- 云端 turn `completed` 只说明那一轮生成结束；它的改动是否已经到 `origin/sprint/default`，以 `git fetch` 后看到的远端提交为准，不要假设。发布仍按「改完代码后部署上线」走。

## 交付口径

回复用户时按实际拿到的证据说状态，不要把前一档说成后一档：

| 状态 | 证据 |
|---|---|
| 本地完成 | 目标行为在本地 `npm run dev` 下验证过（html 应用以打开本地产物页面为准），项目 type:check / build 通过 |
| 已推开发分支 | `git rev-parse HEAD` 与 `origin/sprint/default` 一致 |
| 已发布 | 本轮 `release_id` 的 `+release-get` 返回 `finished`；若它返回了 `commit_id`，等于你 push 的提交 |
| 可交付使用 | `online_url` 已实际打开验证；`+access-scope-get` 回读的可见范围覆盖目标用户 |

最终回复至少给出：app_id、本次 commit、`release_id` 与状态、`online_url`、当前可见范围、还没验证的项和已知风险。commit、push、云端 turn `completed`、页面 toast 都不能替代"已发布"或"可交付"的证据。

## 何时不用

- 用户明确要云端妙搭 Agent 生成/迭代，而不是本地写代码：读 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md)。
