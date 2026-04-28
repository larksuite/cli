# im +flag-create（创建标记）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

本 skill 对应 shortcut：`lark-cli im +flag-create`。底层 `POST /open-apis/im/v1/flags`。

**默认行为：走 `(default=0, message=2)` message 类型**。当用户没有明确说要标记 message 还是 feed 时，一律默认按 message 处理 —— 包括话题（`omt_xxx`）以及话题的根消息（同时可被 message 或 feed 标记，但默认 message）。要走 feed 层标记必须显式提供 `--item-type` + `--flag-type` 覆盖。

- `--message-id om_xxx`（无覆盖） → `(default, message)`
- `--thread-id omt_xxx`（无覆盖） → `(default, message)`（默认按消息层标记）
- `--thread-id omt_xxx --item-type thread --flag-type feed` → `(thread, feed)` —— **话题群** 中的话题在 feed 层打标
- `--thread-id omt_xxx --item-type msg_thread --flag-type feed` → `(msg_thread, feed)` —— **普通群** 中的话题在 feed 层打标

## 命令

```bash
# 给一条消息打标（默认路径）
lark-cli im +flag-create --as user --message-id om_x100xxx

# 话题/话题根消息默认也是 message 层
lark-cli im +flag-create --as user --thread-id omt_1a8xxx

# 话题群中的话题：feed 层打标（须显式指定）
lark-cli im +flag-create --as user --thread-id omt_1a8xxx --item-type thread --flag-type feed

# 普通群中的话题：feed 层打标
lark-cli im +flag-create --as user --thread-id omt_xxx --item-type msg_thread --flag-type feed

# 预览请求（不发送）
lark-cli im +flag-create --as user --message-id om_xxx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--message-id <om_xxx>` | 二选一 | 消息 ID |
| `--thread-id <omt_xxx>` | 二选一 | Thread ID |
| `--item-type <name>` | 否 | 覆盖推断：`default\|chat\|doc\|thread\|box\|open_app\|subscription\|msg_thread\|my_ai\|app_feed\|knowledge_ai` |
| `--flag-type <name>` | 否 | 覆盖推断：`message\|feed` |
| `--as user` | 必须 | 当前仅支持 user 身份 |

## 合法组合约束

Execute 前会校验 `(item_type, flag_type)` 组合；服务端目前只接受：

- `(default, message)`
- `(thread, feed)`
- `(msg_thread, feed)`

其他组合会在本地直接报 validation 错误。

## 权限

- Scope：`im:feed.flag:write`
- 缺失时 CLI 会给出 `lark-cli auth login --scope "..."` 提示
