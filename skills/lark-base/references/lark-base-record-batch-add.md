# base +record-batch-add

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

批量新增记录。

## 推荐命令

```bash
lark-cli base +record-batch-add \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --json '{"fields":["标题","状态"],"rows":[["任务 A","Open"],["任务 B","Done"]]}'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--json <body>` | 是 | 批量新增请求体，必须是 JSON 对象 |

## API 入参详情

**HTTP 方法和路径：**

```
POST /open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch
```

## `--json` Raw JSON Schema

```json
{"type":"object","properties":{"fields":{"type":"array","items":{"type":"string","minLength":1,"maxLength":100,"description":"Field id or name"},"minItems":1,"maxItems":200},"rows":{"type":"array","items":{"type":"array","items":{"anyOf":[{"anyOf":[{"type":"string","description":"text field cell, example: \"one string and [one url](https://foo.bar)\""},{"type":"number","description":"number field cell, can be any float64 value"},{"type":"array","items":{"type":"string","description":"option name"},"description":"select field cell, example: [\"option_1\", \"option_2\"]"},{"type":"string","description":"datetime field cell. accepts common datetime strings and timestamp-like values. Prefer \"YYYY-MM-DD HH:mm:ss\" in requests because it is the most stable format and matches the API output. Example: \"2026-01-01 19:30:00\""},{"type":"array","items":{"type":"object","properties":{"id":{"type":"string","description":"record id"}},"required":["id"],"additionalProperties":false},"description":"link field cell, example: [{\"id\": \"rec_123\"}]"},{"type":"array","items":{"type":"object","properties":{"id":{"type":"string","description":"user id"}},"required":["id"],"additionalProperties":false},"description":"user field cell, example: [{\"id\": \"ou_123\"}]"},{"type":"object","properties":{"lng":{"type":"number","description":"Longitude"},"lat":{"type":"number","description":"Latitude"}},"required":["lng","lat"],"additionalProperties":false,"description":"location field cell, example: {\"lng\": 113.94765, \"lat\": 22.528533}"},{"type":"boolean","description":"checkbox field cell"},{"type":"array","items":{"type":"object","properties":{"file_token":{"type":"string","minLength":0,"maxLength":50},"name":{"type":"string","minLength":1,"maxLength":255},"mime_type":{"type":"string","maxLength":255,"description":"deprecated field"},"size":{"type":"integer","minimum":0,"description":"deprecated field"},"image_width":{"type":"integer","minimum":0,"description":"deprecated field"},"image_height":{"type":"integer","minimum":0,"description":"deprecated field"},"deprecated_set_attachment":{"type":"boolean","description":"deprecated field"}},"required":["file_token","name"],"additionalProperties":false},"description":"attachment field cell. temporary compatibility for attachment writes."},{"type":"null"}]},{"type":"null"}]}},"minItems":1,"maxItems":200}},"required":["fields","rows"],"additionalProperties":false,"$schema":"http://json-schema.org/draft-07/schema#"}
```

## 返回重点

- 返回对象键：
  - `fields`
  - `field_id_list`
  - `record_id_list`
  - `data`
  - `ignored_fields`（可选）

## 坑点

- ⚠️ `--json` 必须是对象。
- ⚠️ `fields` 与 `rows` 列顺序必须一一对应。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
- [lark-base-shortcut-record-value.md](lark-base-shortcut-record-value.md) — 记录值格式规范
