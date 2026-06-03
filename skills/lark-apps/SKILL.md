---
name: lark-apps
description: "妙搭（Miaoda）应用的开发与托管，覆盖三条路径：(1) 本地全栈开发——把应用源码拉到本地、用原生 git 和本地 code agent 编码调试、连开发库验证、再部署；(2) 本地 HTML / 静态网站一键托管成公网可分享 URL；(3) 云端会话开发——通过 CLI 给云端妙搭 Agent 发消息，让它在云端生成 / 迭代应用。当用户要创建 / 列出妙搭应用、把 HTML / 静态网站发布成可访问链接、拉取应用源码到本地、查看或操作应用数据库（看表结构 / 跑 SQL / 初始化 dev 环境）、部署应用、给云端 Agent 发消息开发，或提到妙搭 / Miaoda / app_id 时使用。不用于：上传普通文件到云空间 / 云盘（用 lark-drive）、编辑飞书云文档内容（用 lark-doc）、创建飞书原生幻灯片 / 演示文稿（用 lark-slides）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli apps --help"
---

# apps (v1)

妙搭（Miaoda）是飞书的「应用开发与托管」能力域。`lark-cli apps` 把妙搭应用的全生命周期暴露成子命令，覆盖三条主路径：

| 路径 | 一句话 | app 类型 |
|------|--------|---------|
| **A. 本地全栈开发** | 拉源码到本地 → 原生 git + 本地 code agent 编码调试 → 部署 | `fullstack` |
| **B. HTML 一键托管** | 本地 HTML / 静态网站 → 发布成可分享 URL | `HTML` |
| **C. 云端会话开发** | 给云端妙搭 Agent 发消息 → 它在云端生成 / 迭代应用 | 任意 |

```bash
# A. 本地全栈：建应用 → 拿凭证 → 原生 git clone → 起本地开发 → 编码 → 部署
lark-cli apps +create --app-type full_stack --name "审批系统" --message "部门审批系统，支持登录、提交申请、多级审批"
lark-cli apps +git-credential-init --app-id app_xxx       # 配 git 凭证（一次性，后续自动用）
git clone <repo>; cd <repo>; npm install && npm dev run   # 起本地开发，自动拉 env（非阻塞）
git push miaoda develop; lark-cli apps +publish --app-id app_xxx

# B. HTML 托管
lark-cli apps +create --app-type HTML --name "客户调研问卷"
lark-cli apps +html-publish --app-id app_xxx --path ./dist

# C. 云端会话
lark-cli apps +session-create --app-id app_xxx --message "做一个待办清单页面"
lark-cli apps +session-read --session-id sess_xxx
```

## 意图路由（先读这张表）

| 用户表达 | 走哪条 | 必读 playbook |
|---------|--------|--------------|
| "拉代码到本地开发"、"本地用 cc/Codex 写"、"连数据库调试"、"全栈应用"、"React/Nest 项目" | **A 本地全栈** | [`lark-apps-local-dev.md`](references/lark-apps-local-dev.md) |
| "把 HTML / 静态网站发布成可分享链接"、"部署 ./dist"、"用 HTML 写个 PPT/demo 发出去" | **B HTML 托管** | [`lark-apps-html-publish.md`](references/lark-apps-html-publish.md) |
| "让妙搭 Agent 在云端帮我做个 X"、"给云端 Agent 发消息开发"、"一句话生成应用" | **C 云端会话** | [`lark-apps-cloud-dev.md`](references/lark-apps-cloud-dev.md) |
| "建应用 / 列应用 / 改名 / 设可见范围" | 元数据操作，见下方命令索引 | 对应命令 reference |
| **只描述了需求 + 部署诉求，没明确 app 类型、也没说本地 / 云端** | 先按需求判类型、再问开发方式，见下方 | —— |

### 输入模糊：没指定 app 类型 / 开发方式时

用户只说了"想要什么 + 要个可访问 / 可分享的结果"，但没点明 app 类型、也没说本地还是云端开发时：

