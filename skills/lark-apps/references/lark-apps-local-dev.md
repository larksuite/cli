# lark-apps 本地全栈开发

适用：用户要把妙搭全栈应用源码拉到本地，用本地 code agent/IDE 开发、调试数据库，再发布。

## 先分清：已有应用拉到本地 还是 新建

进 local-dev 第一步先判断意图，别默认新建：

- “开发 X 应用 / 把 X 拉到本地 / 接着开发”，或用户给了 app_id / 妙搭链接 → **已有应用**：跳过 `+create`，先按下方「存量应用入口」拿 `app_id`，再 `+init`（或 `+git-credential-init` + `git clone`）把它拉到本地。
- “做一个 / 新建一个 X” → **新建**：从 `+create` 开始走下面的流程。
- 带名字的“开发 xxx 应用”通常指已有应用；拿不准先问一句，不要直接 `+create`。

## 端到端流程（新建应用）

`+create(full_stack)` -> `+init`（或手动 `+git-credential-init` + `git clone`）-> `npm install && npm run dev` -> 按需 `+db-*` 调库 -> `git push origin sprint/default` -> `+publish` -> `+publish-status`。

```bash
# 新建 full_stack 应用
lark-cli apps +create --name "审批系统" --app-type full_stack \
  --description "支持登录、提交申请、多级审批、状态查询"

# 初始化本地仓库（--dir 目标目录由用户选定，见下方「领域规则」，勿照抄此处示例值）
lark-cli apps +init --app-id app_xxx --dir ./approval-app

# 进入仓库后按项目脚手架启动
cd ./approval-app
npm install
npm run dev

# 开发完成后：原生 git 推工作分支，再发布
git status
git push origin sprint/default
lark-cli apps +publish --app-id app_xxx
```

`+init` 是推荐便捷入口；想逐步手动控制时，先 `+git-credential-init` 拿 `repository_url`，再用原生 `git clone` / `git checkout sprint/default`。

## 改完代码后部署上线

已拉到本地、改完代码，用户说"推上去""部署""上线""发布到云端"时，按此序列：

1. `git status` 确认改动已提交，工作区干净。
2. `git push origin sprint/default` 把工作分支推到云端（遇非 fast-forward：先 `git pull --rebase origin sprint/default` 解决冲突再推，绝不 force-push）。
3. `lark-cli apps +publish --app-id <app_id>` 发起部署上线，记下返回的 `release_id`。
4. `lark-cli apps +publish-status --app-id <app_id> --release-id <release_id>` 轮询：`publishing` 继续轮询、`finished` 成功、`failed` 接 `+publish-error-log`。

`+publish` 部署上线属高影响动作，按 SKILL.md「失败与高影响动作」先征得用户同意再发布。

## 领域规则

- 代码读写走原生 `git`；CLI 负责凭证、初始化、发布和数据库调试。不存在 `apps +pull` / `apps +push` / `apps code +read` 这类代码读写 shortcut，不要臆造。
- `+init` 会编排 `+git-credential-init`、`git clone`、切到 `sprint/default`、运行脚手架，并在有变更时提交/推送。
- `+init --dir` 的目标目录由用户决定：调用前先给出目录建议/选项让用户选，拿到选择再传 `--dir`。
- `sprint/default` 是工作分支；`main` 是发布态快照，由 `+publish` 成功后服务端 fast-forward 推进；服务端护栏禁直推 `main`、拒 force-push、要求 `sprint/default` fast-forward。
- 已拉到本地后，pull/push/diff/log 都用原生 git；云端 `sprint/default` 比本地新时，先 `git pull --rebase origin sprint/default`，解决冲突后再 push 和 publish。
- 环境变量由脚手架在本地启动时处理；需要手动刷新时用 `+env-pull`。
- DB 调试用 `+db-table-list` / `+db-table-get` / `+db-sql`；不要裸连数据库或自行拼连接串。
- DB 分 `dev` / `online`；日常调试优先 `--env dev`。dev 的库结构变更要上线时，仍按应用发布链路走 `+publish`，不要另造“数据库发布”步骤。
- 存量单库应用需要 dev/online 多环境时，用 `+db-env-create --env dev`。这是不可逆 high-risk 操作。

## 存量应用入口

已有项目目录先读 `.spark/meta.json` 取 `app_id`；没有本地项目但知道应用名时用：

```bash
lark-cli apps +list --keyword "应用名"
```

拿到 `app_id` 后再 `+init` 或 `+git-credential-init`。

## 何时不用

- 用户只想发布现成 HTML / 静态目录拿分享链接：读 [`lark-apps-html-publish.md`](lark-apps-html-publish.md)。
- 用户明确要云端妙搭 Agent 生成/迭代，而不是本地写代码：读 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md)。
