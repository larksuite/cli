# base +record-batch-create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

批量创建记录。

## 适用场景（重点）

- 适合导入 CSV / Excel、外部系统一次性写入新数据。
- 先把每条输入数据映射为独立的字段对象，再组装到 `create_records`。
- `+record-batch-create` 只创建，不会按业务字段自动查重。全新导入且每个输入行都应成为独立记录时可直接创建；向已有表增量补齐或用户要求避免重复时，先完成下列差集检查。

### 增量补齐：只创建缺失业务键

1. 从业务语义确定稳定唯一键；它可以是一个字段，也可以是多个字段的组合。没有可靠唯一键时停止猜测，不要批量创建。
2. 用 `+record-list --filter-json` 限定本次目标范围，读取唯一键所需字段；范围内有分页时读完全部页。
3. 只按用户或数据契约明确的规则规范化候选键和现有键；没有规则时按原始类型和值精确比较，不自行忽略大小写、裁剪空格或改写格式。候选键缺失、空值或无法规范化时将该行标记为 `blocked`，不得放入 `create_records`；现有记录存在无效键时停止写入并报告对应 `record_id`，因为此时无法证明去重范围完整。
4. 多个候选使用同一规范化键时，只有各字段值一致的重复候选才能去重为一条；字段值不一致时停止并报告冲突，不得静默选择其中一条。多个现有记录共享同一键时视为已存在，不再创建；若用户要求更新，先报告冲突，不得任选一个 `record_id`。
5. 对有效候选键去重后求与现有键的差集；`create_records` 只放差集中的缺失键。
6. 已有键默认跳过；用户要求修改已有记录时，先取得唯一对应的 `record_id`，再改用 `+record-upsert --record-id` 或 `+record-batch-update`。不要把已有键再次交给 batch-create。

## 推荐命令

```bash
lark-cli base +record-batch-create --base-token <base_token> --table-id <table_id> \
  --json '{"create_records":[{"标题":"任务 A","状态":"Open"},{"标题":"任务 B","状态":"Done"}]}'

lark-cli base +record-batch-create --base-token <base_token> --table-id <table_id> --json @batch-create.json
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--json <body>` | 是 | 批量创建请求体，必须是 JSON 对象。支持直接传 JSON 字符串，或 `@<file_path>` 从文件读取 |

## API

`POST /open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_create`

## `--json` 结构

本节只说明 `+record-batch-create` 的外层 JSON 形状；CellValue 统一看 [lark-base-cell-value.md](lark-base-cell-value.md)。

对象形态：

```json
{"create_records":[{"标题":"任务 A","状态":"Open"},{"标题":"任务 B","状态":"Done"}]}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `create_records` | `Array<Map<FieldNameOrID, CellValue>>` | 是 | 记录字段对象数组；每条记录可以提交不同字段，单次最多 200 条 |

## 返回重点

返回 `record_id_list` 和可选的 `ignored_fields`。

## 坑点

- 每个 `create_records` 元素都是独立的记录字段对象，只提交该记录需要写入的字段。
- 单次最多 200 条；`1254104` 表示超过单批上限，拆成多个批次。
- `1254045` 表示字段不存在，重新 `+field-list` 后使用真实字段名或 `field_id`。
- `1254015` 表示 CellValue 类型不匹配，按真实 Field schema 和 CellValue 规范修正。
- 返回 `ignored_fields` / `READONLY` 时，从普通 Record 写入中移除 Formula、Lookup、系统字段和自动编号等只读字段。
- 同一 Table 连续批量写入使用串行执行；`1254291` 表示并发写冲突，短暂等待后重试当前批次。
- `select` 字段只支持写入字段中已有的选项；构造 CellValue 前先用 `+field-list` 或 `+field-search-options` 确认目标选项存在。

## 参考

- [lark-base-cell-value.md](lark-base-cell-value.md) — CellValue 格式规范