1. **app type 从需求本身推断，不用问**（类型是可从需求判定的）：
   - 多人协作 / 数据持久化 / 登录 / 后端逻辑 / 增删改查 → **`fullstack`**
   - 纯静态展示 / 单页 / 一次性只读内容 → **`HTML`**
   - 真拿不准、无明显信号就默认 `HTML`（更轻、现有成熟流程），必要时追问一句澄清，而不是把"选什么类型"抛给用户。
2. **开发方式（本地 vs 云端）问一句再定**：它和"做什么"正交、**无法从需求推断**——这是"谁来写、怎么写"的选择，用户没明示就不要替他假设。问法（中性，不预设用户会不会写代码）：

   > 这个有两种做法：① 让云端 AI 直接生成并自动部署；② 在本地用代码开发后再部署。你想用哪种？

   - 明确偏好云端 / 直接生成 → **C 云端会话**
   - 明确偏好本地 / 自己写 / 交给研发 → **A 本地全栈**
   - 若是 `HTML` 且用户已有现成 HTML 文件 → 直接 **B HTML 托管**，无需再问

## ⚠️ 五个反直觉心智模型（动手前务必建立）

新接入者几乎都会在这几点踩坑，**先读完再操作**：

1. **代码读写走原生 `git`，不走 CLI。** CLI 在本地开发里只做两件事：`+git-credential-init`（配推送凭证）和 `+publish`（部署）。`clone` / `pull` / `push` / `diff` / `log` / `blame` / 解冲突**全部用原生 `git`**。不存在 `apps +pull` / `apps +push` / `apps code +read` 这类命令，别去找、别去拼。
2. **`develop` / `main` 双分支模型。** `develop` 是唯一工作分支（本地 push、云端 chat 提交都进它）；`main` 是「当前部署态」只读快照，**只能由 `+publish` 推进**（发布成功后服务端自动 fast-forward `main ← develop`）。客户端不要 push main、不要 force-push，服务端 pre-receive hook 会硬拒。
3. **DB 调试走 `+db-*` 命令（经妙搭封装），不是裸连数据库。** 用 `+db-table-list` / `+db-table-schema` / `+db-sql` 经妙搭服务端鉴权访问应用的多环境数据库，做查表 / 看 schema / 跑 SQL 调试，不用自己拼连接串。（应用运行时自己连库用的是 env 里的凭证，那是另一回事。）
4. **凭证自动管理，不用手动刷新。** `+git-credential-init` 配好后，后续 git 操作的鉴权由 git credential helper **自动触发**；DB 凭证在环境变量里、`npm dev run` 时**自动更新**——不存在"PAT 过期要手动刷新"这回事，别去找刷新 / 续期命令。

5. **环境变量不走独立命令。** env 由脚手架的 `npm dev run` 在启动本地开发时**自动拉取**，且**不阻塞** `npm dev run` 执行（拉取在后台进行）。没有 `apps +env-pull` 命令，别去找。

## 前置条件 — 执行操作前必读

**CRITICAL — 执行对应操作前，MUST 先用 Read 工具读取以下文件：**

