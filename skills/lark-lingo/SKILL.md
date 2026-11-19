---
name: lark-lingo
version: 1.0.0
description: "飞书词典（Lingo / 百科）：查询和搜索企业词典词条。当用户需要查询内部术语/缩写/黑话释义、精准匹配词条、按 entity_id 或外部系统 ID 获取词条详情、或模糊搜索词典时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli lingo --help"
---

# lark-lingo — 飞书词典（Lingo / 百科）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) 了解认证、全局参数与权限处理。

## 1. 快速路由

| 用户意图 | 命令 |
| :--- | :--- |
| 查询特定术语/缩写完整释义（已知词名或别名） | `lark-cli lingo +get --word "<词条名>"` |
| 按 `entity_id` 获取词条详情 | `lark-cli lingo +get --entity-id "<entity_id>"` |
| 按外部系统 ID 反查词条详情 | `lark-cli lingo +get --provider "<provider>" --outer-id "<outer_id>"` |
| 关键词模糊搜索候选词条 | `lark-cli lingo +search --query "<关键词>"` |

- **查解释优先用 `+get --word`**：它会自动执行精准匹配（`entities/match`）并拉取命中词条的完整详情（`entities/{entity_id}`），一步返回主词、别名、释义和关联文档/群/联系人（`related_meta`）。
- 如果 `+get --word` 未命中，再用 `lark-cli lingo +search --query "<关键词>"` 做模糊召回；若仍无结果，如实告知用户词典未收录，不要编造释义。

## 2. 命令用法与示例

```bash
# 按词名或别名一步获取完整释义
lark-cli lingo +get --word "PSM"

# 指定私有词库按词名获取详情
lark-cli lingo +get --word "PSM" --repo-id "7152790921053274113"

# 按词条 ID 获取详情
lark-cli lingo +get --entity-id "enterprise_40217521"

# 按外部系统标识获取详情
lark-cli lingo +get --provider "bytetree" --outer-id "node_123"

# 模糊搜索词条（默认 page-size=20，范围 1-100）
lark-cli lingo +search --query "服务树" --page-size 20

# 翻页查询
lark-cli lingo +search --query "服务树" --page-token "<page_token>"
```

## 3. 权限与授权

- `+get` 与 `+search` 均为只读命令（`risk: read`），默认使用应用身份（`--as bot`，即 `tenant_access_token`），所需权限为 `baike:entity:readonly`。显式 `--as user` 会被接受，但该权限不能授予用户身份，因此 `--as user` 会因缺权限失败。
- 飞书开放平台中 `baike:entity*` 权限仅作为**应用身份权限（Tenant token scopes）**提供，因此不带 `--as` 时默认以 `bot` 身份调用，无需对 `user` 执行 `auth login`。
- 若报缺权限（`99991672`）：请前往飞书开放平台开发者后台，在「权限管理」中为应用开通 `baike:entity:readonly`（应用身份权限），并在「版本管理与发布」中创建版本并申请发布生效。
