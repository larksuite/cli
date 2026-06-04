---
name: lark-shared
version: 1.0.0
description: "Use when first setting up lark-cli, running auth login, switching user/bot identity (--as), handling permission denied or scope errors, needing to update lark-cli, or seeing _notice in JSON output."
---

# lark-cli 共享规则

本技能指导你如何通过lark-cli操作飞书资源, 以及有哪些注意事项。

## 配置初始化

首次使用需运行 `lark-cli config init` 完成应用配置。

当你帮用户初始化配置时，使用background方式使用下面的命令发起配置应用流程，启动后读取输出，从中提取授权链接并发给用户。

**URL 转发规则**：当命令输出 `verification_url`、`verification_uri_complete`、`console_url` 等 URL 字段时：**必须生成二维码**：你必须调用 `lark-cli auth qrcode` 将 URL 转为二维码并展示给用户，这是必须步骤，不要跳过。优先生成 PNG 二维码（--output）；仅当用户明确要求时才使用 ASCII（--ascii）。**URL 输出规则**：将 URL 视为不可修改的 opaque string，不要做任何修改（包括 URL 编码/解码、添加空格或标点、重新拼接 query），二维码和链接请一起展示给用户。

```bash
# 发起配置（该命令会阻塞直到用户打开链接并完成操作或过期）
lark-cli config init --new
```

## 认证

### 身份类型

两种身份类型，通过 `--as` 切换：

| 身份 | 标识 | 获取方式 | 适用场景 |
|------|------|---------|---------|
| user 用户身份 | `--as user` | `lark-cli auth login` 等 | 访问用户自己的资源（日历、云空间/云盘/云存储等） |
| bot 应用身份 | `--as bot` | 自动，只需 appId + appSecret | 应用级操作,访问bot自己的资源 |

### 身份选择原则

输出的 `[identity: bot/user]` 代表当前身份。bot 与 user 表现差异很大，需确认身份符合目标需求：

- **Bot 看不到用户资源**：无法访问用户的日历、云空间（云盘/云存储）文档、邮箱等个人资源。例如 `--as bot` 查日程返回 bot 自己的（空）日历
- **Bot 无法代表用户操作**：发消息以应用名义发送，创建文档归属 bot
- **Bot 权限**：只需在飞书开发者后台开通 scope，无需 `auth login`
- **User 权限**：后台开通 scope + 用户通过 `auth login` 授权，两层都要满足


### 权限不足处理

遇到权限相关错误时，**根据当前身份类型采取不同解决方案**。

错误响应中包含关键信息：
- `permission_violations`：列出缺失的 scope (N选1)
- `console_url`：飞书开发者后台的权限配置链接
- `hint`：建议的修复命令

#### Bot 身份（`--as bot`）

将错误中的 `console_url` 原样提供给用户，引导去后台开通 scope。**禁止**对 bot 执行 `auth login`。

#### User 身份（`--as user`）

```bash
lark-cli auth login --domain <domain>           # 按业务域授权
lark-cli auth login --scope "<missing_scope>"   # 按具体 scope 授权（推荐,符合最小权限原则）
```

**规则**：auth login 必须指定范围（`--domain` 或 `--scope`）。多次 login 的 scope 会累积（增量授权）。

#### Agent 代理发起认证（推荐）

当你作为 AI agent 需要帮用户完成认证时，优先使用 split-flow，避免在同一轮对话中阻塞等待用户授权：

```bash
# 发起授权（立即返回 device_code 和 verification_url）
lark-cli auth login --scope "calendar:calendar:readonly" --no-wait --json
```

拿到 `verification_url` 后，将它原样作为本轮最终消息发给用户，并结束本轮/交还控制权。不要在同一轮中展示 URL 后立刻执行 `--device-code` 阻塞轮询；在不透传中间输出的 agent harness 里，这会导致用户永远看不到 URL。

用户回复已完成授权后，再在后续步骤执行：

```bash
lark-cli auth login --device-code <device_code>
```

**Split-Flow 完整步骤**：

**第一步：发起授权（当前轮）**

