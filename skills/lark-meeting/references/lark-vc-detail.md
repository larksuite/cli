
# vc +detail

通过会议 ID 获取会议详情，包括基本信息、可选聊天绑定（`chat_id`）、关联的纪要 ID（`note_id`）和妙记 Token（`minute_token`）。只读，支持 `--as user` / `--as bot`。

## 命令

```bash
# 单个 / 批量（逗号分隔，最多 50 个）
lark-cli vc +detail --meeting-ids <meeting_id1>,<meeting_id2>

# 应用身份（只能查应用有权限的会议）
lark-cli vc +detail --meeting-ids <meeting_id1>,<meeting_id2> --as bot
```

## 输出字段

| 字段 | 说明 |
|------|------|
| `meeting_id` | 会议 ID |
| `meeting_no` | 会议 9 位号码 |
| `chat_id` | 可选的当前 Chat 绑定；未绑定或为 0 时省略，不证明群有效或调用者具有 IM 权限。 |
| `topic` | 会议主题 |
| `start_time` | 开始时间 |
| `end_time` | 结束时间 |
| `note_id` | 关联的纪要 ID。 |
| `minute_token` | 关联的妙记 Token。 |
| `calendar_event_id` | 该会议关联的日程ID。**并非所有会议都有**：即时会议不由日程发起，没有此字段；仅当会议由日程发起时才返回。 |

跨产物选择和后续命令链由 [`query-meeting-and-artifacts`](../scenes/query-meeting-and-artifacts.md) 统一编排。`note_id` / `minute_token` 由本命令取得后，后续 `note +detail`、`minutes +detail` 和 Doc 读取命令必须显式沿用同一个 `--as`。

## 反查关联日程

`calendar_event_id` 提供从会议反向定位日程的入口。拿到后进入日历域读取日程：

先检查对应会议条目的 `error`；查询成功后仍缺少 `calendar_event_id` 时，只说明未取得关联日程 ID，不猜测 ID 反查。

## 相关场景
- [查询会议及其产物](../scenes/query-meeting-and-artifacts.md)

本命令只读。缺少 `chat_id` 时不自动创建、绑定或入群；只有明确需要创建或复用 Chat 时，才沿用来源身份调用 [`vc +meeting-chat`](lark-vc-meeting-chat.md)。

## 聊天绑定结果

JSON 结果位于 `data.meetings[]`，按 `meeting_id` 选择对应项后读取可选 `chat_id`，不要取批量结果第一项或读取顶层 `data.chat_id`。条目有 `error` 时报告该会议查询失败；无 `error` 但没有 `chat_id` 表示未取得绑定，不是创建授权。`hint` 可能仅描述纪要或录制不可用，不影响已经返回的聊天绑定。

会中和会后均沿用详情查询的原有可见性。已有结果可直接复用；单独查询绑定使用 `vc meeting get`，避免本命令的录制查询及权限要求，见 [会议查询场景](../scenes/query-meeting-and-artifacts.md#获取会议群聊)；会中创建见 [会中互动场景](../scenes/live-meeting-interact.md#获取或创建会议群聊)。
