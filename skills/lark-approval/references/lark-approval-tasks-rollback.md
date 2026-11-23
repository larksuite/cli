
# approval tasks rollback

将一个审批任务退回到指定节点（用户级写操作）。通常先通过 `tasks query` 拿到 `task_id` 和 `instance_code`，再结合实例详情确认可退回的目标节点 `node_ids`，最后执行退回。

> [!CAUTION]
> 这是 **high-risk-write** 写操作。建议先用 `--dry-run` 预览；真正执行时，如果用户已明确要退回该审批且目标任务、退回节点都无误，再带 `--yes` 运行。不要在未获用户明确同意时静默追加 `--yes`。

需要的 scopes: ["approval:task:write"]

## 命令

```bash
# 先预览请求，不实际执行
lark-cli approval tasks rollback \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","node_ids":["<NODE_ID>"],"comment":"退回补充材料"}' \
  --as user \
  --dry-run

# 退回到单个节点
lark-cli approval tasks rollback \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","node_ids":["<NODE_ID>"],"comment":"请补充附件后重新提交"}' \
  --as user \
  --yes

# 退回到发起节点（发起节点 ID 为 START）
lark-cli approval tasks rollback \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","node_ids":["START"],"comment":"退回发起人补充材料"}' \
  --as user \
  --yes

# 传多个候选节点 ID（以实际审批定义支持情况为准）
lark-cli approval tasks rollback \
  --data '{"instance_code":"<INSTANCE_CODE>","task_id":"<TASK_ID>","node_ids":["<NODE_ID_1>","<NODE_ID_2>"],"comment":"退回上一处理节点"}' \
  --as user \
  --yes

# 通过文件传入请求体，适合较长 comment 或较多 node_ids
lark-cli approval tasks rollback \
  --data @./rollback-body.json \
  --as user \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--data '{...}'` | 是 | 请求体 JSON，使用 JSON 传入 |
| `instance_code` | 是 | 审批实例 Code；通常先通过 `tasks query` 或 `instances initiated` / `instances get` 获取 |
| `task_id` | 是 | 审批任务 ID；通常先通过 `tasks query` 获取 |
| `node_ids` | 是 | 退回目标节点 ID 数组；发起节点 ID 为 `START`；执行前应先确认这些节点确实可作为退回目标 |
| `comment` | 否 | 审批意见或退回说明，例如 `请补充附件后重新提交`、`预算说明不完整，请补充` |
| `--as user` | 否 | 建议显式指定用户身份；审批退回通常必须以用户身份执行 |
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
| `tasks[].support_api_operate` | 是否支持通过 API 处理该任务；退回前建议先检查 |

如需确认流程节点、当前进度和可退回位置，可先查看实例详情：

```bash
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user
```

## 使用建议

- **`instance_code` 和 `task_id` 要成对使用**：仅有实例 ID 或仅有任务 ID 都不足以准确执行退回操作。
- **`node_ids` 是必填项**：退回并不是“自动退回上一步”，而是要明确给出目标节点 ID 数组；退回发起节点时传 `START`。
- **先确认节点是否可退回**：不同审批定义支持的退回目标可能不同；在不确定时，先通过 `instances get` 或业务侧流程信息核实。
- **优先从 `tasks query` 的待办列表拿任务参数**：尤其是 `topic=1` 的待办审批，最适合作为 rollback 的输入来源。
- **先检查是否支持 API 操作**：如果 `tasks[].support_api_operate` 为 `false`，说明该任务可能不支持通过 API 执行处理动作，退回前应谨慎验证。
- **`comment` 建议写清退回原因**：例如 `附件缺失，请补齐后重新提交`、`费用说明不完整，请补充明细`，方便发起人或上一步处理人理解原因。
- **先 `--dry-run` 再执行**：尤其在节点来源不明确、审批链路复杂或批量处理时，先预览更安全。