1. 执行 `lark-cli auth login --scope "xxx" --no-wait --json`（必须加 `--no-wait --json`）
2. 从 JSON 输出中提取 `verification_url` 和 `device_code`
3. 生成二维码：`lark-cli auth qrcode <verification_url> --output "xxx"`
4. 将 URL 和二维码展示给用户（先 URL，后二维码）
5. **结束本轮对话前，必须明确告知用户**："请完成授权后，回来告诉我已授权完成，我会帮你完成后续步骤"

**第二步：完成授权（后续轮）**

1. 等待用户回复"已完成授权"
2. **由你（AI agent）亲自执行**：`lark-cli auth login --device-code <device_code>`
3. 此命令会轮询授权状态并完成登录
4. 如果返回授权成功，流程结束

**关键规则**：

- **你必须亲自执行 `--device-code` 命令**，不要指示用户自行执行
- **不要在同一轮中展示 URL 后立刻执行 `--device-code`**，这会导致用户看不到 URL
- **禁止缓存 `verification_url` 或 `device_code`**：每次需要授权时，必须重新执行 `lark-cli auth login --no-wait --json` 生成新的链接。不要将授权链接和 device code 存入上下文供后续复用

#### `auth login --json` 输出契约（NDJSON）

`auth login --json` 输出的是 **NDJSON**（每行一个 JSON 对象）—— **只解析最后一条非空行作为成功载荷**。三种调用路径下输出形态如下：

| 调用 | 输出行（顺序） |
|---|---|
| `--no-wait --json` | 1 行：`{verification_url, device_code, expires_in, hint}`（**无 `event` 字段**） |
| `--device-code <code> --json` | 1 行：`{event: "authorization_complete", ...}` |
| `--json`（同步阻塞，罕见） | 2 行：先 `{event: "device_authorization", verification_uri, verification_uri_complete, user_code, expires_in, agent_hint}`，再 `{event: "authorization_complete", ...}` |

`authorization_complete` 行的关键字段：

- `event`（恒为 `"authorization_complete"`）
- `user_open_id` —— 实际授权的 open_id（**不是** profile 上的 active user）
- `user_name` —— 实际授权身份的 display name
- `scope` —— 单数，本次最终授予的 scope 列表，空格分隔字符串
- `requested` / `granted` / `newly_granted` / `already_granted` / `missing` —— 数组（永不为 `null`，无内容时为 `[]`）；`requested` 是本次请求的 scope，`granted` 是 `scope` 字段的数组形式，`newly_granted` 是与上次登录相比新拿到的，`already_granted` 是延续的，`missing` 是请求了但被拒的
- 可选 `warning` —— scope 不足软告警，schema 见下文
- 可选 `holder_mismatch_warning` —— 多用户软告警，schema 见下文

授权失败会发 `{event: "authorization_failed", error}`。

只看最后一行就够了：split-flow 第一步只有 `--no-wait` 那一行；第二步只有 `authorization_complete`；同步路径下最后一行也是 `authorization_complete`。

##### `holder_mismatch_warning` 字段（多用户软告警）

当存在以下条件**全部成立**时，`authorization_complete` 会带 `holder_mismatch_warning`：

- 用户**没有**通过 `--user` / `LARKSUITE_CLI_OPEN_ID` 显式指定目标身份
- 当前 profile 的 `currentUser`（来自上次 `auth login` 或 `auth users use`）与本次设备授权的 open_id 不一致

这是**软告警，不是错误**：本次登录会成功，新身份会追加到 profile 的 `Users[]`，但 active user **不会**自动切换。要切换 active user，运行 `lark-cli auth users use <open_id>`。

干净登录（首次登录或 currentUser 与本次授权的身份相同）**完全不会**有这个 key —— consumer 应通过 key 是否存在判断（不要 nil-check）。

字段 schema：

```json
{
  "type": "holder_currentuser_mismatch",
  "message": "[lark-cli] [WARN] auth login: ...",
  "holder_open_id": "ou_alice",
  "holder_user_name": "Alice",
  "fresh_open_id": "ou_bob",
  "fresh_user_name": "Bob"
}
```

