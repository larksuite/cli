---
name: lark-approval
version: 1.2.0
description: "飞书审批：查询和处理审批待办/已办/实例，搜索可发起审批定义、查看定义详情并发起原生审批实例。当用户要处理审批任务、查看审批实例、搜索或发起审批时使用。审批待办不是飞书任务；非审批类待办走 lark-task。不负责创建审批定义；三方审批定义不走原生提单。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli approval --help"
---

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

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

发起审批属于高风险写操作，按下表处理：

| 规则 | 处理 |
|---|---|
| 用户意图是发起审批 / 提单 / 提交请假审批 / 提交报销审批 / 创建审批实例 | 先读 [`references/lark-approval-initiate.md`](references/lark-approval-initiate.md)、[`references/lark-approval-instance-form-control-parameters.md`](references/lark-approval-instance-form-control-parameters.md) 和 [`references/lark-approval-instance-value-sourcing.md`](references/lark-approval-instance-value-sourcing.md)，并运行 `lark-cli schema approval.instances.create` |
| 编排顺序 | 固定走 `approvals.search` -> `approvals.get` -> `instances.create`；未拿到定义详情前不要猜 `form`、`node_approver_list` 或 `node_cc_list` |
| 三方定义 | `is_external=true` 时不要调用 `approval instances create`，返回 `create_link` 并说明需通过链接发起 |
| 表单与节点参数 | 控件 `value` 结构看 [`references/lark-approval-instance-form-control-parameters.md`](references/lark-approval-instance-form-control-parameters.md)；值来源看 [`references/lark-approval-instance-value-sourcing.md`](references/lark-approval-instance-value-sourcing.md) |
| 真正执行前 | 让用户确认最终定义、表单值和节点参数；执行时显式传 `--yes`，成功后回报 `instance_code` 与 `instance_link` |

## 不在本 skill 范围

创建审批定义（走飞书客户端或审批管理后台）；三方定义发起（返回 `create_link`，引导用户通过链接发起）；非审批类待办 → [`lark-task`](../lark-task/SKILL.md)
