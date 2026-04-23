# base +record-delete

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

删除一条或多条记录。

## 推荐命令

```bash
lark-cli base +record-delete \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --record-id rec_xxx \
  --yes
```

```bash
lark-cli base +record-delete \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --record-id rec_001 \
  --record-id rec_002 \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--record-id <id>` | 否 | 与 `--json` 二选一；记录 ID，可重复使用；这是主推荐用法 |
| `--json <object>` | 否 | 与 `--record-id` 二选一；脚本/代理场景可传 `{"record_id_list":["rec_xxx"]}` |

## API 入参详情

**HTTP 方法和路径：**

```http
DELETE /open-apis/base/v3/bases/:base_token/tables/:table_id/records/:record_id
POST /open-apis/base/v3/bases/:base_token/tables/:table_id/records/batch_delete
```

## 返回重点

- 单个 `--record-id` 时，沿用单记录删除接口并返回 `deleted: true` 与 `record_id`。
- 多个 `--record-id`，或使用 `--json` 时，自动切换到批量删除接口并返回接口原始 `data`。

## 工作流

> 这是**高风险写入操作**。CLI 层要求显式传 `--yes`；如果用户已经明确要求删除且目标明确，直接执行并带上 `--yes`，不要再补一次确认。

1. 建议先用 `+record-get` 确认目标记录。
2. 只有当目标记录仍不明确时，才继续追问；如果删除意图和目标都明确，直接执行。

## 坑点

- ⚠️ 高风险写操作，删除后不可恢复。
- ⚠️ 忘记带 `--yes` 会被 CLI 拦截。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
