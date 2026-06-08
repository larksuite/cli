# apps +publish-status

按 release ID 查询单次发布详情。运行时命令事实以 `lark-cli apps +publish-status --help` 为准。

## 何时用

用于跟进已知 `release_id` 的发布状态。没有 `release_id` 时先读 [`lark-apps-publish-history.md`](lark-apps-publish-history.md)，不要让用户手填。

`release_id` 是妙搭发布 ID（`+publish` 返回），不是飞书审批实例号；查发布进度/失败都在 `apps +publish-*` 命令族内完成，不要路由到 lark-approval。

## 命令骨架

- 必填：`--app-id`、`--release-id`。
- `release_id` 来自 `+publish` 或 `+publish-history`。

## 示例

```bash
lark-cli apps +publish-status --app-id app_xxx --release-id release_yyy
```

## 输出契约

- 成功可能直接返回 release 字段，也可能包在 `data.release`；读取 `release_id`、`status`、`created_at`、`updated_at`。
- `status=publishing` 继续轮询；`finished` 发布成功——此时用 `lark-cli apps +list --keyword <应用名>` 取 `online_url` 返回用户（`+publish-status` 本身不含可分享 URL）；`failed` 接 [`+publish-error-log`](lark-apps-publish-error-log.md) 取错误日志。
