# Base NDJSON：pandas 示例

仅在统一数据分析 SOP 已选择 pandas 后读取。本页不重复 Base 的粒度与关系规则，只展示对应实现。

示例假设 `records.ndjson` 包含 `record_id`、`日期`、`状态`、`金额`、`负责人`、`标签`、`关联客户`；`customers.ndjson` 包含 `record_id`、`客户名称`。多值列使用统一数据分析 SOP 定义的数组结构。

## 加载与日期解析

```python
import pandas as pd

records = pd.read_json("records.ndjson", lines=True)
raw_dates = records["日期"].astype("string")
records["日期_local"] = pd.to_datetime(
    raw_dates.str.slice(0, 10), format="%Y-%m-%d", errors="coerce"
)
records["日期_instant"] = pd.to_datetime(
    raw_dates, format="ISO8601", utc=True, errors="coerce"
)
```

按来源 Base 的日、周、月分组使用 `日期_local`；计算真实时长、排序或跨时区比较使用 `日期_instant`。实际任务只需构造所需的一列。

## 集合谓词：保持 record 粒度

筛选“状态”包含“进行中”的记录，并在 record 粒度汇总：

```python
active = records[records["状态"].map(lambda values: "进行中" in values)]
summary = {
    "records_count": len(active),
    "amount_sum": active["金额"].sum(min_count=1),
}
```

## 单数组展开：切换到人员粒度

```python
owners = records[["record_id", "负责人"]].explode("负责人", ignore_index=True)
owners = owners[owners["负责人"].notna()].assign(
    user_id=lambda df: df["负责人"].map(lambda user: user["id"]),
    user_name=lambda df: df["负责人"].map(lambda user: user["name"]),
)
by_owner = (
    owners.groupby(["user_id", "user_name"], as_index=False)
    .agg(records_count=("record_id", "nunique"))
    .sort_values("records_count", ascending=False)
)
```

## Link JOIN：先建立边表

```python
edges = (
    records[["record_id", "关联客户"]]
    .rename(columns={"record_id": "source_record_id"})
    .explode("关联客户", ignore_index=True)
)
edges = edges[edges["关联客户"].notna()].assign(
    target_record_id=lambda df: df["关联客户"].map(lambda link: link["id"])
)[["source_record_id", "target_record_id"]]

customers = pd.read_json("customers.ndjson", lines=True).rename(
    columns={"record_id": "target_record_id"}
)
joined = edges.merge(
    customers[["target_record_id", "客户名称"]],
    on="target_record_id",
    how="left",
)
```

## 多数组共现：显式生成行内笛卡尔积

连续两次 `explode` 表示同一 source record 内的 `负责人 × 标签`：

```python
pairs = (
    records[["record_id", "负责人", "标签"]]
    .explode("负责人", ignore_index=True)
    .explode("标签", ignore_index=True)
    .dropna(subset=["负责人", "标签"])
    .assign(
        user_id=lambda df: df["负责人"].map(lambda user: user["id"]),
        user_name=lambda df: df["负责人"].map(lambda user: user["name"]),
    )
)
cooccurrence = (
    pairs.groupby(["user_id", "user_name", "标签"], as_index=False)
    .agg(records_count=("record_id", "nunique"))
    .sort_values("records_count", ascending=False)
)
```
