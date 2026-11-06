# approval comments delete

删除审批实例下的一条评论或回复（用户级高风险写操作）。这个命令只删除指定 `comment_id`，不会清空实例下的全部评论。

> [!CAUTION]
> 这是 **high-risk-write** 写操作。建议先用 `--dry-run` 预览；真正执行时，如果用户已明确要删除该条审批评论或回复且 `instance_id`、`comment_id` 都无误，再带 `--yes` 运行。不要在未获用户明确同意时静默追加 `--yes`。

需要的 scopes: ["approval:instance:write"]

## 命令

```bash
# 先预览请求，不实际执行
lark-cli approval comments delete \
  --params '{"instance_id":"<INSTANCE_CODE>","comment_id":"<COMMENT_ID>"}' \
  --as user \
  --dry-run

# 删除一条顶层评论或回复
lark-cli approval comments delete \
  --params '{"instance_id":"<INSTANCE_CODE>","comment_id":"<COMMENT_ID>"}' \
  --as user \
  --yes

# 删除后回查确认 is_delete
lark-cli approval comments list \
  --params '{"instance_id":"<INSTANCE_CODE>"}' \
  --as user
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--params '{...}'` | 是 | 路径参数，使用 JSON 传入 |
| `instance_id` | 是 | 审批实例 Code；也兼容租户自定义审批实例 ID |
| `comment_id` | 是 | 要删除的评论或回复 ID |
| `--as user` | 否 | 建议显式指定用户身份；当前操作人来自用户身份令牌，不从参数指定 |
| `--yes` | 是 | 确认执行高风险写操作；未带时可能返回 `confirmation_required` / exit 10 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行；dry-run 不需要 `--yes` |

## 使用建议

- 先用 `comments list` 获取并确认目标 `comment_id`，再执行删除。
- 这个命令删除单条评论或回复，不是清空全部评论；不要使用或暴露 `clear`。
- 只能删除当前用户有权删除的评论或回复。通常本人评论可删除，非本人评论会被服务端权限校验拦截。
- 删除后建议回查 `comments list`，确认目标评论或回复的 `is_delete=1`。
- 不要通过额外参数指定操作人；UAT 接口会从当前用户身份令牌识别操作人。
