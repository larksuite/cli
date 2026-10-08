# recording +list

查询当前用户拥有的录音，按需通过返回的产物标识继续读取妙记或智能纪要。参数用法见 `lark-cli recording +list --help`。

录音 API 是与 `vc` 视频会议 API 齐平的 CLI 分组；底层复用 `/open-apis/vc/v1/recordings`，权限仍为 `vc:record:readonly`。

## 身份与范围

- 显式使用 `--as user`，需要 `vc:record:readonly`。不支持应用机器人身份，也不能指定其他 owner。
- 查询覆盖当前用户的全部来源录音，不要求先提供 `meeting_id`。
- 已知会议、需要查该会议录制时，使用 [`vc +recording`](lark-vc-recording.md)。按妙记标题搜索时，使用 [`minutes +search`](lark-minutes-search.md)。

## 查询与分页

```bash
lark-cli recording +list --as user --page-size 20

# 按录音开始时间过滤：日期或 ISO 8601 时间，包含上下界
lark-cli recording +list --as user --start 2026-09-01 --end 2026-09-20

# 继续分页，保持原时间范围不变，并原样传入上一页 page_token
lark-cli recording +list --as user --start 2026-09-01 --end 2026-09-20 --page-size 20 --page-token '<page_token>'

# 在上述时间范围内自动分页；按需要设置最大页数
lark-cli recording +list --as user --start 2026-09-01 --end 2026-09-20 --page-all --page-limit 5
```

默认每页 20 条，最大 50 条。日期和不带时区的时间采用本地时区；日期形式的 `--end` 包含当日 23:59:59.999。需要跨机器一致时使用带时区的 ISO 8601 时间；纯数字输入单位为 Unix 秒。`--page-all` 默认最多 10 页，`--page-limit` 支持 1–1000；返回最后一页的游标，可继续查询。`end_time_ms` 过滤的是录音开始时间上界；双边范围必须满足开始时间不晚于结束时间。只有 `has_more` 为 false 才表示查完，达到 `--page-limit` 时不能声称已列出全部录音。

## 继续读取产物

- `recording_id` 是录音身份，不能作为 `meeting_id`、`minute_token` 或 `note_id` 使用。标识和毫秒时间字段均保留原始字符串。
- `artifacts[].type = 1`：妙记，取 `artifact_token` 作为 `minute_token`；若为空，不能用 `artifact_id` 代替，应待 token 可用后再查询。转入[妙记及其产物场景](../scenes/query-minutes-and-artifacts.md)。
- `artifacts[].type = 2`：智能纪要，取 `artifact_id` 作为 `note_id`，转入[智能纪要及关联产物场景](../scenes/query-note-and-artifacts.md)；不要求有 `artifact_token`。
- 产物可能正在生成、暂不可用或尚未返回；缺少产物不代表录音不存在。产物详情仍按各自接口校验访问权限。
- 后续调用显式沿用 `--as user`，不为绕过权限错误切换身份。
