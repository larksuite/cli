
# approval tasks transfer

转交一个审批任务给其他用户处理（用户级写操作）。通常先通过 `tasks query` 拿到 `task_id` 和 `instance_code`，确认目标任务后，再提供被转交人的用户 ID 执行转交。

> [!CAUTION]
> 这是 **high-risk-write** 写操作。建议先用 `--dry-run` 预览；真正执行时，如果用户已明确要转交该审批且目标任务、转交对象都无误，再带 `--yes` 运行。不要在未获用户明确同意时静默追加 `--yes`。

需要的 scopes: ["approval:task:write"]

## 命令

```bash
# 先预览请求，不实际执行
lark-cli approval tasks transfer \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","transfer_user_id":"ou_xxx","comment":"请你继续处理"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --dry-run

# 按 open_id 转交审批任务
lark-cli approval tasks transfer \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","transfer_user_id":"ou_xxx","comment":"转交给你处理"}' \
  --params '{"user_id_type":"open_id"}' \
  --as user \
  --yes

# 按 user_id 转交审批任务
lark-cli approval tasks transfer \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","transfer_user_id":"123456789","comment":"请补充审核"}' \
  --params '{"user_id_type":"user_id"}' \
  --as user \
  --yes

# 通过文件传入请求体，适合较长 comment
lark-cli approval tasks transfer \
  --data @./transfer-body.json \
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
| `transfer_user_id` | 是 | 被转交人的用户 ID；需要和 `user_id_type` 保持一致 |
| `comment` | 否 | 审批意见或转交说明，例如 `转交给你处理`、`请继续审核该单据` |
| `--params '{"user_id_type":"..."}'` | 否 | 查询参数 JSON；用于声明 `transfer_user_id` 的 ID 类型 |
| `user_id_type` | 否 | 用户 ID 类型：`user_id`、`union_id`、`open_id`；未显式指定时要特别确认 `transfer_user_id` 的真实类型 |
| `--as user` | 否 | 建议显式指定用户身份；审批转交通常必须以用户身份执行 |
| `--yes` | 否 | 确认执行高风险写操作；未带时可能返回 `confirmation_required` / exit 10 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 典型前置步骤

先查到待办任务：

```bash
lark-cli approval tasks query --params '{"topic":"1"}' --as user
```

常用到的字段：

| 字段 | 说明 |
|------|------|
| `tasks[].instance_code` | 审批实例 Code；执行 approve / reject / transfer / rollback 等操作时通常都需要 |
| `tasks[].task_id` | 审批任务 ID；与 `instance_code` 配对使用 |
| `tasks[].support_api_operate` | 是否支持通过 API 处理该任务；转交前建议先检查 |

如果你手里只有姓名或邮箱，建议先通过联系人能力解析出正确的用户 ID，再执行转交。

如需先确认表单、节点、审批流进度，可继续查看实例详情：

```bash
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user
```

## 使用建议

- **`instance_code` 和 `task_id` 要成对使用**：仅有实例 ID 或仅有任务 ID 都不足以准确执行转交操作。
- **`transfer_user_id` 与 `user_id_type` 必须匹配**：例如传 open_id 就把 `user_id_type` 设为 `open_id`；不要混用。
- **优先显式传 `user_id_type`**：这样 agent 更容易判断参数含义，也能减少 ID 类型不匹配带来的失败。
- **优先从 `tasks query` 的待办列表拿任务参数**：尤其是 `topic=1` 的待办审批，最适合作为 transfer 的输入来源。
- **先检查是否支持 API 操作**：如果 `tasks[].support_api_operate` 为 `false`，说明该任务可能不支持通过 API 执行同意/拒绝等处理动作，转交前也应谨慎验证。
- **`comment` 建议写明转交原因**：例如 `你更熟悉该项目，请继续处理`、`转交给预算 owner 审核`，方便接收人理解上下文。
- **先 `--dry-run` 再执行**：尤其在跨部门转交、批量处理或转交对象来源不明确时，先预览更安全。
