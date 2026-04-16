# base +record-list

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

分页列出一张表里的记录；可按视图过滤，也可按字段裁剪返回列。

> 默认优先使用 `+record-list`；仅当用户提供明确搜索关键词时，才使用 [lark-base-record-search.md](lark-base-record-search.md)。

## 返回关键字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `has_more` | boolean | 是否还有下一页数据；`true` 表示可继续翻页，`false` 表示已到末页 |
| `query_context.record_scope` | string | 记录范围：`all_records`（全表）或 `view_filtered_records`（按视图过滤） |
| `query_context.field_scope` | string | 字段范围：`selected_fields`（显式传 `--field-id`）/ `view_visible_fields`（未传 `--field-id` 且按视图可见字段）/ `all_fields`（未传 `--field-id` 且无视图限制） |

## 字段返回优先级

- `query_context.field_scope` 的优先级为：`selected_fields`（explicit `--field-id`） > `view_visible_fields`（view visible fields） > `all_fields`（table all fields）。

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
  --base-token XXXXXX \
  --table-id tblXXX \
  --offset 0 \
  --limit 100

lark-cli base +record-list \
  --base-token XXXXXX \
  --table-id tblXXX \
  --view-id vewXXX \
  --field-id fldStatus \
  --field-id 项目名称 \
  --offset 0 \
  --limit 50
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | Base Token |
| `--table-id <id_or_name>` | 是 | 表 ID 或表名 |
| `--view-id <id>` | 否 | 视图 ID；传入后只读该视图结果 |
| `--field-id <id_or_name>` | 否 | 字段 ID 或字段名；可重复传入多个 `--field-id` 裁剪返回列 |
| `--offset <n>` | 否 | 分页偏移，默认 `0` |
| `--limit <n>` | 否 | 分页大小，默认 `100`，范围 `1-200`（最大 `200`，超过会报错） |

## API 入参详情

**HTTP 方法和路径：**

```
GET /open-apis/base/v3/bases/:base_token/tables/:table_id/records
```

- 查询参数会附带 `view_id / field_id(repeatable) / offset / limit`。


## 坑点

- ⚠️ `+record-list` 禁止并发调用；批量拉多个视图或多张表时必须串行。
- ⚠️ `--limit` 最大 `200`，不要传超过 `200` 的值。
- ⚠️ 分页时优先根据返回的 `has_more` 判断是否继续请求，不要盲目预拉全量数据。
- ⚠️ `--field-id` 接受字段 ID 或字段名。
- ⚠️ 复杂筛选优先落到视图里，再用 `--view-id` 读取。
- ⚠️ **分页字段顺序不稳定**：API 返回的 `field_id_list` 顺序在不同分页中可能不同，必须用 `field_id` 定位字段，且每页都要重新计算索引。详见下方"分页最佳实践"。

## 分页最佳实践

### 问题：字段顺序可能变化

API 返回的数据中：
1. `data[i][j]` 对应 `field_id_list[j]` 的字段
2. **不同分页的 `field_id_list` 顺序可能不同**
3. 用硬编码索引定位字段会导致数据错位

### 解决方案：用 field_id 定位字段

```python
import subprocess
import json

# 先获取字段 ID（一次性）
# lark-cli base +field-list --base-token xxx --table-id xxx
ID_FIELD_ID = "fld64hLsFo"  # 编号字段的 field_id（固定不变）

def run_lark_cli(args):
    """执行 lark-cli 命令并返回 JSON 结果"""
    result = subprocess.run(
        ['lark-cli'] + args,
        capture_output=True, text=True
    )
    if result.returncode != 0:
        raise RuntimeError(f"lark-cli 失败: {result.stderr}")
    return json.loads(result.stdout)

def load_all_records(base_token, table_id):
    all_records = []
    offset = 0
    page_size = 200  # API 最大 200

    while True:
        result = run_lark_cli([
            'base', '+record-list',
            '--base-token', base_token,
            '--table-id', table_id,
            '--limit', str(page_size),
            '--offset', str(offset)
        ])

        data = result.get('data', {})
        field_ids = data.get('field_id_list', [])
        records = data.get('data', [])

        # 先检查空记录，避免后续索引操作出错
        if len(records) == 0:
            break

        # 检查目标字段是否存在
        try:
            id_idx = field_ids.index(ID_FIELD_ID)
        except ValueError:
            raise ValueError(
                f"找不到字段 {ID_FIELD_ID}，请检查 field_id 是否正确。"
                f"可用命令查看: lark-cli base +field-list --base-token {base_token} --table-id {table_id}"
            )

        for record_data in records:
            # 关键：将整行转成 {field_id: value} 字典，彻底消除顺序依赖
            record_dict = {field_ids[i]: record_data[i] for i in range(len(field_ids))}
            sample_id = record_dict.get(ID_FIELD_ID)
            all_records.append({'id': sample_id, 'data': record_dict})

        offset += len(records)

        if not data.get('has_more', False):
            break

    return all_records
```

### 如何获取 field_id

```bash
lark-cli base +field-list --base-token app_xxx --table-id tbl_xxx
```

输出中每个字段都有 `field_id`，格式为 `fld` 开头的字符串。

## 参考

- [lark-base-record.md](lark-base-record.md) — record 索引页
- [lark-base-view-set-filter.md](lark-base-view-set-filter.md) — 配筛选
