# mail +draft-send

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

发送一封或多封已有草稿。逐封调用 `POST /drafts/:draft_id/send`，单封失败不影响其余草稿，结果按成功/失败聚 合返回。高危写操作，必须传 `--yes`。

## 何时使用

- 草稿已经创建好（来自 `+draft-create` / `+send` / `+reply` / `+forward` 的草稿模式，或飞书客户端），用户明确确认后发送。
- 需要在发送前覆盖草稿的「分别发送」设置时，使用 `--send-separately`（见下）。

## 命令

```bash
# 发送单封草稿（用户确认后）
lark-cli mail +draft-send --draft-id <draft-id> --yes

# 批量发送（逗号分隔或重复 --draft-id，上限 50）
lark-cli mail +draft-send --draft-id <id1>,<id2> --yes

# 发送并覆盖「分别发送」设置（先保存新值再发送）
lark-cli mail +draft-send --draft-id <draft-id> --send-separately true --yes
lark-cli mail +draft-send --draft-id <draft-id> --send-separately false --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--draft-id <ids>` | 是 | 草稿 ID，逗号分隔或重复传 flag（上限 50，不允许空值/重复） |
| `--mailbox <email>` | 否 | 草稿所属邮箱（默认 `me`） |
| `--send-separately <bool>` | 否 | 发送前覆盖每封草稿的分别发送设置：`true` 开启、`false` 显式取消。每封草稿先保存新值、成功后才发送（save-then-send）；保存失败的草稿**不会**被发送，计入该封的 failed 条目。省略时不做任何额外调用，直接沿用服务端已保存的设置。非法值在任何调用前被拒绝（退出码 2） |
| `--stop-on-error` | 否 | 首个可恢复的单封失败即停止（默认继续并聚合）。认证/权限/网络/邮箱级配额等致命错误总是立即中止 |
| `--dry-run` | 否 | 仅打印请求，不执行。带 `--send-separately` 时每封草稿展示 GET → PUT → POST 三步 |

## 分别发送（--send-separately）语义

- `true`：服务端按客户端既有语义为每个收件人单独投递一封；`false`：显式取消（取消值同样写入草稿，不会丢失）。
- 省略：使用服务端已保存的设置——之前 `+send` / `+draft-create` / `+draft-edit` 或飞书客户端保存的分别发送状态原样生效，不会被清零。
- 覆盖保存是对草稿的读-改-写（仅改 `X-Cli-Send-Separately` 头，其余内容、收件人、附件原样保留）；保存失败不发送，发送失败保留已保存设置与草稿。
- 发送由服务端单次提交完成，CLI 不拆分收件人逐封调用；请求成功不等于所有收件人投递成功，逐收件人投递状态用 `send_status` 查询（见 [lark-mail-send-status](lark-mail-send-status.md)）。

## 返回值

```json
{
  "ok": true,
  "data": {
    "mailbox_id": "me",
    "total": 2,
    "success_count": 1,
    "failure_count": 1,
    "sent": [{"draft_id": "d1", "message_id": "msg_1", "thread_id": "t1"}],
    "failed": [{"draft_id": "d2", "error": "..."}]
  }
}
```

- 任一草稿失败时整体为 `ok:false`（多状态 envelope），按 `sent[]` / `failed[]` 向用户逐封报告，失败项展示原始错误信息。
- `aborted: true` 表示账号/邮箱级中止（认证、权限、网络、配额等）：剩余草稿未尝试，原样重试也会以相同方式失败。

## 安全约束

- 高危写操作：必须先向用户确认草稿 ID 列表与收件人范围，确认后传 `--yes`。
- 草稿不存在或已发送时沿用服务端原始错误；不要自动重建同内容草稿重发，新一轮发送必须由用户明确发起。

## 相关命令

- `lark-cli mail +draft-create` / `+draft-edit` — 创建 / 编辑草稿（可设置分别发送）
- `lark-cli mail +send` — 组装并发送新邮件（支持 `--send-separately`）
