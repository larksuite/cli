
# approval instances cancel

撤回一个已发起的审批实例（用户级写操作）。通常先通过 `instances initiated`、`tasks query` 或 `instances get` 确认目标审批实例，拿到 `instance_code` 后再执行撤回。

> [!CAUTION]
> 这是 **high-risk-write** 写操作。建议先用 `--dry-run` 预览；真正执行时，如果用户已明确要撤回该审批实例且目标实例无误，再带 `--yes` 运行。不要在未获用户明确同意时静默追加 `--yes`。

需要的 scopes: ["approval:instance:write"]

## 命令

```bash
# 先预览请求，不实际执行
lark-cli approval instances cancel \
  --data '{"instance_code":"<INSTANCE_CODE>"}' \
  --as user \
  --dry-run

# 撤回一个审批实例
lark-cli approval instances cancel \
  --data '{"instance_code":"<INSTANCE_CODE>"}' \
  --as user \
  --yes

# 通过文件传入请求体
lark-cli approval instances cancel \
  --data @./cancel-body.json \
  --as user \
  --yes
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--data '{...}'` | 是 | 请求体 JSON，使用 JSON 传入 |
| `instance_code` | 是 | 审批实例 Code；通常先通过 `instances initiated`、`tasks query` 或 `instances get` 获取 |
| `--as user` | 否 | 建议显式指定用户身份；审批实例撤回通常必须以用户身份执行 |
| `--yes` | 否 | 确认执行高风险写操作；未带时可能返回 `confirmation_required` / exit 10 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 典型前置步骤

如果你要找“我发起的审批实例”，可先查询已发起列表：

```bash
lark-cli approval instances initiated --params '{"page_size":20}' --as user
```

如果你已经在任务列表中定位到某个审批，也可以从任务里拿到实例 Code：

```bash
lark-cli approval tasks query --params '{"topic":"1"}' --as user
```

常用到的字段：

| 字段 | 说明 |
|------|------|
| `instances[].instance_code` | 审批实例 Code；撤回时必须提供 |
| `tasks[].instance_code` | 审批任务关联的审批实例 Code；也可作为撤回输入 |
| `tasks[].instance_status` | 审批实例状态；可用于判断是否仍处于可撤回阶段 |

如需先确认审批表单、当前节点、流转状态，可继续查看实例详情：

```bash
lark-cli approval instances get --params '{"instance_code":"<INSTANCE_CODE>"}' --as user
```

## 使用建议

- **撤回的是审批实例，不是单个任务**：`instances cancel` 只需要 `instance_code`，不需要 `task_id`。
- **优先确认实例是否仍可撤回**：已经通过、已拒绝、已撤销或已终止的实例通常不适合继续撤回。
- **优先从 `instances initiated` 获取目标实例**：因为撤回通常针对“我发起的审批”，这个入口最直接。
- **也可从 `tasks query` 反查 `instance_code`**：当你是从某个待办/已办上下文进入时，这样更方便。
- **先 `--dry-run` 再执行**：尤其在实例来源不明确、用户只给了标题关键字，或一次要核对多个实例时，先预览更安全。
