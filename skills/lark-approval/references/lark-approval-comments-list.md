# approval comments list

查询审批实例的评论树（用户级只读操作）。适合查看顶层评论、回复、评论创建人、删除标记、@ 人信息和图片 / 文件内容结构。

需要的 scopes: ["approval:instance:read"]

## 命令

```bash
# 查询审批实例评论
lark-cli approval comments list \
  --params '{"instance_id":"<INSTANCE_CODE>"}' \
  --as user

# 指定返回用户字段的 ID 类型
lark-cli approval comments list \
  --params '{"instance_id":"<INSTANCE_CODE>","user_id_type":"open_id"}' \
  --as user

# 表格格式输出，便于快速浏览顶层字段
lark-cli approval comments list \
  --params '{"instance_id":"<INSTANCE_CODE>"}' \
  --format table \
  --as user

# 预览 API 调用，不执行
lark-cli approval comments list \
  --params '{"instance_id":"<INSTANCE_CODE>"}' \
  --as user \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--params '{...}'` | 是 | 路径和查询参数，使用 JSON 传入 |
| `instance_id` | 是 | 审批实例 Code；也兼容租户自定义审批实例 ID |
| `user_id_type` | 否 | 返回结果中用户字段的 ID 类型：`open_id`、`user_id`、`union_id`；不填默认 `open_id` |
| `--as user` | 否 | 建议显式指定用户身份；审批评论是用户态接口 |
| `--format` | 否 | 输出格式：`json`（默认）、`ndjson`、`table`、`csv` |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 输出重点字段

| 字段 | 说明 |
|------|------|
| `comments[].id` | 顶层评论 ID；回复、编辑、删除时常用 |
| `comments[].content` | 评论内容字符串；按服务端原样返回，可能包含 `text`、`files` 等结构 |
| `comments[].commentator` | 评论创建人，格式受 `user_id_type` 影响 |
| `comments[].create_time` | 评论创建时间，毫秒级时间戳字符串 |
| `comments[].update_time` | 评论更新时间，毫秒级时间戳字符串 |
| `comments[].is_delete` | 是否已删除，`0` 未删除，`1` 已删除 |
| `comments[].replies[]` | 当前评论下的回复列表，字段结构与顶层评论类似 |
| `comments[].at_info_list[]` | 评论中的 @ 人信息 |

## 使用建议

- 审批评论挂在审批实例上，输入字段叫 `instance_id`，实际通常传审批实例 Code。
- 如果要回复、编辑或删除评论，先用 `comments list` 获取目标 `comment_id`。
- `comments list` 返回评论树；删除后的评论或回复可能仍在树中出现，但 `is_delete=1`。
- `user_id_type` 只影响返回结果中的用户 ID 展示，不用于指定当前操作人；当前操作人来自用户身份令牌。
- 这不是 Drive 文档评论；不要把审批实例评论误路由到 `drive +add-comment` 或 Drive comment API。
