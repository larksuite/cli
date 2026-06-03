# apps +init

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

初始化妙搭应用的**本地开发仓库**。这是一个编排命令（orchestrator）：先解析目标目录 → **若目录已初始化则友好 no-op 直接返回** → 检查 `git` / `npx` 依赖 → 调 `apps +git-credential-init` 签发带凭据的仓库地址 → `git clone` → 切到 `sprint/default` 分支 → **跑 npx 脚手架**（空仓库 `app init`，非空仓库 `app upgrade` + 补 `.spark/meta.json` 的 `app_id` + 按需 `skills sync`）→ 若工作区有改动则 `git add -A` + commit + `git push origin sprint/default`，工作区干净则跳过 commit/push。返回本地克隆路径。

> 💡 **跑 `+init` 前先问用户「clone 到哪」：** `+init` 会把仓库克隆到本地磁盘。Agent **应先询问用户希望克隆的目标目录**，并通过 `--dir` 传入；`--dir` 接受**绝对路径或相对路径**。用户没有偏好时，默认克隆到 `./<app-id>`。

> ⚠️ **依赖 `apps +git-credential-init`：** `+init` 内部 shell out 调用 `apps +git-credential-init` 签发仓库凭据。该命令已实现并注册。运行时若凭据签发失败或远端不可达，`+init` 会在该步以结构化 `credential_init` 错误失败——这是预期的错误回传，不是命令本身坏了。

## 命令

```bash
# 最小调用（克隆到 ./<app-id>）
lark-cli apps +init --app-id app_xxx

# 指定克隆目录（绝对或相对路径均可；推荐先问用户克隆到哪）
lark-cli apps +init --app-id app_xxx --dir ./my-app
lark-cli apps +init --app-id app_xxx --dir /Users/me/code/my-app

# 指定脚手架模板（可选；省略时未来按 app 技术栈派生，当前回退 nestjs-react-fullstack）
lark-cli apps +init --app-id app_xxx --template nestjs-react-fullstack

# 预演（打印计划步骤，不执行任何 git / npx 操作）
lark-cli apps +init --app-id app_xxx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--app-id <id>` | ✅ | 妙搭应用 ID；缺失时 Validate 阶段以结构化 `validation` 错误退出（exit code 2），**不是**纯文本错误 |
| `--dir <path>` | ❌ | 克隆目标目录，默认 `./<app-id>`。**接受绝对路径或相对路径**（不再强制 cwd 内）；只拒绝控制字符。目标目录是**符号链接**、或已存在且**非空**会被拒绝（不存在则由 git clone 创建）。**例外：** 目标目录若已是初始化过的妙搭仓库（含 `.spark/meta.json`），即使非空也不拒绝，而是走「已初始化 no-op」（见下） |
| `--template <tpl>` | ❌ | 空仓库脚手架（`app init`）使用的模板，**可选、无硬编码默认**：显式传入则用传入值；**省略时未来按 app 技术栈派生**（按 app_id 经 apps API 查到应用、再把技术栈经枚举映射到模板，**尚未实现**），**在该能力落地前省略时回退到 `nestjs-react-fullstack`**。非空仓库走 `app upgrade`，不使用该模板 |
| `--format <fmt>` | ❌ | 输出格式，默认 `json` |
| `--dry-run` | ❌ | 仅打印计划步骤，不执行 |

## 已初始化目录：友好 no-op

如果 `--dir`（或默认目录）已经包含 `.spark/meta.json`，说明该目录已是初始化过的妙搭仓库。此时 `+init` **不会重新初始化**，而是直接以 `scaffold:"already_initialized"`、exit 0 友好返回，不做 clone / 脚手架 / commit。

**注意：** 这条路径**没有 clone 任何东西**，所以输出 `data` 里**没有 `repository_url`、也没有 `branch`**，只含 `app_id` / `clone_path` / `scaffold` / `committed` / `pushed` / `message`：

