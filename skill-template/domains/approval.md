所有命令默认 `--as user`（审批是人的动作）。调用前先 `lark-cli schema approval.<resource>.<method>` 查参数结构，不要猜字段。

## 选哪个命令

| 想做什么 | 命令 |
|---|---|
| 搜可发起定义 | `approvals search` |
| 看审批定义详情/提单前确认表单与流程 | `approvals get` |
| 发起原生审批实例 | `instances create` |
| 查待办/已办 | `tasks query`（`topic`：1待办 2已办 17未读 18已读）|
| 看表单/进度/当前节点 | `instances get` |
| 同意/拒绝 | `tasks approve` / `tasks reject` |
| 转交/加签/退回 | `tasks transfer` / `tasks add_sign` / `tasks rollback` |
| 催办 | `tasks remind` |
| 撤回/抄送/按定义查已发起 | `instances cancel` / `instances cc` / `instances initiated` |

处理链：

- 发起审批：`approvals search` -> `approvals get` -> `instances.create`
- 处理审批：`tasks query` 拿 `instance_code` + `task_id`（操作必须成对带上）→ 需要细节再 `instances get` → 执行操作

```bash
lark-cli approval approvals search --data '{"keyword":"请假"}' --as user
lark-cli approval approvals get --params '{"approval_code":"<code>"}' --as user
lark-cli approval instances create --data '{"approval_code":"<code>","form":"[...]"}' --yes --as user
lark-cli approval tasks query --params '{"topic":"1"}' --as user
lark-cli approval tasks approve --data '{"instance_code":"<ic>","task_id":"<tid>","comment":"同意"}' --as user
```

## 发起原生审批

**BLOCKING REQUIREMENT: 只要用户意图是“发起审批 / 提单 / 提交请假审批 / 提交报销审批 / 创建审批实例”，第一步 MUST 先用 Read 工具读取 [`lark-approval-initiate.md`](../../skills/lark-approval/references/lark-approval-initiate.md)、[`approval-instance-form-control-parameters.md`](../../skills/lark-approval/references/approval-instance-form-control-parameters.md) 和 [`approval-instance-value-sourcing.md`](../../skills/lark-approval/references/approval-instance-value-sourcing.md)，并运行 `lark-cli schema approval.instances.create`。未完成前，禁止直接调用 `approval instances create`。**

**CRITICAL: 发起审批实例必须固定走 `approvals.search` -> `approvals.get` -> `instances.create`。禁止跳过 `approvals.get` 后直接猜测 `form`、`node_approver_list` 或 `node_cc_list`。**

**CRITICAL: `approvals.search` 返回的 `is_external=true` 表示三方定义。三方定义不要调用 `approval instances create`，应优先返回 `create_link` 并向用户说明需要通过该链接发起。只有 `is_external=false` 的原生审批定义，才进入 `get -> create` 流程。**

**CRITICAL: 详情规则全部下沉到 reference。** 控件 `value` 长什么样，看 [`approval-instance-form-control-parameters.md`](../../skills/lark-approval/references/approval-instance-form-control-parameters.md)；值从哪里拿，看 [`approval-instance-value-sourcing.md`](../../skills/lark-approval/references/approval-instance-value-sourcing.md)。不要在顶层入口重复展开这些细节。**

**CRITICAL: `approval instances create` 是写操作。真正执行前必须让用户确认最终定义、表单值和节点参数；执行时显式传 `--yes`，并在返回后回报 `instance_code` 与 `instance_link`。**
