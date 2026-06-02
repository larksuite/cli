# apps +list

> **Agent 可用本命令定位存量应用的 `app_id`**：当用户没直接给 app_id 时，用 `+list --filter <关键词>` 按应用名 / 描述过滤出来。也可优先读本地项目根的 `.spark/meta.json`（若已在项目目录内，那里记录了 app_id）。完整解析顺序见 [`../SKILL.md`](../SKILL.md) 「拿到存量应用的 app_id」。

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

列出当前用户名下的妙搭应用。**cursor 分页**：默认拉一页（`--page-size 20`），通过 `--page-token` 拉下一页。

## 命令

```bash
# 按关键词过滤定位存量应用（推荐，Agent 找 app_id 的主路径）
lark-cli apps +list --filter 客户管理

# 拉第一页（默认 page_size=20）
lark-cli apps +list

# 自定义页大小
lark-cli apps +list --page-size 50

# 翻页（拿上一次响应的 page_token）
lark-cli apps +list --page-token "eyJQaW5PcmRlciI6..."

# 取 ID 列表（脚本场景）
lark-cli apps +list -q '.data.items[].app_id'

# 按名字找 app_id
lark-cli apps +list -q '.data.items[] | select(.name=="客户调研问卷") | .app_id'
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `--filter <str>` | ❌ | `""` | 按应用名 / 描述模糊过滤（本期新增；定位存量应用首选） |
| `--page-size <int>` | ❌ | `20` | 每页条数 |
| `--page-token <str>` | ❌ | `""` | 翻页 cursor，从上次响应的 `data.page_token` 拿 |

## 返回值

**成功：**

```json
{
  "ok": true,
  "data": {
    "items": [
      {
        "app_id": "app_4k5jepcbjmv6m",
        "name": "客户调研问卷",
        "description": "...",
        "icon_url": "...",
        "created_at": "2026-05-18T10:00:00Z",
        "updated_at": "2026-05-18T10:05:00Z"
      }
    ],
    "page_token": "cursor_next_xxx",
    "has_more": true
  }
}
```

**成功（空列表）：**

```json
{ "ok": true, "data": { "items": [], "has_more": false } }
```

**失败：**

```json
{ "ok": false, "error": { "type": "api_error", "message": "...", "hint": "..." } }
```

## 字段语义

- `data.items` 长度可能为 0（用户没建过应用）
- `data.has_more=true` 表示还有下一页；用 `data.page_token` 作为下次 `--page-token` 传入
- `data.has_more=false` 且 `data.page_token` 为空 / 缺省表示已经到末尾

## 用途

定位存量应用的 `app_id`：用户没给 app_id 时，`+list --filter <关键词>` 按名字 / 描述过滤出目标应用。新建场景仍直接 `apps +create`，不必先 list。完整解析顺序（用户给的 → `.spark/meta.json` → `+list --filter`）见 [`../SKILL.md`](../SKILL.md) 「拿到存量应用的 app_id」。

## 协同命令

| 场景 | 命令 |
|---|---|
| 创建新应用 | `apps +create` |
| 修改应用 | `apps +update` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
