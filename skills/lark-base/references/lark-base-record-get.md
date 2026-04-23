# base +record-get

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

获取一条或多条记录；批量读取时建议显式裁剪字段，避免响应体过大。

## 推荐命令

```bash
lark-cli base +record-get \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --record-id rec_xxx
```

```bash
lark-cli base +record-get \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --record-id rec_001 \
  --record-id rec_002 \
  --field-id 标题 \
  --field-id fld_status
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--record-id <id>` | 否 | 记录 ID，可重复使用；这是主推荐用法 |
| `--field-id <id_or_name>` | 否 | 字段 ID 或字段名；可重复传入多个 `--field-id` 裁剪返回字段 |
| `--json <object>` | 否 | 脚本/代理场景可传 `{"record_id_list":["rec_xxx"]}`；也可附带 `select_fields` |

## API 入参详情

**HTTP 方法和路径：**

```
GET /open-apis/base/v3/bases/:base_token/tables/:table_id/records/:record_id
POST /open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_get
```

## 返回重点

- CLI 内部统一通过 `batch_get` 读取记录；单个 `--record-id` 仍会整理成单记录输出形态。
- 多个 `--record-id`，或使用 `--json` 时，返回批量读取结果。
- `--field-id` 会映射为 `batch_get` body 的 `select_fields`，确保真正只返回所选字段。
- 建议只传需要的字段，减少响应体体积和 AI 上下文消耗。
- 成功时直接返回接口 `data` 字段内容。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