- `type`：discriminator，与现有 scope `warning.type`（`missing_scope`）共用 schema 形状但取值独立。`holder_currentuser_mismatch` 是当前唯一取值；后续若新增 holder 告警子型，会用 `holder_*` 前缀复用同一字段。
- `message`：人类可读 stderr WARN 文本的副本，**已经过 terminal escape 净化**（没有 ANSI / C0 控制字节）。
- `holder_*` / `fresh_*` 字段：**未净化的原始字节**（按 JSON 消费者契约，由消费者自行 escape）。`holder_*` 是 profile 中保留的 active user，`fresh_*` 是这次刚授权的身份。

scope 告警（`warning`，type=`missing_scope`）和 holder 告警（`holder_mismatch_warning`，type=`holder_currentuser_mismatch`）是**两个独立的 key**，互不影响。一次登录可以同时触发两者；分别 branch 即可。

stderr WARN 与这个 JSON 字段是**双通道**：在 JSON 模式下 stderr WARN **仍然会输出**，方便人 tail `2>&1`。

## 更新检查

lark-cli 命令执行后，如果检测到新版本，JSON 输出中会包含 `_notice.update` 字段（含 `message`、`command` 等）。

**当你在输出中看到 `_notice.update` 时，完成用户当前请求后，主动提议帮用户更新**：

1. 告知用户当前版本和最新版本号
2. 提议执行更新（同时更新 CLI 和 Skills）：
   ```bash
   lark-cli update
   ```
3. 更新完成后提醒用户：**退出并重新打开 AI Agent** 以加载最新 Skills

**重要**：始终使用 `lark-cli update` 更新，它会同时更新 CLI 和 AI Skills。

**规则**：不要静默忽略更新提示。即使当前任务与更新无关，也应在完成用户请求后补充告知。

## 安全规则

- **禁止输出密钥**（appSecret、accessToken）到终端明文。
- **写入/删除操作前必须确认用户意图**。
- 用 `--dry-run` 预览危险请求。

## 高风险操作的审批协议（exit 10）

lark-cli 对高风险写操作（`risk: "high-risk-write"`）有强制确认门禁。当你不带 `--yes` 调用这类命令时，CLI 会退出码 `10`、并在 stderr 返回如下结构化 envelope：

```json
{
  "ok": false,
  "error": {
    "type": "confirmation_required",
    "message": "drive +delete requires confirmation",
    "hint": "add --yes to confirm",
    "risk": {
      "level": "high-risk-write",
      "action": "drive +delete"
    }
  }
}
```

**遇到这种情况，不要当普通错误放弃。** 按以下流程处理：

1. **识别**：看到子进程 exit code = `10` 且 stderr JSON 里 `error.type == "confirmation_required"`
2. **向用户确认**：把 `error.risk.action` 和关键参数展示给用户，明确告知"这是高风险操作"，等待用户显式同意
3. **用户同意** → 在你**原始 argv 的末尾追加 `--yes`** 后重试
4. **用户拒绝** → 终止流程，不要擅自改写参数或跳过门禁

**绝对不允许**：
- 看到 exit 10 就默认加 `--yes` 静默重试（这等于禁用门禁）
- 把 `confirmation_required` 当网络错误/权限错误处理
- 在用户没明确同意的前提下追加 `--yes` 重试
- 用 `sh -c` 等 shell 方式拼接命令重试——用 `exec.Command(argv...)` 参数数组形式，避免 shell 解析把用户参数当作语法

提前预判：想先让用户 review 危险操作的具体请求，调用时加 `--dry-run`——它不触发门禁，会打印完整请求详情（URL / body / params），你可以把这个预览给用户看过再去真正执行。

### 如何识别一条命令是高风险

- shortcut：`lark-cli <service> +<cmd> --help` 顶部会显示 `Risk: high-risk-write`
- service 命令：`lark-cli schema <service>.<resource>.<method> --format json` 的返回值里 `"risk": "high-risk-write"`
