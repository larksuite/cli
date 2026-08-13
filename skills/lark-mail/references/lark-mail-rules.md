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

# 3. 删除规则
lark-cli mail user_mailbox.rules delete --as user \
  --params '{"user_mailbox_id":"me","rule_id":"<rule_id>"}'
```

Quick codes above: condition `type=6` = subject, `operator=1` = contains, action `type=3` = mark as read.

## 调整规则顺序

优先使用快捷命令。只需要传希望置前的规则 ID，CLI 会先读取当前全部规则，按“输入 ID 去重置前 + 未输入 ID 保持当前相对顺序追加”补齐完整列表，再调用规则重排序接口。

```bash
lark-cli mail +rule-reorder --as user \
  --rule-ids "<rule_id_to_move_first>" \
  --rule-ids "<rule_id_to_move_second>"
```

动态 API 命令同样支持部分 `rule_ids` 输入；CLI 会在提交前自动补齐：

```bash
lark-cli mail user_mailbox.rules reorder --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"rule_ids":["<rule_id_to_move_first>","<rule_id_to_move_second>"]}'
```

## 原生 API

收信规则走 `user_mailbox.rules` 资源。参数不确定时先运行：

```bash
lark-cli mail user_mailbox.rules -h
lark-cli schema mail.user_mailbox.rules.<method>
```
