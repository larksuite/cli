# mail +reorder-rules

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

安全调整收信规则顺序。原生 `mail user_mailbox.rules reorder` 要求 `rule_ids` 是当前全部规则 ID 的完整有序排列；本 shortcut 会先列出当前规则，按你给的子序列补齐其余规则，再提交完整顺序。

**依赖 Scope：** `mail:user_mailbox.rule:write`、`mail:user_mailbox.rule:read`

登录时申请：

```bash
lark-cli auth login --scope "mail:user_mailbox.rule:write mail:user_mailbox.rule:read"
```

业务命令不接受 `--scope`，不要把 scope 传给 `mail +reorder-rules`。

## 命令

```bash
# 将 C、A 排到最前，其余规则保留原相对顺序
lark-cli mail +reorder-rules --rule-ids C,A

# 将 D、B 排到最后，其余规则保留原相对顺序
lark-cli mail +reorder-rules --rule-ids D,B --append

# 指定邮箱
lark-cli mail +reorder-rules --mailbox user@example.com --rule-ids C,A

# 预览补齐后的完整顺序，不提交 reorder
lark-cli mail +reorder-rules --rule-ids C,A --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--rule-ids <ids>` | 是 | 逗号分隔的规则 ID，按目标相对顺序填写。可以只填要移动的子序列 |
| `--append` | 否 | 默认把 `--rule-ids` 放到最前；传入后把它们放到最后 |
| `--mailbox <email>` | 否 | 邮箱地址或 `me`（默认 `me`） |
| `--dry-run` | 否 | 先 list 并计算 `after`，但不提交 reorder |

## 补齐规则

当前顺序为 `[A,B,C,D]`：

- `--rule-ids C,A` → `after=[C,A,B,D]`
- `--rule-ids D,B --append` → `after=[A,C,D,B]`
- `--rule-ids C,A,B,D` → `after=[C,A,B,D]`，全量输入也支持

## 返回值

成功提交时输出标准 envelope：

```json
{
  "ok": true,
  "data": {
    "reordered": true,
    "before": ["A", "B", "C", "D"],
    "after": ["C", "A", "B", "D"],
    "moved": [
      {"id": "C", "from": 2, "to": 0},
      {"id": "A", "from": 0, "to": 1},
      {"id": "B", "from": 1, "to": 2}
    ],
    "rule_name_map": {
      "A": "Invoices",
      "C": "VIP"
    }
  }
}
```

`--dry-run` 不提交 reorder，输出预览对象本身，不包裹在 `ok/data` 内：

```json
{
  "dry_run": true,
  "before": ["A", "B", "C", "D"],
  "after": ["C", "A", "B", "D"],
  "moved": [
    {"id": "C", "from": 2, "to": 0},
    {"id": "A", "from": 0, "to": 1},
    {"id": "B", "from": 1, "to": 2}
  ],
  "rule_name_map": {
    "A": "Invoices",
    "C": "VIP"
  }
}
```

没有任何收信规则且未提供有效规则 ID 时返回 no-op success，不会调用 reorder；如果提供了 `--rule-ids`，会返回校验错误。

## 常见错误

| 症状 | 原因 | 解决 |
|------|------|------|
| `--rule-ids is required` | 未提供有效规则 ID，或只传了空逗号 | 传入至少一个规则 ID，如 `--rule-ids A` |
| `duplicate rule id` | `--rule-ids` 内有重复 ID | 去重后重试 |
| `rule id "... " not found; valid rule ids: ...` | 传入了当前规则集中不存在的 ID | 使用错误提示里的合法 ID 重试 |
| `mailbox has no mail rules` | 当前邮箱没有任何收信规则，但传入了要重排的 ID | 先创建规则，或确认邮箱是否正确 |
| `rule set may have changed` | list 与 reorder 之间规则集被并发增删，后端拒绝旧全集 | 重新执行命令，让 shortcut 基于最新规则集补齐 |
| 403 / missing scope | 登录 token 没有规则 read/write scope | 重新执行 `lark-cli auth login --scope "mail:user_mailbox.rule:write mail:user_mailbox.rule:read"` |

## 相关命令

- `lark-cli mail user_mailbox.rules list` — 查看现有规则
- `lark-cli mail user_mailbox.rules create` — 创建规则
- `lark-cli mail user_mailbox.rules update` — 更新规则
