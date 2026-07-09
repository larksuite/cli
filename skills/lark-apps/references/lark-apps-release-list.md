# apps +release-list

分页查询妙搭应用发布历史，最新发布在前。运行时命令事实以 `lark-cli apps +release-list --help` 为准。

## 何时用

用户问"最近发布""历史版本""上次为什么失败"，但没有提供 `release_id` 时使用。拿到候选 release 后再接 `+release-get`。

## 命令骨架

- 必填：`--app-id`。
- 可选 `--status`：`publishing` / `finished` / `failed`。
- 可选 `--page-size`：默认 20，最大 500；总是发送给服务端。
- 可选 `--page-token`：上一页 cursor。

## 示例

```bash
lark-cli apps +release-list --app-id app_xxx --page-size 10
lark-cli apps +release-list --app-id app_xxx --status failed
```

## 输出契约

- 成功读取 `data.releases[]`；关键字段是 `release_id`、`status`、`created_at`、`updated_at`。
- `release_id` 用于继续查 `+release-get`。
- 若 `has_more=true`，用 `next_page_token` / `page_token` 翻页。

## Agent 规则

用户限定只看 N 条（"最近 N 条""最新 N 个""只要前 N 条"）时用 `--page-size N`（如"最近一次发布"→ `--page-size 1`），而不是取全量再本地截断。
