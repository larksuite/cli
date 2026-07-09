# okr +reorder

> **前置条件：** 先阅读 [`lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

调整 OKR 周期下目标（Objective）或目标下关键结果（Key Result）的顺序。

## 推荐命令

```bash
# 调整 Objective 顺位
lark-cli okr +reorder \
  --cycle-id 7000000000000000001 \
  --level objective \
  --ops '[
    {"id": "7000000000000000002", "position": 2},
    {"id": "7000000000000000003", "position": 1}
  ]' \
  --as user

# 调整 KR 顺位（需指定 --objective-id）
lark-cli okr +reorder \
  --cycle-id 7000000000000000001 \
  --level key-result \
  --objective-id 7000000000000000002 \
  --ops '[
    {"id": "7000000000000000004", "position": 1},
    {"id": "7000000000000000005", "position": 2}
  ]' \
  --as user

# 从文件读取 ops
lark-cli okr +reorder \
  --cycle-id 7000000000000000001 \
  --level objective \
  --ops @reorder_ops.json \
  --as user
```

- 不允许将多个 objective/key-result 放在同一个位置下

## 参数

| 参数               | 必填 | 默认值    | 说明                                                      |
|------------------|----|--------|---------------------------------------------------------|
| `--level`        | 是  | —      | 调整层级：`objective`（调整周期下目标顺序）\| `key-result`（调整目标下 KR 顺序） |
| `--cycle-id`     | 是  | —      | OKR 周期 ID（int64 类型）。                                    |
| `--objective-id` | 条件 | —      | 目标 ID。当 `--level=key-result` 时**必填**，用于定位父目标。           |
| `--ops`          | 是  | —      | JSON 数组格式的顺位调整操作。支持 `@文件路径` 或 `@-` 从 stdin 读取。          |
| `--dry-run`      | 否  | —      | 预览 API 调用而不实际执行                                         |
| `--format`       | 否  | `json` | 输出格式                                                    |

## 工作流程

1. 使用 `+cycle-list` 和 `+cycle-detail` 获取周期 ID、目标 ID 和 KR ID。
2. 构造 `--ops` JSON 数组，指定要调整的 ID 和新 position，执行命令。
3. 返回调整后的完整顺序。

## 输出

成功返回 JSON（以调整 Objective 位置为例）：

```json
{
  "ok": true,
  "data": {
    "level": "objective",
    "cycle_id": "7000000000000000001",
    "total": 3,
    "ordered": [
      "7000000000000000003",
      "7000000000000000002",
      "7000000000000000004"
    ]
  }
}
```

## 参考

- [OKR 业务实体](lark-okr-entities.md) -- OKR 实体结构定义
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
