# apps +create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建一个新的妙搭应用。一次 `POST /apps` 调用，返回新建应用的元信息。

## 命令

```bash
# 最小调用
lark-cli apps +create --name "客户调研问卷" --app-type HTML

# 全参数
lark-cli apps +create \
  --name "客户调研问卷" \
  --app-type HTML \
  --description "本季度客户满意度调研" \
  --icon-url "https://lf3-static.bytednsdoc.com/.../feisuda/avatar/5.svg"

# Dry-run（仅打印请求，不执行）
lark-cli apps +create --name "Demo" --app-type HTML --dry-run

# 创建 fullstack（全栈）应用：从用户描述生成 name/description
lark-cli apps +create --name "团队任务看板" --app-type fullstack \
  --description "支持登录、任务增删改查、按人筛选的团队任务看板"
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--name <str>` | ✅ | 应用显示名 |
| `--app-type <enum>` | ✅ | 应用类型，可选值：`HTML` 或 `fullstack`（均区分大小写） |
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
    "type": "api_error",
    "code": "api_error",
    "message": "...",
    "hint": "可执行的修复建议（可能为空）"
  }
}
```

## 字段语义

- `app_type` 是应用类型枚举，**区分大小写**，当前支持 `HTML` 和 `fullstack`（两者均大小写敏感精确匹配，不在白名单的取值 CLI 端直接拒绝）
- `created_at` 是 ISO 8601 UTC 时间字符串
- `error.hint` 是 CLI 给出的可执行修复建议，**优先**转述给用户；hint 为空时退回 `error.message`
- 不要原样把 envelope JSON 复述给用户

## 意图识别：HTML vs fullstack

在调用 `+create` 前，先根据用户描述判断应用类型：

| 用户信号 | 判定 | `--app-type` |
|---------|------|-------------|
| 纯静态展示：HTML、PPT、幻灯片、单页、静态站点、Web demo（无后端逻辑） | 静态页面 | `HTML` |
| 需要后端能力：数据库、登录鉴权、API、表单提交存储、用户系统、增删改查、持久化、服务端逻辑、"全栈"、"带后台 / 后台管理" | 全栈应用 | `fullstack` |
| 模糊不清、无法判断 | 默认 `HTML`（更轻、且为现有成熟流程），必要时追问一句澄清 | `HTML` |

判定类型后：从用户的自然语言输入**生成**一个简洁的 `name` 和一句 `description`，通过 `--name` / `--description` 传入（HTML 与 fullstack 都适用），不要求用户显式给出应用名。

## 典型场景

### 场景 1：用户说"创建一个妙搭应用，名字叫 X"

先按意图识别判断类型。若用户未表达后端需求（纯展示/静态）或意图模糊，默认用 `--app-type HTML`（更轻、且为现有成熟流程）：

```bash
lark-cli apps +create --name "X" --app-type HTML
```

向用户报告：

> 应用「{name}」已创建（ID: `{app_id}`）。

可选建议下一步：

> 接下来用 `apps +html-publish --app-id {app_id} --path <你的 HTML 目录>` 发布内容。

### 场景 2：用户提供完整元信息

```bash
lark-cli apps +create --name "Q4 调研" --app-type HTML --description "..."
```

返回后同场景 1。

### 场景 3：创建 fullstack 应用

识别到用户需要全栈/带后端能力的应用后（如需要登录、数据库、增删改查、用户系统等），使用 `--app-type fullstack`，并从用户描述生成 `name` 和 `description`：

```bash
lark-cli apps +create --name "团队任务看板" --app-type fullstack \
  --description "支持登录、任务增删改查、按人筛选的团队任务看板"
```

向用户报告：

> 全栈应用「{name}」已创建（ID: `{app_id}`）。

> ⚠️ **注意**：fullstack 应用创建后续的本地开发链路（git 凭据初始化 + git clone）**待 `+git-credential-init` 命令就绪后补充**，当前版本到 `+create` 为止。

### 场景 4：失败处理

转述 `error.hint`（优先）或 `error.message`，**不要**原样输出 envelope JSON。

## 协同命令

| 场景 | 命令 |
|---|---|
| 修改应用名 / 描述 | `apps +update` |
| 发布 HTML | `apps +html-publish` |
| 拿现有应用 ID | 从用户提供的妙搭应用链接 `https://miaoda.feishu.cn/app/app_xxx` 的 `/app/` 后面提取，或让用户直接给 `app_xxx` 字符串（详见 `../SKILL.md`） |

## 参考

- [lark-apps](../SKILL.md) — 妙搭应用全部命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
