
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

## 排查失效的会议纪要关联

`+meeting` 返回 `meeting_note` 后，可用 `docs +fetch` 检查文档是否仍可访问：

```bash
lark-cli docs +fetch --api-version v2 --doc <meeting_note> --doc-format markdown
```

若返回文档已删除的错误，说明日程仍保留着指向已删除文档的旧关联；不要把该 Token 当成可用纪要继续处理。

## 创建原生会议纪要

当前 CLI 没有对应的 typed Calendar 命令。确认用户确实要创建新纪要后，可使用官方 Calendar OpenAPI 的 raw API：

```bash
# 先获取当前身份的主日历 ID
lark-cli calendar calendars primary --as user --format json

# 为指定日程创建一个新的原生会议纪要文档
lark-cli api POST \
  /open-apis/calendar/v4/calendars/<calendar_id>/events/<event_id>/meeting_minute \
  --as user \
  --format json
```

成功响应的 `data.doc_url` 是新建会议纪要的文档 URL。调用前需满足：

- 目标日历是当前身份的主日历，且当前身份有 writer 权限；
- 日程至少有 1 个参与人；
- 日程的参与人权限不是 `none`；
- 应用具备 `calendar:calendar` 或 `calendar:calendar.event:update` 任一权限。

## 能力边界：不能通过 OpenAPI 绑定任意已有文档

创建会议纪要接口没有请求体，只会新建原生会议纪要并返回 `doc_url`。当前官方 `calendar.events.patch` 请求字段也不包含可写的 `meeting_note`、`doc_url` 或已有文档 Token，因此不能通过 Calendar OpenAPI 把任意已有 Doc/Wiki 文档重新绑定为日程会议纪要。

用户要求绑定已有文档时：

1. 先用 `calendar +meeting` 检查现有关联；
2. 若关联文档已删除，明确说明这是失效的旧关联；
3. 提供可行替代方案：创建新的原生会议纪要后复制或链接内容，或把已有文档 URL 写入日程描述；
4. 若 Lark 客户端当前版本提供手动绑定/替换入口，可让用户在客户端完成，但不要声称 CLI 或 OpenAPI 已支持该操作。
