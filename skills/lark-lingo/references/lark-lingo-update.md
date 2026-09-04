# lingo +update

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

修改一条已有词条。**必须 `--as bot`**。**PUT 整体覆盖，不是 PATCH**。

## 命令

```bash
# 必须同时传主词 + 你要保留的字段（纯文本释义）
lark-cli lingo +update --as bot \
  --entity-id "enterprise_xxxx" \
  --main-key "KYC" \
  --aliases "Know Your Customer" \
  --description "更新后的释义"

# 富文本释义（与 --description 互斥）
lark-cli lingo +update --as bot \
  --entity-id "enterprise_xxxx" \
  --main-key "KYC" \
  --aliases "Know Your Customer" \
  --rich-text '<p><b>Know Your Customer 了解你的客户</b><span>，…</span></p>'

# 同时更新 related_meta（PUT 整体覆盖 — 不传会清空原有 related_meta!）
lark-cli lingo +update --as bot \
  --entity-id "enterprise_xxxx" \
  --main-key "KYC" \
  --aliases "Know Your Customer" \
  --description "更新后的释义" \
  --related-meta '{"classifications":[{"id":"7517595051844222977","father_id":"7517595051644862466"}]}'

# 只改 description：主词仍必须带
lark-cli lingo +update --as bot \
  --entity-id "enterprise_xxxx" \
  --main-key "KYC" \
  --description "新释义"
#  ⚠️ 这样会清空原有 aliases 和 rich_text！先 +get 取全量再合并。

# 安全模式：先 +get，手工合并，再 +update
ENTITY_ID="enterprise_xxxx"
lark-cli lingo +get --entity-id "$ENTITY_ID"      # 看当前 main_key / aliases / description / rich_text
lark-cli lingo +update --as bot \
  --entity-id "$ENTITY_ID" \
  --main-key "<从上面 copy>" \
  --aliases "<从上面 copy，改动的地方在这里合进去>" \
  --rich-text "<从上面 copy 的 rich_text，调整后>"

# 从文件读富文本（适合长内容）
lark-cli lingo +update --as bot \
  --entity-id "enterprise_xxxx" \
  --main-key "KYC" \
  --rich-text @./new-desc.html

# 预览底层请求
lark-cli lingo +update --as bot \
  --entity-id "enterprise_xxxx" \
  --main-key "KYC" \
  --description "…" \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--entity-id` | 是 | 目标词条 ID |
| `--main-key` | 是 | 主词（PUT 整体覆盖要求必传） |
| `--aliases` | 否 | 别名列表，逗号分隔；**不传会被清空** |
| `--description` | 否 | **纯文本**释义；与 `--rich-text` 互斥；**不传会被清空**；支持 `@file` 和 `-`（stdin） |
| `--rich-text` | 否 | **HTML 富文本**释义；与 `--description` 互斥；**不传会被清空**；支持 `@file` 和 `-`（stdin） |
| `--related-meta` | 否 | 相关元数据 JSON 对象；**不传会被清空** —— PUT 整体覆盖，先 `+get` 取原值再合并；支持 `@file` 和 `-`（stdin） |
| `--allow-highlight` | 否 | 是否在文档中高亮，默认 `true` |
| `--allow-search` | 否 | 是否参与搜索，默认 `true` |

## 关键约束

- ⚠️ **PUT 整体覆盖**：未传的可选字段会被远端清空。任何更新前请**先 [`+get`](lark-lingo-get.md) 取当前值，再合并后调本 shortcut**。特别是：
  - 只传 `--description` 会清空 `rich_text`（不过通常 `description` 会被服务端从 `rich_text` 自动 strip 回填）
  - 只传 `--rich-text` 会清空 `description`
  - **不传 `--related-meta` 会清空所有分类、关联词条、用户、文档、链接、图片** — 必要时先 `+get` 拿到当前 `related_meta`，原样再传一遍
- **`--description` 与 `--rich-text` 互斥**：同时传 CLI 直接报错。
- **必须 `--as bot`**：API 端点拒收 user token，user 调用返 `99991668`。
- **修改走审核**：除非应用已开通 `baike:entity:exempt_review`，更新要等管理员审批通过才生效。
- **不支持局部字段**：本 shortcut 映射到 `PUT /open-apis/lingo/v1/entities/:entity_id`；飞书 API 没有 PATCH 端点。

## rich_text HTML 格式

参见 [`+create`](lark-lingo-create.md#rich_text-html-格式)。惯例：`<p><b>主词</b><span>释义</span></p>`。

## related_meta JSON 格式

参见 [`+create`](lark-lingo-create.md#related_meta-json-格式)。**记得 PUT 是整体覆盖** — 哪怕只改一个字段，也要先 `+get` 拿到完整 `related_meta`，合并后再传。

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "entity": {
      "id": "enterprise_xxxx",
      "main_keys": [{"key": "KYC"}],
      "aliases": [{"key": "Know Your Customer"}],
      "description": "更新后的释义",
      "rich_text": "<p>…</p>"
    }
  }
}
```

## 权限

| 操作 | 所需 scope |
|------|-----------|
| 更新（走审核） | `baike:entity` |
| 免审更新 | `baike:entity` + `baike:entity:exempt_review`（仅自建应用） |

## 参考

- [lark-lingo](../SKILL.md) — Lingo 域总览
- [lark-lingo +get](lark-lingo-get.md) — 改之前取当前值
- [lark-lingo +create](lark-lingo-create.md) — 新建词条
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
