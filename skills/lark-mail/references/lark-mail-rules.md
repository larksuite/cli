# 收信规则

管理自动处理收到邮件的规则。规则写操作需使用真实 `rule_id`，不要猜测 ID。规则写操作执行前需按 SKILL.md 的写操作确认规则获得用户确认。

## 重排序

`user_mailbox.rules reorder` 可以只传需要优先排序的一部分规则 ID。CLI 会先用同一个 `user_mailbox_id` / `--as` 身份上下文调用 `user_mailbox.rules list` 读取当前全部规则，再按“用户输入顺序优先 + 未输入规则保持当前相对顺序”补齐完整 `rule_ids` 后调用 reorder。

```bash
lark-cli mail user_mailbox.rules reorder --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"rule_ids":["<rule_id_to_move_first>","<rule_id_to_move_second>"]}'
```

空 `rule_ids`、重复 ID、未知 ID 会在 CLI 侧报 validation error，且不会调用 reorder。list 失败时也不会调用 reorder；reorder 失败时透传 API error。

## 主题包含文本 → 标记为已读

```bash
# 1. 创建规则：主题包含指定文本时标记为已读
lark-cli mail user_mailbox.rules create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"name":"<rule_name>","is_enable":true,"ignore_the_rest_of_rules":false,"condition":{"match_type":1,"items":[{"type":6,"operator":1,"input":"<subject_text>"}]},"action":{"items":[{"type":3}]}}'

# 2. 验证规则
lark-cli mail user_mailbox.rules list --as user \
  --params '{"user_mailbox_id":"me"}'

# 3. 删除规则
lark-cli mail user_mailbox.rules delete --as user \
  --params '{"user_mailbox_id":"me","rule_id":"<rule_id>"}'
```

Quick codes above: condition `type=6` = subject, `operator=1` = contains, action `type=3` = mark as read.

## 原生 API

收信规则走 `user_mailbox.rules` 资源。参数不确定时先运行：

```bash
lark-cli mail user_mailbox.rules -h
lark-cli schema mail.user_mailbox.rules.<method>
```
