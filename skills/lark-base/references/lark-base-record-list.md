# base +record-list

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

分页列出一张表里的记录；可按视图过滤。

## 返回关键字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `has_more` | boolean | 是否还有下一页数据；`true` 表示可继续翻页，`false` 表示已到末页 |

## 按需翻页规则

1. 先执行一次 `+record-list` 获取首批结果。
2. 检查返回的 `has_more`。
3. 仅当同时满足以下条件时才继续翻页：
   - `has_more = true`
   - 用户问题需要更多数据（例如“全部导出”“统计全量明细”“继续加载下一页”）
4. 若用户只要部分结果（例如“先看前 20 条”“先给示例数据”），即使 `has_more = true` 也不继续翻页。
5. 继续翻页时，`offset` 按已读取数量递增，直到满足用户需求或 `has_more = false`。

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
  --offset 0 \
  --limit 50
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--view-id <id>` | 否 | 视图 ID；传入后只读该视图结果 |
| `--offset <n>` | 否 | 分页偏移，默认 `0` |
| `--limit <n>` | 否 | 分页大小，默认 `100`，范围 `1-200`（最大 `200`，超过会报错） |

## API 入参详情

**HTTP 方法和路径：**

```
GET /open-apis/base/v3/bases/:base_token/tables/:table_id/records
```

- 查询参数会附带 `view_id / offset / limit`。


## 坑点

- ⚠️ `+record-list` 禁止并发调用；批量拉多个视图或多张表时必须串行。
- ⚠️ `--limit` 最大 `200`，不要传超过 `200` 的值。
- ⚠️ 分页时优先根据返回的 `has_more` 判断是否继续请求，不要盲目预拉全量数据。
- ⚠️ 复杂筛选优先落到视图里，再用 `--view-id` 读取。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
- [lark-base-view-set-filter.md](lark-base-view-set-filter.md) — 配筛选
