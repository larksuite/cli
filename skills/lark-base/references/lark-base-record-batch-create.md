# base +record-batch-create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

批量创建记录。

## 适用场景（重点）

- 适合导入 CSV / Excel、外部系统一次性写入新数据。
- 先把每条输入数据映射为独立的字段对象，再组装到 `create_records`。

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
- 单次最多 200 条，超出需分批写入。
- `select` 字段只支持写入字段中已有的选项；构造 CellValue 前先用 `+field-list` 或 `+field-search-options` 确认目标选项存在。

## 参考

- [lark-base-cell-value.md](lark-base-cell-value.md) — CellValue 格式规范
