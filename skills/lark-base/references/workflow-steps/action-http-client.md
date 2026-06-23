# HTTPClientAction

```json
{
  "method": "POST",
  "url": [{ "value_type": "text", "value": "https://api.example.com/webhook" }],
  "queries": [
    { "key": "source", "value": [{ "value_type": "text", "value": "workflow" }] }
  ],
  "headers": [
    { "key": "Content-Type", "value": [{ "value_type": "text", "value": "application/json" }] }
  ],
  "body_type": "raw",
  "raw_body": [
    { "value_type": "text", "value": "{\"record_id\":\"" },
    { "value_type": "ref", "value": "$.step_1.recordId" },
    { "value_type": "text", "value": "\"}" }
  ],
  "response_type": "json",
  "response_value": "{\"success\":true,\"message\":\"data fetched successfully\"}"
}
```

| 字段 | 必填 | 说明 |
|------|-----|------|
| `method` | 否 | 请求方法：`GET` / `POST` / `PUT` / `PATCH` / `DELETE`，默认 `POST` |
| `url` | 是 | ValueInfo[]，请求 URL，支持 `text` / `ref` 拼接 |
| `queries` | 否 | KeyValue[]，查询参数 |
| `headers` | 否 | KeyValue[]，请求头 |
| `body_type` | 否 | 请求体类型：`none` / `raw` / `form-data` / `form-urlencoded`，默认 `raw` |
| `raw_body` | 否 | ValueInfo[]，原始请求体，仅 `body_type=raw` 时使用 |
| `form_body` | 否 | KeyValue[]，表单数据，仅 `body_type=form-data` 或 `body_type=form-urlencoded` 时使用 |
| `response_type` | 否 | 响应类型：`none` / `text` / `json`，默认 `json` |
| `response_value` | 否 | string，JSON 字符串形式的响应结果示例；仅当 `response_type=json` 时必填 |

`KeyValue`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `key` | string | 参数名 / 请求头名 |
| `value` | ValueInfo[] | 参数值 / 请求头值，支持 `text` / `ref` |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
