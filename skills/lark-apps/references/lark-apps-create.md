# apps +create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建一个新的妙搭应用：服务端拉脚手架 + 初始化 git + 接好妙搭远端，返回新建应用的元信息（含 `app_id` 与 git 仓库地址）。**所有类型共用这套创建流程**，无 cloud / local 模式之分。

## 命令

```bash
# HTML 托管类（最小调用）
lark-cli apps +create --name "客户调研问卷" --app-type HTML

# 全栈应用（fullstack 必带 --message，原样透传用户需求原话）
lark-cli apps +create --name "审批系统" --app-type full_stack \
  --message "做一个部门审批系统，支持登录、提交申请、多级审批、状态查询"

# Dry-run（仅打印请求，不执行）
lark-cli apps +create --name "Demo" --app-type HTML --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--name <str>` | ✅ | 应用显示名 |
| `--app-type <enum>` | ✅ | 应用类型：`HTML`（静态托管）/ `fullstack`（全栈，React+Nest+PG）。区分大小写；以 `--help` 为准 |
| `--enable-multi-env-db <bool>` | ❌ | **创建时即开启多环境数据库（推荐）**，开了之后本地就能用 `+db-*` 命令调库，无需再单独 `+db-multi-env-init`。仅**存量应用**才需事后手动开启。flag 名 / 默认值待接口确定 |
| `--description <str>` | ❌ | 应用描述 |
| `--icon-url <url>` | ❌ | 应用图标 URL；不传服务端给默认图标 |

## 返回值

**成功：**

```json
{
  "ok": true,
  "data": {
    "app": {
      "app_id": "app_4k5jepcbjmv6m",
      "name": "客户调研问卷",
      "description": "本季度客户满意度调研",
      "icon_url": "https://lf3-static.bytednsdoc.com/.../feisuda/avatar/5.svg",
      "created_at": "2026-05-18T10:00:00Z"
    }
  }
}
```

**失败：**

```json
{
  "ok": false,
  "error": {
    "type": "api",
    "code": 99991400,
    "message": "...",
    "hint": "可执行的修复建议（可能为空）"
  }
}
```

## 字段语义

- `app_type` 是应用类型枚举，**区分大小写**，当前支持 `html` 和 `full_stack`
- `created_at` 是 ISO 8601 UTC 时间字符串
- `error.hint` 是 CLI 给出的可执行修复建议，**优先**转述给用户；hint 为空时退回 `error.message`
- 不要原样把 envelope JSON 复述给用户

## 意图识别：HTML vs fullstack

调 `+create` 前先按用户描述判类型：

| 用户信号 | 判定 | `--app-type` |
|---------|------|-------------|
| 纯静态展示：HTML / PPT / 幻灯片 / 单页 / 静态站点 / Web demo（无后端逻辑） | 静态页面 | `HTML` |
| 需要后端能力：数据库 / 登录鉴权 / API / 表单存储 / 用户系统 / 增删改查 / 持久化 / 多人协作 / "全栈" / "带后台" | 全栈应用 | `full_stack` |
| 模糊不清、无明显信号 | 默认 `HTML`（更轻、现有成熟流程），必要时追问一句澄清 | `HTML` |

判定类型后：从用户的自然语言输入**生成**一个简洁的 `name` 和一句 `description`，通过 `--name` / `--description` 传入（HTML 与 fullstack 都适用），不要求用户显式给出应用名。

## 典型场景

### 场景 1：用户要发布 HTML / 静态网站

`--app-type html`，建完走 [`lark-apps-html-publish.md`](lark-apps-html-publish.md)：

```bash
lark-cli apps +create --name "X" --app-type html
```

向用户报告：

> 应用「{name}」已创建（ID: `{app_id}`）。接下来用 `apps +html-publish --app-id {app_id} --path <你的 HTML 目录>` 发布内容。

### 场景 2：用户要在本地做全栈开发

`--app-type fullstack`（**必带 `--message`** = 用户需求原话），建完走 [`lark-apps-local-dev.md`](lark-apps-local-dev.md)（配 git 凭证 → clone → `npm run dev` 起本地开发（自动拉 env）→ 编码 → publish）：

```bash
lark-cli apps +create --name "审批系统" --app-type full_stack \
  --message "做一个部门审批系统，支持登录、提交申请、多级审批、状态查询"
```

> 应用「{name}」已创建（ID: `{app_id}`，git 仓库：`{repo_url}`）。下一步配置 git 凭证：`apps +git-credential-init --app-id {app_id}`。

### 场景 3：用户想让云端直接生成

同样 `--app-type fullstack --message "<需求原话>"`（这一步服务端已触发首轮生成），但用户**不在本地写代码** → 走 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md) 轮询拿 `app_url`，而不是配 git 凭证。

> 场景 2 与 3 的 `+create` 调用相同，差别只在 create 之后：本地开发（配凭证 + clone）还是云端轮询。这正是 [`../SKILL.md`](../SKILL.md)「输入模糊」里要问用户的那一步。

### 场景 4：失败处理

转述 `error.hint`（优先）或 `error.message`，**不要**原样输出 envelope JSON。

## 协同命令

| 场景 | 命令 |
|---|---|
| 修改应用名 / 描述 | `apps +update` |
| 发布 HTML | `apps +html-publish --path <目录或文件>` |
| 本地全栈开发 | `apps +git-credential-init` → 原生 git → `apps +publish`（见 [local-dev](lark-apps-local-dev.md)） |
| 云端会话生成 | `apps +session-create --message`（见 [cloud-dev](lark-apps-cloud-dev.md)） |
| 拿现有应用 ID | 从用户提供的妙搭应用链接 `https://miaoda.feishu.cn/app/app_xxx` 的 `/app/` 后面提取，或让用户直接给 `app_xxx` 字符串（详见 `../SKILL.md`） |

## 参考

- [lark-apps](../SKILL.md) — 妙搭应用全部命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
