# mail +draft-send

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

发送一个或多个已经存在的邮件草稿。此命令只适用于用户已经明确确认要发送这些草稿的场景。

`+draft-send` 会按顺序调用每个草稿的发送接口，并把每个草稿的成功或失败结果聚合输出。它不会创建或编辑草稿；需要新建草稿时使用 `+send` 或 `+draft-create`，需要修改草稿时使用 `+draft-edit`。

## 安全约束

- **必须先获得用户明确确认**，再执行发送。不要因为草稿已存在就自动发送。
- 默认继续处理后续草稿并聚合失败；传 `--stop-on-error` 后遇到第一个可恢复失败即停止。
- 认证、权限、网络或邮箱级配额这类致命错误会立即中止，不会重复发送后续草稿。
- 仅支持 user 身份。草稿是用户邮箱资源，bot 身份没有一致语义。

## 命令

```bash
# 发送单个草稿
lark-cli mail +draft-send --draft-id <draft_id> --yes

# 批量发送多个草稿
lark-cli mail +draft-send --draft-id <draft_id_1>,<draft_id_2> --yes

# 重复传 flag
lark-cli mail +draft-send --draft-id <draft_id_1> --draft-id <draft_id_2> --yes

# 预览将要发送的草稿
lark-cli mail +draft-send --draft-id <draft_id_1>,<draft_id_2> --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--draft-id <ids>` | 是 | 草稿 ID 列表，支持逗号分隔或重复传 flag。最多 50 个。 |
| `--mailbox <email>` | 否 | 草稿所属邮箱，默认 `me`。使用公共邮箱草稿时传对应邮箱地址。 |
| `--stop-on-error` | 否 | 遇到第一个可恢复的单草稿失败后停止。默认继续发送后续草稿并聚合结果。 |
| `--yes` | 是 | 高风险写操作确认。发送草稿前必须显式传入。 |
| `--dry-run` | 否 | 仅打印将要调用的发送请求，不执行。 |
| `--format <mode>` | 否 | 输出格式：`json`（默认）/ `pretty` / `table` / `ndjson` / `csv`。 |

## 返回值

全部成功时：

```json
{
  "ok": true,
  "data": {
    "mailbox_id": "me",
    "total": 2,
    "success_count": 2,
    "failure_count": 0,
    "sent": [
      {"draft_id": "draft_1", "message_id": "message_1"},
      {"draft_id": "draft_2", "message_id": "message_2"}
    ]
  }
}
```

部分失败时，输出为结构化的 partial failure envelope，`sent` 中保留已发送成功的草稿，`failed` 中列出失败草稿和错误原因。不要把部分失败当作全部失败重试；重试前先确认 `sent` 中的草稿已经发出。

## 典型场景

### 用户确认发送已有草稿

```bash
lark-cli mail +draft-send --draft-id <draft_id> --yes
```

发送后向用户报告 `message_id`。如果需要确认投递状态，再调用：

```bash
lark-cli mail user_mailbox.messages send_status \
  --params '{"user_mailbox_id":"me","message_id":"<message_id>"}'
```

### 批量发送草稿

```bash
lark-cli mail +draft-send --draft-id draft_1,draft_2,draft_3 --yes
```

如输出包含 `failed`，只针对失败草稿继续处理；不要重复发送 `sent` 中已经成功的草稿。

## 相关命令

- `lark-cli mail +send` — 创建新邮件草稿，或在用户明确确认时直接发送。
- `lark-cli mail +draft-create` — 从零创建一封新草稿。
- `lark-cli mail +draft-edit` — 编辑已有草稿。
- `lark-cli mail user_mailbox.drafts send` — 原生单草稿发送 API。
