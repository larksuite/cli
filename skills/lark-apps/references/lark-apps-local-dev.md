# 本地全栈开发 playbook（Story 1）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)（认证 / 全局参数 / 安全），并读完 [`../SKILL.md`](../SKILL.md) 的「五个反直觉心智模型」。
>
> **接口状态：** 本期新增命令（`+git-credential-*` / `+db-*` / `+publish*`）接口仍在收敛，下面的 flag 以 `lark-cli apps +<cmd> --help` 为准；标 `（待定）` 的是设计中、可能变动的参数。环境变量不提供独立命令，由脚手架 `npm dev run` 启动时自动拉取（非阻塞）。

适用场景：研发要把妙搭应用的**源码拉到本地**，用自己熟悉的本地 code agent（cc / Codex / Kimi Code）+ **原生 git** 开发调试，再部署到云端。应用类型 `fullstack`（React + Nest + Drizzle + PostgreSQL）。

## 核心心智（再强调一遍，这里最容易错）

- **代码读写一律走原生 `git`。** CLI 只做 `+git-credential-init`（配凭证）和 `+publish`（部署）两件事，clone/pull/push/diff/log 全用原生 git。
- **`sprint/default` 是工作分支，`main` 是部署态只读快照。** 平时只在 `sprint/default` 上 push；`main` 只能由 `+publish` 推进。
- **DB 调试经 `+db-*` 命令（妙搭封装）+ 一次性开启。** 用 `+db-table-list` / `+db-table-schema` / `+db-sql` 经妙搭服务端访问应用的多环境数据库，不是裸连。**多环境数据库推荐在 `+create` 时就勾选开启**（`--enable-multi-env-db`），开了就能本地调库；**只有存量应用**才需要事后手动跑一次 `+db-multi-env-init` 开启，再做 DB 调试。
- **env 不走独立命令。** 环境变量由脚手架的 `npm dev run` 在起本地开发时**自动拉取**，且**不阻塞** `npm dev run`（后台拉，不会卡住启动）。没有 `apps +env-pull`。
- **凭证自动管理。** `+git-credential-init` 配一次，后续 git 鉴权由 credential helper 自动触发；DB 凭证在 env 里、`npm dev run` 时自动更新。不用手动刷新 PAT，也没有续期命令。

## 端到端流程

```
+create(fullstack, 勾选开启多环境数据库) → +git-credential-init → git clone
   → npm install && npm dev run（起本地开发，自动拉 env，非阻塞）
   → [按需用 +db-* 调库] → 本地编码 → git push origin sprint/default
   → +publish → +publish-status 查结果
```

> 想一步到位：`apps +init --app-id <id> --dir <目录>` 把 step 2-4（配凭证 + clone + 脚手架 + 推初始代码）合成一步，见 [`lark-apps-init.md`](lark-apps-init.md)；想逐步手动控制再走下面的分步。

