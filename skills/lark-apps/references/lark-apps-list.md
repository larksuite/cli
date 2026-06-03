# apps +list

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

列出当前用户可见的妙搭应用，支持**名称模糊搜索**（`--keyword`）、**协作者维度过滤**（`--scope`）和**类型过滤**（`--app-type`）。**cursor 分页**：默认拉一页（`--page-size 20`），通过 `--page-token` 拉下一页。

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

# 按名称模糊搜索本人创建的应用，拿 app_id
lark-cli apps +list --keyword 问卷 --scope created_by_me

# 只看全栈应用
lark-cli apps +list --app-type full_stack

# 取 ID 列表（脚本场景）
lark-cli apps +list -q '.data.items[].app_id'

# 按名字找 app_id
lark-cli apps +list --keyword 客户调研问卷 -q '.data.items[] | select(.name=="客户调研问卷") | .app_id'
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `--keyword <str>` | ❌ | `""` | 应用名称模糊匹配 |
| `--scope <str>` | ❌ | `all` | 协作者维度：`all`（本人+协作）/ `created_by_me`（仅本人创建）/ `shared_with_me`（仅他人分享给我）|
| `--app-type <str>` | ❌ | `""` | 类型过滤：`html` / `full_stack`；不填返回所有受支持类型 |
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
{ "ok": false, "error": { "type": "api", "message": "...", "hint": "..." } }
```

## 字段语义

- `data.items` 长度可能为 0（用户没建过应用）
- `data.has_more=true` 表示还有下一页；用 `data.page_token` 作为下次 `--page-token` 传入
- `data.has_more=false` 且 `data.page_token` 为空 / 缺省表示已经到末尾

## Agent 使用契约

何时该调用本命令（默认仍走 `+create`、仅凭用户给的应用名解析 `app_id`、禁止无条件枚举）以 [`../SKILL.md`](../SKILL.md) "用户没给 app_id" 一节为准，不在此重复。命中触发条件后，用上面的 `--keyword` 示例搜索，并用 `-q '.data.items[] | select(.name=="<名称>") | .app_id'` 提取候选交用户确认。

## 协同命令

| 场景 | 命令 |
|---|---|
| 创建新应用 | `apps +create` |
| 修改应用 | `apps +update` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
