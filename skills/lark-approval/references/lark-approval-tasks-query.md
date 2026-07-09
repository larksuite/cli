
# approval tasks query

查询当前用户的审批任务列表，可用于查看待办、已办、知会等分组。只读操作，不会修改审批状态。

需要的 scopes: ["approval:task:read"]

## 命令

```bash
# 查询待办审批
lark-cli approval tasks query --params '{"topic":"1"}' --as user

# 查询已办审批
lark-cli approval tasks query --params '{"topic":"2"}' --as user

# 使用 page_token 翻页
lark-cli approval tasks query --params '{"topic":"1","page_token":"example_page_token"}' --as user

# 表格格式输出，便于快速浏览
lark-cli approval tasks query --params '{"topic":"1"}' --format table --as user
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--params '{"topic":"..."}'` | 是 | 查询参数，使用 JSON 传入 |
| `topic` | 是 | 任务分组主题，见下方“topic 枚举” |
| `definition_code` | 否 | 审批定义 Code，用于仅查询某个审批定义下的任务 |
| `locale` | 否 | 返回语言：`zh-CN`、`en-US`、`ja-JP` |
| `page_size` | 否 | 分页大小 |
| `page_token` | 否 | 翻页标记；首次请求不填，后续使用上一次返回的 `page_token` |
| `user_id_type` | 否 | 用户 ID 类型：`user_id`、`union_id`、`open_id` |
| `--as user` | 否 | 建议显式指定用户身份；审批任务查询通常应使用用户身份 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## topic 枚举

| 值 | 含义 |
|----|------|
| `1` | 待办审批 |
| `2` | 已办审批 |
| `17` | 未读知会 |
| `18` | 已读知会 |

## 输出重点字段

返回结果中常见字段：

| 字段 | 说明 |
|------|------|
| `count` | 列表计数，只在第一页返回；当任务数大于等于 100 时返回 `99` |
| `has_more` | 是否还有更多数据 |
| `page_token` | 下一页翻页 Token |
| `tasks[].task_id` | 任务 ID，全局唯一 |
| `tasks[].instance_code` | 审批实例 Code；后续执行 approve / reject / rollback 等操作时通常需要与 `task_id` 成对使用 |
| `tasks[].title` | 任务标题 |
| `tasks[].status` | 任务状态：`1` 待办、`2` 已办、`17` 未读、`18` 已读、`33` 处理中、`34` 撤回 |
| `tasks[].topic` | 任务所属分组主题 |
| `tasks[].instance_status` | 审批实例状态：`0` 无状态、`1` 流转中、`2` 已通过、`3` 已拒绝、`4` 已撤销、`5` 已终止 |
| `tasks[].definition_code` | 审批定义 Code |
| `tasks[].definition_name` | 审批定义名称 |
| `tasks[].initiator` | 发起人 ID |
| `tasks[].initiator_name` | 发起人姓名 |
| `tasks[].summaries` | 表单摘要字段列表 |
| `tasks[].support_api_operate` | 是否支持通过 API 同意或拒绝该任务 |
| `tasks[].user_id` | 任务所属用户 ID |

## 使用建议

- 常见处理链：先用 `tasks query` 拿到 `task_id` 和 `instance_code`，若用户需要查看详情、当前节点、表单内容、流程进度等内容，则调用 `instances get` 查看详情，最后执行 `tasks approve` / `tasks reject` / `tasks transfer` / `tasks add_sign` / `tasks rollback`。
- 如果你只想看“已发起的审批实例”，使用 `instances initiated`；`tasks query` 更适合围绕“任务分组”来拉取列表。
- 需要继续翻页时，直接把上一次返回的 `page_token` 放回 `--params`。
- 当结果量较大时，优先使用 `--format table` 提升可读性。
