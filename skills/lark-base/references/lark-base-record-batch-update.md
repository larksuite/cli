# base +record-batch-update (batch update)

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过 `update_records` 为每条记录提交字段值。

## 推荐命令

```bash
lark-cli base +record-batch-update --base-token <base_token> --table-id <table_id> \
  --json '{"update_records":{"<record_id_a>":{"状态":["完成"]},"<record_id_b>":{"分数":20}}}'

lark-cli base +record-batch-update --base-token <base_token> --table-id <table_id> --json @batch-update.json
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--json <body>` | 是 | 批量更新请求体，必须是 JSON 对象。支持直接传 JSON 字符串，或 `@<file_path>` 从文件读取 |

## API

`POST /open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_update`

## `--json` 结构

本节只说明 `+record-batch-update` 的外层 JSON 形状；CellValue 统一看 [lark-base-cell-value.md](lark-base-cell-value.md)。

对象形态：

```json
{"update_records":{"recA":{"状态":["完成"]},"recB":{"分数":20}}}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `update_records` | `Map<RecordID, Map<FieldNameOrID, CellValue>>` | 是 | record ID 到字段更新对象的映射（单次最多 200 条） |

## 返回重点

成功响应只包含可选的 `ignored_fields`；没有忽略字段时 `data` 为空对象。请求不会预先校验 record ID 是否存在，因此需要确认实际写入结果时，应再用 `+record-get` 读回目标记录。

## 坑点

- 单次最多更新 200 条记录，超过会被接口校验拒绝。
- 命令不会自动做字段/行映射转换，传什么就发什么。
- 如果字段映射包含只读字段，返回里可能出现 `ignored_fields`；这些字段不会被更新。

## 参考

- [lark-base-cell-value.md](lark-base-cell-value.md) — CellValue 格式规范
