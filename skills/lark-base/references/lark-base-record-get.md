# base +record-get

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

获取单条记录，可选裁剪输出字段。

## 推荐命令

```bash
lark-cli base +record-get \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --record-id rec_xxx

lark-cli base +record-get \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --record-id rec_xxx \
  --field-id fld_status \
  --field-id 项目名称
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--record-id <id>` | 是 | 记录 ID |
| `--field-id <id_or_name>` | 否 | 字段 ID 或字段名；可重复传入多个 `--field-id` 裁剪返回列；不指定时默认返回所有字段 |

## API 入参详情

**HTTP 方法和路径：**

```
GET /open-apis/base/v3/bases/:base_token/tables/:table_id/records/:record_id
```

## 返回重点

- 成功时直接返回接口 `data` 字段内容。
- 传了 `--field-id` 时，由服务端按字段裁剪返回结果。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
