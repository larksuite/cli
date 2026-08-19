# base CellValue 规范（lark-base-cell-value）

> 适用命令：`lark-cli base +record-batch-create`、`lark-cli base +record-batch-update`

本文件定义 **shortcut 写记录** 时 `CellValue` 的推荐格式，目标是让 AI 一次写对。不同命令的外层 JSON 形状不同，但每个 cell 都以本文为 source of truth。

## 1. 顶层规则（必须遵守）

- `--json` 必须是 JSON 对象。
- `+record-batch-create --json` 使用 `{"create_records":[{"字段名或字段ID": CellValue}, ...]}`，数组中的每个对象代表一条新 Record。
- `+record-batch-update --json` 使用 `{"update_records":{"rec_xxx":{"字段名或字段ID": CellValue}, ...}}`，以 `record_id` 定位每条待更新 Record。
- 一次 payload 里同一字段只用一种 key（字段名或字段 ID），不要重复。
- 写入前先 `+field-list` 获取字段 `type/style/multiple`，再构造值。
- 需要清空字段时优先传 `null`（字段允许清空时）。

## 2. 各类型 CellValue

### 2.1 text

text 字段的 `style.type` 影响单元格检查逻辑：
`type=plain` 传 Markdown 格式的字符串。
`type=url` 传一个带 title 的 Markdown 格式链接，或单独传一个链接。
`type=phone` 传合法电话号码。
`type=email` 传合法邮箱字符串。

```json
{
    "标题": "Hello, [lark-cli](https://github.com/larksuite/cli)",
    "官网": "[官网](https://example.com)",
    "联系电话": "1380000000000",
    "邮箱": "owner@example.com"
}
```

### 2.2 number

用 JSON number，不要用带单位或千分位的字符串。货币、百分比、进度、评分等数字类字段也按数字写入，展示格式由字段配置决定。

```json
{
    "工时": 12.5,
    "预算": 3000,
    "完成度": 0.65,
    "评分": 4
}
```

### 2.3 select（单选/多选）

`select` 字段统一传选项名称数组。`multiple=false` 时数组只能包含一个元素，`multiple=true` 时可以包含多个元素。只支持写入字段中已有的选项；构造 CellValue 前先用 `+field-list` 或 `+field-search-options` 确认目标选项存在。

```json
{
    "单选": ["Todo"],
    "多选": ["后端", "高优"]
}
```

读取单元格时与写入的数据结构一致。

### 2.4 datetime

写入可省略时区偏移量，系统会按 Base 时区解析输入字符串；优先使用 `YYYY-MM-DD HH:mm`。Base 默认按分钟展示，但底层以毫秒级精度存储时间

```json
{
    "截止时间": "2026-03-24 10:00"
}
```

读取单元格时，日期时间输出为标准 RFC3339 字符串并固定保留三位毫秒，例如 `"2026-03-24T10:00:00.000+08:00"`。

### 2.5 checkbox

用 JSON boolean：`true` 或 `false`，不要用 `"true"`、`"是"`、`1`。

```json
{
    "已完成": true
}
```

### 2.6 user / group_chat

`user` 和 `group_chat` 字段统一传对象数组。`multiple=false` 时数组只能包含一个元素，`multiple=true` 时可以包含多个元素。每个元素至少包含 `id`；人员字段传用户 ID（如 `ou_xxx`），群字段传群 ID（如 `oc_xxx`）。

> **人员字段：不要猜 ID。** 不知道 `open_id` 时，先用 `lark-contact` 查 id：`lark-cli contact +search-user --query "<姓名/邮箱/手机号>" --as user`。

> **群组字段：不要猜 ID。** 不知道 `chat_id` 时，先用 `lark-im` 搜群：`lark-cli im +chat-search --query "<群名关键词>" --as user`；取结果里的 `oc_xxx`。

```json
{
    "负责人": [
      { "id": "ou_xxx" },
      { "id": "ou_xxx2" }
    ],
    "协作群": [
      { "id": "oc_xxx" }
    ]
}
```

读取单元格时仍为对象数组，每个元素为 `{id, name}`，例如 `[{"id":"ou_xxx","name":"张三"}]`。

### 2.7 link

用对象数组，元素包含 `id`，值为目标记录的 `record_id`。不要传记录标题；先用 `+record-list` / `+record-search` 找到目标记录 ID。

```json
{
    "关联任务": [
      { "id": "<record_id>" }
    ]
}
```

读取单元格时与写入的数据结构一致。

### 2.8 location

- 读取：`{lng, lat, full_address}`，三个成员均非空。
- 写入：`{lng, lat}`，经纬度均为数字；`full_address` 由平台根据坐标解析，不允许手动指定。
- 筛选行为：按照 `full_address` 做字符串筛选，将 Location 当作文本列使用文本 operator。

```json
{
    "坐标": {
      "lng": 116.397428,
      "lat": 39.90923
    }
}
```


### 2.9 attachment（不作为普通 CellValue 写入）

读取单元格时，附件为数组，每个元素为 `{file_token, size, name}`，例如 `[{"file_token":"box_xxx","size":1024,"name":"report.pdf"}]`。

- 追加附件：使用 `lark-cli base +record-upload-attachment --record-id <record_id> --field-id <field_id> --file <path>`；可重复 `--file` 一次追加多个附件，不能用普通记录操作接口写附件值。
- 删除附件：使用 `lark-cli base +record-remove-attachment --record-id <record_id> --field-id <field_id> --file-token <file_token> --yes`；可重复 `--file-token` 一次删除同一单元格里的多个附件。
- 下载附件：使用 `lark-cli base +record-download-attachment --record-id <record_id> --file-token <file_token> --output <dir>`；不传 `--file-token` 时下载整行所有附件，也可重复 `--file-token` 只下载指定附件。Base 附件必须用这个命令下载，用其他下载入口可能失败。

## 3. 只读字段（不要写）

写记录时，`auto_number`、`lookup`、`formula`、`created_at/updated_at`、`created_by/updated_by` 均为只读字段。

写入只读字段通常不会更新数据；返回里可能出现 `ignored_fields`，reason 会说明 `READONLY`。看到这种返回时，不要重试同一 payload，应移除只读字段，只写存储字段。

读取单元格时，`auto_number`、`formula`、`lookup` 为 `string | null`；`created_at`、`updated_at` 为 RFC3339 字符串或 `null`；`created_by`、`updated_by` 为 `array<{id, name}>`。

## 4. 完整示例

```json
{
    "标题": "Created from shortcut",
    "状态": ["Todo"],
    "标签": ["高优", "外部依赖"],
    "工时": 8,
    "截止时间": "2026-03-24 10:00",
    "已完成": false,
    "负责人": [{ "id": "ou_123" }],
    "关联任务": [{ "id": "rec_456" }],
    "坐标": { "lng": 116.397428, "lat": 39.90923 }
}
```
