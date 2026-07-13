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

所有命令默认 `--as user`（审批是人的动作）。调用前先按需读取 references 下对应的文件，查参数结构，不要猜字段；**references 是第一信息源**，只有在 reference 未覆盖的原生 / 高级场景下，才额外用 `lark-cli ... --help`、`lark-cli schema` 等方式补充确认字段。

## 路由优先级（先判断是不是审批，再选命令）

审批待办不是飞书任务。**只要用户的核心对象是审批单据 / 审批待办 / 审批实例，就优先使用 `lark-approval`，不要让渡给 `lark-task`。**

### 明确归 `lark-approval` 的高优先级语义

出现以下任一语义时，优先走 `lark-approval`：

- 审批待办 / 审批单据 / 审批实例 / 审批意见 / 审批定义
- 同意 / 拒绝 / 转交 / 退回 / 撤回 / 催办 / 加签 / 抄送
- 待办列表 / 待办单据 / 已发起审批 / 已办审批 / 审批详情 / 同意可编辑

**判定规则：** 只要最终动作是对审批单据做同意、拒绝、转交、退回、撤回、催办、加签、抄送、查详情、查已发起/已办/待办，就归 `lark-approval`。只有当用户处理的是**非审批类任务/待办**时，才走 [`lark-task`](../lark-task/SKILL.md)。

## 选哪个命令

| 想做什么 | 命令 | 按需读取 reference                                                                  |
|---|---|---------------------------------------------------------------------------------|
| 搜可发起定义 | `approvals search` | [`lark-approval-approvals-search.md`](references/lark-approval-approvals-search.md) |
| 看审批定义详情/提单前确认表单与流程 | `approvals get` | [`lark-approval-approvals-get.md`](references/lark-approval-approvals-get.md)   |
| 发起原生审批实例/提交请假审批/提交报销审批/创建审批实例 | `instances create` | [`lark-approval-initiate.md`](references/lark-approval-initiate.md)             |
| 查待办/已办 | `tasks query`（`topic`：1待办 2已办 17未读 18已读） | [`lark-approval-tasks-query.md`](references/lark-approval-tasks-query.md)       |
| 看表单/进度/当前节点 | `instances get` | [`lark-approval-instances-get.md`](references/lark-approval-instances-get.md)   |
| 同意审批 | `tasks approve` | [`lark-approval-tasks-approve.md`](references/lark-approval-tasks-approve.md)   |
| 拒绝审批 | `tasks reject` | [`lark-approval-tasks-reject.md`](references/lark-approval-tasks-reject.md)     |
| 转交审批 | `tasks transfer` | [`lark-approval-tasks-transfer.md`](references/lark-approval-tasks-transfer.md) |
| 加签审批 | `tasks add_sign` | [`lark-approval-tasks-add-sign.md`](references/lark-approval-tasks-add-sign.md) |
| 退回审批 | `tasks rollback` | [`lark-approval-tasks-rollback.md`](references/lark-approval-tasks-rollback.md) |
| 催办审批 | `tasks remind` | [`lark-approval-tasks-remind.md`](references/lark-approval-tasks-remind.md)     |
| 撤回已发起审批 | `instances cancel` | [`lark-approval-instances-cancel.md`](references/lark-approval-instances-cancel.md) |
| 给审批实例追加抄送 | `instances cc` | [`lark-approval-instances-cc.md`](references/lark-approval-instances-cc.md)     |
| 按定义查已发起审批 | `instances initiated` | [`lark-approval-instances-initiated.md`](references/lark-approval-instances-initiated.md) |

处理链：

- 发起审批：`approvals search` -> `approvals get` -> `instances create`
- 处理审批：`tasks query` 拿 `instance_code` + `task_id`（操作必须成对带上）→ 只有用户明确需要查看详情、当前节点、表单内容、或流程进度时，再 `instances get` → 执行操作

## 执行原则（减少误路由、误重试和无效消耗）

### 1) 先拿最小必要信息，再执行

- 目标只是处理待办时，优先 `tasks query` 获取 `instance_code` + `task_id`
- **只有**用户明确要看详情、当前节点、表单内容、流程进度时，才调用 `instances get`
- 用户已经明确给出 `instance_code` / `task_id` 时，不要先查列表再过滤

### 2) 已知对象时直达动作

- 已拿到 `instance_code` + `task_id` 后，优先直接执行 `tasks approve/reject/transfer/add_sign/rollback/remind`
- 同一轮里如果已有足够的新鲜查询结果，不要重复 `tasks query`
- 不要默认走 `list -> filter -> detail -> write` 全链路；对象已明确时应压缩步骤

### 3) 错误码驱动，而不是盲目重试

- 写操作失败后，先看错误码和报错语义，再决定是否补查或结束
- **除非错误明确提示可恢复或需要补充参数，否则不要重复刷同一个写操作**
- 同一个失败原因不要连续多次重试，避免 token 和耗时失控，最多重试1次

## 写操作失败处理：1395001 决策树

当拒绝 / 转交 / 退回 / 撤回 / 同意等写操作返回 `1395001`（任务状态异常 / 写前置校验失败）时，按下面规则处理：

1. **先停止盲目重试**，不要连续重复提交相同写操作，最多重试1次
2. 优先从以下角度解释：
   - 任务可能已被他人处理
   - 单据状态已变化，当前动作已不再允许
   - 当前用户已不具备该任务的操作资格
   - 当前节点或单据状态不支持该操作
3. 如需确认，只补 **一次** 状态查询（`tasks query` 或 `instances get`），不要陷入 query/write 循环
4. 最终给用户明确结论和下一步建议，而不是继续无意义重试

**特别注意：** 对拒绝 / 转交 / 撤回场景更要严格执行上述规则；这些场景最容易因状态切换而失败。

```bash
lark-cli approval approvals search --data '{"keyword":"请假"}' --as user
lark-cli approval approvals get --params '{"approval_code":"<code>"}' --as user
lark-cli approval instances create --data '{"approval_code":"<code>","form":"[...]"}' --yes --as user
lark-cli approval tasks query --params '{"topic":"1"}' --as user
lark-cli approval tasks approve --data '{"instance_code":"<ic>","task_id":"<tid>","comment":"同意"}' --as user
```

## 不在本 skill 范围

创建审批定义（走飞书客户端或审批管理后台）；三方定义发起（返回 `create_link`，引导用户通过链接发起）；非审批类待办 → [`lark-task`](../lark-task/SKILL.md)
