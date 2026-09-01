# okr +comment-delete
> **前置条件：** 先阅读 [lark-shared/SKILL.md](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则；

永久删除一条评论。删除划词评论时只删除指定评论，不会删除同一 selection.id 下的其他评论。

## 功能简介

删除一条特定评论。本 shortcut 为高风险接口，删除的评论不可找回，如果只是暂时结束讨论，可使用 +comment-solve。只支持 user 身份。

## 推荐命令
```bash
# 预览删除请求，不实际执行永久删除。
lark-cli okr +comment-delete --comment-id 7000000000000000004 --dry-run
# 确认删除目标后，执行不可恢复的删除操作。
lark-cli okr +comment-delete --comment-id 7000000000000000004 --yes
```


## 参数

| 参数         | 必填         | 默认值 | 说明                                                        |
|--------------|--------------|--------|-------------------------------------------------------------|
| --comment-id | 是           | —      | 要删除的评论 ID，int64 正整数。建议先由 +comment-get 核对。 |
| --yes        | 真实执行时是 | —      | 确认 high-risk-write 操作。--dry-run 时不需要。             |
| --dry-run    | 否           | —      | 预览 API 调用而不实际执行。                                 |
| --format     | 否           | json   | 输出格式。                                                  |

## 工作流程

1. 使用 [+comment-list](lark-okr-comment-list.md)、[+comment-detail](lark-okr-comment-detail.md) 或 [+comment-get](lark-okr-comment-get.md) 定位并确认 comment-id。
2. 判断是否真的需要删除：解决评论使用 [+comment-solve](lark-okr-comment-solve-reopen.md)，删除只用于永久移除内容。
3. 先执行带 --dry-run 的命令检查 URL 和 comment-id。
4. 向用户明确说明删除不可恢复；得到确认后，在原始命令末尾追加 --yes 执行。
5. 根据 deleted=true 和返回的 comment_id 确认结果。

## 输出

删除成功返回 JSON：
```json
{
  "deleted": true,
  "comment_id": "7000000000000000004"
}
```


## 注意事项

- 删除是单条评论级操作，即使评论属于划词评论串，也不会连带删除其他评论。
- 删除后不能使用 +comment-reopen 恢复；暂时关闭讨论应使用 +comment-solve。
- 该命令不需要 style，因为接口没有返回 Comment 正文。

## 参考

- [lark-okr](../SKILL.md) — OKR 命令、路由和通用约定
- [OKR 实体定义](lark-okr-entities.md) — Comment、评论串和状态规则
- [okr +comment-get](lark-okr-comment-get.md) — 删除前核对评论
- [okr +comment-solve / +comment-reopen](lark-okr-comment-solve-reopen.md) — 暂时解决和恢复评论
- [lark-shared](../../lark-shared/SKILL.md) — 高风险操作确认协议
