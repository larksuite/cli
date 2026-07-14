# Base Filter 条件结构（公共协议）

Filter 是一组「字段/操作符/值」条件的组合，用 `logic`（`and` / `or`）把多条 `conditions` 连接起来，用于描述「满足什么条件」。视图筛选 `filter`、记录读取/搜索的 `--filter-json`、表单题目显隐条件 `visible_rule` 复用同一套 tuple 结构，本文件是其公共协议（SSOT）。

## 0. 适用范围

本协议只适用于以下场景：

- `+view-set-filter` / `+view-get-filter` 的视图筛选配置。
- `+record-list --filter-json` / `+record-search --filter-json` 的结构化记录筛选。
- `+form-questions-create` / `+form-questions-update` 中的 `visible_rule` 显隐条件。

本协议**不适用于 `+data-query`**。`+data-query` 支持过滤，但使用的是 LiteQuery DSL 的 `filters` 对象结构：`{"type":1,"conjunction":"and","conditions":[{"field_name":"状态","operator":"is","value":["有效"]}]}`，不是这里的 tuple 条件 `["状态","==","有效"]`。构造 `+data-query --dsl` 时请阅读 [lark-base-data-query.md](lark-base-data-query.md) 的 FilterGroup / Condition 章节。

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

用字符串：

```json
["标题", "intersects", "发布"]
```

### `location`

location 筛选只按 `full_address` 字符串匹配，不能直接按经纬度筛选；优先使用 `intersects` 做包含匹配，例如查深圳：

```json
["位置", "intersects", "深圳"]
```

不推荐写 `["位置", "==", "深圳"]` 这类精确匹配，除非确保筛选值与完整 `full_address` 完全一致。

### `number` / `auto_number`

用数字：

```json
["工时", ">=", 3.5]
```

### `select`

用选项名数组：

```json
["状态", "intersects", ["Doing", "Blocked"]]
```

### `user` / `created_by` / `updated_by`

用对象数组：

> **人员筛选：不要猜 ID。** 不知道 `open_id` 时，先用 `lark-contact` 查 id：`lark-cli contact +search-user --query "<姓名/邮箱/手机号>" --as user`。

```json
["负责人", "intersects", [{ "id": "ou_xxx" }]]
```

### `group_chat`

用对象数组：

> **群组筛选：不要猜 ID。** 不知道 `chat_id` 时，先用 `lark-im` 搜群：`lark-cli im +chat-search --query "<群名关键词>" --as user`；取结果里的 `oc_xxx`。

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

- 筛选值类型由字段计算结果类型动态决定。
- 拿不准时，先把 `value` 当作单个字符串填入做一次尝试。
- 如果报错，再按错误提示把 `value` 改成对应类型。

字符串示例：

```json
["风险说明", "intersects", "高风险"]
```

数字示例：

```json
["汇总分", ">=", 80]
```

## 4. 易错点

- 不要再写旧对象风格：`{"field_name":...,"operator":...}`。
- `user` / `group_chat` / `link` 不要写成单个标量。
- `empty` / `non_empty` 不要硬塞无意义的 value。
- 日期条件稳定写法用 `ExactDate(...)` 或 `Today` / `Yesterday` / `Tomorrow`。
- `formula` / `lookup` 的 value 形状不固定；拿不准时先读当前配置或字段定义，或根据错误提示修正类型。

## 5. 参考
- [lookup-field-guide.md](lookup-field-guide.md)
