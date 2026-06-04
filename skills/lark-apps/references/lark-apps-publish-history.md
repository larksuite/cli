# apps +publish-history

分页查询妙搭应用发布历史，最新发布在前。运行时命令事实以 `lark-cli apps +publish-history --help` 为准。

## 何时用

用户问“最近发布”“历史版本”“上次为什么失败”，但没有提供 `release_id` 时使用。拿到候选 release 后再接 `+publish-status` 或 `+publish-error-log`。

## 命令骨架

- 必填：`--app-id`。
- 可选 `--status`：`publishing` / `finished` / `failed`。
- 可选 `--limit`：0 表示省略，非 0 必须在 1-500。
- 可选 `--page-token`：上一页 cursor。

## 示例

```bash
lark-cli apps +publish-history --app-id app_xxx --limit 10
lark-cli apps +publish-history --app-id app_xxx --status failed
```

## 输出契约

- 成功读取 `data.releases[]`；关键字段是 `release_id`、`status`、`created_at`、`updated_at`。
- `release_id` 用于继续查 `+publish-status` 或 `+publish-error-log`。
- 若 `has_more=true`，用 `next_page_token` / `page_token` 翻页。

## Agent 规则

用户说“最近一次发布”时先 `--limit 1`。
