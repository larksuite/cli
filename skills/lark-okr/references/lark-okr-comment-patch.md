# okr +comment-patch
> **前置条件：** 先阅读 [lark-shared/SKILL.md](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则；

修改指定评论的正文。评论目标、划词定位、引用关系则一经创建不可修改。只支持 user 身份。

`--content` 是业务必填项：OpenAPI schema 中该字段可能表现为可选，但实际修改评论必须提供非空正文。

## 推荐命令

```bash
# 使用 simple 风格修改评论正文。
lark-cli okr +comment-patch --comment-id 7000000000000000004 --content '{"text":"更新后的评论"}'

# 使用 richtext 文件修改评论正文。
lark-cli okr +comment-patch --comment-id 7000000000000000004 --style richtext --content '@comment.json'

# 写入前预览修改评论的 API 调用，不实际执行。
lark-cli okr +comment-patch --comment-id 7000000000000000004 --content '{"text":"预览更新"}' --dry-run
```

## 参数

| 参数           | 必填 | 默认值  | 说明                                                                                                                          |
|----------------|------|---------|-------------------------------------------------------------------------------------------------------------------------------|
| --comment-id   | 是   | —       | 评论 ID，int64 正整数；可从 [+comment-list](lark-okr-comment-list.md) 或 [+comment-detail](lark-okr-comment-detail.md) 获取。 |
| --content      | 是   | —       | 新正文；输入风格：`simple`（半纯文本 JSON，推荐） \| `richtext`（完整 ContentBlock JSON），支持 @文件路径。                   |
| --style        | 否   | simple  | 输入/输出风格：simple 或 richtext。                                                                                           |
| --user-id-type | 否   | open_id | open_id、union_id、user_id 或 user_key。                                                                                      |
| --dry-run      | 否   | —       | 预览 API 调用而不实际执行。                                                                                                   |
| --format       | 否   | json    | 输出格式。                                                                                                                    |

## 工作流程

1. 使用 [+comment-list](lark-okr-comment-list.md)、[+comment-detail](lark-okr-comment-detail.md) 或 [+comment-get](lark-okr-comment-get.md) 确认 comment-id 和目标评论。
2. 准备 content：通常建议使用 simple 格式，需要精确控制 @用户的位置时，可以使用 richtext 格式，参考 [ContentBlock 格式](lark-okr-contentblock.md)
3. 执行 +comment-patch；真实写入前用 --dry-run 检查请求。
4. 如果要解决或重新打开评论，不要使用 patch，改用 [+comment-solve](lark-okr-comment-solve-reopen.md) 或 [+comment-reopen](lark-okr-comment-solve-reopen.md)。

## 输出

返回 JSON：

```json
{
  "comment": {
    "id": "7000000000000000004",
    "target": {"target_type": "progress", "target_id": "3456789012345678901"},
    "commentator_id": "ou_xxx",
    "status": "open",
    "create_time": "2025-01-15 10:30:00",
    "update_time": "2025-01-15 11:00:00",
    "content": {"text": "更新后的评论", "mention": [], "docs": [], "images": []}
  },
  "style": "simple"
}
```

simple 风格的 content 为 SemiPlainContent；richtext 风格的 content 为 ContentBlock。

## 注意事项

- patch 不会改变评论的 target、selection、ref_comment_id 或 status。
- simple 输入不支持 docs/images；需要富文本元素时使用 richtext。
- 空正文不允许提交；如需删除评论，请使用 [+comment-delete](lark-okr-comment-delete.md)，删除不可恢复。

## 参考

- [lark-okr](../SKILL.md) — OKR 命令、路由和通用约定
- [OKR 实体定义](lark-okr-entities.md) — Comment 字段与评论串规则
- [ContentBlock 格式](lark-okr-contentblock.md) — 评论正文格式
- [okr +comment-get](lark-okr-comment-get.md) — 获取更新前后的评论
- [okr +comment-delete](lark-okr-comment-delete.md) — 永久删除评论
- [lark-shared](../../lark-shared/SKILL.md) — 认证、身份、权限和安全规则