1. [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) — 认证、权限处理、全局参数（所有操作通用）
2. **本地全栈开发** → 必读 [`lark-apps-local-dev.md`](references/lark-apps-local-dev.md)（端到端：建应用 → 凭证 → 原生 git → `npm dev run` 起本地开发（自动拉 env）→ DB 调试 → publish）
3. **HTML / 静态网站托管** → 必读 [`lark-apps-html-publish.md`](references/lark-apps-html-publish.md)（`--path` 文件 vs 目录、index.html 入口、凭据文件扫描）
4. **云端会话开发** → 必读 [`lark-apps-cloud-dev.md`](references/lark-apps-cloud-dev.md)（session 生命周期、chat、附件、轮询拿状态）
5. **创建 / 更新 / 列出应用** → 必读 [`lark-apps-create.md`](references/lark-apps-create.md) / [`lark-apps-update.md`](references/lark-apps-update.md)
6. **设置 / 查看可用范围** → 必读 [`lark-apps-access-scope-set.md`](references/lark-apps-access-scope-set.md) / [`lark-apps-access-scope-get.md`](references/lark-apps-access-scope-get.md)
7. **初始化 / 查看 / 删除妙搭 Git 凭证（`apps +git-credential-init` / `apps +git-credential-list` / `apps +git-credential-remove`）** → 必读 [`lark-apps-git-credential.md`](references/lark-apps-git-credential.md)（只处理 Git credential，不与 setup / env pull 混用；输出 Repository URL 后继续用原生 Git；list 会自动扫描本地所有 app 配置，不需要 `--app-id`）
8. **数据库表 / schema / SQL（`apps +db-table-list` / `+db-table-schema` / `+db-sql`）** → 必读 [`lark-apps-db-table-list.md`](references/lark-apps-db-table-list.md) / [`lark-apps-db-table-schema.md`](references/lark-apps-db-table-schema.md) / [`lark-apps-db-sql.md`](references/lark-apps-db-sql.md)（游标分页、`--format pretty` 出建表 DDL、SQL 多语句默认不包裹事务 + 失败逐条定位）
9. **初始化 dev 环境（`apps +db-dev-init`）** → 必读 [`lark-apps-db-dev-init.md`](references/lark-apps-db-dev-init.md)（单库→online/dev，**不可逆**，需 `--yes` 确认）
10. **发布管理（`apps +publish` / `+publish-history` / `+publish-status` / `+publish-error-log`）** → 必读对应参考文档：[`lark-apps-publish.md`](references/lark-apps-publish.md)、[`lark-apps-publish-history.md`](references/lark-apps-publish-history.md)、[`lark-apps-publish-status.md`](references/lark-apps-publish-status.md)、[`lark-apps-publish-error-log.md`](references/lark-apps-publish-error-log.md)。
## 身份与一次性授权

妙搭应用是用户的个人资产，**统一使用 `--as user`**（默认 `--as auto` 会按 shortcut 声明落到 user）。

**首次操作前一次性把本域 scope 全拿到**，避免每条命令首次跑都触发新一轮授权：

```bash
lark-cli auth login --domain apps
```

命令失败且 `error.type == "missing_scope"` 时，统一引导用户跑上面这条。

## 命令索引

按功能族组织；「本期新增」的命令接口可能仍在收敛，以 `lark-cli apps +<cmd> --help` 为准。

### 元数据 / 生命周期
| 命令 | 用途 | 状态 |
|------|------|------|
| [`+create`](references/lark-apps-create.md) | 新建应用，必选 `--app-type`（`HTML` / `fullstack`）；`fullstack` 必带 `--message`（需求原话，触发服务端首轮生成） | 已有，扩展 |
| [`+list`](references/lark-apps-list.md) | 列出 / 过滤当前身份可见的应用（`--filter`） | 已有，扩展 |
| [`+update`](references/lark-apps-update.md) | 改应用名 / 描述（部分更新） | 已有 |

### 本地开发 · Git 凭证（配好后用原生 git）
| 命令 | 用途 | 状态 |
|------|------|------|
| `+git-credential-init` | 配置本地 git 推送凭证（配一次，后续 git 操作经 credential helper 自动用） | 本期新增 |
| `+git-credential-list` | 列出本地已配置的妙搭 git 凭证 | 本期新增 |
| `+git-credential-remove` | 移除某应用的本地 git 凭证 | 本期新增 |

### 本地开发 · 环境与数据库
> 环境变量**不提供独立命令**：由脚手架的 `npm dev run` 在起本地开发时自动拉取（非阻塞）。
> 多环境数据库**推荐在 `+create` 时勾选开启**（`--enable-multi-env-db`）；DB 分 **dev / online 两环境**，**dev 的库结构变更随 `+publish` 一起发布到 online**（与代码同一条发布链路）。flag / scope 细节见各命令 reference。

