# lark-apps 本地开发

适用：用户要把妙搭应用（full_stack 或 html）源码拉到本地，用本地 code agent/IDE 开发、调试数据库，再发布。

## 新建 vs 已有应用

新建还是修改已有，由上方入口（SKILL.md「选择开发路径」）判定；进到本地流程后按分支走：

- **新建**：从 `+create` 开始走下面的端到端流程。
- **已有应用**（本地还没有源码）：跳过 `+create`，先按下方「存量应用入口」拿 `app_id`，再 `+init`（或 `+git-credential-init` + `git clone`）把它拉到本地，然后照常开发。

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

### html

#### 首次开发（无 app，无代码）

`+create(html)` → `+init` → 加载 [`creative-design`](../creative-design/SKILL.md) skill 在 repo 根目录产出文件 → `git add .` + `git commit` → `git push origin sprint/default` → `+release-create` → `+release-get`。

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

**`+init` 完成后必须执行**：`cat <project-path>/.agents/skills/plugin-guide/SKILL.md`，读取仓库插件指引。该文件包含插件目录、实例配置规则和调用代码生成方式——不读就无法正确集成插件能力。文件不存在则跳过。

## Trigger guide 的项目边界

涉及自动化业务代码时，先查看工作区 `.agents/skills/`，读取与自动化任务匹配的 `trigger-guide`。它定义业务 handler 的实现与接入约束；Apps 触发器配置细节见 [automation SOP](lark-apps-automation.md)。

文件缺失或不能覆盖当前任务时，报告项目缺少可用的领域 guide；不要在本 lark-cli reference 中猜测安装命令、版本或包内目录。由项目维护方通过其受支持的初始化或同步流程补齐后，再继续代码闭环；`+init` 只负责准备本地项目，不能替代领域 guide。

## 改完代码后部署上线

已拉到本地、改完代码，用户说"推上去""部署""上线""发布到云端"时，按此序列。

若本次改动包含自动化 handler，在执行本节通用 commit/push/release 序列前就转到 [automation SOP](lark-apps-automation.md) 的匹配路径，由该 SOP 负责完整的状态门禁、commit/push、release 和可选 enable/test；不要先按本节发布再补 trigger 状态检查。下列通用序列只用于不含自动化 handler 的改动。

> `+release-create` 部署的是远端 `sprint/default` 上**已 push** 的代码，不是你本地工作区——未 commit / 未 push 的改动不会进入这次发布。所以发布前务必先把本次改动提交并推送。

