# 收信规则 Shortcut

管理自动处理收到邮件的规则。优先使用 `mail +rule-*` shortcut，通过稳定英文 alias 编写条件和动作；只有需要当前 shortcut 尚未建模的服务端字段时，才回退到 `mail user_mailbox.rules` 原子 raw 命令。规则写操作需使用真实 `rule_id`，不要猜测 ID。创建、更新、删除规则需要按 SKILL.md 的高风险写规则获得用户确认并传 `--yes`；启停和排序是普通写操作，免 `--yes`。

## 常用 shortcut

```bash
# 列出规则，输出 semantic_spec、description、unknowns
lark-cli mail +rule-list --as user --user-mailbox-id me --format json

# 查看单条规则
lark-cli mail +rule-get --as user --user-mailbox-id me --rule-id "<rule_id>"

# dry-run 创建：主题包含 Alpha 时标为已读，不产生服务端副作用
lark-cli mail +rule-create --as user --dry-run \
  --name "Alpha通知已读" \
  --condition "subject:contains:Alpha" \
  --action "mark_read"

# 创建同一规则
lark-cli mail +rule-create --as user \
  --name "Alpha通知已读" \
  --condition "subject:contains:Alpha" \
  --action "mark_read" \
  --yes

# 更新规则：未传字段会先读当前规则并保留；传 --condition/--action 会替换对应完整集合
lark-cli mail +rule-update --as user \
  --rule-id "<rule_id>" \
  --name "Alpha通知归档" \
  --action "archive" \
  --yes

# 启停规则
lark-cli mail +rule-disable --as user --rule-id "<rule_id>"
lark-cli mail +rule-enable --as user --rule-id "<rule_id>"

# 删除规则：真实删除必须显式 --yes；不确定时先 --dry-run
lark-cli mail +rule-delete --as user --rule-id "<rule_id>" --dry-run
lark-cli mail +rule-delete --as user --rule-id "<rule_id>" --yes

# 调整顺序：完整顺序或单条移动二选一
lark-cli mail +rule-reorder --as user --rule-ids "<rule_id_1>,<rule_id_2>,<rule_id_3>"
lark-cli mail +rule-reorder --as user --move-rule-id "<rule_id_3>" --before-rule-id "<rule_id_1>"
```

## Alias 速查

条件 grammar:

```text
--condition field:op:value
--condition field:op
--condition field
```

常用字段：`from`/`sender`、`to`/`recipient`、`cc`、`to_or_cc`、`subject`/`title`、`body`、`attachment_name`、`attachment_type`、`any_address`、`all_mail`/`all`、`external`、`spam`、`not_spam`、`has_attachment`。

常用操作符：`contains`/`include`、`not_contains`/`exclude`、`starts_with`/`prefix`、`ends_with`/`suffix`、`equals`/`eq`/`is`、`not_equals`/`ne`、`contains_self`/`self`、`empty`/`is_empty`。

动作 grammar:

```text
--action kind
--action kind:key=value
--action kind:json={"key":"value"}
```

常用动作：`archive`、`delete_mail`/`trash`、`mark_read`/`read`、`move_spam`/`spam`、`not_spam`/`never_spam`、`star`/`flag`、`mute_notification`/`mute`、`move_folder:folder_id=<id>`。

`--conditions` / `--actions` 支持 JSON 或 `@file`。JSON 示例：

```json
[
  {"field":"subject","operator":"contains","value":"Alpha"},
  {"field":"has_attachment"}
]
```

## Unknown raw 策略

- 读路径宽容：`+rule-list` / `+rule-get` 遇到未知枚举或扩展字段仍输出规则，`unknowns[]` 会说明无法识别的 raw 片段，`raw` 会保留原始规则。
- 更新规则：`+rule-update` 是“传什么改什么”。只改名称、启停、match 或 stop-after-match 时保留未触碰的 raw；传入新的 `--condition(s)` 时替换 condition items，未传 `--match` 就保留当前 match_type；传入新的 `--action(s)` 时替换 action items。
- 输入校验：用户输入 alias/语义字符串时必须能映射到当前 shortcut 支持的枚举，否则报错；用户直接输入当前 shortcut 不认识的枚举数字，也报错。
- raw fallback：需要写入当前 shortcut 尚未建模的服务端字段时，读取 `raw` 后使用原子 `user_mailbox.rules` 命令。

## 原子 raw fallback：主题包含文本 → 标记为已读

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
  --params '{"user_mailbox_id":"me","rule_id":"<rule_id>"}' \
  --yes
```

Quick codes above: condition `type=6` = subject, `operator=1` = contains, action `type=3` = mark as read.

## 原生 API

收信规则走 `user_mailbox.rules` 资源。参数不确定时先运行：

```bash
lark-cli mail user_mailbox.rules -h
lark-cli schema mail.user_mailbox.rules.<method>
```
