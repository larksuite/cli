# okr +progress-list

> **前置条件：** 先阅读 [`lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

获取目标（Objective）或关键结果（Key Result）的一页进展记录列表，支持外部控制翻页。

## 推荐命令

```bash
# 获取目标进展记录第一页 (默认页大小为 100，一般不用翻页)
lark-cli okr +progress-list \
  --target-id 1234567890123456789 \
  --target-type objective

# 获取下一页进展记录 
lark-cli okr +progress-list \
  --target-id 1234567890123456789 \
  --target-type objective \
  --page-size 100 \
  --page-token "7000000000000000002"

# 获取关键结果进展记录第一页
lark-cli okr +progress-list \
  --target-id 9876543210987654321 \
  --target-type key_result
```

## 参数

| 参数                    | 必填 | 默认值             | 说明                                             |
|-------------------------|----|--------------------|--------------------------------------------------|
| `--target-id`           | 是  | —                  | 目标 ID 或关键结果 ID（int64 类型，正整数）       |
| `--target-type`         | 是  | —                  | 目标类型：`objective` \| `key_result`            |
| `--user-id-type`        | 否  | `open_id`          | 用户 ID 类型：`open_id` \| `union_id` \| `user_id` |
| `--department-id-type`  | 否  | `open_department_id` | 部门 ID 类型：`department_id` \| `open_department_id` |
| `--page-size`           | 否  | `100`              | 每页数量，范围 `1-100`。                           |
| `--page-token`          | 否  | `""`               | 上一次响应中的 `page_token`，留空表示第一页。        |
| `--dry-run`             | 否  | —                  | 预览 API 调用而不实际执行。                       |
| `--format`              | 否  | `json`             | 输出格式。                                        |

## 工作流程

1. 使用 `+cycle-list` 和 `+cycle-detail` 获取目标或关键结果的 ID。
2. 执行 `lark-cli okr +progress-list --target-id "..." --target-type objective --page-size 100`。
3. 如果响应中 `has_more=true`，继续用返回的 `page_token` 调用下一页。
4. 获取该目标或关键结果下的进展记录列表。

## 输出

返回 JSON：

```json
{
  "progress_list": [
    {
      "progress_id": "1234567890123456789",
      "modify_time": "2025-01-15 10:30:00",
      "content": "{...}",
      "progress_rate": {
        "percent": 80.0,
        "status": "done"
      }
    }
  ],
  "has_more": true,
  "page_token": "7000000000000000002"
}
```

其中：

- `progress_list` — 进展记录数组
- `has_more` 和 `page_token` 用于外部控制翻页；`has_more=true` 时，用 `--page-token` 原样传入本次返回的 `page_token` 获取下一页。
- `content` 字段是 JSON 字符串，为 OKR ContentBlock 富文本格式。请参考 [lark-okr-contentblock.md](lark-okr-contentblock.md) 了解详细信息。
- `progress_rate.status` 返回可读字符串：`normal`（正常）、`overdue`（逾期）、`done`（已完成）。

## 与 +progress-get 的区别

| 命令             | 用途                               | API 版本 |
|------------------|------------------------------------|----------|
| `+progress-list` | 分页获取某个目标/关键结果的进展记录 | v2       |
| `+progress-get`  | 根据进展记录 ID 获取单条记录        | v1       |

`+progress-list` 返回的 `progress_list` 数组中每条记录的结构与 `+progress-get` 返回的 `progress` 结构相同。

## 参考

- [lark-okr](../SKILL.md) -- 所有 OKR 命令(shortcut 和 API 接口)
- [ContentBlock 格式](lark-okr-contentblock.md) -- 进展内容使用的富文本格式
- [lark-okr-progress-get](lark-okr-progress-get.md) -- 根据 ID 获取单条进展记录
- [lark-okr-progress-create](lark-okr-progress-create.md) -- 创建进展记录
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
