# 审批提单值来源

## 目的

本文用于回答一个固定问题：在调用 `approval instances create` 发起原生审批实例时，**每个要填写的值从哪里拿**。

阅读顺序固定如下：

1. [`lark-approval-initiate.md`](./lark-approval-initiate.md) 中的创建请求参数、节点参数和返回结果说明
2. `approval approvals get` 返回的 `form` / `node_list`
3. [`lark-approval-instance-form-control-parameters.md`](./lark-approval-instance-form-control-parameters.md)
4. 本文

## 总原则

- `lark-approval-initiate.md` 决定创建请求字段名、字段层级、节点参数结构。
- `approvals.get.form` 决定控件 `id`、`type`、选项值范围、子控件结构。
- `approvals.get.node_list` 决定节点 key、是否必须补审批人、是否允许多人。
- [`lark-approval-instance-form-control-parameters.md`](./lark-approval-instance-form-control-parameters.md) 决定各控件 `value` 的最终结构。
- 除非本文明确允许，否则不要猜值来源，不要把展示文案直接当成可提交值。

## 默认来源

- 审批定义、`approval_code`、`is_external`、`create_link` 等基础信息，默认从 `approval approvals search` 获取。
- 控件 `id`、`type`、选项值、子控件结构，默认从 `approval approvals get.form` 获取。
- 节点 key、`need_approver`、`approver_chosen_multi` 等节点信息，默认从 `approval approvals get.node_list` 获取。
- 本文只补充 **这些默认来源之外** 的取值规则，以及当前必须由用户直接提供的值。

## 控件值来源规则

### 联系人 `contact`

- 只推荐写 `open_ids`。
- 不再推荐双写 `value(user_id)` + `open_ids`，避免复杂度继续上升。
- 如果用户给的是姓名、邮箱或账号，先用 `lark-contact` 解析成 `open_id`。

### 部门 `department`

- 最优先：用户直接提供 `open_department_id`。
- 若用户说“我的部门”或“张三的部门”，先用 `lark-contact` 查询对应人员信息，再取其所属部门里的 `open_department_id`。
- 如果查到该人员只有一个部门，可直接使用。
- 如果查到多个部门，不自动猜，必须让用户明确选一个，或直接输入 `open_department_id`。
- 如果仍无法确定，则明确告知当前不支持自动决定部门值。

### 附件 `attachmentV2`

- 当前 `lark-approval` 不负责上传文件。
- 用户必须直接提供 file code。
- 如果用户无法提供 file code，应明确告知当前无法仅通过 `lark-approval` 完成该控件提单。

### 图片 `image` / `imageV2`

- 当前 `lark-approval` 不负责上传图片。
- 用户必须直接提供 file code。
- 如果用户无法提供 file code，应明确告知当前无法仅通过 `lark-approval` 完成该控件提单。

### 文档 `document`

- 用户可直接提供 `token` / `document_id`。
- 如果用户给的是飞书文档链接，应先尝试从链接中提取 token。
- 若链接提取失败，再要求用户手动输入 token。

### 关联审批 `connect`

- 用户直接提供目标审批实例的 `instance_code`。
- 当前不默认做“搜索关联实例再反查 code”的自动流程。

### 地址 `address`

- 用户直接提供地理库 `id`。
- 若用户无法提供该 `id`，当前不支持自动取值。

## 特殊控件组

以下控件组的结构仍按 [`lark-approval-instance-form-control-parameters.md`](./lark-approval-instance-form-control-parameters.md) 组装：

- `leaveGroupV2`
- `workGroup`
- `outGroup`
- `shiftGroup`

补充规则：

- 控件组自身和子控件的 `id` / `type` 从 `approval approvals get.form` 中识别。
- 组内单选/多选或业务枚举值，优先从 `approval approvals get.form` 返回的选项结构中取。
- 不要把控件组整体当成普通字符串或扁平对象提交。

## 不支持自动准备的值

以下值当前不建议由 `lark-approval` 自动准备：

- 文件上传后的 file code
- 图片上传后的 file code
- 地址控件的地理库 `id`
- 无法唯一确定的部门 `open_department_id`

遇到这类值时，应明确告诉用户需要提供什么，而不是继续猜测。

## 最小决策表

| 场景 | 处理 |
|---|---|
| 用户说“找张三当审批人” | 用 `lark-contact` 解析张三，取 `open_id` |
| 用户说“我的部门” | 先查当前用户部门；若多个部门，让用户选 |
| 用户给了文档链接 | 先尝试提取 token |
| 用户要填图片/附件 | 要求直接提供 file code |
| 用户要填关联审批 | 要求直接提供 `instance_code` |
| 用户要填地址 | 要求直接提供地理库 `id` |
