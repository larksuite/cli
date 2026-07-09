# apps +session-messages-list

按 page_token 分页读取某个会话轮次（turn）的回复消息。运行时命令事实以 `lark-cli apps +session-messages-list --help` 为准。

## 何时用

用于拉取妙搭应用一轮对话（turn）产生的回复消息列表。只读，scope `spark:app:read`，用户身份。对仍在 running 的 turn 也可读——消息随生成增量出现，配合 `--page-token` 续拉新消息，可用于云端开发期间实时播报本轮进展。它不发消息、也不判断轮次状态；想知道某轮是否跑完、拿 `turn_id`，仍先用 `+session-get`。

## 命令骨架

```bash
lark-cli apps +session-messages-list --app-id <app_id> --session-id <session_id> --turn-id <turn_id> [--page-token <token>]
```

| 旗标 | 必填 | 说明 |
|------|:----:|------|
| `--app-id` | 是 | 应用 ID |
| `--session-id` | 是 | 会话 ID |
| `--turn-id` | 是 | 轮次 ID，来自 `+session-get` 的 `latest_turn.turn_id` |
| `--page-token` | 否 | string，上一页响应里的 `next_page_token`；首页省略 |

## turn_id 来源

`--turn-id` 不是用户能直接提供的，必须先跑 `+session-get` 拿 `latest_turn.turn_id`。没有 `turn_id` 时不要猜，先 `+session-get`。

## 示例

先取最新轮次的 `turn_id`，再拉第一页，最后用 `next_page_token` 续拉下一页：

```bash
# 1. 从 +session-get 提取 latest_turn.turn_id
TURN_ID=$(lark-cli apps +session-get --app-id app_xxx --session-id conv_xxx -q '.data.latest_turn.turn_id')

# 2. 拉第一页（省略 --page-token）
lark-cli apps +session-messages-list --app-id app_xxx --session-id conv_xxx --turn-id "$TURN_ID"

# 3. has_more=true 时，把上一页的 next_page_token 作为 --page-token 续拉
lark-cli apps +session-messages-list --app-id app_xxx --session-id conv_xxx --turn-id "$TURN_ID" --page-token tok_next
```

## 输出契约

- `data.messages[]`：每条含 `message_id`、`role`、`content`。
- `data.next_page_token`（string）：下一页分页令牌，作为下次调用的 `--page-token`。**注意它在最后一页仍非空**（解码形如 `{"offset":N}`），不能用它是否为空判断还有没有下一页。
- `data.has_more`（bool）：是否还有更多消息。**这是判断要不要续拉的唯一依据。**
- pretty 输出为消息表 + 末行 `next_page_token: <token>  has_more: <bool>`；自动化取字段用 JSON 或 `-q`。
- 业务失败（app/session/turn 不存在或 ID 写错）通常带 `error.hint` 指向 `+session-get`，优先转述 hint。

## 分页规则

单次调用只返回一页。Agent 自行续拉：把本次响应的 `next_page_token` 作为下次的 `--page-token`，直到 `has_more` 为 `false` 才停。首页不要传 `--page-token`。

> ⚠️ **终止条件只看 `has_more`，不要拿 `next_page_token` 是否为空判断。** 即使 `has_more=false`（已是最后一页），后端仍会返回一个非空的 `next_page_token`（解码形如 `{"offset":N}`）；若以「token 非空就继续」为循环条件，会在末页之后继续翻出空页（每页 0 条），白费调用。读到 `has_more=false` 立即停止，不要再用该 token 续拉。
