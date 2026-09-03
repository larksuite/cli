
# vc +detail

通过会议 ID 获取会议详情，包括基本信息、关联的纪要 ID（`note_id`）和妙记 Token（`minute_token`）。只读，支持 `--as user` / `--as bot`。

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
| `topic` | 会议主题 |
| `start_time` | 开始时间 |
| `end_time` | 结束时间 |
| `note_id` | 关联的纪要 ID。 |
| `minute_token` | 关联的妙记 Token。 |
| `calendar_event_id` | 该会议关联的日程ID。**并非所有会议都有**：即时会议不由日程发起，没有此字段；仅当会议由日程发起时才返回。 |

跨产物选择和后续命令链由 [`query-meeting-and-artifacts`](../scenes/query-meeting-and-artifacts.md) 统一编排。`note_id` / `minute_token` 由本命令取得后，后续 `note +detail`、`minutes +detail` 和 Doc 读取命令必须显式沿用同一个 `--as`。

## 反查关联日程

`calendar_event_id` 提供从会议反向定位日程的入口。拿到后进入日历域读取日程：

字段为空说明该会议没有关联日程（如即时会议），不要据此反查，也不视为错误。

## 相关场景
- [查询会议及其产物](../scenes/query-meeting-and-artifacts.md)
