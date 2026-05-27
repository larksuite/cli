# mail +rule-reorder

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

对收件箱规则重新排序。用户只需传入**部分**规则 ID（期望的相对顺序），其余未指定的规则由 CLI 自动从服务端当前顺序中补全，使用 **slot-replacement 算法**生成完整列表后调用 Reorder API。

本 skill 对应 shortcut：`lark-cli mail +rule-reorder`。

## 使用时机

- 用户想把某几条规则的优先级调高/调低，但不想手动列出所有规则 ID
- 用户只知道"让规则 E 排在规则 A 前面"，而不关心其他规则的顺序
- 需要一次性重排全部规则时，也可传入完整 ID 列表（直接替换）

## 命令

```bash
# 将规则 E 提前到规则 A 之前（只需传入希望调整的规则 ID，其余自动补全）
lark-cli mail +rule-reorder --rule-ids "E,A"

# 指定邮箱
lark-cli mail +rule-reorder --mailbox shared@example.com --rule-ids "E,A"

# 传入全量 ID 列表（全量重排）
lark-cli mail +rule-reorder --rule-ids "D,E,G,C,B,A"

# Dry Run（不真改，仅显示请求意图）
lark-cli mail +rule-reorder --rule-ids "E,A" --dry-run
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `--rule-ids <ids>` | 是 | — | 按目标顺序排列的规则 ID，逗号分隔，禁止重复。至少填 1 个。未传入的规则 ID 由 slot-replacement 算法自动补全 |
| `--mailbox <email>` | 否 | `me` | 操作哪个邮箱；`me` 代表当前 OAuth token 对应的邮箱 |
| `--dry-run` | 否 | — | 仅打印请求意图，不执行写操作 |

## 行为细节

### slot-replacement 算法

1. 调用 `GET /open-apis/mail/v1/user_mailboxes/{mailbox}/rules` 获取当前全量规则顺序 `currentIDs`
2. 找出 `--rule-ids` 中每个 ID 在 `currentIDs` 中的槽位（下标），按升序收集为 `slots`
3. 将 `currentIDs` 复制为 `result`，依次把用户期望顺序中的第 j 个 ID 填入 `slots[j]` 位置
4. 非指定 ID 原位不动
5. 用合并后的完整列表调用 `POST /open-apis/mail/v1/user_mailboxes/{mailbox}/rules/reorder`

**示例**：当前顺序 `[D, A, G, C, B, E]`，用户传 `--rule-ids "E,A"`：

- A 在 index=1，E 在 index=5 → slots=[1, 5]
- result[1]=E，result[5]=A → 最终：`[D, E, G, C, B, A]`

### 前置校验（Validate 阶段，无 API 调用）

- `--rule-ids` 非空且解析后长度 ≥ 1
- `--rule-ids` 中无重复 ID

### 执行阶段校验

- 所有传入的 rule ID 必须存在于当前邮箱规则列表中（不存在时返回 validation 错误）

### 写操作安全

- POST Reorder 为写操作，**不自动重试**
- List 与 Reorder 之间存在极小竞态窗口；若 Reorder 失败，CLI 提示用户重新执行完整命令

## 返回值

```json
{
  "ok": true,
  "meta": { "count": 6 },
  "data": {
    "rule_ids": ["D", "E", "G", "C", "B", "A"]
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.rule_ids` | `[]string` | 最终写入服务端的完整规则顺序（合并后） |
| `meta.count` | int | 规则总数 |

## 典型场景

### 场景 1：将高优先级规则提前

```bash
# 当前顺序：[D, A, G, C, B, E]
# 目标：让 E 排到 A 前面

lark-cli mail +rule-reorder --rule-ids "E,A"
# CLI 内部：slot-replacement → [D, E, G, C, B, A]
# 输出：reordered 6 rules. new order: [D, E, G, C, B, A]
```

### 场景 2：先查看当前规则顺序

```bash
# 查当前规则列表（获取 ID）
lark-cli api GET '/open-apis/mail/v1/user_mailboxes/me/rules' --as user | jq '.data.items[].id'

# 按需调整
lark-cli mail +rule-reorder --rule-ids "rule-id-3,rule-id-1"
```

### 场景 3：Dry Run 确认意图

```bash
lark-cli mail +rule-reorder --rule-ids "E,A" --dry-run
# 不执行写操作，仅打印 GET 和 POST 请求意图
```

## 不要这样做

- 不要传入邮箱中不存在的规则 ID — CLI 会在 Execute 阶段报 validation 错误
- 不要传入重复的规则 ID — Validate 阶段即报错
- 不要省略 `--rule-ids` — 该参数为必填，缺少时返回 validation 错误
- 不要假设返回的 `rule_ids` 顺序就是"最终服务端顺序" — 极小概率存在 List-Reorder 竟态窗口（建议重跑确认）

## 相关命令

- `lark-cli api GET '/open-apis/mail/v1/user_mailboxes/me/rules' --as user` — 查看当前规则列表及优先级顺序
- `lark-cli mail +rule-reorder --dry-run` — 预览操作意图，不实际执行
