# lingo +match

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

**精准匹配**一个词与词典中的主词 / 别名。用于判断"这个词是否已收录"，是创建词条前的幂等检查首选。

## 命令

```bash
# 判断是否已收录
lark-cli lingo +match --word "KYC"

# 预览底层请求
lark-cli lingo +match --word "KYC" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--word` | 是 | 要精确匹配的词面（主词或别名） |

## 关键约束

- 是**完全等值匹配**：`KYC` 不会匹配上 `KYCM` 或 `KYC-V2`。要找候选请用 [`+search`](lark-lingo-search.md)。
- 命中可能有**多条**（同一个词面对应多个不同的 `entity_id`，如多个团队各自收录的版本）。

## 返回值

命中：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "results": [
      {"entity_id": "enterprise_xxxx", "type": 0}
    ]
  }
}
```

未命中：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "results": []
  }
}
```

## 典型用法：幂等收录

```bash
# Step 1: match 判断
lark-cli lingo +match --word "KYC"

# Step 2a: 命中 → 拿 entity_id 取详情
lark-cli lingo +get --entity-id "<entity_id>"

# Step 2b: 未命中 → 跟用户确认释义后创建
lark-cli lingo +create --as bot --main-key "KYC" --description "..."
```

## 权限

| 身份 | 所需 scope |
|------|-----------|
| user / bot | `baike:entity:readonly` |

## 参考

- [lark-lingo](../SKILL.md) — Lingo 域总览
- [lark-lingo +search](lark-lingo-search.md) — 模糊搜索
- [lark-lingo +get](lark-lingo-get.md) — 获取详情
- [lark-lingo +create](lark-lingo-create.md) — 创建词条
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
