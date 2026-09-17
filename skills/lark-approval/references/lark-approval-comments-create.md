# approval comments create

创建、回复或编辑审批实例评论（用户级写操作）。同一个命令通过请求体区分三种行为：不传 `parent_comment_id` 和 `comment_id` 是创建顶层评论；传 `parent_comment_id` 是回复；传 `comment_id` 是编辑本人已有评论或回复。

需要的 scopes: ["approval:instance:write"]

## 命令

```bash
# 创建顶层评论
lark-cli approval comments create \
  --params '{"instance_id":"<INSTANCE_CODE>"}' \
  --data '{"content":"{\"text\":\"请补充报销说明\"}"}' \
  --as user

# 回复一条顶层评论
lark-cli approval comments create \
  --params '{"instance_id":"<INSTANCE_CODE>"}' \
  --data '{"parent_comment_id":"<PARENT_COMMENT_ID>","content":"{\"text\":\"已补充，请查看\"}"}' \
  --as user

# 编辑本人评论或回复
lark-cli approval comments create \
  --params '{"instance_id":"<INSTANCE_CODE>"}' \
  --data '{"comment_id":"<COMMENT_ID>","content":"{\"text\":\"更新后的评论内容\"}"}' \
  --as user

# 只同步评论数据，不触发 bot
lark-cli approval comments create \
  --params '{"instance_id":"<INSTANCE_CODE>"}' \
  --data '{"content":"{\"text\":\"仅同步评论\"}","disable_bot":true}' \
  --as user

# 创建带 @ 人的文本评论
lark-cli approval comments create \
  --params '{"instance_id":"<INSTANCE_CODE>","user_id_type":"open_id"}' \
  --data '{"content":"{\"text\":\"@张三 请补充说明\"}","at_info_list":[{"user_id":"ou_xxx","name":"张三","offset":"0"}]}' \
  --as user

# 预览 API 调用，不执行
lark-cli approval comments create \
  --params '{"instance_id":"<INSTANCE_CODE>"}' \
  --data '{"content":"{\"text\":\"请补充说明\"}"}' \
  --as user \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--params '{...}'` | 是 | 路径和查询参数，使用 JSON 传入 |
| `instance_id` | 是 | 审批实例 Code；也兼容租户自定义审批实例 ID |
| `user_id_type` | 否 | 评论相关用户字段的 ID 类型：`open_id`、`user_id`、`union_id`；不填默认 `open_id` |
| `--data '{...}'` | 是 | 请求体 JSON，使用 JSON 传入 |
| `content` | 是 | 评论内容字符串；当前 CLI 引导只支持 `{"text":"..."}` 文本结构 |
| `at_info_list` | 否 | @ 人信息数组；元素包含 `user_id`、`name`、`offset`，并与 `content.text` 中的 @ 文案对应 |
| `parent_comment_id` | 回复时必填 | 父评论 ID；传入后创建回复 |
| `comment_id` | 编辑时必填 | 要编辑的评论或回复 ID；传入后编辑本人已有评论或回复 |
| `disable_bot` | 否 | `true` 表示只同步评论数据，不触发 bot |
| `extra` | 否 | 附加字段字符串 |
| `--as user` | 否 | 建议显式指定用户身份；当前操作人来自用户身份令牌，不从参数指定 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## content 格式

`content` 是字符串，不是 JSON 对象。当前只按文本评论暴露：

```json
{
  "content": "{\"text\":\"请补充说明\"}"
}
```

@ 人评论需要同时设置 `content.text` 和 `at_info_list`：

```json
{
  "content": "{\"text\":\"@张三 请补充说明\"}",
  "at_info_list": [
    {
      "user_id": "ou_xxx",
      "name": "张三",
      "offset": "0"
    }
  ]
}
```

其中 `offset` 从 0 开始，表示 `content.text` 中对应 @ 符号的位置；`user_id` 的类型需要和 `user_id_type` 保持一致。

## 使用建议

- 创建、回复、编辑是同一个 OpenAPI action；根据 `parent_comment_id` 和 `comment_id` 判断语义。
- 编辑只能编辑当前用户自己的评论或回复；不要尝试用参数伪造操作人。
- `user_id_type` 只影响评论相关用户字段的 ID 类型，不用于指定当前操作人。
- 先用 `comments list` 获取 `comment_id`，再执行回复、编辑或删除。
- 当前只引导创建文本评论；不要写入非 `text` 的内容结构。
- 这个接口不是清空评论；本期不要暴露或使用 `clear`。
