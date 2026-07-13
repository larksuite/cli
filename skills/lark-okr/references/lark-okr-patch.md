# okr +patch

> **前置条件：** 先阅读 [`lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

部分更新 OKR 目标（Objective）或关键结果（Key Result）的 content、notes、score、deadline 字段。支持增量更新，只需提供要修改的字段。

## 推荐命令

```bash
# 更新目标的 content（默认 simple 风格，半纯文本格式）
lark-cli okr +patch \
  --level objective \
  --target-id 1234567890123456789 \
  --content '{"text":"更新后的目标内容","mention":["ou_123"]}'

# 更新关键结果的分数（0.0-1.0 的一位小数）
lark-cli okr +patch \
  --level key-result \
  --target-id 2345678901234567890 \
  --score 0.7

# 同时更新目标的多个字段（richtext 风格，完整 ContentBlock 格式）
lark-cli okr +patch \
  --level objective \
  --target-id 1234567890123456789 \
  --style richtext \
  --content '{"blocks":[{"block_element_type":"paragraph","paragraph":{"elements":[{"paragraph_element_type":"textRun","text_run":{"text":"更新后的目标内容"}}]}}]}' \
  --notes '{"blocks":[{"block_element_type":"paragraph","paragraph":{"elements":[{"paragraph_element_type":"textRun","text_run":{"text":"更新后的备注"}}]}}]}' \
  --score 0.5 \
  --deadline 1735776000000

# 预览 API 调用而不实际执行
lark-cli okr +patch \
  --level objective \
  --target-id 1234567890123456789 \
  --content '{"text":"测试更新"}' \
  --dry-run
```

## 参数

| 参数             | 必填 | 默认值       | 说明                                                                                                                                   |
|----------------|----|-----------|--------------------------------------------------------------------------------------------------------------------------------------|
| `--level`      | 是  | —         | 更新级别：`objective`（目标） \| `key-result`（关键结果）                                                                                    |
| `--target-id`  | 是  | —         | 目标 ID 或关键结果 ID（int64 类型，正整数）                                                                                                         |
| `--style`      | 否  | `simple`  | 输入风格：`simple`（半纯文本 JSON，推荐） \| `richtext`（完整 ContentBlock JSON）。请参考 [ContentBlock 格式](lark-okr-contentblock.md) 了解两种格式。          |
| `--content`    | 否¹ | —         | 内容。根据 `--style` 指定格式。支持 `@文件路径` 从文件读取。                                                                                                |
| `--notes`      | 否¹ | —         | 备注（仅 `--level=objective` 时支持）。根据 `--style` 指定格式。支持 `@文件路径` 从文件读取。                                                                           |
| `--score`      | 否¹ | —         | 分数值，0-1 之间，最多一位小数（如 0.5、1.0）。                                                                                                            |
| `--deadline`   | 否¹ | —         | 截止时间，毫秒级时间戳（如 1735776000000）。                                                                                                      |
| `--user-id-type` | 否  | `open_id` | 用户 ID 类型：`open_id` \| `union_id` \| `user_id`                                                                                        |
| `--dry-run`    | 否  | —         | 预览 API 调用而不实际执行。                                                                                                                     |
| `--format`     | 否  | `json`    | 输出格式。                                                                                                                                |

> ¹ 至少需要提供 `--content`、`--notes`、`--score`、`--deadline` 中的一个字段。

## 工作流程

1. 使用 `+cycle-list` 和 `+cycle-detail` 获取目标或关键结果的 ID。
2. 确定要更新的字段：
   - **content/notes**：构造内容
     - **推荐**：使用 `simple` 风格（默认），构造 SemiPlainContent JSON：`{"text":"内容","mention":["ou_xxx"]}`
     - 如需复杂格式：使用 `richtext` 风格，构造 ContentBlock JSON。请参考 [ContentBlock 格式](lark-okr-contentblock.md)。
   - **score**：0-1 之间的数字，最多一位小数（如 0.3、0.7、1.0）
   - **deadline**：毫秒级时间戳
3. 执行 `lark-cli okr +patch --level objective --target-id "..." --content "..."`。
4. 报告结果：更新的级别、目标 ID、以及哪些字段被更新。

## 输出

返回 JSON：

```json
{
  "level": "objective",
  "target_id": "1234567890123456789",
  "patched": {
    "content": true,
    "notes": true,
    "score": true,
    "deadline": true
  }
}
```

其中 `patched` 对象中的每个字段表示该字段是否被更新。

## 注意事项

- **`--notes` 仅适用于目标**：关键结果（key-result）不支持 notes 字段，使用时会报错。
- **score 格式**：必须在 0-1 之间，且最多一位小数（如 0.5 正确，0.51 错误）。
- **严格验证**：输入格式严格根据 `--style` 值验证，不会自动检测。使用 ContentBlock JSON 时必须指定 `--style richtext`。
- **simple 风格输入限制**：simple 风格的输入不支持 `docs` 和 `images` 字段，如需包含文档或图片请使用 `richtext` 风格。

## 关于 1001001 错误

有时，当你涉及修改目标或关键结果的分数时，即使输入的参数完全正确， +patch 也会返回 1001001 错误(invalid parameters)。
这可能是因为在用户的租户设置中停用了目标/关键结果的分数功能，或禁用了目标分数的手动计算。此时可以先去掉 --score 参数再修改，并向用户确认是否启用了对应的功能。

## 参考

- [lark-okr](../SKILL.md) -- 所有 OKR 命令(shortcut 和 API 接口)
- [ContentBlock 格式](lark-okr-contentblock.md) -- content/notes 使用的富文本格式
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
