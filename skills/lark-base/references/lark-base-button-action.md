# Base button actions

按钮字段的显示属性和点击动作是两套独立配置：字段 schema 只保存字段名称及 `button_config.title` 等外观信息；点击动作由 `+button-rule-bind`、`+button-rule-get` 和 `+button-rule-unbind` 单独管理。

当用户要让按钮触发 Workflow、打开当前记录、打开表单或打开动态链接时，使用本指南。不要把动作配置写进 `+field-create` / `+field-update` 的 Field JSON。

## 选择命令

- `+button-rule-bind --workflow-id <wkf...>`：绑定 Workflow，并兼容已有脚本和服务端。
- `+button-rule-bind --target-json '<target>'`：绑定 `workflow`、`open_record`、`open_form` 或 `open_link` 公共 Target。
- `+button-rule-get`：读取当前持久化的按钮动作。
- `+button-rule-unbind`：清除任意类型的按钮动作，不删除字段或动作引用的资源。

`--workflow-id` 和 `--target-json` 必须且只能提供一个。外层 `--table-id`、`--field-id` 可以使用 ID 或名称；Target 内的 Workflow、Table、Form 和 View 必须使用稳定 ID。

## 推荐工作流

1. 用 `+field-get` 确认目标是按钮字段；需要创建时，只用 Field JSON 配置按钮名称和文案。
2. 根据动作类型执行一次 `+button-rule-bind`。
3. 用 `+button-rule-get` 做 fresh readback，核对 `bound` 和完整 `target`。
4. 需要验证最终行为时，在真实 Web 或桌面端点击按钮；PUT 成功或 dry-run 不能证明动作已经持久化并可正常运行。

创建按钮字段的最小示例：

```bash
lark-cli base +field-create \
  --base-token <base_token> \
  --table-id <table_id> \
  --json '{"type":"button","name":"查看详情","button_config":{"title":"查看详情"}}' \
  --as user
```

字段 JSON 的根级或 `button_config` 内都不能包含 `action`、`action_type` 或 `target`；Workflow ID 也不属于字段 schema。

## 绑定 Workflow

Workflow 默认使用兼容入口 `--workflow-id`。必须传 Workflow 命令返回的公开 `wkf...` ID，不能传内部数字 ID。绑定动作不会启用 Workflow；只有用户明确要求启用时才另行调用 `+workflow-enable`。

```bash
lark-cli base +button-rule-bind \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id <button_field_id_or_name> \
  --workflow-id <wkf_workflow_id> \
  --as user
```

`--target-json` 也接受等价的公共 Target：

```json
{"type":"workflow","id":"wkfXXX"}
```

不要在同一次调用中同时传 `--workflow-id` 和 `--target-json`。

## 打开当前记录

`open_record` 打开被点击按钮所在的当前记录，不接受其他字段：

```bash
lark-cli base +button-rule-bind \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id <button_field_id_or_name> \
  --target-json '{"type":"open_record"}' \
  --as user
```

## 打开表单

`open_form` 需要目标表的稳定 `tbl...` ID 和该表单的稳定 `vew...` ID。Target 内不解析名称，也不接收分享 token；需要时先用 `+table-list` 和 `+form-list` 定位 ID。

```bash
lark-cli base +button-rule-bind \
  --base-token <base_token> \
  --table-id <button_table_id> \
  --field-id <button_field_id_or_name> \
  --target-json '{"type":"open_form","table_id":"tblXXX","form_id":"vewXXX"}' \
  --as user
```

目标 Table 必须属于当前 Base，`form_id` 必须是该 Table 下真实存在的 Form。

## 打开链接

`open_link.link` 是有序片段数组，片段只能是固定文本 `text` 或按钮上下文引用 `ref`。CLI 和服务端按数组顺序拼接最终 URL，不要把多个片段预先合并或改序。

```bash
lark-cli base +button-rule-bind \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id <button_field_id_or_name> \
  --target-json '{"type":"open_link","link":[{"value_type":"text","value":"https://example.com/tickets/"},{"value_type":"ref","value":"$.button.fldTicketId"}]}' \
  --as user
```

较长的 Target 可以写入文件后使用 `--target-json @target.json`。

公开引用使用 `$.button` namespace：

| 引用 | 含义 |
|---|---|
| `$.button.fldXXX` | 当前记录中指定字段的值 |
| `$.button.triggerTime` | 点击触发时间 |
| `$.button.recordId` | 当前记录 ID |
| `$.button.recordLink` | 当前记录链接 |
| `$.button.recordCreatedBy` / `$.button.recordCreatedTime` | 当前记录的创建人 / 创建时间 |
| `$.button.recordModifiedBy` / `$.button.recordModifiedTime` | 当前记录的最后修改人 / 修改时间 |
| `$.button.baseLink` | 当前 Base 链接 |
| `$.button.tableLink.tblXXX` | 当前 Base 中指定 Table 的链接 |
| `$.button.viewLink.tblXXX.vewXXX` | 当前 Base 中指定 Table 和 View 的链接 |

字段引用必须属于按钮所在 Table；Table 和 View 引用必须属于当前 Base，并保持正确父子关系。不要使用 Workflow step 的 `$.<step_id>...` 引用格式。

## 查询和验收

```bash
lark-cli base +button-rule-get \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id <button_field_id_or_name> \
  --as user
```

已绑定时核对 `bound=true` 和完整 `target`；未配置时应返回：

```json
{"bound":false,"target":null}
```

GET 还可能返回 `warnings` 或同步状态，应原样保留并据此处理。若 `target.type=unknown`，表示服务端保存了当前 CLI 尚不支持的动作；它只能读取，不能把 raw Target 重新传给 `+button-rule-bind`。

## 解绑

```bash
lark-cli base +button-rule-unbind \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id <button_field_id_or_name> \
  --as user
```

解绑会清除直接动作或 Workflow 绑定，但不会删除按钮字段、Workflow、表单、Table 或 View。重复解绑是安全的；完成后再次调用 `+button-rule-get`，确认 `bound=false`、`target=null`。

## 权限与错误处理

- 查询需要 `base:field:read`；绑定和解绑需要 `base:field:read` 与 `base:field:update`。
- `--target-json` 使用严格 JSON：未知字段、类型不匹配、分支缺少必填字段或携带冗余字段都会被拒绝。
- `open_form` 或 `open_link` 引用的资源不存在、跨 Base 或父子关系错误时，修正稳定 ID 后重试，不要改用名称规避校验。
- 写入响应中的 `verification_hint` 是下一步提示，不是持久化证据；以 fresh `+button-rule-get` 为准。
- 可以先加 `--dry-run` 检查字段解析请求和最终公开 PUT body；dry-run 不发起绑定。
