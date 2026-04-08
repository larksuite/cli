# base +record-search

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

按关键词在指定字段范围内检索记录；CLI 侧通过 `--json` 透传请求体。

## 适用场景

- 需要“按关键词 + 指定字段”快速检索记录。
- 需要附带 `view_id / select_fields` 控制检索范围与返回字段。
- 不用于聚合统计。涉及 SUM/AVG/COUNT/MAX/MIN 时改用 `+data-query`。

## 推荐命令

```bash
lark-cli base +record-search \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --json '{"keyword":"Created","search_fields":["Title","fld_owner"],"offset":0,"limit":100}'

lark-cli base +record-search \
  --base-token app_xxx \
  --table-id tbl_xxx \
  --json '{"view_id":"viw_xxx","keyword":"Alice","search_fields":["Title","Owner"],"select_fields":["Title","Owner","Status"],"offset":0,"limit":50}'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--json <object>` | 是 | 搜索请求体 JSON（结构要求见下方“JSON 要求”） |

## API 入参详情

**HTTP 方法和路径：**

```
POST /open-apis/base/v3/bases/:base_token/tables/:table_id/records/search
```

### JSON 格式要求

| 字段 | 必填 | 类型 | 约束 |
|------|------|------|------|
| `view_id` | 否 | string | 无额外约束 |
| `keyword` | 是 | string | 非空，最小长度 `1` |
| `search_fields` | 是 | string[] | 数组长度 `1-100`；每项是 `FieldRef`（字符串，长度 `1-100`） |
| `select_fields` | 否 | string[] | 数组长度 `<=100`；每项是 `FieldRef`（字符串，长度 `1-100`） |
| `offset` | 否 | int | `>=0`，默认 `0` |
| `limit` | 否 | int | `1-200`，默认 `10` |

## 坑点

- ⚠️ `+record-search` 用于检索，不用于聚合分析；聚合场景使用 `+data-query`。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
- [lark-base-record-list.md](lark-base-record-list.md) — 分页列表读取
- [lark-base-data-query.md](lark-base-data-query.md) — 聚合分析
