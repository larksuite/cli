# lark-apps 本地全栈开发

适用：用户要把妙搭全栈应用源码拉到本地，用本地 code agent/IDE 开发、调试数据库，再发布。

## 新建 vs 已有应用

新建还是修改已有，由上方入口（SKILL.md「选择开发路径」）判定；进到本地流程后按分支走：

- **新建**：从 `+create` 开始走下面的端到端流程。
- **已有应用**（本地还没有源码）：跳过 `+create`，先按下方「存量应用入口」拿 `app_id`，再 `+init`（或 `+git-credential-init` + `git clone`）把它拉到本地，然后照常开发。

## 端到端流程（新建应用）

`+create(full_stack)` -> `+init`（或手动 `+git-credential-init` + `git clone`）-> 读仓库 Skill -> `npm install && npm run dev` -> 按需 `+db-*` 调库 -> `git add` + `git commit`（提交本次改动）-> `git push origin sprint/default` -> `+release-create` -> `+release-get`。

```bash
# 新建 full_stack 应用
lark-cli apps +create --name "审批系统" --app-type full_stack \
  --description "支持登录、提交申请、多级审批、状态查询"

# 初始化本地仓库（--dir 取值见下方「领域规则」，勿照抄此处示例值）
lark-cli apps +init --app-id app_xxx --dir ./approval-app

# 进入仓库后按项目脚手架启动
cd ./approval-app
npm install
npm run dev

# 开发完成后：提交本次改动 -> git push origin sprint/default -> +release-create。
# +release-create 部署的是远端 sprint/default 上已 push 的代码，不是本地工作区——没 commit + push 的改动不会进入发布。
git add <本次开发的文件>          # 提交粒度见下方「改完代码后部署上线」
git commit -m "feat: ..."
git push origin sprint/default
lark-cli apps +release-create --app-id app_xxx
```

`+init` 是推荐便捷入口；想逐步手动控制时，先 `+git-credential-init` 拿 `repository_url`，再用原生 `git clone` / `git checkout sprint/default`。

**`+init` 完成后必须执行**：`cat <project-path>/.agents/skills/plugin-guide/SKILL.md`，读取仓库插件指引。该文件包含插件目录、实例配置规则和调用代码生成方式——不读就无法正确集成插件能力。文件不存在则跳过。

## 改完代码后部署上线

已拉到本地、改完代码，用户说"推上去""部署""上线""发布到云端"时，按此序列。

> `+release-create` 部署的是远端 `sprint/default` 上**已 push** 的代码，不是你本地工作区——未 commit / 未 push 的改动不会进入这次发布。所以发布前务必先把本次改动提交并推送。

1. `git status` 看本次改动；`git add <本次相关文件>` 暂存后 `git commit` 提交。只提交本次任务相关的改动即可，无关的零散文件不必强求清空——发布门禁是「**本次相关改动已提交并推送**」，不是「工作区绝对干净」。
2. `git push origin sprint/default` 把工作分支推到云端（遇非 fast-forward：先 `git pull --rebase origin sprint/default` 解决冲突再推，绝不 force-push）。
3. `lark-cli apps +release-create --app-id <app_id>` 发起部署上线，记下返回的 `release_id`。
4. `lark-cli apps +release-get --app-id <app_id> --release-id <release_id>` 轮询：`publishing` 继续轮询；`finished` 成功时该命令输出已含 `online_url`，直接读取它返回给用户（这是本轮发布完成后的可分享链接），无需再调 `+list`；`failed` 时该命令输出已含 `error_logs`，直接据此给出失败原因（`+list` 仅作独立查询入口）。

## 领域规则

- 代码读写走原生 `git`；CLI 负责凭证、初始化、发布和数据库调试。不存在 `apps +pull` / `apps +push` / `apps code +read` 这类代码读写 shortcut，不要臆造。
- `+init` 会编排 `+git-credential-init`、`git clone`、切到 `sprint/default`、运行脚手架，并在有变更时提交/推送。
- `+init --dir` 选目录：用户已预授权或表达"不要询问"（见 SKILL.md「预授权判定」）→ 按应用名派生 `./<app-name>` 直接传 `--dir`、不停问；否则先问用户用哪个目录再传。目标已存在/非空时回问换目录。
- `sprint/default` 是工作分支；`main` 是发布态快照，由 `+release-create` 成功后服务端 fast-forward 推进；服务端护栏禁直推 `main`、拒 force-push、要求 `sprint/default` fast-forward。
- 已拉到本地后，pull/push/diff/log 都用原生 git；云端 `sprint/default` 比本地新时，先 `git pull --rebase origin sprint/default`，解决冲突后再 push 和 publish。
- 环境变量由脚手架在本地启动时处理；需要手动刷新时用 `+env-pull`。
- DB 调试用 `+db-table-list` / `+db-table-get` / `+db-execute`；不要裸连数据库或自行拼连接串。
- DB 分 `dev` / `online`；日常调试优先 `--env dev`。dev 的库结构变更要上线时，仍按应用发布链路走 `+release-create`，不要另造“数据库发布”步骤。
- 存量单库应用需要 dev/online 多环境时，用 `+db-env-create --env dev`。这是不可逆 high-risk 操作。
- 只从 `+list` 看到 `is_published=true`，不能证明本地刚推送的代码已经部署；必须有本轮 `+release-get finished`。

## 存量应用入口

已有项目目录先读 `.spark/meta.json` 取 `app_id`；没有本地项目但知道应用名时用：

```bash
lark-cli apps +list --keyword "应用名"
```

拿到 `app_id` 后再 `+init` 或 `+git-credential-init`。

## 何时不用

- 用户只想发布现成 HTML / 静态目录拿分享链接：读 [`lark-apps-html-publish.md`](lark-apps-html-publish.md)。
- 用户明确要云端妙搭 Agent 生成/迭代，而不是本地写代码：读 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md)。
