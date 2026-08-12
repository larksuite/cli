# 收信规则

管理自动处理收到邮件的规则。规则写操作需使用真实 `rule_id`，不要猜测 ID。规则写操作执行前需按 SKILL.md 的写操作确认规则获得用户确认。

## 主题包含文本 → 标记为已读

```bash
# 1. 创建规则：主题包含指定文本时标记为已读
lark-cli mail user_mailbox.rules create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"name":"<rule_name>","is_enable":true,"ignore_the_rest_of_rules":false,"condition":{"match_type":1,"items":[{"type":6,"operator":1,"input":"<subject_text>"}]},"action":{"items":[{"type":3}]}}'

# 2. 验证规则
lark-cli mail user_mailbox.rules list --as user \
  --params '{"user_mailbox_id":"me"}'

# 3. 将指定规则排到前面。可以只输入部分 rule_id；CLI 会自动补齐剩余规则。
lark-cli mail user_mailbox.rules reorder --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"rule_ids":["<rule_id_to_move_first>"]}'

# 4. 删除规则
lark-cli mail user_mailbox.rules delete --as user \
  --params '{"user_mailbox_id":"me","rule_id":"<rule_id>"}'
```

Quick codes above: condition `type=6` = subject, `operator=1` = contains, action `type=3` = mark as read.

## 排序规则

`rules.reorder` 可以输入部分合法 `rule_id`。CLI 会先读取当前全部收信规则 ID，再把输入的 ID 按给定顺序排到前面，未输入规则保持当前列表中的原相对顺序并自动追加后再提交。输入重复 ID 或未知 ID 时，CLI 会返回参数错误且不会调用 reorder。

## 原生 API

收信规则走 `user_mailbox.rules` 资源。参数不确定时先运行：

```bash
lark-cli mail user_mailbox.rules -h
lark-cli schema mail.user_mailbox.rules.<method>
```
