
# approval tasks add_sign

给一个审批任务加签（用户级写操作）。通常先通过 `tasks query` 拿到 `task_id` 和 `instance_code`，确认目标任务后，再提供被加签人的用户 ID、加签方式等参数执行加签。

> [!CAUTION]
> 这是 **high-risk-write** 写操作。建议先用 `--dry-run` 预览；真正执行时，如果用户已明确要对该审批任务加签且目标任务、加签对象、加签方式都无误，再带 `--yes` 运行。用户已明确要求立即执行且列清本轮的加签、转交对象时，该确认覆盖这两个已明确的动作，不要逐条重复询问；不要在未获用户明确同意时静默追加 `--yes`。

需要的 scopes: ["approval:task:write"]

## 先选择加签类型

`add_sign_type` 会影响当前用户的审批任务是否继续可操作，不能只按示例顺序或任意选择：

| 用户意图 | `add_sign_type` | 处理方式 |
|----------|-----------------|----------|
| 明确说前加签 / “先让某人审核，我再审” | `1` | 前加签，并按用户要求选择 `approval_method` |
| 明确说后加签 / “我处理完，再让某人审核” | `2` | 后加签，并按用户要求选择 `approval_method` |
| “拉进来一起审” / “共同审核” / “一并确认” | `3` | 并加签；不传 `approval_method` |
| 同一请求要求先加签、再转交当前用户这一环 | `3` | **必须并加签**；加签成功后再转交当前任务 |

前加签或后加签可能推动当前用户的 task 流转，使原 `task_id` 不再支持后续转交。因此，“先加签，再把我这一环转交给其他人”不能使用前加签或后加签；先以 `add_sign_type: 3` 并加签，确认 `tasks add_sign` 成功后，再使用同一组 `instance_code` + `task_id` 执行 `tasks transfer`。

只有在上下文完全无法判断是哪种加签方式、且不同选择会改变审批流程时，才向用户二次询问。用户已经说“一起审”或已经要求“加签后转交当前环节”时，信息足够，不要再询问加签类型。

## 再选择 approval_method

只有前加签、后加签需要 `approval_method`；并加签不传。先遵循用户明确指定的审批方式；用户未指定时，按人数和语义选择：

| 加签人数与语义 | `approval_method` | 处理方式 |
|----------------|-------------------|----------|
| 只有 1 名加签人 | `1` | 使用或签；单人时或签、会签的实际效果相同，不再询问 |
| 多人，明确“任一人审批即可” / “一人通过即可” | `1` | 或签；任一加签人完成审批即可 |
| 多人，明确“所有人都要审批” / “全部确认” | `2` | 会签；所有加签人都必须完成审批 |
| 多人，明确“依次审批” / “先 A 后 B” | `3` | 依次审批；每个人按 `add_sign_user_ids` 的数组顺序逐一审批 |
| 多人，无法从上下文推断 | 不预设 | 询问用户选择或签、会签或依次审批，并说明三者效果 |

依次审批必须保留用户给出的人员顺序。如果已经确定要依次审批，但上下文无法判断先后顺序，先询问人员顺序，再构造 `add_sign_user_ids`；不要自行排序。

## 命令

```bash
# 先预览并加签请求，不实际执行
lark-cli approval tasks add_sign \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","add_sign_type":3,"add_sign_user_ids":["ou_xxx"],"comment":"请项目 owner 一起审核"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --dry-run

# 单人前加签：未指定方式时使用或签
lark-cli approval tasks add_sign \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","add_sign_type":1,"add_sign_user_ids":["ou_xxx"],"approval_method":1,"comment":"请先补充审核"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes

# 多人后加签：所有人都需要审批，使用会签
lark-cli approval tasks add_sign \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","add_sign_type":2,"add_sign_user_ids":["ou_xxx","ou_yyy"],"approval_method":2,"comment":"当前审批完成后请两位都完成审核"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes

# 多人前加签：按数组中的人员顺序依次审批
lark-cli approval tasks add_sign \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","add_sign_type":1,"add_sign_user_ids":["ou_first","ou_second"],"approval_method":3,"comment":"请先由第一位审核，再由第二位审核"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes

# 同一请求要求先加签、再转交：必须并加签；两条命令按顺序执行
lark-cli approval tasks add_sign \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","add_sign_type":3,"add_sign_user_ids":["ou_reviewer"],"comment":"请一起审核"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes

# 仅在上面的 add_sign 成功后，转交同一当前任务
lark-cli approval tasks transfer \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","transfer_user_id":"ou_transferee","comment":"出差期间请代为处理"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes

# 通过文件传入请求体，适合较长 comment 或较多加签人
lark-cli approval tasks add_sign \
  --data @./add-sign-body.json \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--data '{...}'` | 是 | 请求体 JSON，使用 JSON 传入 |
