# Base data-query guide

Read this guide after the [data analysis SOP](lark-base-data-analysis-sop.md) enters the Cloud path and selects `+data-query`, or directly when the user explicitly asks about the `+data-query` command or DSL. It provides common aggregation fewshots; use [lark-base-data-query.md](lark-base-data-query.md) only for complete DSL fields, operators, limits, response details, or error recovery.

## When to use

Use `+data-query` when the user asks for server-side:

- group by / aggregation
- sum, average, min, max, count, distinct count
- filtered aggregation
- sorted Top N or Bottom N
- global statistical conclusions

`+data-query` can return dimension field rows, but those rows are grouped by dimension values and do not include `record_id`. Use `+record-list`, `+record-search`, or `+record-get` for row-level output, record identity, or full raw record details.

## Common Fewshots

Count records by a category field:

```bash
lark-cli base +data-query \
  --base-token <base_token> \
  --dsl '{"datasource":{"type":"table","table":{"tableId":"<table_id>"}},"dimensions":[{"field_name":"Status","alias":"status"}],"measures":[{"field_name":"Status","aggregation":"count","alias":"count"}],"shaper":{"format":"flat"}}'
```

Sum a number field by category and return Top 10:

```bash
lark-cli base +data-query \
  --base-token <base_token> \
  --dsl '{"datasource":{"type":"table","table":{"tableId":"<table_id>"}},"dimensions":[{"field_name":"Region","alias":"region"}],"measures":[{"field_name":"Amount","aggregation":"sum","alias":"total_amount"}],"sort":[{"field_name":"total_amount","order":"desc"}],"pagination":{"limit":10},"shaper":{"format":"flat"}}'
```

Aggregate only records matching a filter:

```bash
lark-cli base +data-query \
  --base-token <base_token> \
  --dsl '{"datasource":{"type":"table","table":{"tableId":"<table_id>"}},"dimensions":[{"field_name":"Owner","alias":"owner"}],"measures":[{"field_name":"Amount","aggregation":"sum","alias":"total_amount"}],"filters":{"type":1,"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":["Done"]}]},"shaper":{"format":"flat"}}'
```

## Common filter values

Common `Condition.value` shapes: select `is` / `isNot` uses exactly one option
name; datetime `is` / `isGreater` / `isLess` uses `["Today"]` or
`["ExactDate","<epoch_ms>"]`; `isEmpty` / `isNotEmpty` uses `[]`.
Use relative date keywords only for relative requests; see
[lark-base-data-query.md](lark-base-data-query.md) for other field types and operators.

Use `tableName` when the table ID is unavailable but the table name is known:

```bash
lark-cli base +data-query \
  --base-token <base_token> \
  --dsl '{"datasource":{"type":"table","table":{"tableName":"Orders"}},"measures":[{"field_name":"Amount","aggregation":"sum","alias":"total_amount"}],"shaper":{"format":"flat"}}'
```

## Routing to the DSL SSOT

Read [lark-base-data-query.md](lark-base-data-query.md) when you need:

- the full DSL field reference
- supported aggregations and field types
- filter operator details
- pagination and result limits
- response shape and error recovery
