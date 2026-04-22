# IM Events

> **前置条件：** 先阅读 [`../SKILL.md`](../SKILL.md) 了解 `event consume` 的通用机制（命令、子进程启动契约、JQ 用法）。

## Key 目录（11 个）

| EventKey | 用途 |
|---|---|
| `im.message.receive_v1` | 接收 IM 消息（text / post / image / file / audio / media / sticker / interactive / share_chat / share_user / system 所有类型） |
| `im.message.message_read_v1` | 用户已读机器人发送的**单聊**消息（群消息不触发） |
| `im.message.reaction.created_v1` | 消息被添加表情回复 |
| `im.message.reaction.deleted_v1` | 消息被删除表情回复 |
| `im.chat.updated_v1` | 群配置（群主、头像、名称、权限等）被修改 |
| `im.chat.disbanded_v1` | 群被解散 |
| `im.chat.member.bot.added_v1` | 机器人被添加至群聊 |
| `im.chat.member.bot.deleted_v1` | 机器人被移出群聊 |
| `im.chat.member.user.added_v1` | 新用户进群（含话题群） |
| `im.chat.member.user.deleted_v1` | 用户主动退群**或**被移出群聊 |
| `im.chat.member.user.withdrawn_v1` | 撤销拉用户进群（邀请方撤回邀请，用户未真正加入） |


## 注意点（`im.message.receive_v1`）

**sender_id 只有 open_id**：事件 payload 里没有姓名字段；要拿发送者显示名得另调 contact API。

**`.content` 对 `interactive`（卡片）消息是原始 JSON 字符串**，要先 fromjson：

```bash
# 错的：直接当对象取，会拿到 null 或 error
lark-cli event consume im.message.receive_v1 --jq '.content.header.title.content'

# 对的：fromjson 先解析成对象
lark-cli event consume im.message.receive_v1 \
  --jq 'select(.message_type=="interactive") | .content | fromjson | .header.title.content'
```

## 常用 JQ 范式

### 1. 按会话类型过滤（私聊 vs 群聊）

`chat_type` 枚举只有 `p2p` / `group` 两个值。

```bash
# 只看私聊
lark-cli event consume im.message.receive_v1 \
  --jq 'select(.chat_type=="p2p") | {from: .sender_id, msg: .content}'

# 只看群聊
lark-cli event consume im.message.receive_v1 \
  --jq 'select(.chat_type=="group") | {chat: .chat_id, from: .sender_id, msg: .content}'
```

### 2. 按消息类型过滤

```bash
# 只看纯文本（排除卡片、图片等）
lark-cli event consume im.message.receive_v1 \
  --jq 'select(.message_type=="text") | .content'
```