| 命令 | 用途 | 状态 |
|------|------|------|
| [`+db-dev-init`](references/lark-apps-db-dev-init.md) | 开启 dev/online 多环境库（high-risk，**不可逆**，需 `--yes`；仅存量应用需手动跑） | 本期新增 |
| [`+db-table-list`](references/lark-apps-db-table-list.md) | 列出某环境库的表（游标分页，含预估行数 / 占用空间） | 本期新增 |
| [`+db-table-schema`](references/lark-apps-db-table-schema.md) | 查看某张表的 schema（默认结构化；`--format pretty` 出建表 DDL） | 本期新增 |
| [`+db-sql`](references/lark-apps-db-sql.md) | 经妙搭跑 SQL（SELECT / DML / DDL；多语句默认不包裹事务，失败逐条定位） | 本期新增 |

### 部署
| 命令 | 用途 | 状态 |
|------|------|------|
| [`+publish`](references/lark-apps-publish.md) | 为应用创建发布（release），返回 `{ release_id, status }`（成功后服务端发布 `develop`；dev 库结构变更一并发布到 online） | 已上线 |
| [`+publish-status`](references/lark-apps-publish-status.md) | 按 `--release-id` 查单个发布的状态 / 详情 | 已上线 |
| [`+publish-history`](references/lark-apps-publish-history.md) | 分页查发布历史（`--status publishing\|finished\|failed` / `--limit 1-500` / `--page-token`） | 已上线 |
| [`+publish-error-log`](references/lark-apps-publish-error-log.md) | 按 `--release-id` 查失败发布的错误日志（失败步骤 + `error_log`） | 已上线 |
| [`+html-publish`](references/lark-apps-html-publish.md) | 把本地 HTML 文件 / 目录打包发布（`--path`），返回访问 URL | 已有 |

### 云端会话开发
| 命令 | 用途 | 优先级 / 状态 |
|------|------|------|
| `+session-create` | 在应用下创建会话并发首条需求 | P0 新增 |
| `+session-read` | 读取会话内容 / 进度（轮询用） | P0 新增 |
| `+session-list` | 列出某应用下的会话 | P1 新增 |
| `+session-stop` | 终止会话 | P1 新增 |
| `+chat` | 在会话内继续发消息 | 新增 |

### 可用范围
| 命令 | 用途 | 状态 |
|------|------|------|
| [`+access-scope-set`](references/lark-apps-access-scope-set.md) | 设置可用范围（specific / public / tenant，三态互斥） | 已有 |
| [`+access-scope-get`](references/lark-apps-access-scope-get.md) | 查看当前可用范围 | 已有 |

## 三组权限别混淆（正交，必须分清）

| 维度 | 决定什么 | 入参 | 相关命令 |
|------|---------|------|---------|
| **认证 scope** | 当前身份能调哪些 API | `auth login --domain apps` | `lark-cli auth login` |
| **协作者**（开发期） | 谁能一起 push 代码 / 跟云端 Agent 对话 | 飞书 user_id | （collaborator，后续版本） |
| **可用范围 / visibility**（运行时） | 谁能打开部署后的应用使用 | scope / 用户 / 部门 | `+access-scope-set/get` |

> 一个协作者不一定能用部署后的应用（若 visibility 不含他）；visibility 内的普通用户也不能改代码。

## 通用约定

- 失败时**优先转述 `error.hint`**（CLI 给的可执行修复建议），hint 为空再退回 `error.message`；不要原样把 envelope JSON 复述给用户。
- `error.type == "missing_scope"` → 按上面「身份与一次性授权」走。
- 高风险写操作（如 `+delete` / visibility 收窄）exit 10 + `confirmation_required` 时，先向用户确认，同意后在原 argv 末尾追加 `--yes` 重试；**不要静默加 `--yes`**（见 lark-shared）。
- **拿到存量应用的 `app_id`（按顺序试，不要直接问用户要）**：
  1. 用户已给 `app_id` 或**妙搭链接**（`https://miaoda.feishu.cn/app/app_xxx` → 取 `/app/` 后的 segment）→ 用它；
  2. 已在本地项目目录里 → 读项目根的 **`.spark/meta.json`**（记录了 `app_id`）；
  3. 都没有 → **`+list --keyword <关键词>`** 按应用名 / 描述过滤定位。
