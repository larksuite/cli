# minutes +speaker-replace

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

替换妙记逐字稿中的说话人身份：把妙记逐字稿里"原说话人"对应的所有发言段，重新归属到"新说话人"。常用于解决妙记自动识别错说话人，或需要手工把某段语音绑定到正确用户的场景。

本 skill 对应 shortcut：`lark-cli minutes +speaker-replace`。

## 典型触发表达

- "把这条妙记里 A 的发言改成 B"
- "妙记说话人识别错了，帮我把张三的部分换成李四"
- "妙记说话人修改 / 替换 / 重新归属"
- "改一下妙记的说话人"

## 命令示例

```bash
lark-cli minutes +speaker-replace \
  --minute-token obcnxxxxxxxxxxxxxxxxxxxx \
  --from-user-id ou_old_speaker_open_id \
  --to-user-id ou_new_speaker_open_id
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--minute-token <token>` | 是 | 妙记的唯一标识，可从妙记 URL 末尾路径提取 |
| `--from-user-id <ou_xxx>` | 是 | 被替换的原说话人，**必须是 `ou_` 开头的 open_id**，不支持用户名 |
| `--to-user-id <ou_xxx>` | 是 | 新的说话人，**必须是 `ou_` 开头的 open_id**，不支持用户名 |

> **重要**：`--from-user-id` 和 `--to-user-id` 仅支持 `ou_` 开头的用户 ID，**不支持直接传姓名**。如果用户只给了姓名，请先用 [lark-contact](../../lark-contact/SKILL.md) 把姓名解析成 `open_id`，再调用本命令。

## 认证与权限

- 所需 scope：`minutes:minutes:update`。

## 输出结果

| 字段 | 说明 |
|------|------|
| `minute_token` | 被修改的妙记 Token，与输入的 `--minute-token` 一致 |
| `from_user_id` | 被替换的原说话人 open_id，与输入的 `--from-user-id` 一致；必须是妙记逐字稿中已存在的说话人 |
| `to_user_id` | 替换后的新说话人 open_id，与输入的 `--to-user-id` 一致 |

## 参考

- [lark-minutes](../SKILL.md) -- 妙记相关功能说明
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
