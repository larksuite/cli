# mail +rule-reorder

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

重新排列收信规则的执行优先级。后端 `ReorderUserMailboxRule` 接口要求传入全量规则 ID，本命令支持只传入需要调整位置的规则 ID，自动补齐其余 ID 后调用后端。

本 skill 对应 shortcut：`lark-cli mail +rule-reorder`。

## 使用时机

当用户需要调整收信规则的执行顺序时使用。后端要求传入所有规则 ID 的完整排序列表，但用户通常只想调整部分规则的相对位置——本命令自动补齐缺失的 ID。

## 命令

```bash
# 标准用法：将规则 E 和 A 调整到指定位置
lark-cli mail +rule-reorder --rule-ids E,A

# 指定邮箱（公共邮箱场景）
lark-cli mail +rule-reorder --rule-ids E,A --mailbox shared@example.com

# Dry Run（预览补齐后的完整列表，不实际排序）
lark-cli mail +rule-reorder --rule-ids E,A --dry-run

# 传入全量 ID（跳过自动补齐）
lark-cli mail +rule-reorder --rule-ids D,A,G,C,B,E
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `--rule-ids <ids>` | 是 | — | 逗号分隔的规则 ID 列表，按期望优先级从高到低排列。支持部分列表，缺失的 ID 将自动补齐 |
| `--mailbox <email>` | 否 | `me` | 邮件归属的邮箱 |
| `--dry-run` | 否 | — | 仅展示补齐前后的规则 ID 列表对比，不实际调用排序接口 |

## 行为细节

1. 调用 `ListUserMailboxRule` 获取当前全量规则列表（按执行优先级排序）
2. 校验用户传入的每个规则 ID 都存在于当前规则列表中，不存在则报错
3. 若用户传入了全量 ID，直接使用
4. 否则执行补齐算法：
   - 用户指定的 ID 视为"锚点"，按用户输入顺序排列
   - 未提及的 ID 按当前相对顺序填入锚点间隙
   - 每个 ID 归属哪个间隙，由它在当前排序中被哪两个锚点夹住决定
5. 调用 `ReorderUserMailboxRule` 传入补齐后的完整 ID 列表

## 补齐算法示例

**当前顺序**：D, A, G, C, B, E

**用户输入**：E, A（希望 E 在 A 前面）

**补齐过程**：
- 锚点：E(位置5), A(位置1)
- 未提及 ID：D(0), G(2), C(3), B(4)
- D 在所有锚点之前 → 归入 E 之前的间隙
- G/C/B 在 A 和 E 之间 → 归入 E~A 间隙
- **结果**：D, E, G, C, B, A

## 返回值

排序成功：

```json
{
  "ok": true,
  "data": {
    "rule_ids": ["D", "E", "G", "C", "B", "A"]
  },
  "meta": {
    "count": 6
  }
}
```

## 典型场景

### 场景 1：调整两条规则的相对顺序

```bash
# 将规则 E 移到规则 A 前面
lark-cli mail +rule-reorder --rule-ids E,A
```

### 场景 2：先预览再执行

```bash
# 预览补齐结果
lark-cli mail +rule-reorder --rule-ids E,A --dry-run

# 确认后执行
lark-cli mail +rule-reorder --rule-ids E,A
```

### 场景 3：传入全量 ID（跳过自动补齐）

```bash
lark-cli mail +rule-reorder --rule-ids D,E,G,C,B,A
```

## 错误处理

| 错误 | 原因 | 解决方式 |
|------|------|---------|
| `--rule-ids is required` | 未传入 --rule-ids | 传入至少一个规则 ID |
| `--rule-ids contains empty ID` | 逗号分隔中有空值 | 检查是否有连续逗号或末尾逗号 |
| `duplicate rule ID: X` | 传入了重复的规则 ID | 去除重复 ID |
| `rule ID X not found in current rules` | 传入的 ID 在当前规则列表中不存在 | 确认 ID 是否正确，可通过 `user_mailbox.rules list` 查看 |
| `list rules failed` | 获取当前规则列表失败 | 检查网络和权限（需要 `mail:user_mailbox.rule:read` scope） |
| `reorder rules failed` | 排序 API 调用失败 | 检查权限（需要 `mail:user_mailbox.rule:write` scope） |

## 不要这样做

- ❌ 手动拼接全量 ID 列表——容易遗漏或顺序错误，使用本命令的自动补齐功能
- ❌ 在 bot 身份下使用 `--mailbox me`——bot 使用 tenant token 无法解析 "me"，需传入显式邮箱地址
- ❌ 传入不存在的规则 ID——会直接报错，不会静默跳过

## 相关命令

- `lark-cli mail user_mailbox.rules list` — 列出当前所有收信规则及其 ID
- `lark-cli mail user_mailbox.rules create` — 创建新的收信规则
- `lark-cli mail user_mailbox.rules update` — 更新收信规则的条件或动作
- `lark-cli mail user_mailbox.rules delete` — 删除收信规则
