---
name: lark-approval
version: 1.1.0
description: "飞书审批：当前用户审批的查询与全部处理操作，覆盖待本人审批的任务与本人发起的实例。审批待办不是飞书任务（任务类待办走 lark-task）；不负责创建审批定义和发起新审批。"
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
| 查待办/已办 | `tasks query`（`topic`：1待办 2已办 17未读 18已读）|
| 看表单/进度/当前节点 | `instances get` |
| 同意/拒绝 | `tasks approve` / `tasks reject` |
| 转交/加签/退回 | `tasks transfer` / `tasks add_sign` / `tasks rollback` |
| 催办 | `tasks remind` |
| 撤回/抄送/按定义查已发起 | `instances cancel` / `instances cc` / `instances initiated` |

处理链：`tasks query` 拿 `instance_code` + `task_id`（操作必须成对带上）→ 需要细节再 `instances get` → 执行操作。

```bash
lark-cli approval tasks query --params '{"topic":"1"}' --as user
lark-cli approval tasks approve --data '{"instance_code":"<ic>","task_id":"<tid>","comment":"同意"}' --as user
```

## 不在本 skill 范围

创建审批定义/发起新审批（走飞书客户端或审批管理后台）；非审批类待办 → [`lark-task`](../lark-task/SKILL.md)
