
# calendar +search-event

按关键词、时间范围和参会人搜索日历日程。只读。

## 命令

```bash
# 按关键词
lark-cli calendar +search-event --query "周会"

# 按时间范围（ISO 8601 或 YYYY-MM-DD）
lark-cli calendar +search-event --start "2026-04-20T00:00:00+08:00" --end "2026-04-27T23:59:59+08:00"

# 按参会人（自动识别 ou_ 用户 / oc_ 群聊 / omm_ 会议室前缀）
lark-cli calendar +search-event --attendee-ids "ou_user1,oc_chat1,omm_room1"

# 组合
lark-cli calendar +search-event --query "周会" --start 2026-04-20 --end 2026-04-27 --attendee-ids "ou_user1"
```

## 输出字段

`items` 列表每条返回 `event_id` / `summary` / `start` / `end` / `is_all_day` / `app_link`；外层有 `has_more`、`page_token`。**仅返回基础字段，要拿日程详情用 `calendar events get`。**

## 注意事项

- 分页：`has_more=true` 时持续用 `page_token` 翻页直到 false，不要遗漏；`page-size` 最大 30。
- 已结束的会议优先用 `vc +search`——日历不收录"即时会议"，只查日程会漏。
