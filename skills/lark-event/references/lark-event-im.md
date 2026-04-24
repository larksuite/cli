# IM Events

> **前置条件：** 先阅读 [`../SKILL.md`](../SKILL.md) 了解命令、子进程契约、jq 用法、`root_path_hint` 语义。

## Key 速览（11 个，按业务分组）

### 消息类（4）

| EventKey | 用途 |
|---|---|
| `im.message.receive_v1` | 接收 IM 消息（text / post / image / file / audio / media / sticker / interactive / share_chat / share_user / system 所有类型） |
| `im.message.message_read_v1` | 用户已读机器人发送的**单聊**消息（群消息不触发） |
| `im.message.reaction.created_v1` | 消息被添加表情回复 |
| `im.message.reaction.deleted_v1` | 消息被删除表情回复 |

### 群类（2）

| EventKey | 用途 |
|---|---|
| `im.chat.updated_v1` | 群配置（群主、头像、名称、权限等）被修改 |
| `im.chat.disbanded_v1` | 群被解散 |

### 群成员类（5）

| EventKey | 用途 |
|---|---|
| `im.chat.member.bot.added_v1` | 机器人被添加至群聊 |
| `im.chat.member.bot.deleted_v1` | 机器人被移出群聊 |
| `im.chat.member.user.added_v1` | 新用户进群（含话题群） |
| `im.chat.member.user.deleted_v1` | 用户主动退群**或**被移出群聊 |
| `im.chat.member.user.withdrawn_v1` | 撤销拉用户进群（邀请方撤回邀请，用户未真正加入） |

## Shape 速览

| Key | shape | jq 前缀（`root_path_hint`） |
|---|---|---|
| `im.message.receive_v1` | **flat** — 字段直接在顶层 | `.` |
| 其余 10 个 | **V2 信封** — `{schema, header, event}`，字段在 event 内 | `.event` |

全部声明 `auth_types = ["bot"]`，用 `--as bot` 或 `--as auto`（auto 会解析到 bot）。

## 从 0 到 1：抓一条样本

```bash
lark-cli event consume im.message.receive_v1 --max-events 1 --timeout 60s --as bot
# 然后到 bot 所在会话 / 群手动发一条消息
```

**预期 stderr**：

```
[event] ready event_key=im.message.receive_v1                  ← 订阅就绪，后续事件保证送 stdout
[source] feishu-websocket: connected                            ← 上游连上
[event] exited — received 1 event(s) in 3s (reason: limit)      ← 收到 1 条退出，exit 0
```

**预期 stdout**（单行 NDJSON）：

```json
{"type":"im.message.receive_v1","event_id":"...","message_id":"om_x...","chat_id":"oc_...","chat_type":"p2p","message_type":"text","sender_id":"ou_...","content":"{\"text\":\"hi\"}"}
```

**常见卡点**：

- 60s 过了没事件 → bot 不在目标会话，把它加进去手动发一条触发
- `Error: missing required scopes ...` → bot 缺 scope，查 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) 补权限
- `.content` 看起来是 JSON 字符串而不是对象 → 这是 text/post/interactive 消息的**正常形态**，见下文 "关键字段注意 → `.content`"

## 场景 pipeline

### 1. 按会话类型过滤（p2p vs group）

`chat_type` 枚举只有 `p2p` / `group` 两值。

```bash
# 只看私聊
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.chat_type=="p2p") | {from: .sender_id, msg: .content}'

# 只看群聊（带 chat_id）
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.chat_type=="group") | {chat: .chat_id, from: .sender_id, msg: .content}'
```

### 2. 按消息类型过滤（text / interactive / 等）

```bash
# 只看纯文本，直接投影到文本
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.message_type=="text") | .content | fromjson | .text'
```

### 3. 单群过滤（已知 chat_id）

`chat_id` 可用 `lark-cli im chat +chat-search --keyword "<群名>"` 反查。

```bash
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.chat_id=="oc_xxxx") | {from: .sender_id, msg: .content}'
```

### 4. 私聊自动回复（最小 bot 范例）

所有到达的私聊 text 消息 → 用 `im +messages-reply` 回复：

```bash
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.chat_type=="p2p" and .message_type=="text") | {mid: .message_id, text: (.content | fromjson | .text)}' \
| while IFS= read -r line; do
    mid=$(echo "$line" | jq -r .mid)
    text=$(echo "$line" | jq -r .text)
    lark-cli im +messages-reply --message-id "$mid" --text "echo: $text" --as bot
  done
```

要点：

- text / post / interactive 的 `.content` 都是 JSON 字符串，要 `fromjson` 才能取内部字段
- `--message-id` 来自事件顶层 `.message_id`
- reply 必须 `--as bot`（bot 身份才能以 bot 发话）

### 5. 多 key 组合（多 shell 模板）

起 N 个子进程，共用一个 bus daemon，汇到同一个文件：

```bash
lark-cli event consume im.message.receive_v1          --as bot >> /tmp/im-all.ndjson 2>> /tmp/im-all.stderr &
lark-cli event consume im.message.reaction.created_v1 --as bot >> /tmp/im-all.ndjson 2>> /tmp/im-all.stderr &
lark-cli event consume im.chat.member.user.added_v1   --as bot >> /tmp/im-all.ndjson 2>> /tmp/im-all.stderr &
wait
```

注意：

- 单行 NDJSON 并发 append 是安全的（单事件 << PIPE_BUF），**跨进程事件顺序不保证**；要区分来源，从事件 `.type` 字段读
- 停止用 `kill %1 %2 %3` 发 SIGTERM，**不要 `kill -9`** —— 见 [SKILL.md](../SKILL.md) 铁规则

## 关键字段注意

### `.content` 对 text / post / interactive 是 JSON 字符串

flat 事件里 `.content` 不是对象，是 JSON 编码后的字符串：

```bash
# 错：把 .content 当对象取，会拿到 null 或 error
lark-cli event consume im.message.receive_v1 --jq '.content.text'

# 对：fromjson 先解析
lark-cli event consume im.message.receive_v1 --jq '.content | fromjson | .text'
```

post / interactive 同理，只是 fromjson 后的结构不同（post 是嵌套 elements，interactive 是卡片 JSON）。

### `.sender_id` 只有 open_id

事件 payload 里没有姓名字段。要拿显示名得另调 contact API（如 `lark-cli contact +user-get`）。

### `@bot` 识别：mentions 藏在 `.content`

事件 schema 顶层**没有** `mentions` 字段。at 信息在 `.content` 里：

- text 消息：content 是 `{"text":"...","mentions":[...]}`，其中 mentions 数组每项含 `id.open_id` / `key` / `name`
- post 消息：嵌套 elements，at 作为 `{"tag":"at","user_id":"..."}` 元素

写 "@bot 才回" 判断前，先 `lark-cli event consume im.message.receive_v1 --max-events 1 --timeout 60s --as bot`，实际抓一条 @ 自己 bot 的消息，观察 `.content | fromjson` 后的结构再写 jq，**不要凭印象**。
