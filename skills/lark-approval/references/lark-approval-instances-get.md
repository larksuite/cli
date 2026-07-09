
# approval instances get

获取单个审批实例详情（用户级只读操作）。适合在执行 approve / reject / transfer / rollback / cancel / cc / remind 之前，先查看审批表单、当前节点、任务列表、审批动态和整体状态。

需要的 scopes: ["approval:instance:read"]

## 命令

```bash
# 按实例 Code 查询详情
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user

# 表格格式输出，便于快速浏览顶层字段
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --format table --as user

# 预览 API 调用，不执行
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--params '{...}'` | 是 | 查询参数，使用 JSON 传入 |
| `instance_code` | 是 | 审批实例 Code |
| `locale` | 否 | 返回语言，例如 `zh-CN`、`en-US`、`ja-JP` |
| `user_id_type` | 否 | 用户 ID 类型：`user_id`、`union_id`、`open_id` |
| `--as user` | 否 | 建议显式指定用户身份；审批实例详情查询通常应使用用户身份 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 常见输入来源

如果你已经有实例 Code，可直接查询：

```bash
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user
```

如果你还没有实例 Code，可先从以下命令获取：

```bash
# 查询我发起的审批实例
lark-cli approval instances initiated --params '{"page_size":20}' --as user

# 或从任务列表里拿到关联实例 Code
lark-cli approval tasks query --params '{"topic":"1"}' --as user
```

## 输出重点字段

返回结果中常见字段：

| 字段 | 说明 |
|------|------|
| `instance_code` | 审批实例 Code |
| `serial_number` | 审批单编号 |
| `definition_code` | 审批定义 Code |
| `definition_name` | 审批名称 |
| `user_id` | 发起审批的用户 ID |
| `department_id` | 发起人所在部门 ID |
| `status` | 审批实例状态，见下方“status 枚举” |
| `reverted` | 单据是否已被撤销 |
| `start_time` | 审批创建时间 |
| `end_time` | 审批完成时间，未完成时通常为 `0` |
| `form` | 表单数据，JSON 字符串 |
| `current_nodes` | 当前审批节点列表 |
| `tasks` | 审批任务列表 |
| `operation_records` | 审批动态，例如通过、拒绝、转交、加签、回退、撤回、抄送 |
| `comments` | 评论列表 |

## status 枚举

| 值 | 含义 |
|----|------|
| `PENDING` | 审批中 |
| `APPROVED` | 已通过 |
| `REJECTED` | 已拒绝 |
| `CANCELED` | 已撤回 |
| `DELETED` | 已删除 |

## current_nodes 重点字段

`current_nodes` 常用于判断审批流当前卡在哪一层：

| 字段 | 说明                                       |
|------|------------------------------------------|
| `current_nodes[].node_id` | 当前审批节点 ID                                |
| `current_nodes[].node_name` | 当前审批节点名称                                 |
| `current_nodes[].type` | 审批方式：`AND` 会签、`OR` 或签、`SEQUENTIAL` 依次审批等 |
| `current_nodes[].approvers[].task_id` | 当前审批人关联任务 ID                             |
| `current_nodes[].approvers[].user_id` | 当前审批人用户 ID                               |

## tasks 重点字段

`tasks` 常用于把实例和具体审批任务关联起来：

| 字段 | 说明 |
|------|------|
| `tasks[].id` | 审批任务 ID |
| `tasks[].node_id` | 任务所属节点 ID |
| `tasks[].node_name` | 任务所属节点名称 |
| `tasks[].user_id` | 审批人用户 ID |
| `tasks[].status` | 任务状态：`PENDING`、`APPROVED`、`REJECTED`、`TRANSFERRED`、`DONE` |
| `tasks[].start_time` | 任务开始时间 |
| `tasks[].end_time` | 任务完成时间 |

## operation_records 重点字段

`operation_records` 常用于审计审批过程：

| 字段 | 说明 |
|------|------|
| `operation_records[].type` | 事件类型，如 `PASS`、`REJECT`、`TRANSFER`、`ROLLBACK`、`CANCEL`、`CC` |
| `operation_records[].create_time` | 事件发生时间 |
| `operation_records[].user_id` | 触发该事件的用户 ID |
| `operation_records[].task_id` | 关联任务 ID |
| `operation_records[].node_id` | 关联节点 ID |
| `operation_records[].comment` | 理由 / 备注 |
| `operation_records[].cc_user_ids` | 被抄送人列表（抄送事件时） |

## 使用建议

- **这是最适合做“详情确认”的只读命令**：当你已经拿到 `instance_code`，需要确认表单、当前节点、任务状态、审批动态时，优先使用它。
- **在执行写操作前先看详情**：例如做 `tasks rollback` 前确认可退回节点，做 `instances cancel` 前确认实例状态，做 `tasks remind` 前确认当前任务是否仍待处理。
- **`form` 是 JSON 字符串**：调用方通常还需要再解析一层，才能拿到表单字段值。
- **`current_nodes` 和 `tasks` 可以联动看**：前者看“当前卡在哪个节点”，后者看“每个任务目前由谁处理、状态如何”。
- **`operation_records` 适合做时间线回溯**：例如排查谁转交过、谁加签过、什么时候撤回或抄送过。
- **优先显式传 `locale` 和 `user_id_type`**：这样 agent 更容易理解返回文本和 ID 语义，减少歧义。

## 输出与后续操作

读取详情后，常见下一步：

```bash
# 同意审批任务
lark-cli approval tasks approve --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>"}' --as user --yes

# 撤回审批实例
lark-cli approval instances cancel --data '{"instance_code":"<INSTANCE_CODE>"}' --as user --yes

# 催办审批任务
lark-cli approval tasks remind --data '{"instance_code":"<INSTANCE_CODE>","task_ids":["<TASK_ID>"]}' --as user --yes
```