| `instance_code` | 是 | 审批实例 Code；通常先通过 `tasks query` 或 `instances initiated` / `instances get` 获取 |
| `task_id` | 是 | 审批任务 ID；通常先通过 `tasks query` 获取 |
| `add_sign_type` | 是 | 加签类型：`1` 前加签、`2` 后加签、`3` 并加签 |
| `add_sign_user_ids` | 是 | 被加签人 ID 数组；需要和 `user_id_type` 保持一致 |
| `approval_method` | 否 | 审批方式：`1` 或签、`2` 会签、`3` 依次审批；**仅在前加签、后加签时需要填写**。单人未指定时使用 `1`；多人无法从语义推断时先询问用户 |
| `comment` | 否 | 审批意见或加签说明，例如 `前加签给财务复核`、`请项目 owner 一并确认` |
| `--params '{"user_id_type":"..."}'` | 否 | 查询参数 JSON；用于声明 `add_sign_user_ids` 内用户 ID 的类型 |
| `user_id_type` | 否 | 用户 ID 类型：`user_id`、`union_id`、`open_id`；未显式指定时要特别确认被加签人的 ID 类型 |
| `--as user` | 否 | 建议显式指定用户身份；审批加签通常必须以用户身份执行 |
| `--yes` | 否 | 确认执行高风险写操作；未带时可能返回 `confirmation_required` / exit 10 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 枚举说明

### add_sign_type

| 值 | 含义 | 对当前任务的影响 |
|----|------|------------------|
| `1` | 前加签 | 在当前审批前插入审批人，可能推动当前用户的 task 流转 |
| `2` | 后加签 | 在当前审批后追加审批人，可能推动当前用户的 task 流转 |
| `3` | 并加签 | 增加并行审批人；需要随后转交当前环节时使用 |

### approval_method

仅适用于前加签、后加签；并加签不传。

| 值 | 含义 | 完成条件 |
|----|------|----------|
| `1` | 或签 | 任一加签人完成审批即可 |
| `2` | 会签 | 所有加签人都必须完成审批 |
| `3` | 依次审批 | 所有加签人按 `add_sign_user_ids` 数组顺序逐一审批 |

## 典型前置步骤

先查到待办任务：

```bash
lark-cli approval tasks query --params '{"topic":"1"}' --as user
```

常用到的字段：

| 字段 | 说明 |
|------|------|
| `tasks[].instance_code` | 审批实例 Code；执行 approve / reject / transfer / rollback / add_sign 等操作时通常都需要 |
| `tasks[].task_id` | 审批任务 ID；与 `instance_code` 配对使用 |
| `tasks[].support_api_operate` | 是否支持通过 API 处理该任务；加签前建议先检查 |

如果你手里只有姓名或邮箱，建议先通过联系人能力解析出正确的用户 ID，再执行加签。

如需先确认表单、节点、审批流进度，可继续查看实例详情：

```bash
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user
```

## 使用建议

- **`instance_code` 和 `task_id` 要成对使用**：仅有实例 ID 或仅有任务 ID 都不足以准确执行加签操作。
- **`add_sign_user_ids` 与 `user_id_type` 必须匹配**：例如传 open_id 就把 `user_id_type` 设为 `open_id`；不要混用。
- **优先显式传 `user_id_type`**：这样 agent 更容易判断参数含义，也能减少 ID 类型不匹配带来的失败。
- **`add_sign_type` 要和业务意图一致**：前加签是在当前审批前插入审批人，后加签是在当前审批后追加审批人，并加签则是增加并行审批人。
- **加签后还要转交当前环节时必须并加签**：使用 `add_sign_type: 3`，等待加签成功后再用同一组任务参数转交；不要用前加签或后加签导致当前 task 提前流转。
- **无法推断类型时才询问**：如果用户没有说明先后或并行关系，且后续动作也不能帮助判断，再请用户选择；不要对“一起审”或“加签后转交”重复提问。
- **前加签 / 后加签要补 `approval_method`**：单人未指定时使用或签；多人优先按语义选择，无法推断时询问用户，不要静默默认。
- **依次审批保留人员顺序**：按用户指定的先后顺序构造 `add_sign_user_ids`；顺序不明确时先询问，不要自行排序。
- **优先从 `tasks query` 的待办列表拿任务参数**：尤其是 `topic=1` 的待办审批，最适合作为 add_sign 的输入来源。
- **先检查是否支持 API 操作**：如果 `tasks[].support_api_operate` 为 `false`，说明该任务可能不支持通过 API 执行处理动作，加签前应谨慎验证。
- **`comment` 建议写明加签原因**：例如 `增加财务复核`、`增加项目 owner 并行确认`，方便相关人员理解上下文。
- **先 `--dry-run` 再执行**：尤其在多人加签、跨部门加签或加签对象来源不明确时，先预览更安全。
