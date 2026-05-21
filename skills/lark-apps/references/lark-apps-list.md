# apps +list

> **⚠️ Hidden 命令（`Hidden: true`）—— 不对 Agent 暴露**：本命令从 `--help` / tab completion / SKILL.md 的 Shortcuts 表中隐去，**Agent 不应主动调用**。
>
> 需要拿现有应用的 `app_id` 时让用户提供 **妙搭应用链接**（如 `https://miaoda.feishu.cn/app/app_xxxxxxxxxxxxx`）然后从 URL 中提取，或者让用户直接给 `app_id` 字符串。详见 [`../SKILL.md`](../SKILL.md) "用户没给 app_id" 一节。
>
> 本文件保留是因为命令仍然功能可用（手动调用），下面内容仅供人类参考。

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

列出当前用户名下的妙搭应用。**cursor 分页**：默认拉一页（`--page-size 20`），通过 `--page-token` 拉下一页。

## 命令

```bash
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

本命令保留可供人类操作员手动调用（例如运维 / 调试场景，按 `name` 搜应用 ID）。**Agent 不应主动调用**：默认行为是 `apps +create` 新建；要复用现有应用，**让用户给妙搭应用链接或 app_id**，详见 [`../SKILL.md`](../SKILL.md) "用户没给 app_id" 一节。

## 协同命令

| 场景 | 命令 |
|---|---|
| 创建新应用 | `apps +create` |
| 修改应用 | `apps +update` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
