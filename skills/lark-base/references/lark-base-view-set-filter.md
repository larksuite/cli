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

### 名称 → 配置 → 记录

创建或重命名视图前，先做视图名称拆解，把名称承诺分别映射到真实配置：

| 名称语义 | 必须核验的配置 |
|---|---|
| “按 X 分组”“X 看板” | 视图类型符合意图，且 `+view-get-group` 的字段就是 X |
| “未完成”“未归还”“待处理”等集合 | `+view-get-filter` 覆盖该业务集合的全部状态或原始事实 |
| “本周”“本月”“今年”等时间范围 | `+view-get-filter` 为动态时间条件或动态 Formula，不是固定日期 |
| “优先”“最早”“最新”等顺序 | `+view-get-sort` 的字段和方向正确 |

保存后执行三层验收：

1. **名称**：逐项列出名称承诺的视图类型、筛选、分组、排序和时间范围。
2. **配置**：调用对应的 `+view-get-filter/group/sort/card/timebar/visible-fields`，逐项比对字段、operator、值、方向和范围；不能只确认返回非空。
3. **记录**：用 `+record-list --view-id <view_id>` 读取代表性命中项，同时从原表检查至少一条应排除项；结果必须符合名称和用户意图。

若配置正确但名称不准确，改名；若名称准确但配置不符，修正配置。禁止保留误导性名称后交付。

Few-shot：

- **订单状态看板**：正确做法是创建 `kanban` 并让 `+view-get-group` 返回 `订单状态`；按 `贸易术语`、客户或负责人分组却仍叫“订单状态看板”是错误。
- **未归还钥匙**：优先筛 `实际归还时间 empty`，这样会同时包含借用中和逾期未还；只筛一个展示状态而漏掉另一类未归还记录是错误。
- **本周排班**：若创建此名称，必须用动态本周条件或 `TODAY()` 派生字段筛选；只有名称、空 filter 或固定日期区间都不成立。用户没有要求且未创建该名称时，不强制额外创建。

## 5. 易错点

- 本 tuple DSL 由 `+view-set-filter` 与 `+record-list` / `+record-search` 的 `--filter-json` 共用；不要写成 `+data-query` 的对象风格 `{"field_name":...,"operator":...}`（会报校验失败）。
- 标量类字段（`text` / `number` / `datetime` 等）的 value 用标量、别包成数组（各类型详见 value 写法一节）。
- `user` / `group_chat` / `link` 不要写成单个标量。
- `empty` / `non_empty` 不要硬塞无意义的 value。
- 日期条件稳定写法用 `ExactDate(...)` 或 `Today` / `Yesterday` / `Tomorrow`。
- `formula` / `lookup` 的 value 形状不固定；拿不准时先读当前 filter 或字段定义，或根据错误提示修正类型。

### 持久视图的相对时间

用户要求“本月、今年、年度、年底、近 N 天/月、超过 N 天/月、即将到期”等会随时间推进的范围时，不能把运行当天换算成固定 `ExactDate(...)` 边界后保存到 View。优先使用受支持的相对时间关键字；无法直接表达时，创建使用 `TODAY()` / `YEAR(TODAY())` 等动态函数的辅助 Formula，再让 View 筛选该字段。

设置后必须用 `+view-get-filter` 回读保存条件，并在有代表性记录时用 `+record-list --view-id <view_id>` 验证范围。名称里写“本月”或当前样例恰好命中，均不能证明动态范围正确；发现固定当前年月、缺少年度范围或筛选结果与语义不一致时必须返工。

## 6. 参考

- [lark-base-filter-condition.md](lark-base-filter-condition.md)：filter/visible_rule 条件结构公共协议 SSOT
- [Lookup Field](lark-base-field-lookup.md)
