# im +flag-cancel（取消标记）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli im +flag-cancel`。底层 `POST /open-apis/im/v1/flags/cancel`。

## 双删语义（重要）

同一个 item 可能同时在两层存在标记：

- 话题群（`chat_mode=topic`）中的话题：`(thread, feed)` + `(default, message)`
- 普通群（`chat_mode=group`）中话题的根消息：`(default, message)` + `(msg_thread, feed)`

**当用户没有从上下文（即 `--flag-type` / `--item-type`）指明要删哪一层时**，shortcut 会自动查询 chat API 的 `chat_mode` 来判断话题群/普通群，然后执行双删；服务端对不存在的标记取消请求幂等，所以多余那条是安全的 no-op。

**chat_mode 判定流程**：
1. 对于 `om_xxx`，先查询 `GET /open-apis/im/v1/messages/{id}` 获取 `chat_id`，判断是否为 thread root
2. 如果是 thread root，再查询 `GET /open-apis/im/v1/chats/{chat_id}` 获取 `chat_mode`
3. `chat_mode=topic` → feed 层用 `ItemTypeThread`；`chat_mode=group` → feed 层用 `ItemTypeMsgThread`
4. 对于 `omt_xxx`，同样查询 chat API 获取 `chat_mode` 来决定 feed 层的 item_type

> **注意**：`+messages-search` 返回的 `chat_type` 字段不可靠（已废弃），不能用来判断话题群/普通群，必须通过 chat API 的 `chat_mode` 字段判断。

行为对照：

- `--message-id om_xxx` 不带 `--flag-type` → 自动判定：话题根消息双删（feed 层 item_type 由 chat_mode 决定），普通消息单删 `(default, message)`
- `--thread-id omt_xxx` 不带 `--flag-type` → 自动判定 chat_mode，双删对应的两层
- 任意 id + `--flag-type message` → 单删 message 层
- 任意 id + `--flag-type feed` → 单删 feed 层（item-type 由 id 推断或显式指定）

## 命令

```bash
# 自动判定话题群/普通群：双删两层（推荐默认用法）
lark-cli im +flag-cancel --as user --message-id om_xxx

# 自动判定话题群/普通群：双删两层
lark-cli im +flag-cancel --as user --thread-id omt_xxx

# 已知只想清 message 层
lark-cli im +flag-cancel --as user --message-id om_xxx --flag-type message

# 已知只想清 feed 层 —— 普通群话题根消息
lark-cli im +flag-cancel --as user --message-id om_xxx --item-type msg_thread --flag-type feed

# 已知只想清 feed 层 —— 话题群中的话题
lark-cli im +flag-cancel --as user --thread-id omt_xxx --item-type thread --flag-type feed

# 预览请求
lark-cli im +flag-cancel --as user --thread-id omt_xxx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--message-id <om_xxx>` | 二选一 | 消息 ID（包括话题根消息） |
| `--thread-id <omt_xxx>` | 二选一 | Thread ID |
| `--item-type <name>` | 否 | 覆盖推断 |
| `--flag-type <name>` | 否 | 覆盖推断；**未指定时自动判定 chat_mode 并双删** |
| `--as user` | 必须 | 当前仅支持 user 身份 |

## 幂等性

服务端对"标记不存在"的取消请求不会返回错误，所以重复 `+cancel` 幂等。

## 权限

- Scope：`im:feed.flag:write`
