# mail +read-shared-card

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

使用 IM 分享邮件卡片中的 `ownerId` 和 `cardId` 读取卡片授权范围内的邮件。这个命令只支持用户身份；即使当前用户没有邮箱账户，只要仍具备同租户的卡片访问权限，也可以读取卡片指向的邮件。

服务端会在每次详情和附件 URL 请求中重新校验卡片权限。不要把已经解析出的 `message_id` 改用于普通邮箱查询链路，也不要缓存或记录返回的短时效附件下载 URL。

## 命令

```bash
# 默认返回适合快速查看的摘要
lark-cli mail +read-shared-card --as user \
  --owner-id <ownerId> --card-id <cardId>

# 返回完整邮件详情
lark-cli mail +read-shared-card --as user \
  --owner-id <ownerId> --card-id <cardId> --message-format full

# 显式请求附件下载 URL
lark-cli mail +read-shared-card --as user \
  --owner-id <ownerId> --card-id <cardId> --message-format full \
  --include-attachment-download-urls
```

## 参数

| 参数 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `--owner-id <ownerId>` | 是 | — | IM 分享邮件卡片携带的 ownerId |
| `--card-id <cardId>` | 是 | — | IM 分享邮件卡片携带的 cardId |
| `--message-format <mode>` | 否 | `summary` | `summary` / `id` / `metadata` / `full` / `plain_text_full` |
| `--include-attachment-download-urls` | 否 | `false` | 为有附件的邮件请求短时效下载 URL；无附件邮件返回空数组 |
| `--dry-run` | 否 | — | 仅展示请求计划，不执行 |

## 返回值

- `id`：只返回卡片授权解析出的 `message_ids`。
- `summary`：返回 `message_id` 和邮件正文摘要 `body_preview`。
- `metadata` / `full` / `plain_text_full`：返回卡片详情接口对应格式的 `messages`。
- 开启附件 URL 后，每条邮件附带 `attachment_download_urls` 和 `attachment_failed_ids`；单个附件无法生成 URL 时保留在该邮件的失败列表中。
- 不开启附件 URL 时不会调用附件 URL 接口，也不会在输出中附加签名 URL。

API 返回的权限、租户、卡片失效、邮件不存在和附件不存在错误会保留服务端错误码、消息及请求日志 ID，便于诊断。

## 相关命令

- `lark-cli mail +message` — 已知且有普通邮箱权限时读取单封邮件
- `lark-cli mail +messages` — 已知且有普通邮箱权限时批量读取多封邮件
- `lark-cli mail +share-to-chat` — 将邮件或会话分享到 IM
