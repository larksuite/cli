# okr +indicator-update

> **前置条件：** 先阅读 [`lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

直接更新目标（Objective）或关键结果（Key Result）的指标当前值，无需手动查询指标 ID。

> **查询指标：** 如需查看指标详情，请使用原生 API：
> - 目标指标：`lark-cli okr objective.indicators list --objective-id <id>`
> - KR 指标：`lark-cli okr key_result.indicators list --key-result-id <id>`

## 推荐命令

```bash
# 更新 Objective 的指标值
lark-cli okr +indicator-update \
  --level objective \
  --id 7000000000000000001 \
  --value 75.5 \
  --as user

# 更新 Key Result 的指标值
lark-cli okr +indicator-update \
  --level key-result \
  --id 7000000000000000002 \
  --value 100 \
  --as user
```

## 参数

| 参数         | 必填 | 默认值    | 说明                                                                 |
|------------|----|--------|--------------------------------------------------------------------|
| `--level`  | 是  | —      | 操作层级：`objective`（更新目标指标）\| `key-result`（更新 KR 指标） |
| `--id`     | 是  | —      | 目标 ID 或 KR ID（int64 类型）                                       |
| `--value`  | 是  | —      | 新的指标当前值（数字，范围：-99999999999 到 99999999999）              |
| `--dry-run`| 否  | —      | 预览 API 调用而不实际执行                                            |
| `--format` | 否  | `json` | 输出格式                                                             |

## 工作流程

1. 使用 `+cycle-list` 和 `+cycle-detail` 获取目标 ID 或 KR ID。
2. 如需查看当前指标值，使用 `objective.indicators list` 或 `key_result.indicators list` 查询。
  若当前量化指标没有 start_value/current_value/target_value/unit 这些字段，代表当前量化指标为未设置的默认初始进度。
3. 执行 `+indicator-update` 指定层级、ID 和新值。 
  使用 +indicator-update 为默认初始进度设置当前值会将该量化指标配置为默认的百分比模式。若用户不希望将指标设置为百分比，请使用原生 API 详细设置，参考 [lark-okr-indicators.md](lark-okr-indicators.md)
4. 命令自动查询指标 ID 并更新当前值。

## 输出

### JSON 格式

```json
{
  "ok": true,
  "data": {
    "indicator_id": "7000000000000000003",
    "current_value": 75.5,
    "level": "objective",
    "target_id": "7000000000000000001"
  }
}
```

### 字段说明

| 字段             | 类型     | 说明                     |
|----------------|--------|------------------------|
| `indicator_id` | string | 被更新的指标 ID            |
| `current_value`| number | 更新后的指标当前值           |
| `level`        | string | 操作层级：`objective` / `key-result` |
| `target_id`    | string | 目标或 KR 的 ID            |

## 注意事项
- 仅更新 `current_value` 字段，`unit`、`start_value`、`target_value` 等其他字段保持不变
  - 若需要这些字段进行修改，使用原生接口 indicators.patch
- 指标的 `current_value_calculate_type` 必须为「手动更新」才能通过此命令修改。

## 参考

- [OKR 指标更新 API](https://open.feishu.cn/api-explorer?from=op_doc_tab&apiName=patch&project=okr&resource=okr.indicator&version=v2)
- [`lark-okr-progress-create.md`](./lark-okr-progress-create.md) — 创建进度记录
- [`lark-okr-cycle-detail.md`](./lark-okr-cycle-detail.md) — 查询周期详情获取 ID
