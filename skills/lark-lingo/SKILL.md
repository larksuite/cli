---
name: lark-lingo
version: 1.0.0
description: "飞书词典（Lingo / 百科）：管理企业词条。支持模糊搜索、精准匹配、获取详情、创建、修改、删除词条。当用户需要查询飞书词典、查词条、新建词条、维护企业术语/缩写/黑话词库时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli lingo --help"
---

# lingo (v1) — 飞书词典

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理。**

## 快速决策

- 用户问**"X 是什么意思"** → 先 [`+match`](references/lark-lingo-match.md)（精准匹配），未命中再 [`+search`](references/lark-lingo-search.md)（模糊搜索）；都没结果就直接告诉用户"词典里没收录"，**不要编造解释**。
- 用户要**判断"这个词是否已收录"** → [`+match`](references/lark-lingo-match.md)
- 用户要**列出候选 / 关键词召回** → [`+search`](references/lark-lingo-search.md)
- 用户要**看某条词条的完整释义** → [`+get`](references/lark-lingo-get.md)
- 用户要**新增企业术语 / 黑话** → [`+create`](references/lark-lingo-create.md)（必须 `--as bot`）
- 用户要**改词条释义** → [`+update`](references/lark-lingo-update.md)（PUT 整体覆盖；先 `+get` 取当前值再合并；必须 `--as bot`）
- 用户要**删词条** → [`+delete`](references/lark-lingo-delete.md)（不可逆；需 `--yes`；必须 `--as bot`）

## 核心边界

- **写操作（create/update/delete）只接受 tenant_access_token**：API 端点拒收 user token，会返 `99991668 "user access token not support"`。所以这三个 shortcut **必须 `--as bot`**，没有 user 路径可走。读操作（search/match/get）user/bot 都行。
- **bot scope 不在 `auth login` 申请**：bot 走 tenant_access_token，scope 是在飞书开发者后台 app 配置页 → 权限管理勾选 → 创建版本 → 管理员审批。`lark-cli auth login --scope` 申请的是 user scope，对 bot 调用无效（`auth status` 显示的也是 user scope，不是 bot 的）。
- **`+update` 是 PUT 整体覆盖，不是 PATCH**：未传字段会被远端清空。改之前先 `+get` 拿当前值，合并后再 `+update`。
- **审核机制**：API 创建/修改的词条**默认进入审核队列**，管理员审批通过后才公开可见。需要免审写入时，应用须开通 `baike:entity:exempt_review`（**仅自建应用**可申请）。
- **词典库 (repo_id)**：可选参数。不传时操作**全公司共享词典**；传入 `repo_id` 则操作指定的私有词典库。

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli lingo +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 | 写入风险 |
|----------|------|---------|
| [`+search`](references/lark-lingo-search.md) | 模糊搜索词条 | 只读 |
| [`+match`](references/lark-lingo-match.md) | 精准匹配词条（判断是否已收录） | 只读 |
| [`+get`](references/lark-lingo-get.md) | 通过 entity_id 获取词条详情 | 只读 |
| [`+create`](references/lark-lingo-create.md) | 创建词条（默认进入审核队列） | 写入 |
| [`+update`](references/lark-lingo-update.md) | 修改词条（PUT 整体覆盖） | 写入 |
| [`+delete`](references/lark-lingo-delete.md) | 删除词条（不可逆） | high-risk-write |

## 典型流程：词条幂等收录

"这个词没收录就新建，已收录就跳过 / 取详情"：

```bash
# Step 1: 精准匹配看是否已存在
lark-cli lingo +match --word "KYC"
#  └─ 命中 → 用返回的 results[0].entity_id 走 Step 2a
#  └─ 未命中 → 走 Step 2b

# Step 2a: 已存在 → 取详情
lark-cli lingo +get --entity-id "<entity_id>"

# Step 2b: 不存在 → 创建（与用户确认释义后；必须 bot）
lark-cli lingo +create --as bot \
  --main-key "KYC" \
  --aliases "Know Your Customer" \
  --description "..."
```

## 权限

| 操作 | 所需 scope | 申请方式 |
|------|-----------|---------|
| `+search` / `+match` / `+get` | `baike:entity:readonly` | `lark-cli auth login --scope "..."`（user）或开发者后台勾选（bot） |
| `+create` / `+update` / `+delete` | `baike:entity` | **只能在开发者后台勾选 + 管理员审批**（bot 路径） |
| 免审写入（不进队列） | `baike:entity:exempt_review` | 同上，**仅自建应用** |

> 缺 scope 时返回 `99991672`，hint 会指向开发者后台 console_url；缺别的 token 类型支持时返回 `99991668`。

## 安全规则

- **写操作（+create / +update / +delete）**：调用前必须向用户确认词面、释义、操作意图。
- **+update 整体覆盖**：未传字段会被清空。先 `+get` 取当前值，合并后再 `+update`。
- **+delete 不可逆**：必须二次确认 `entity_id` 对应的词条名称后再执行；shortcut 默认要求 `--yes`。
- **创建后告知用户**「已提交审核，需管理员通过后生效」（除非应用已开通 `baike:entity:exempt_review`）。

## 参考

- [lark-shared](../lark-shared/SKILL.md) — 认证和全局参数
- [Lingo 概述](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/lingo-v1/overview)
- [词条 API 总览](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/lingo-v1/entity)
