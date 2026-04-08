# base +record-batch-set

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

批量更新记录（将同一份 `patch` 批量应用到一批 `record_id_list`）。

## 推荐命令

```bash
lark-cli base +record-batch-set \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --json '{"record_id_list":["rec_xxx"],"patch":{"field_id_or_name":"value"}}'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--json <body>` | 是 | 批量更新请求体，必须是 JSON 对象 |

## 生成 `patch` 前必看

- 先阅读 [lark-base-shortcut-record-value.md](lark-base-shortcut-record-value.md)，按字段类型构造 `patch` 的 value，避免类型不匹配。

## API 入参详情

**HTTP 方法和路径：**

```
PATCH /open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch
```

## `--json` 结构

- 对象形态：`{"record_id_list":[...],"patch":{...}}`。
- `record_id_list`：要更新的记录 ID 列表（单次最多 200 条）。
- `patch`：同一份字段更新对象，会应用到 `record_id_list` 内所有记录。

## 返回重点

- 返回对象（`OpenAPIBatchPatchResult`）：
  - `has_more`（`boolean`，必返）
  - `record_id_list`（`string[]`，必返）
  - `update`（`object`，必返；可能为空对象）
  - `ignored_fields`（`{id,name,reason}[]`，可选）

## 坑点

- ⚠️ `--json` 必须是对象。
- ⚠️ 该接口是“同值批量更新”：同一请求内所有 `record_id_list` 都会应用同一份 `patch`。
- ⚠️ `record_id_list` 最大 200 条，超过会被接口校验拒绝。
- ⚠️ 命令不会自动做字段/行映射转换，传什么就发什么。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
