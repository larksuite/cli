# 仪表盘按钮绑定工作流

本页用于已有 Dashboard 按钮组件的工作流绑定、解绑和查询。定位需要同一
Base 下的 `base_token + dashboard_id + block_id`。用 `+dashboard-list`
定位仪表盘，再用 `+dashboard-block-list` / `+dashboard-block-get` 确认目标
组件的 ID 和 `type=button`；不要把 Dashboard 容器 ID 当作按钮组件 ID。

表格中的按钮字段使用 `+button-rule-bind/get/unbind`，参数是
`table_id + field_id`。这些命令不接受仪表盘组件坐标。

## 接口入口

目标环境必须已部署以下 GET/PUT 路由，并允许当前身份调用。优先使用
`lark-cli base --help` 中实际发布的类型化命令；服务元数据尚未收录时，
使用本页的 `lark-cli api`。不要猜测新的 `+dashboard-button-*` 命令或
给现有字段按钮命令添加未声明的参数。

```text
/open-apis/base/v3/bases/{base_token}/dashboards/{dashboard_id}/blocks/{block_id}/button_rule
```

GET 无请求体、无专用 query 参数。PUT 直接提交 JSON 对象，不包装成
`Payload` 字符串，不提交网关注入的用户、租户、应用或 Meta 信息。

## 查询绑定，再获取工作流

```bash
lark-cli api GET \
  '/open-apis/base/v3/bases/<base_token>/dashboards/<dashboard_id>/blocks/<block_id>/button_rule' \
  --as user
```

成功时 CLI 的 `data` 为：

```json
{
  "dashboard_id": "<dashboard_id>",
  "block_id": "<block_id>",
  "bound": true,
  "target": {"type": "workflow", "id": "wkf_xxx"}
}
```

先检查 `ok=true` 和两个目标 ID，再解释绑定状态：

| 返回值 | 含义与后续动作 |
|---|---|
| `bound=false`，`target=null` | 未绑定，无需查询工作流定义 |
| `bound=true`，`target.type=workflow`，`target.id` 为公开 `wkf` ID | 使用 `target.id` 调用 `+workflow-get` |
| `bound=true`，`target.type=unknown`，无 `id` | 存在旧类型绑定；报告已绑定但目标类型无法由此接口解析，不猜测 workflow ID |

失败、缺字段或不符合以上组合的结果不能当作未绑定。V2 工作流缺少合法的
公开 ID 时，服务端返回错误，不会降级成 `bound=false`。

专用 GET 查询的是实际 ButtonRule 关系。组件 `data_config.workflow_id`
是快照中的值，可能使用内部 ID；不要拿它替代 `target.id`，也不要把内部
数字 ID 拼成公开 ID。

获取完整定义时，把查询成功的 `data.target.id` 传入下列命令：

```bash
lark-cli base +workflow-get \
  --base-token <base_token> --workflow-id <target.id> --as user
```

`+workflow-get` 返回工作流的 `title`、`status` 和 `steps`；它与读取按钮
绑定是两个操作。寻找待绑定的工作流时，可用 `+workflow-list --base-token
<base_token>`，但列表并不表示每个流程都适合由仪表盘按钮触发。

## 绑定或更换工作流

```bash
lark-cli api PUT \
  '/open-apis/base/v3/bases/<base_token>/dashboards/<dashboard_id>/blocks/<block_id>/button_rule' \
  --data '{"workflow_id":"wkf_xxx"}' \
  --as user
```

`workflow_id` 必须来自当前 Base 的工作流，使用 `+workflow-list/get/create`
返回的公开 ID：以 `wkf` 开头、长度大于 3，且不含任何空白字符。
已有绑定时 PUT 更新该关系。只提交 `workflow_id`，不要附加 `label`、
`data_config` 等字段；样式和文本配置由组件配置入口处理。

也可使用 `--data @button-rule.json` 或 `--data -` 从文件或 stdin 读取相同
对象。绑定成功时返回 `bound=true` 和服务端确认的 `target`；需要验证
持久化时，再调用专用 GET 核对目标 ID 和 `target.id`。

## 解绑

```bash
lark-cli api PUT \
  '/open-apis/base/v3/bases/<base_token>/dashboards/<dashboard_id>/blocks/<block_id>/button_rule' \
  --data '{"workflow_id":""}' \
  --as user
```

解绑仍使用 PUT，必须保留 `workflow_id` 键和精确的空字符串。
省略字段、空请求体、`null`、数字和空白字符串均不表示解绑；接口会拒绝。
成功结果为 `bound=false, target=null`。需要验收时用 GET 确认该组合，
重复解绑仍应保持未绑定状态。

解绑只解除按钮关联，不删除按钮或工作流，也不代表停用工作流。

## 仪表盘按钮的触发器

为仪表盘按钮创建工作流时，触发器使用 `ButtonTrigger`，其 `data` 为：

```json
{"button_type":"buttonElement"}
```

把它放入完整的 `steps` 执行图后交给 `+workflow-create`；完整结构见
[Workflow schema](lark-base-workflow-schema.md)。`buttonElement` 不填写
`table_name`，只提供 `time`、`user`、`buttonName` 等基础触发属性，不提供
表格按钮的当前记录字段、`recordId` 或 `recordLink`。需要查询或处理记录时，
在后续步骤明确选择目标数据表和记录。

绑定不会自动启用工作流。读取 `+workflow-get` 的状态后，按用户要求使用
`+workflow-enable` / `+workflow-disable`；仅查询或仅解除关联时，不附带
启停操作。创建按钮组件的配置协议另属 Dashboard Block 配置，不向绑定
接口传创建参数，也不把绑定成功当作组件已创建的证据。

## 失败处理与验证边界

保留已有的按钮和工作流 ID，按 CLI 返回的业务错误、hint 和 log ID 排查。
写入失败或结果不确定时，先 GET 读取当前关系，再决定是否重试；不要重建
按钮/工作流或宣称解绑成功。权限问题按 `lark-shared` 修复当前身份的 scope
或资源权限，不能照抄表格按钮的 `base:field:*` 权限作为本接口要求。

追加 `--dry-run` 可核对方法、路径和请求体；它不调用服务端，不能证明路由
已部署、ID 有效、权限足够或实际关系已改变。正式类型化命令仍依赖上游
元数据发布；其响应定义应保留 `bound` 和可为 null 的 `target`，而非平铺的
`workflow_id`。
