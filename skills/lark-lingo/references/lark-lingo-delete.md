# lingo +delete

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过 `entity_id` 删除词条。**必须 `--as bot`**。**不可逆**。

## 命令

```bash
# 删除（必须同时带 --yes 才能实际执行）
lark-cli lingo +delete --as bot \
  --entity-id "enterprise_xxxx" \
  --yes

# 预览底层请求（不执行，可省 --yes）
lark-cli lingo +delete --as bot \
  --entity-id "enterprise_xxxx" \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--entity-id` | 是 | 要删除的词条 ID |
| `--yes` | 是（执行时） | 确认高危写入；不传会被框架拦下 |

## 关键约束

- ⚠️ **不可逆**：删除后无法恢复。调用前**必须**用 [`+get`](lark-lingo-get.md) 二次确认 `entity_id` 对应的词条名称和释义是否真的要删。
- **必须 `--as bot`**：user token 会返 `99991668`。
- **框架级高危确认**：本 shortcut 的 risk = `high-risk-write`；不加 `--yes` 时框架会直接拒绝执行，只有 `--dry-run` 能跳过。
- **不支持批量**：一次只能删一条。批量场景自己在外层循环 + 节流。

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "deleted": true,
    "entity_id": "enterprise_xxxx"
  }
}
```

## 安全流程

```bash
# Step 1: 用 +get 确认目标词条是你想删的那条
lark-cli lingo +get --entity-id "enterprise_xxxx"

# Step 2: 向用户复述主词 + 释义片段，得到明确确认

# Step 3: 执行
lark-cli lingo +delete --as bot --entity-id "enterprise_xxxx" --yes
```

## 权限

| 身份 | 所需 scope |
|------|-----------|
| bot | `baike:entity` |

## 参考

- [lark-lingo](../SKILL.md) — Lingo 域总览
- [lark-lingo +get](lark-lingo-get.md) — 删前必做：确认目标
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
