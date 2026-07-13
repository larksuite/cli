
# approval approvals get

获取单个审批定义详情（用户级只读操作）。适合在发起审批实例前，先确认审批名称、表单控件结构、选项值范围以及流程节点信息。

需要的 scopes: ["approval:approval:read"]

## 命令

```bash
# 按 approval_code 查询审批定义详情
lark-cli approval approvals get --params '{"approval_code":"<APPROVAL_CODE>"}' --as user

# 表格格式输出，便于快速浏览顶层字段
lark-cli approval approvals get --params '{"approval_code":"<APPROVAL_CODE>"}' --format table --as user

# 预览 API 调用，不执行
lark-cli approval approvals get --params '{"approval_code":"<APPROVAL_CODE>"}' --as user --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--params '{...}'` | 是 | 查询参数，使用 JSON 传入 |
| `approval_code` | 是 | 审批定义 Code；通常来自 `approval approvals search` 的结果 |
| `locale` | 否 | 返回语言，例如 `zh-CN`、`en-US`、`ja-JP` |
| `--as user` | 否 | 建议显式指定用户身份；审批定义详情通常按当前用户可见范围读取 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 常见输入来源

如果你已经有 `approval_code`，可直接查询：

```bash
lark-cli approval approvals get --params '{"approval_code":"<APPROVAL_CODE>"}' --as user
```

如果你还没有 `approval_code`，先搜索可发起审批定义：

```bash
lark-cli approval approvals search --data '{"keyword":"请假"}' --as user
```

## 输出重点字段

返回结果中，优先关注以下字段：

| 字段 | 说明 |
|------|------|
| `approval_code` | 审批定义 Code |
| `approval_name` | 审批定义名称；确认是不是用户想发起的那张单 |
| `form` | 表单定义快照；用于识别控件 `id`、`type`、选项值范围、明细子控件结构 |
| `node_list` | 流程节点列表；用于识别节点 key、是否需要补充审批人、是否允许多人 |

## form 的使用重点

`form` 最重要的作用是帮助 agent **识别怎么组装 `instances.create.data.form`**，而不是直接把它原样提交出去。

重点看：

| 字段 / 结构 | 说明 |
|------|------|
| `form[].id` | 控件 ID；后续创建实例时必须使用 |
| `form[].type` | 控件类型，例如 `input`、`date`、`radio`、`checkbox`、`fieldList` |
| `form[].value` / 选项定义 | 用来识别可选值范围、默认值或选项值 |
| 明细 / 子控件结构 | 用于识别 `fieldList`、控件组等复杂控件的子字段结构 |

**注意：`approvals.get.form` 不是 `instances.create` 可直接复用的 payload 模板。** 它是“定义快照”，主要用于识别字段结构与选项值范围。

## node_list 的使用重点

`node_list` 主要用于后续决定是否要补 `node_approver_list` / `node_cc_list`。

重点看：

| 字段 | 说明 |
|------|------|
| `node_list[].custom_node_id` | 自定义节点标识；后续补节点参数时优先作为 key |
| `node_list[].node_id` | 节点 ID；若没有 `custom_node_id`，通常退回用它做 key |
| `node_list[].need_approver` | 是否要求发起人补充审批人 |
| `node_list[].approver_chosen_multi` | 是否允许为该节点选择多个审批人 |

## 使用建议

- **这是发起原生审批实例前的必要只读步骤。** 推荐固定走：`approvals search` -> `approvals get` -> `instances create`。
- **如果用户已经明确给了 `approval_code`，直接用这个命令。** 不必再走 `approvals search`。
- **先确认 `approval_name`。** 避免把相似名称的审批定义搞混。
- **先用 `form` 识别控件结构，再组装创建 payload。** 不要在未看详情时猜控件 `id`、`type` 或选项值。
- **先用 `node_list` 看是否需要补审批人。** 若某节点 `need_approver=true`，创建实例时通常要补 `node_approver_list`。
- **`node_list` 的 key 优先取 `custom_node_id`。** 若不存在，再使用 `node_id`。
- **`approver_chosen_multi=false` 时，一个节点通常只能补一个审批人。**

## 输出与后续操作

读取定义详情后，常见下一步：

```bash
# 发起原生审批实例
lark-cli approval instances create --data '{"approval_code":"<APPROVAL_CODE>","form":"[...]"}' --as user --yes
```

如果需要进一步理解控件取值与节点参数，优先参考：

- `lark-approval-instance-form-control-parameters.md`
- `lark-approval-instance-value-sourcing.md`
- `lark-approval-initiate.md`

## 结果整理方式

**将结果整理为“审批定义概览 + 表单结构摘要 + 节点要求摘要”。**

建议输出成下面这种结构：

```text
审批定义：请假申请
approval_code: 7C468A54-8745-2245-9675-08B7C63E7A85

表单控件摘要：
- leave_type: radio，可选值 [annual_leave, sick_leave]
- reason: textarea
- start_end: dateInterval

节点要求摘要：
- manager_node：need_approver=true，approver_chosen_multi=false
- hr_node：need_approver=false
```
