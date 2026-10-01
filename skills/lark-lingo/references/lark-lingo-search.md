# lingo +search

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

在词典中**模糊搜索**词条。返回按相关度排序的候选列表。

## 命令

```bash
# 基本模糊搜索
lark-cli lingo +search --query "AML"

# 调大分页
lark-cli lingo +search --query "飞书" --page-size 50

# 指定私有词典库搜索
lark-cli lingo +search --query "KYC" --repo-id "<repo_id>"

# 获取下一页
lark-cli lingo +search --query "AML" --page-token "<next_page_token>"

# 预览底层请求
lark-cli lingo +search --query "AML" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--query` | 是 | 搜索关键词（支持部分匹配） |
| `--repo-id` | 否 | 词典库 ID；省略时搜索全公司共享词典 |
| `--page-size` | 否 | 每页条数，取值 1–100，默认 20 |
| `--page-token` | 否 | 上一页返回的 `page_token`，用于翻页 |

## 关键约束

- 是**模糊搜索**，不要求关键词完全等于主词或别名；如果要"完全相等判断"，改用 [`+match`](lark-lingo-match.md)。
- 没命中时 `data.entities = []`，**不要编造**词条，直接告诉用户"词典里没收录"。

## 返回值

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "entities": [
      {
        "id": "enterprise_xxxx",
        "main_keys": [{"key": "AML"}],
        "description": "反洗钱 Anti-Money Laundering …"
      }
    ],
    "page_token": "...",
    "has_more": false
  }
}
```

## 权限

| 身份 | 所需 scope |
|------|-----------|
| user / bot | `baike:entity:readonly` |

## 参考

- [lark-lingo](../SKILL.md) — Lingo 域总览
- [lark-lingo +match](lark-lingo-match.md) — 精准匹配
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
