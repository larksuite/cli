# lingo +get

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过 `entity_id` 获取单条词条的**完整详情**（主词、别名、释义、富文本、关联元数据、外部系统信息）。

## 命令

```bash
# 最常用：按 entity_id 取详情
lark-cli lingo +get --entity-id "enterprise_xxxx"

# 按外部系统（provider + outer_id）查找对应词条
lark-cli lingo +get --entity-id "" --provider "myhr" --outer-id "EMP-001"

# 预览底层请求
lark-cli lingo +get --entity-id "enterprise_xxxx" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--entity-id` | 是 | 词条 ID（通常来自 `+match` / `+search` 返回的 `entity_id`） |
| `--provider` | 否 | 外部系统名；配合 `--outer-id` 使用，用于按外部系统 ID 反查已绑定的词条 |
| `--outer-id` | 否 | 外部系统 ID；必须与 `--provider` 同时使用 |

## 关键约束

- 拿不到 `entity_id` 时，**先用 [`+match`](lark-lingo-match.md) 或 [`+search`](lark-lingo-search.md) 查到 id 再调**。不要编造 id。
- `--provider` / `--outer-id` 是成对使用的，配合 `outer_info` 绑定场景（外部数据源和词条做关联）；只填其一会被服务端拒绝。

## 返回值

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "entity": {
      "id": "enterprise_xxxx",
      "main_keys": [{"key": "KYC"}],
      "aliases": [{"key": "Know Your Customer"}],
      "description": "了解你的交易 …",
      "rich_text": "<p>…</p>",
      "related_meta": {},
      "outer_info": {}
    }
  }
}
```

## 权限

| 身份 | 所需 scope |
|------|-----------|
| user / bot | `baike:entity:readonly` |

## 参考

- [lark-lingo](../SKILL.md) — Lingo 域总览
- [lark-lingo +match](lark-lingo-match.md) — 精准匹配得到 entity_id
- [lark-lingo +update](lark-lingo-update.md) — 改词条前先 `+get` 取当前值
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
