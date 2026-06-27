# 收信规则重排序

当后端 `user_mailbox.rules reorder` 要求传入**全部**规则 ID、但调用方只知道一部分规则 ID 时，优先使用 `+rule-reorder`。该 Shortcut 会先读取当前规则顺序，再把未显式传入的规则按原相对顺序补齐后发起重排。

```bash
lark-cli mail +rule-reorder --as user \
  --mailbox me \
  --rule-ids 345,123
```

上面的调用会先读取当前规则列表；如果当前顺序是 `[123,234,345,456]`，最终实际提交的 `rule_ids` 会是 `[345,123,234,456]`。

## Dry run

```bash
lark-cli mail +rule-reorder --as user \
  --mailbox me \
  --rule-ids 345,123 \
  --dry-run
```

`--dry-run` 只展示两步计划：

1. `GET /user_mailboxes/:id/rules`
2. `POST /user_mailboxes/:id/rules/reorder`

并在输出里返回：

- `specified_rule_ids`
- `before`
- `after`
- `completed_rule_ids`

## 参数约束

- `--rule-ids` 必填
- 支持逗号或空白分隔：`1,2,3`、`1 2 3`
- 只接受数字 ID
- 不允许重复 ID
- 如果输入的某个规则 ID 不在当前邮箱规则列表中，命令会直接返回校验错误，不会调用 reorder

## 何时不要用

- 如果你已经有完整且确认无误的全量 `rule_ids`，也可以直接调用原生 `mail user_mailbox.rules reorder`
- 如果你需要创建、删除或更新规则，仍然使用 `user_mailbox.rules create|delete|update`
