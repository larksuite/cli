# apps +list

列出当前用户可见的妙搭应用，用于从应用名定位 `app_id`。运行时命令事实以 `lark-cli apps +list --help` 为准。

## 何时用

在下游操作需要 `app_id`、而用户只给了应用名/描述时,用 `--keyword` 定位。无明确目的的全量枚举会浪费上下文,优先按关键词缩小范围。

## 命令骨架

- 支持 `--keyword` 按应用名模糊搜索。
- `--scope` 枚举：`all` / `created_by_me` / `shared_with_me`。
- `--app-type` 枚举：`html` / `full_stack`。
- 分页：`--page-size` 默认 20，`--page-token` 传上一页 cursor。

## 示例

```bash
lark-cli apps +list --keyword "审批"
lark-cli apps +list --scope created_by_me --app-type full_stack
lark-cli apps +list --page-token "<cursor>"
```

## 输出契约

- 成功读取 `data.items[]`；保留字段为 `description`、`app_id`、`name`、`is_published`、`online_url`、`updated_at`，用于候选展示的核心字段是 `name`、`app_id`、`updated_at`。
- 默认输出已裁掉 `icon_url`（图片 URL，agent 无法渲染）和 `created_at`（与 `updated_at` 冗余）；需要时可用 `--jq` 过滤上述保留字段。
- `data.items` 可能为空；不要把空列表当失败。
- 若有 `has_more=true`，用返回的 `page_token` / `next_page_token` 继续翻页。

## Agent 规则

多候选时展示名称、app_id、updated_at 让用户确认。用户描述里已经有 `app_xxx` 或妙搭链接时，直接提取，不再 `+list`。
