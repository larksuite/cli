# apps +env-pull

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)（认证 / 全局参数 / 安全）。

把妙搭应用的启动期环境变量拉取到本地项目根的 `.env.local`。**通常不需要手动跑**——脚手架的 `npm run dev` 在起本地开发时会**自动后台拉取**（非阻塞）。本命令是给以下兜底场景用的：

- 开发期间
- `.env.local` 被改坏 / 删除，想重新同步
- 不通过 `npm run dev` 启动（如直接跑 `node` / IDE debug）

## 命令

```bash
# 在项目根目录里调（默认写当前目录的 .env.local）
lark-cli apps +env-pull --app-id app_xxx

# 显式指定项目根
lark-cli apps +env-pull --app-id app_xxx --project-path /Users/me/code/my-app

# pretty 输出
lark-cli apps +env-pull --app-id app_xxx --format pretty

# 预演（打印请求结构，不写文件、不调 API）
lark-cli apps +env-pull --app-id app_xxx --dry-run
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `--app-id <id>` | ✅ | — | 妙搭应用 ID |
| `--project-path <path>` | ❌ | 当前工作目录 | 项目根路径，env 写到 `<project-path>/.env.local`。仅做控制字符校验和 `filepath.Clean`，接受绝对 / 相对路径 |

身份固定 `--as user`；scope `spark:app:read`。

## 写入行为（合并 `.env.local`，不会覆盖手写内容）

- **目标文件**：`<project-path>/.env.local`，权限 `0o600`，原子写（temp file + rename）。父目录不存在时以 `0o755` 创建。
- **目标必须是常规文件**：是符号链接 / 设备 / 命名管道时直接拒绝（防止跟随 symlink 写到非预期位置）。不存在则视为空内容。
- **合并算法（关键）**：
  - 按行解析原文件，**保留空行与 `#` 注释**。
  - 支持 `export KEY=...` 前缀（含 `\t` 分隔）；其他不可识别行原样保留。
  - 后端返回的 key **命中本地某行 → 就地替换**该行的值。
  - 后端返回但本地没有的 key → **追加到末尾，按 key 名升序排列**。
  - 输出统一为 `KEY="value"`（用 `strconv.Quote` 转义 `\` 与 `"`）。
  - 输入 `\r\n` 规范化为 `\n`，文件结尾保证一个 `\n`。
- **key 白名单**：`^[A-Za-z_][A-Za-z0-9_]*$`。不合法的 key（如带连字符 / 中文）会被跳过并在 pretty 输出里告警，**不会污染** `.env.local`。

## 返回值

**成功（默认 JSON envelope）：**

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "app_id": "app_xxx",
    "env_file": "/abs/path/to/my-app/.env.local",
    "database_url_expires_at": "1780389006"
  }
}
```

> envelope **不会**回显任何 env key / value（防止 token / 数据库凭据泄漏到日志或 CI 输出）。要看实际值请直接读 `.env.local`。

**成功（`--format pretty`）：**

```
✓ App detected: app_xxx
✓ Development database detected
✓ Local environment written to /abs/path/to/my-app/.env.local

DATABASE_URL is valid until 2026-06-04 18:30:06 CST.
Run `lark-cli apps +env-pull --app-id <app_id>` again to refresh it.
```

不合法的 key 会额外打一行：

```
⚠ Skipped 2 invalid key(s): foo-bar, 中文 (key names must match [A-Za-z_][A-Za-z0-9_]*)
```

**失败（结构化 envelope）：**

| `error.type` / `subtype` | 触发场景 |
|---|---|
| `validation` / `invalid_argument` — `--app-id is required` | 没传 `--app-id` |
| `validation` / `invalid_argument` — `--project-path: ...` | 路径含控制字符 |
| `validation` / `invalid_argument` — `target ... must be a regular file` / `not a symlink` / `cannot inspect ...` | `.env.local` 是符号链接 / 非常规文件 / lstat 失败 |
| `validation` / `invalid_response` — `response field env_vars must be an object or array of key/value entries` | 后端返回结构异常 |
| `internal` / `unknown` — `cannot read/create/write ...` | 本地 IO 失败 |
| `missing_scope` | 没拿到 `spark:app:read`，按 lark-shared 引导 `lark-cli auth login --domain apps` |

优先转述 `error.hint` / `error.message`，**不要原样把 envelope JSON 复述给用户**。

## 字段语义

| 字段 | 含义 |
|---|---|
| `app_id` | 透传的应用 ID |
| `env_file` | 实际写入的 `.env.local` 绝对路径 |
| `database_url_expires_at` | **可选**，仅当后端返回 `SUDA_DATABASE_URL` 且其 `extras.expiresAt` 是合法 unix 时间戳时存在；值是字符串形式的 unix 秒。pretty 模式额外把它转成本地时区可读时间 |

## --dry-run 行为

仅打印将要请求的 `POST {api}/apps/{app_id}/env_vars`、目标 `project_path` / `env_file`，**不调 API、不写文件**。可用于在没登录或没 scope 的环境下验证参数与目标路径是否符合预期。

## 何时不要用这个命令

- 用户已经在用 `npm run dev` 起本地开发：env 由脚手架后台自动拉，跑这条只会**重复做同样的事**，并在用户刚改完 `.env.local` 时把临时改动覆盖掉。

## 参考

- [lark-apps](../SKILL.md) — 妙搭应用全部命令 + 心智模型
- [lark-apps-local-dev](lark-apps-local-dev.md) — 本地全栈开发端到端流程
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
