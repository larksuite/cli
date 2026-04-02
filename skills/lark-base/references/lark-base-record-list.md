# base +record-list

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

分页列出一张表里的记录；可按视图过滤，也可按字段裁剪返回列。

## 推荐命令

```bash
lark-cli base +record-list \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --offset 0 \
  --limit 100

lark-cli base +record-list \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --view-id viw_xxx \
  --field fld_status \
  --field 项目名称 \
  --offset 0 \
  --limit 50
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--view-id <id>` | 否 | 视图 ID；传入后只读该视图结果 |
| `--field <id_or_name>` | 否 | 字段 ID 或字段名；可重复传入多个 `--field` 裁剪返回列 |
| `--offset <n>` | 否 | 分页偏移，默认 `0` |
| `--limit <n>` | 否 | 分页大小，默认 `100`，范围 `1-200`（最大 `200`，超过会报错） |

## API 入参详情

**HTTP 方法和路径：**

```
GET /open-apis/base/v3/bases/:base_token/tables/:table_id/records
```

- 查询参数会附带 `view_id / field(repeatable) / offset / limit`。


## 坑点

- ⚠️ `+record-list` 禁止并发调用；批量拉多个视图或多张表时必须串行。
- ⚠️ `--limit` 最大 `200`，不要传超过 `200` 的值。
- ⚠️ `--field` 接受字段 ID 或字段名。
- ⚠️ 复杂筛选优先落到视图里，再用 `--view-id` 读取。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
- [lark-base-view-set-filter.md](lark-base-view-set-filter.md) — 配筛选
