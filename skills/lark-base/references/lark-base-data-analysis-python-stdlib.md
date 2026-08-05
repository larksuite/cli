# Base NDJSON：Python 标准库示例

仅在 Local SOP 已选择 Python 标准库后读取。本页不重复 Base 的粒度与关系规则，只展示对应实现。

示例假设 `records.ndjson` 包含 `record_id`、`日期`、`状态`、`金额`、`负责人`、`标签`、`关联客户`；`customers.ndjson` 包含 `record_id`、`客户名称`。多值列使用 Local SOP 定义的数组结构。

## 加载与日期解析

NDJSON 每行是一条独立 JSON record；按行读取即可，不要先把文件整体载入字符串。

```python
import json
from datetime import datetime
from decimal import Decimal, ROUND_HALF_UP


def read_ndjson(path):
    with open(path, encoding="utf-8") as stream:
        for line in stream:
            if line.strip():
                yield json.loads(line, parse_float=Decimal)


def as_decimal(value):
    return Decimal(str(value)) if value is not None else Decimal("0")


def round_money(value):
    return value.quantize(Decimal("0.01"), rounding=ROUND_HALF_UP)


records = list(read_ndjson("records.ndjson"))
for record in records:
    value = record["日期"]
    record["日期"] = datetime.fromisoformat(value) if value else None
```

金额分摊使用 `Decimal` 并只在最终输出字段处调用 `round_money`；不要在中间分摊时提前舍入。

## 集合谓词：保持 record 粒度

筛选“状态”包含“进行中”的记录，并在 record 粒度汇总：

```python
active = [record for record in records if "进行中" in record["状态"]]
summary = {
    "records_count": len(active),
    "amount_sum": sum(
        (as_decimal(record["金额"]) for record in active), Decimal("0")
    ),
}
```

## 单数组展开：切换到人员粒度

用嵌套循环表达 lateral expansion；按人员 `id` 聚合，`name` 只用于展示。

```python
from collections import defaultdict

record_ids_by_owner = defaultdict(set)
owner_names = {}
for record in records:
    for owner in record["负责人"]:
        record_ids_by_owner[owner["id"]].add(record["record_id"])
        owner_names[owner["id"]] = owner["name"]

by_owner = sorted(
    (
        {
            "user_id": user_id,
            "user_name": owner_names[user_id],
            "records_count": len(record_ids),
        }
        for user_id, record_ids in record_ids_by_owner.items()
    ),
    key=lambda row: (-row["records_count"], row["user_id"]),
)
```

## Link JOIN：先建立目标表索引

目标表的 `record_id` 是唯一主键，可直接建立哈希索引；Link 的 `id` 用于索引查找。

```python
customers = {
    customer["record_id"]: customer
    for customer in read_ndjson("customers.ndjson")
}

joined = []
for record in records:
    for link in record["关联客户"]:
        customer = customers.get(link["id"])
        joined.append(
            {
                "source_record_id": record["record_id"],
                "target_record_id": link["id"],
                "客户名称": customer["客户名称"] if customer else None,
            }
        )
```

## 多数组共现：显式生成行内笛卡尔积

两层嵌套循环表示同一 source record 内的 `负责人 × 标签`：

```python
record_ids_by_pair = defaultdict(set)
owner_names = {}
for record in records:
    for owner in record["负责人"]:
        owner_names[owner["id"]] = owner["name"]
        for tag in record["标签"]:
            record_ids_by_pair[(owner["id"], tag)].add(record["record_id"])

cooccurrence = sorted(
    (
        {
            "user_id": user_id,
            "user_name": owner_names[user_id],
            "标签": tag,
            "records_count": len(record_ids),
        }
        for (user_id, tag), record_ids in record_ids_by_pair.items()
    ),
    key=lambda row: (-row["records_count"], row["user_id"], row["标签"]),
)
```
