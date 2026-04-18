# mail +template-*

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

飞书邮箱模板（Email Template）CRUD 命令集合，以及基于模板创建草稿/发送邮件的一键命令。本 skill 对应以下 6 个 shortcut：

- `lark-cli mail +template-list`
- `lark-cli mail +template-get`
- `lark-cli mail +template-create`
- `lark-cli mail +template-update`
- `lark-cli mail +template-delete`
- `lark-cli mail +template-send`

## 命令概览

| 命令 | 说明 | Risk | `--as` |
|------|------|------|--------|
| `+template-list` | 列出当前邮箱的所有邮件模板 | read | user |
| `+template-get` | 获取模板详情 | read | user |
| `+template-create` | 创建新模板 | write | user |
| `+template-update` | 更新已有模板（GET → merge → PUT） | write | user |
| `+template-delete` | 删除模板 | delete | user |
| `+template-send` | 基于模板创建草稿/发送邮件 | write | user |

## 参数

### `+template-list`

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mailbox <id>` | 否 | 邮箱 ID 或邮箱地址（默认 `me`） |

### `+template-get`

| 参数 | 必填 | 说明 |
|------|------|------|
| `--template-id <id>` | 是 | 模板 ID |
| `--mailbox <id>` | 否 | 邮箱 ID 或邮箱地址（默认 `me`） |

### `+template-create`

| 参数 | 必填 | 说明 |
|------|------|------|
| `--name <name>` | 是 | 模板名称（≤ 100 字符） |
| `--subject <subject>` | 否 | 邮件主题 |
| `--body <body>` | 否 | 邮件正文（HTML 或纯文本，自动识别）。该值会原样写入请求体 `template.body_html` 字段 |
| `--to <list>` | 否 | 默认收件人（逗号分隔，支持 `Name <email>` 格式） |
| `--cc <list>` | 否 | 默认抄送 |
| `--bcc <list>` | 否 | 默认密送 |
| `--plain-text` | 否 | 强制纯文本模式（`is_plain_text_mode=true`） |
| `--mailbox <id>` | 否 | 邮箱 ID（默认 `me`） |

### `+template-update`

| 参数 | 必填 | 说明 |
|------|------|------|
| `--template-id <id>` | 是 | 待更新的模板 ID |
| `--name`, `--subject`, `--body`, `--to`, `--cc`, `--bcc`, `--plain-text`, `--mailbox` | 否 | 仅传入的字段会覆盖现有值，省略的字段保持原样（CLI 层会先 GET 再 PUT） |

### `+template-delete`

| 参数 | 必填 | 说明 |
|------|------|------|
| `--template-id <id>` | 是 | 待删除的模板 ID |
| `--mailbox <id>` | 否 | 邮箱 ID（默认 `me`） |

### `+template-send`

| 参数 | 必填 | 说明 |
|------|------|------|
| `--template-id <id>` | 是 | 模板 ID |
| `--to <list>` | 否 | 覆盖模板默认 To。若模板默认收件人为空且未传该参数，命令会报错 |
| `--subject <subject>` | 否 | 覆盖模板主题 |
| `--body <body>` | 否 | 覆盖模板正文 |
| `--cc <list>` | 否 | 覆盖抄送 |
| `--bcc <list>` | 否 | 覆盖密送 |
| `--from <email>` | 否 | 发件人地址（默认使用当前邮箱的主地址） |
| `--confirm-send` | 否 | 立即发送（默认只保存草稿，需要 `mail:user_mailbox.message:send` scope） |
| `--mailbox <id>` | 否 | 邮箱 ID（默认 `me`） |

## 典型工作流

### 1. 查看我的模板

```bash
lark-cli mail +template-list --as user
```

### 2. 创建新模板

```bash
lark-cli mail +template-create --as user \
  --name "周报模板" \
  --subject "周报 - {{date}}" \
  --body "<h2>本周工作</h2><ul><li>...</li></ul>"
```

### 3. 用模板发邮件（默认保存为草稿）

```bash
# 先拿到 template_id
lark-cli mail +template-list --as user

# 基于模板创建草稿
lark-cli mail +template-send --as user \
  --template-id <id> \
  --to "alice@example.com"

# 审核草稿后，使用 --confirm-send 发送
lark-cli mail +template-send --as user \
  --template-id <id> \
  --to "alice@example.com" \
  --confirm-send
```

### 4. 部分更新模板

```bash
# 仅改名，其他字段保持不变
lark-cli mail +template-update --as user \
  --template-id <id> \
  --name "每周周报"

# 仅改收件人
lark-cli mail +template-update --as user \
  --template-id <id> \
  --to "hr@company.com"
```

### 5. 删除不用的模板

```bash
lark-cli mail +template-delete --as user --template-id <id>
```

## 返回值

**`+template-list`：**

```json
{
  "ok": true,
  "data": {
    "items": [
      {
        "template_id": "<id>",
        "name": "周报模板",
        "subject": "周报 - {{date}}",
        "body_html": "<h2>本周工作</h2>...",
        "create_time": "1700000000000"
      }
    ]
  }
}
```

**`+template-send`（默认，仅创建草稿）：**

```json
{
  "ok": true,
  "data": {
    "draft_id": "<draft_id>",
    "template_id": "<id>",
    "tip": "draft saved from template. To send: lark-cli mail user_mailbox.drafts send --params ..."
  }
}
```

**`+template-send` + `--confirm-send`：**

```json
{
  "ok": true,
  "data": {
    "message_id": "<message_id>",
    "thread_id": "<thread_id>",
    "template_id": "<id>"
  }
}
```

## 请求体格式（CREATE / UPDATE）

CLI 在发出 POST/PUT 请求时会将模板字段包裹在 `template` 下，以对齐 IDL `api.body="template"` 契约。地址数组字段 JSON key 为 `tos`/`ccs`/`bccs`（CLI 侧仍用 `--to`/`--cc`/`--bcc`）。

```json
{
  "template": {
    "name": "周报模板",
    "subject": "周报 - {{date}}",
    "body_html": "<h2>本周工作</h2><ul><li>...</li></ul>",
    "is_plain_text_mode": false,
    "tos": [{"mail_address": "alice@example.com", "name": "Alice"}],
    "ccs": [{"mail_address": "bob@example.com"}],
    "bccs": []
  }
}
```

## 注意事项

- **身份**：模板操作仅支持 `--as user`（UAT）。Bot 身份（TAT）暂不支持。
- **数量上限**：每个用户最多创建 20 个模板；超限时 Open API 会返回错误码 `150802 TEMPLATE_NUMBER_LIMIT`。
- **名称长度**：模板名称最长 100 字符（按 rune 计数，中文也按 1 个字符算）。
- **正文大小**：单个模板 ≤ 3 MB；所有模板合计 ≤ 50 MB。
- **更新语义**：`+template-update` 在 CLI 层做 GET → merge → PUT，省略的字段保持原值；在后端实际上是全量替换。
- **发送默认草稿**：`+template-send` 默认只保存草稿，需要 `--confirm-send` 才会调用 `drafts.send`，这是为了保护 AI Agent 误发邮件。
- **权限**：
  - 只读（list/get）：`mail:user_mailbox.message:readonly`
  - 写（create/update/delete）：`mail:user_mailbox.message:modify`
  - 发送（`+template-send --confirm-send`）还需要 `mail:user_mailbox.message:send`