> **存量应用入口**：要接着开发一个已有应用而非新建时，**跳过 step 1**，先按 [`../SKILL.md`](../SKILL.md)「拿到存量应用的 app_id」解析（已在项目目录就读 `.spark/meta.json`；否则 `+list --filter <关键词>`），拿到 `app_id` 后从 step 2 开始。存量应用若没开多环境数据库，做 DB 调试前还要先 `+db-multi-env-init`（见下方 DB 段）。

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1. 新建应用 | `lark-cli apps +create --app-type fullstack --name "<名字>" --message "<需求原话>" --enable-multi-env-db`（`--message` fullstack 必填；`--enable-multi-env-db` 待定） | 服务端拉脚手架 + 初始化 git + 接好妙搭远端。**推荐建时就开启多环境数据库**，后续可直接本地调库。从响应拿 `app_id` 和 git 仓库地址 |
| 2. 配 git 凭证 | `lark-cli apps +git-credential-init --app-id app_xxx` | 配置 git credential helper（配一次即可），之后原生 git 操作鉴权自动触发，免手动再输 |
| 3. 拉代码 | `git clone <repo_url>`（用原生 git） | 仓库地址来自 step 1 响应或 `+list` / 应用详情。**不是** CLI 命令 |
| 4. 起本地开发 | `npm install && npm dev run`（脚手架内置脚本） | 启动本地开发服务，**自动拉取 env（非阻塞）**。env 不再需要单独命令 |
| 5.（可选）调库 | `+db-table-list` / `+db-table-schema` / `+db-sql` | 经妙搭跑 migration / 看 schema / 验数据，见下方 DB 小节。**前提**：应用已开启多环境数据库（建时勾选，或存量应用先跑 `+db-multi-env-init`） |
| 6. 本地编码 | 本地 code agent + 原生 git | 编辑代码，`git commit`（脚手架内置 husky hook 会本地自检 lint / secret / 大文件） |
| 7. 推送 | `git push origin sprint/default`（用原生 git） | 推到工作分支 sprint/default；服务端 pre-receive hook 会校验，并触发后台预构建 |
| 8. 部署 | `lark-cli apps +publish --app-id app_xxx` | 发布当前 sprint/default；成功后服务端自动 `main ← sprint/default` fast-forward。**dev 的库结构变更也随这一步发到 online** |
| 9. 查状态 | `lark-cli apps +publish-status --app-id app_xxx` | 看构建 / 部署结果；`+publish-history` 看历史 |

> **存量应用例外**：老应用建时没开多环境数据库的，做本地 DB 调试前要先手动开一次：`lark-cli apps +db-multi-env-init --app-id app_xxx`（待定），开完再用下面的 `+db-*`。新建应用走 step 1 勾选即可，不需要这步。

## 用原生 git 操作（只讲妙搭特有的点）

git 命令本身不赘述，这里只列妙搭特有、容易踩的：

- **remote 叫 `origin`，工作分支是 `sprint/default`**：推送 `git push origin sprint/default`，同步云端 `git fetch origin sprint/default && git rebase origin/sprint/default sprint/default`。
- **服务端护栏**（CLI / 原生 git 都生效）：禁直推 `main`、拒 force-push、`sprint/default` 必须 fast-forward。所以 push 被拒多半是云端 sprint/default 比本地新 → 先 rebase 再 push。

## DB 调试（经 `+db-*` 命令）

应用开启多环境数据库后（新建时 `--enable-multi-env-db` 勾选；**仅存量应用**才需先手动 `+db-multi-env-init` 开一次），可用 `+db-*` 经妙搭服务端访问库做调试，**不裸连、不拼连接串**：

- `+db-table-list` — 列出表
- `+db-table-schema` — 看某张表结构
- `+db-sql` — 跑 SQL（SELECT / DML / DDL）

**dev / online 双环境**（与代码的 sprint/default / main 平行）：日常在 **dev** 调，`+db-*` 用 `--env` 选环境；**dev 的库结构变更会随 `+publish` 一起发布到线上 online**（和代码同一条发布链路，不需要单独的库发布动作）。所以在 dev 改完 schema 别忘了 `+publish` 才会上线。

> 具体 flag / scope / 输出格式 / 报错处理见各命令自带 reference（`lark-cli apps +<cmd> --help`）；`+db-multi-env-init` 是 high-risk-write，需 `--yes`。

## 何时不要用这条 playbook

- 用户只是想**发布 HTML / 静态网站**拿个分享链接 → 走 [`lark-apps-html-publish.md`](lark-apps-html-publish.md)（`HTML` 类型，无需 git / DB）。
- 用户想**让云端 Agent 帮忙生成代码**而不是自己在本地写 → 走 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md)。

## 参考

- [lark-apps](../SKILL.md) — 路由表 + 命令索引 + 心智模型
- [lark-apps-create](lark-apps-create.md) — 建应用参数
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
