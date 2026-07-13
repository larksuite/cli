
# approval tasks reject

拒绝一个审批任务（用户级写操作）。通常先通过 `tasks query` 拿到 `task_id` 和 `instance_code`，必要时再用 `instances get` 查看详情，然后再执行拒绝。

> [!CAUTION]
> 这是 **high-risk-write** 写操作。建议先用 `--dry-run` 预览；真正执行时，如果用户已明确要拒绝该审批且目标任务无误，再带 `--yes` 运行。不要在未获用户明确同意时静默追加 `--yes`。

需要的 scopes: ["approval:task:write"]

## 命令

```bash
# 先预览请求，不实际执行
lark-cli approval tasks reject \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","comment":"拒绝"}' \
  --as user \
  --dry-run

# 拒绝审批任务，并附带审批意见
lark-cli approval tasks reject \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","comment":"拒绝，信息不完整"}' \
  --as user \
  --yes

# 通过文件传入请求体，适合较长 comment
lark-cli approval tasks reject \
  --data @./reject-body.json \
  --as user \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--data '{...}'` | 是 | 请求体 JSON，使用 JSON 传入 |
| `instance_code` | 是 | 审批实例 Code；通常先通过 `tasks query` 或 `instances initiated` / `instances get` 获取 |
| `task_id` | 是 | 审批任务 ID；通常先通过 `tasks query` 获取 |
| `comment` | 否 | 审批意见，例如 `拒绝`、`拒绝，信息不完整` |
| `--as user` | 否 | 建议显式指定用户身份；审批拒绝通常必须以用户身份执行 |
| `--yes` | 否 | 确认执行高风险写操作；未带时可能返回 `confirmation_required` / exit 10 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 典型前置步骤

先查到待办任务：

```bash
lark-cli approval tasks query --params '{"topic":"1"}' --as user
```

常用到的两个字段：

| 字段 | 说明 |
|------|------|
| `tasks[].instance_code` | 审批实例 Code；执行 approve / reject / rollback 等操作时通常都需要 |
| `tasks[].task_id` | 审批任务 ID；与 `instance_code` 配对使用 |

如需先确认表单、节点、审批流进度，可继续查看实例详情：

```bash
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user
```

## 使用建议

- **`instance_code` 和 `task_id` 要成对使用**：仅有实例 ID 或仅有任务 ID 都不足以准确执行拒绝操作。
- **优先从 `tasks query` 的待办列表拿参数**：尤其是 `topic=1` 的待办审批，最适合作为 reject 的输入来源。
- **先检查是否支持 API 操作**：如果上一步 `tasks query` 返回的 `tasks[].support_api_operate` 为 `false`，说明该任务可能不支持通过 API 同意/拒绝。
- **`comment` 建议写清拒绝原因**：例如 `拒绝，缺少合同附件`、`拒绝，预算字段填写不完整`。这有助于发起人理解原因并补充材料。
- **先 `--dry-run` 再执行**：尤其在批量处理或任务来源不明确时，先预览更安全。
