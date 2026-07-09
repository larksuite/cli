
# approval tasks remind

对审批实例中的指定任务发起催办（用户级写操作）。通常先通过 `tasks query` 找到待办任务，拿到 `instance_code` 和要催办的 `task_ids`，必要时再用 `instances get` 查看详情，然后执行催办。

> [!CAUTION]
> 这是 **high-risk-write** 写操作。建议先用 `--dry-run` 预览；真正执行时，如果用户已明确要催办该审批且目标实例、目标任务都无误，再带 `--yes` 运行。不要在未获用户明确同意时静默追加 `--yes`。

需要的 scopes: ["approval:instance:write"]

## 命令

```bash
# 先预览请求，不实际执行
lark-cli approval tasks remind \
  --data '{"instance_code":"<INSTANCE_CODE>","task_ids":["<TASK_ID>"],"comment":"请尽快处理"}' \
  --as user \
  --dry-run

# 催办单个审批任务
lark-cli approval tasks remind \
  --data '{"instance_code":"<INSTANCE_CODE>","task_ids":["<TASK_ID>"],"comment":"请尽快审批该单据"}' \
  --as user \
  --yes

# 同一实例下催办多个任务
lark-cli approval tasks remind \
  --data '{"instance_code":"<INSTANCE_CODE>","task_ids":["<TASK_ID_1>","<TASK_ID_2>"],"comment":"请相关审批人尽快处理"}' \
  --as user \
  --yes

# 通过文件传入请求体，适合较长 comment 或多个 task_ids
lark-cli approval tasks remind \
  --data @./remind-body.json \
  --as user \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--data '{...}'` | 是 | 请求体 JSON，使用 JSON 传入 |
| `instance_code` | 是 | 审批实例 Code；通常先通过 `tasks query` 或 `instances get` 获取 |
| `task_ids` | 是 | 被催办的任务 ID 数组；应与 `instance_code` 属于同一审批实例 |
| `comment` | 否 | 催办说明，例如 `请尽快处理`、`该单据较急，请优先审批` |
| `--as user` | 否 | 建议显式指定用户身份；审批催办通常必须以用户身份执行 |
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
| `tasks[].instance_code` | 审批实例 Code；催办时必须提供 |
| `tasks[].task_id` | 审批任务 ID；放入 `task_ids` 数组中 |
| `tasks[].title` | 任务标题，可用于确认催办对象是否正确 |
| `tasks[].status` | 任务状态；一般优先催办仍处于待处理状态的任务 |

如需进一步确认当前审批流、节点和人员信息，可继续查看实例详情：

```bash
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user
```

## 使用建议

- **`instance_code` 和 `task_ids` 要对应同一个审批实例**：不要把不同实例下的任务 ID 混在同一次催办请求中。
- **`task_ids` 是数组**：即使只催办一个任务，也要按数组形式传入。
- **优先从 `tasks query` 的待办列表拿参数**：尤其是 `topic=1` 的待办审批，最适合作为 remind 的输入来源。
- **催办前先确认任务仍需处理**：已经审批完成、已撤回或已终止的任务一般不适合继续催办。
- **`comment` 建议简洁且明确**：例如 `该单据较急，请优先审批`、`请今天内处理`。避免过长或模糊描述。
- **先 `--dry-run` 再执行**：尤其在一次催办多个任务、任务来源不明确或需让用户复核催办对象时，先预览更安全。
