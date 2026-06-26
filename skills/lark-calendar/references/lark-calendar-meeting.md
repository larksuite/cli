
# calendar +meeting

通过日程 ID（`event_id`） 获取关联的视频会议信息（`meeting_id`、`meeting_note`）。只读。

## 命令

```bash
# 单个 / 批量（逗号分隔，最多 50 个）
lark-cli calendar +meeting --event-ids <event_id1>,<event_id2>

# 默认使用主日历，需要时显式传 --calendar-id
lark-cli calendar +meeting --event-ids <event_id> --calendar-id <calendar_id>
```

## 输出字段

| 字段 | 说明 |
|------|------|
| `event_id` | 日程 ID |
| `meeting_id` | 关联的视频会议 ID |
| `meeting_note` | 用户主动绑定到日程的纪要文档 Token（`MeetingNotes`，由用户在日程页手动添加；）。**与会中产生的 AI 智能纪要 `note_doc_token` 是两份不同文档**，要拿 AI 纪要请继续走 `vc +detail` → `note +detail`。 |

## 下游链路

`calendar +meeting` 只把日程 ID 翻译为 `meeting_id` / `meeting_note`，要拿会中产生的产物（AI 智能纪要、逐字稿、妙记）需继续调用：

```bash
# 1. meeting_id → note_id + minute_token（同一会议两份产物，可能各自为空）
lark-cli vc +detail --meeting-ids <meeting_id>

# 2a. note_id → 纪要文档 token（note_doc_token / verbatim_doc_token / shared_doc_tokens）
lark-cli note +detail --note-id <note_id>

# 2b. minute_token → 妙记 AI 产物（按需获取，不传不返回任何 AI 内容）
lark-cli minutes +detail --minute-tokens <minute_token> --summary --todo --chapter --keyword --transcript

# 3. 任意文档 token（meeting_note / note_doc_token / verbatim_doc_token / shared_doc_token）→ 正文
lark-cli docs +fetch --api-version v2 --doc <doc_token> --doc-format markdown
```