```json
{
  "ok": true,
  "data": {
    "app_id": "app_xxx",
    "clone_path": "/abs/path/to/app_xxx",
    "scaffold": "already_initialized",
    "committed": false,
    "pushed": false,
    "message": "Repository already initialized. You can start developing."
  }
}
```

## 返回值

**成功：**

```json
{
  "ok": true,
  "data": {
    "app_id": "app_xxx",
    "repository_url": "https://***@example.com/org/app_xxx.git",
    "branch": "sprint/default",
    "clone_path": "/abs/path/to/app_xxx",
    "scaffold": "init",
    "committed": false,
    "pushed": false,
    "message": "Repository initialized. You can start developing."
  }
}
```

**失败（结构化 envelope）：**

```json
{
  "ok": false,
  "error": {
    "type": "credential_init",
    "message": "apps +git-credential-init failed: ...",
    "hint": "ensure apps +git-credential-init is available and you are logged in"
  }
}
```

## 进度与输出流（stdout/stderr）

`+init` 是一个多步编排命令，运行时会在每一步往 **stderr** 打一行 `→ ` 前缀的进度提示（如签发凭据、clone、checkout、跑脚手架、commit/push 或工作区干净跳过、已初始化 no-op）。**stdout 始终只承载纯 JSON 结果 envelope**，进度不会污染它。

**stdout / stderr 契约（机器 / AI 消费者务必遵守）：**

- **stdout** = 成功时的 JSON 结果 envelope（即上面「返回值」里的成功结构，未变）。
- **stderr** = 零行或多行纯文本 `→ ` 前缀进度；**失败时，结构化 JSON 错误 envelope 也写到 stderr**，且位于所有进度行**之后**。
- 因此消费者应当：成功路径**读 stdout** 取结果 envelope；错误路径**解析 stderr 上最后一个 JSON 文档**（即末尾那个错误 envelope）。**不要直接 `jq` 整个 stderr**——先按 `→ ` 前缀过滤掉进度行，或只取最后一个 JSON 文档。`→ ` 进度行是纯文本、可安全丢弃。

## 字段语义

普通初始化路径的 `data` 含以下全部字段；**已初始化 no-op 路径**只含其中的 `app_id` / `clone_path` / `scaffold` / `committed` / `pushed` / `message`（**无 `repository_url`、无 `branch`**）：

| 字段 | 含义 |
|---|---|
| `app_id` | 透传的应用 ID |
| `repository_url` | 仓库地址，**凭据已脱敏**：URL 里的 userinfo 段被替换为 `***`（如 `https://***@host/...`）。任何回显仓库地址的错误信息也同样脱敏。**仅普通初始化路径有；已初始化 no-op 路径没有此字段** |
| `branch` | 切出的分支，固定为 `sprint/default`。**仅普通初始化路径有；已初始化 no-op 路径没有此字段** |
| `clone_path` | 本地克隆的**绝对路径** |
| `scaffold` | 走的脚手架路径：空仓库为 `"init"`、非空仓库为 `"upgrade"`、目录已初始化时为 `"already_initialized"` |
| `committed` | 是否产生了 commit（工作区干净时为 `false`） |
| `pushed` | 是否 push 成功（工作区干净时为 `false`；commit 成功但 push 失败时为 `committed=true, pushed=false` 并带 `git_push` 错误） |
| `message` | 固定的成功提示文案 |

错误 `type` 取值随失败阶段不同：`validation`（参数 / 路径 / repository_url scheme 非 http(s)）、`dependency`（PATH 上找不到 `git` 或 `npx`）、`credential_init`（凭据签发失败或返回不可解析）、`git_clone` / `git_checkout` / `git_status` / `git_add` / `git_commit` / `git_push`（对应 git 步骤失败）、`git_ls_files`（探测空仓库时 `git ls-files` 失败）、`npx_app_init` / `npx_app_upgrade` / `npx_skills_sync`（对应 npx 脚手架步骤失败）、`meta_write`（读写 / 解析 `.spark/meta.json` 失败）。优先转述 `error.hint`，为空时退回 `error.message`。

