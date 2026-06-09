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
- `status=publishing` 继续轮询。
- `status=finished` 发布成功——**本命令输出已含 `online_url`，直接读取它作为本轮发布的线上访问链接**返回用户，无需再调 `+list`（`+list` 仍可用于按应用名浏览，但不是发布主流程的必经步骤）。
- `status=failed` 发布失败——**本命令输出已含 `error_logs`（`step`/`error_log`），直接据此向用户转述关键失败步骤和可行动修复**，无需再调 `+publish-error-log`（该命令保留为已知失败 release 的独立查询入口）。
- 只有当这个 `release_id` 已返回 `finished`，随后读到的 `online_url` 才能被表述为“本轮发布后的访问链接”。单独从 `+list` 看到 `is_published=true` 不能证明最新版本已部署。
