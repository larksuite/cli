
# feed +sensitive

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

为群消息卡片（Feed Card）的指定用户开启或关闭即时提醒（时间敏感/置顶）状态。仅支持群聊类型（oc_ 前缀），仅 bot 身份可调用。

需要的 scopes: ["im:datasync.feed_card.time_sensitive:write"]

## 命令

```bash
# 为用户开启即时提醒
lark-cli feed +sensitive --feed-card-id oc_xxx --enable --user-ids ou_yyy

# 为多个用户开启（逗号分隔）
lark-cli feed +sensitive --feed-card-id oc_xxx --enable --user-ids ou_aaa,ou_bbb

# 为多个用户开启（重复 flag）
lark-cli feed +sensitive --feed-card-id oc_xxx --enable --user-ids ou_aaa --user-ids ou_bbb

# 关闭即时提醒
lark-cli feed +sensitive --feed-card-id oc_xxx --disable --user-ids ou_yyy

# 使用 union_id 类型
lark-cli feed +sensitive --feed-card-id oc_xxx --enable --user-ids on_zzz --user-id-type union_id

# 预览 API 调用（不实际执行）
lark-cli feed +sensitive --feed-card-id oc_xxx --enable --user-ids ou_yyy --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--feed-card-id <id>` | 是 | Feed Card ID，必须以 `oc_` 开头，仅支持群聊 |
| `--enable` | 二选一 | 开启即时提醒（将卡片置顶给指定用户） |
| `--disable` | 二选一 | 关闭即时提醒 |
| `--user-ids <ids>` | 是 | 用户 ID 列表，逗号分隔或重复 flag；用户须为该群聊成员 |
| `--user-id-type` | 否 | 用户 ID 类型：`open_id`（默认）\| `union_id` \| `user_id` |
| `--format` | 否 | 输出格式：`json`（默认）\| `pretty` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

`--enable` 和 `--disable` 互斥，必须且只能提供其中一个。

## 输出格式

**JSON 模式（默认）：**

```json
{
  "feed_card_id": "oc_xxx",
  "time_sensitive": true,
  "failed_user_reasons": [
    {
      "error_code": 0,
      "error_message": "The user is not in the chat",
      "user_id": "ou_yyy"
    }
  ]
}
```

**Pretty 模式（`--format pretty`）：**

全部成功时，stdout：
```
Time-sensitive updated for 2 user(s) (feed_card_id: oc_xxx)
```

部分失败时，stdout 显示成功数，stderr 显示警告：
```
warning: 1 user(s) failed:
  ou_yyy: The user is not in the chat
```

## 退出码

| 条件 | 退出码 |
|------|--------|
| 全部成功 | 0 |
| 部分或全部用户失败 | 1 |
| 参数校验错误 | 1 |
| API 错误 | 1 |

## 注意事项

- `--feed-card-id` 必须以 `oc_` 开头，否则报校验错误
- 指定的用户须是该群聊的成员，非成员会出现在 `failed_user_reasons` 中
- 跨租户用户通常不是群聊成员，会产生部分失败（非致命，仍返回退出码 1）
