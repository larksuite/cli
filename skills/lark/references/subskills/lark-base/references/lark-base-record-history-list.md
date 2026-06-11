# base +record-history-list

查询单条记录的变更历史。它返回历史事件，不返回记录当前值，也不支持整表审计扫描。

## 推荐命令

```bash
lark-cli base +record-history-list \
  --base-token <base_token> \
  --table-id <table_id> \
  --record-id <record_id>

lark-cli base +record-history-list \
  --base-token <base_token> \
  --table-id <table_id> \
  --record-id <record_id> \
  --page-size 30 \
  --max-version <next_max_version>
```

## 返回解释

- 历史条目通常按版本号降序返回，最新在前。
- 每条历史包含版本号、操作人、操作时间、操作类型和字段变更。
- `create_time` 是秒级 Unix 时间戳。
- `field_changes` 描述字段变更，重点看字段名/字段类型、`before` 和 `after`。
- `activity_type` 常见值：`create`（创建记录）、`update`（编辑记录）、`delete`（删除记录）。

以下字段类型的变化可能不会出现在 `field_changes` 中：

- 计算字段：`formula`、`lookup`
- 系统字段：自动编号、创建时间、创建人、修改时间、修改人

## 翻页

- 首次请求不传 `--max-version`。
- 如果返回 `has_more=true`，取返回中的 `next_max_version` 作为下一次请求的 `--max-version`。
- `--page-size` 默认 30，最大 50。

## 注意

- `table-id` 和 `record-id` 必须来自同一张表。
- 这是单条记录历史，不是表级审计；需要查多条记录时串行调用。