## --dry-run 行为

预演只打印计划，不执行任何 git / npx 操作，字段包括：`credential_init`（将要跑的 `+git-credential-init` 命令）、`clone`、`checkout`、`scaffold`（空仓库 / 非空仓库两条脚手架路径的说明）、`commit_push`（条件说明）、`template`、`clone_path`。

- 若 `--dir`（或默认目录）已是初始化过的仓库（含 `.spark/meta.json`），dry-run 会额外置 `already_initialized: true`，提示真跑时会走 no-op。
- 若 `--dir` 校验失败（含控制字符）或目标目录已存在非空 / 是符号链接，dry-run **不会**直接报错退出，而是把拒绝原因放进 `dir_error` 字段（路径无法解析时 `clone_path` 退回默认值），并**仍以 exit 0 返回**。所以 dry-run 通过不代表真跑一定通过——要检查输出里有没有 `dir_error`。
- dry-run **不会**往 stderr 打 `→ ` 进度行（那些进度仅在真跑各步骤时输出）；它只打印计划本身。

## 前置条件与注意事项

- **`git` 和 `npx` 都必须在 PATH 上**：缺任一个都以结构化 `dependency` 错误退出（缺 `git` 时 hint 为 `install git and ensure it is on your PATH`；缺 `npx` 时提示安装 Node.js）。`npx` 用于跑妙搭脚手架（`@lark-apaas/miaoda-cli@alpha`）。
- **npx 脚手架**：clone + checkout 后，`+init` 在仓库内跑脚手架。**空仓库**跑 `npx @lark-apaas/miaoda-cli@alpha app init --template <tpl> --app-id <id>`（`scaffold:"init"`）；这里的「空」是 README 感知的：后端给新建应用仓库种了一个默认空 `README.md`，所以判定规则为——`git ls-files` **列不出任何文件**，**或仅列出根目录的 `README.md`**（精确匹配根目录 `README.md`，`docs/README.md`、`readme.md` 不算），即视为空仓库；除此之外任何被跟踪的文件都算**非空**。**非空仓库**跑 `npx ... app upgrade`，随后在 `.spark/meta.json` 存在且缺 `app_id` 时补上该字段（已存在则不动），再在缺 `.agent/skills/steering` 目录时跑 `npx ... skills sync`（`scaffold:"upgrade"`）。
- **依赖 `apps +git-credential-init`**：`+init` 通过 shell out 调用同一个 lark-cli 可执行文件去跑 `apps +git-credential-init --app-id <id> --format json`（设置了 `--as` 时会透传），从其 `data.repository_url` 取仓库地址，再用它 `git clone`。运行时若凭据签发失败或远端不可达，`+init` 在此步返回 `credential_init` 结构化错误。
- **commit message 固定**：push 时的 commit 主题是固定常量 `chore: scaffold app via lark-cli apps +init`，绝不拼接用户输入。脚手架后的自动 commit 跑 `git commit --no-verify`，以跳过脚手架模板可能携带的 pre-commit / commit-msg 钩子（仅跳过本地钩子；随后的 `git push` **不带** `--no-verify`）。
- **repository_url 仅接受 http(s)**：从 `+git-credential-init` 拿到的地址若不是 `http://` / `https://`（如 `ssh://`、`ext::`、`file://`）会被直接拒绝（`validation` 错误），以防危险的 git transport 与参数注入。
- **不要**原样把 envelope JSON 复述给用户。

## 协同命令

| 场景 | 命令 |
|---|---|
| 创建应用拿到 app_id | `apps +create` |
| 签发仓库凭据（`+init` 内部依赖） | `apps +git-credential-init` |

## 参考

- [lark-apps](../SKILL.md) — 妙搭应用全部命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
