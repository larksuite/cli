# base +view-set-filter

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

更新视图筛选配置。

## 1. filter 结构

`--json` 就是一个 filter 条件对象，结构见公共协议 SSOT [lark-base-filter-condition.md](lark-base-filter-condition.md)，即 `{logic?, conditions?}`。此处 `conditions` 中的 `field` 引用**数据表字段名或字段 id**。

- 支持 `filter` 的视图类型：`grid`、`kanban`、`gallery`、`calendar`、`gantt`。

## 2. 推荐命令

```bash
lark-cli base +view-set-filter \
  --base-token <base_token> \
  --table-id <table_id> \
  --view-id <view_id> \
  --json '{"logic":"and","conditions":[["状态","intersects",["Doing"]],["负责人","intersects",[{"id":"ou_xxx"}]],["截止时间","empty"]]}'
```

## 3. JSON 写法

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

完整的 operator 列表与各字段类型的 value 写法（`text` / `number` / `select` / `user` / `datetime` / `formula` / `lookup` 等），见 [lark-base-filter-condition.md](lark-base-filter-condition.md)。

## 4. 使用建议

- 先读取当前筛选配置，理解现有 `logic` 和 `conditions` 的组合关系；只替换用户要求变更的条件，未提到的条件默认保留。
- 优先传字段 id，不要依赖字段名。
- 拿不准字段 type 或真实取值时，先用 `+field-list` / `+record-list` 确认，再按对应字段类型的 value 写法构造条件；别按字段名猜 type、凭印象猜枚举取值。
- 需要清空全部筛选时，直接传 `{"conditions":[]}`。

## 5. 易错点

- 本 tuple DSL 由 `+view-set-filter` 与 `+record-list` / `+record-search` 的 `--filter-json` 共用；不要写成 `+data-query` 的对象风格 `{"field_name":...,"operator":...}`（会报校验失败）。
- 标量类字段（`text` / `number` / `datetime` 等）的 value 用标量、别包成数组（各类型详见 value 写法一节）。
- `user` / `group_chat` / `link` 不要写成单个标量。
- `empty` / `non_empty` 不要硬塞无意义的 value。
- 日期条件稳定写法用 `ExactDate(...)` 或 `Today` / `Yesterday` / `Tomorrow`。
- `formula` / `lookup` 的 value 形状不固定；拿不准时先读当前 filter 或字段定义，或根据错误提示修正类型。

## 6. 参考

- [lark-base-filter-condition.md](lark-base-filter-condition.md)：filter/visible_rule 条件结构公共协议 SSOT
- [lookup-field-guide.md](lookup-field-guide.md)
