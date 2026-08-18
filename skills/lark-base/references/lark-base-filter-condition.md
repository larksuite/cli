# Base Filter 条件结构（公共协议）

Filter 是一组「字段/操作符/值」条件的组合，用 `logic`（`and` / `or`）把多条 `conditions` 连接起来，用于描述「满足什么条件」。视图筛选 `filter`、记录读取/搜索的 `--filter-json`、表单题目显隐条件 `visible_rule` 复用同一套 tuple 结构，本文件是其公共协议（SSOT）。

## 0. 适用范围

本协议只适用于以下场景：

- `+view-set-filter` / `+view-get-filter` 的视图筛选配置。
- `+record-list --filter-json` / `+record-search --filter-json` 的结构化记录筛选。
- `+form-questions-create` / `+form-questions-update` 中的 `visible_rule` 显隐条件。

本协议**不适用于 `+data-query`**。`+data-query` 支持过滤，但使用的是 LiteQuery DSL 的 `filters` 对象结构：`{"type":1,"conjunction":"and","conditions":[{"field_name":"状态","operator":"is","value":["有效"]}]}`，不是这里的 tuple 条件 `["状态","==","有效"]`。需要聚合查询时先返回 [Record 查询与分析 SOP](lark-base-record-query-and-analysis-sop.md) 选路；SOP 选定 `+data-query` 后再读取 guide 和完整 DSL reference。

## 1. 顶层结构

- 必须是 JSON 对象。
- 顶层结构是 `{logic?, conditions?}`。
- `logic` 默认 `and`；推荐只用 canonical 值 `and` / `or`。
- `conditions` 默认空数组。
- 每条条件写成 tuple：`[field, operator, value?]`。
- `empty` / `non_empty` 可写成 2 项：`[field, "empty"]`、`[field, "non_empty"]`。

```json
{
  "logic": "and",
  "conditions": [
    ["状态", "intersects", ["Doing"]],
    ["负责人", "intersects", [{ "id": "ou_xxx" }]],
    ["截止时间", "empty"]
  ]
}
```

清空写法：

```json
{
  "conditions": []
}
```

## 2. operator

可用 operator：
- `==`
- `!=`
- `>`
- `>=`
- `<`
- `<=`
- `intersects`
- `disjoint`
- `empty`
- `non_empty`

## 3. value 写法

value 类型取决于条件引用对象（字段 / 题目）的类型。

### `text`

用字符串；高频的片段包含 / 排除使用 `intersects` / `disjoint`，完整文本比较使用 `==` / `!=`：

```json
["标题", "intersects", "发布"]
```

```json
["标题", "disjoint", "内部"]
```

### `location`

location 筛选只按 `full_address` 字符串匹配，不能直接按经纬度筛选；优先使用 `intersects` 做包含匹配，例如查深圳：

```json
["位置", "intersects", "深圳"]
```

### `number` / `auto_number`

用数字：

```json
["工时", ">=", 3.5]
```

### `select`

用选项名数组；`intersects` 表示命中任意选项，`disjoint` 表示不包含其中任何选项：

```json
["状态", "intersects", ["Doing", "Blocked"]]
```

```json
["状态", "disjoint", ["Archived"]]
```

### `user` / `group_chat` / `created_by` / `updated_by`

用对象数组；人员使用 `ou_xxx`，群组使用 `oc_xxx`。不知道 ID 时，人员用 `lark-contact` 查询，群组用 `lark-im` 搜索。

```json
["负责人", "intersects", [{ "id": "ou_xxx" }]]
```

```json
["负责人", "disjoint", [{ "id": "ou_xxx" }]]
```

```json
["负责群", "intersects", [{ "id": "oc_xxx" }]]
```

### `link`

用记录 id 对象数组：

```json
["关联任务", "intersects", [{ "id": "rec_xxx" }]]
```

### `checkbox`

用布尔值：

```json
["完成", "==", true]
```

### `datetime` / `created_at` / `updated_at`

用相对时间关键字或 `ExactDate(...)`：

```json
["截止时间", "==", "ExactDate(2026-01-01)"]
```

```json
["截止时间", "==", "ExactDate(2026-01-01 11:30)"]
```

```json
["截止时间", "==", "Today"]
```

可用关键字：
- `Today`
- `Yesterday`
- `Tomorrow`

### `formula` / `lookup`

value schema 随计算结果类型变化；拿不准时先读取字段定义，或根据错误提示修正 value 和 operator。

## 4. 易错点

- 不要再写旧对象风格：`{"field_name":...,"operator":...}`。
- `user` / `group_chat` / `link` 不要写成单个标量。
- `empty` / `non_empty` 统一表示格子为空 / 非空，不要传 value；标量空格子和多值字段没有任何元素都属于空。
- 日期条件稳定写法用 `ExactDate(...)` 或 `Today` / `Yesterday` / `Tomorrow`。
- `formula` / `lookup` 的 value schema 是动态的；拿不准 value 类型时先读字段定义，或根据错误提示修正类型。

## 5. 参考
- [Lookup Field](lark-base-field-lookup.md)
