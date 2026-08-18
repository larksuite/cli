
# minutes +detail

通过 `minute_token` 查询妙记详情，按需获取 AI 产物（总结/待办/章节/逐字稿/关键词）。只读，支持 `--as user` / `--as bot`。

`minute_token` 由某个身份取得时，本命令及后续 `note +detail`、`docs +fetch` 都必须显式沿用同一个 `--as`。

> `--summary` / `--todo` / `--chapter` / `--keyword` / `--transcript` 都是可选项；请求相应产物时必须显式传入，不传任何产物 flag 时只返回基础信息（如 `title`），AI 产物字段都不会出现。一次性获取所有产物：`--summary --todo --chapter --keyword --transcript`。

## 命令

```bash
# 仅基础信息
lark-cli minutes +detail --as <source_identity> --minute-tokens obcxxxxxxxxxx

# 批量（逗号分隔，最多 50 个）
lark-cli minutes +detail --as <source_identity> --minute-tokens obcxxx,obcyyy --summary --todo

# 全产物
lark-cli minutes +detail --as <source_identity> --minute-tokens obcxxx --summary --todo --chapter --keyword --transcript

# 仅逐字稿，覆盖已有文件，指定输出目录
lark-cli minutes +detail --as <source_identity> --minute-tokens obcxxx --transcript --overwrite --output-dir ./out
```

## 输出

`minutes` 数组每条含 `minute_token`、`title`、`note_id`、`artifacts`。`note_id` 仅在该妙记关联了会议纪要时返回，可直接传给 [`note +detail`](lark-note-detail.md) 拿纪要文档 token，无需再绕回 `vc +detail`。`artifacts` 中**只包含本次请求的产物**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `artifacts.summary` | string | AI 总结。 |
| `artifacts.todos` | array | 待办事项列表。 |
| `artifacts.chapters` | array | 章节列表。 |
| `artifacts.keywords` | array | 关键词列表。 |
| `artifacts.transcript_file` | string | 逐字稿本地文件路径。 |

逐字稿默认落地 `./minutes/{minute_token}/transcript.txt`，与 `minutes +download` 同目录便于聚合。指定 `--output-dir <dir>` 时改写到 `<dir>/artifact-{title}-{minute_token}/transcript.txt`。

## minute_token 来源

| 来源 | 取值字段 |
|------|---------|
| 妙记 URL `https://*.feishu.cn/minutes/obcxxx` | 截路径最后一段 `obcxxx` |
| `vc +detail --meeting-ids` | `minute_token` |
| `vc +recording --meeting-ids` | `minute_token` |
| `minutes +search` | `minute_token` |

`minute_token` 不要直接传给 `note +detail`：需要关联 Note 时，先从本命令结果读取 `note_id`。跨产物流程由 [`query-minutes-and-artifacts`](../scenes/query-minutes-and-artifacts.md) 编排。

## 相关场景
- [查询妙记及其产物](../scenes/query-minutes-and-artifacts.md)