1. `git status` 看本次改动；`git add <本次相关文件>` 暂存后 `git commit` 提交。只提交本次任务相关的改动即可，无关的零散文件不必强求清空——发布门禁是「**本次相关改动已提交并推送**」，不是「工作区绝对干净」。
2. `git push origin sprint/default` 把工作分支推到云端（遇非 fast-forward：先 `git pull --rebase origin sprint/default` 解决冲突再推，绝不 force-push；遇 Git 认证失败 / 401 / 403 / credential helper 缺失 / token 过期：先执行 `lark-cli apps +git-credential-init --app-id <app_id> --as user` 刷新本地 Git 凭证，再重试原 git 命令；刷新凭证也失败时，停止并向用户报告错误，不要换路）。
3. `lark-cli apps +release-create --as user --app-id <app_id> --branch sprint/default` 发起部署上线，记下返回的 `release_id`。
4. `lark-cli apps +release-get --as user --app-id <app_id> --release-id <release_id>` 轮询：`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟；超时仍未完成时停止本轮轮询、报告 `release_id` 和当前 status。`finished` 成功时，若返回 `online_url`，可直接使用；未返回时不要编造链接。交付线上访问链接给他人前，注意 `online_url` 默认仅创建者可见，需先告知当前仅本人可见、按需用 `+access-scope-set` 放开可见范围。无需再调 `+list`；`failed` 时若返回非空 `error_logs`，据此给出失败原因；否则只报告 `release_id` 和当前 status，不要编造原因（`+list` 仅作独立查询入口）。

用户只要求启用已有 trigger 时，转到 [automation SOP 的「仅启用已有 disabled trigger」路径](lark-apps-automation.md#仅启用已有-disabled-trigger)；不得因 enable 反向修改 handler、commit/push 或 release。

## 领域规则

- 代码读写走原生 `git`；CLI 负责凭证、初始化、发布和数据库调试。不存在 `apps +pull` / `apps +push` / `apps code +read` 这类代码读写 shortcut，不要臆造。
- 工作环境没有 `git` 时，先引导安装 Git（macOS 可用 `xcode-select --install` 或 `brew install git`；Linux 按发行版包管理器安装），安装后重试原 `+init` / git 命令；不要因此改走其他发布链路。
- `+init` 会编排 `+git-credential-init`、`git clone`、切到 `sprint/default`、运行脚手架，并在有变更时提交/推送。
- `+init --dir` 选目录：用户已预授权或表达"不要询问"（见 SKILL.md「预授权判定」）→ 按应用名派生 `./<app-name>` 直接传 `--dir`、不停问；否则先问用户用哪个目录再传。目标已存在/非空时回问换目录。
- `sprint/default` 是工作分支；`main` 是发布态快照，由 `+release-create` 成功后服务端 fast-forward 推进；服务端护栏禁直推 `main`、拒 force-push、要求 `sprint/default` fast-forward。
- 已拉到本地后，pull/push/diff/log 都用原生 git；云端 `sprint/default` 比本地新时，先 `git pull --rebase origin sprint/default`，解决冲突后再 push 和 publish。
- `git clone` / `git pull` / `git push` 如果报认证失败、401/403、credential helper 缺失或 token 过期，优先重新执行 `lark-cli apps +git-credential-init --app-id <app_id> --as user` 更新本地 Git 凭证，然后重试原 git 命令；刷新凭证也失败时，停止并向用户报告错误，不要换路；不要手动复制 token、不要把 token 拼进 remote URL。
- 环境变量由脚手架在本地启动时处理；需要手动刷新时用 `+env-pull`。
- 资源型文件（图片、字体、音视频等）不要直接引用本地路径，也不要提交到 git 仓库或以 base64 内联到代码中。先通过 `lark-cli apps +file-upload --app-id <app_id> --file <local_path>` 上传到应用文件存储，拿到返回的远端 URL 后在代码中引用该 URL。详情读 [`lark-apps-file.md`](lark-apps-file.md)。上传返回的链接按 app 隔离，不同应用必须各自重新上传，不能跨应用复用同一链接。
- DB 调试用 `+db-table-list` / `+db-table-get` / `+db-execute`；不要裸连数据库或自行拼连接串。
- DB 分 `dev` / `online`；使用 `--environment dev|online`，不要使用旧的 `--env`。只有确认应用已开启多环境时才引导 `--environment dev`；单环境应用省略 `--environment`（服务端选 online）或显式传 `--environment online`。在 dev 写入不能证明线上 handler 已验证。dev 的库结构变更要上线时，仍按应用发布链路走 `+release-create`，不要另造“数据库发布”步骤。
- 存量单库应用需要 dev/online 多环境时，用 `+db-env-create --environment dev`。这是不可逆 high-risk 操作。
- 只从 `+list` 看到 `is_published=true`，不能证明本地刚推送的代码已经部署；必须有本轮 `+release-get finished`。

## 存量应用入口

已有项目目录先读 `.spark/meta.json` 取 `app_id`；没有本地项目但知道应用名时用：

```bash
lark-cli apps +list --keyword "应用名"
```

拿到 `app_id` 后再 `+init` 或 `+git-credential-init`。

## 何时不用

- 用户明确要云端妙搭 Agent 生成/迭代，而不是本地写代码：读 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md)。
