# apps +publish-error-log

获取某次发布失败日志。运行时命令事实以 `lark-cli apps +publish-error-log --help` 为准。

## 何时用

只在发布状态是失败、或用户明确要求看失败原因时调用。没有 `release_id` 时先查发布历史。

`release_id` 是妙搭发布 ID（`+publish` 返回），不是飞书审批实例号；查失败原因留在 `apps +publish-*` 命令族内，不要路由到 lark-approval。

## 命令骨架

- 必填：`--app-id`、`--release-id`。
- 输出包含 `status` 和 `error_logs`；pretty/table 主要展示 step 与 error_log。

## 示例

```bash
lark-cli apps +publish-error-log --app-id app_xxx --release-id release_yyy
```

## 输出契约

- 成功读取 `data.status` 和 `data.error_logs[]`。
- 每条日志关注 `step` 与 `error_log`；向用户转述关键失败步骤和可行动修复，不要整段倾倒。
- `error_logs` 为空但发布失败时，说明没有步骤级日志；回到发布状态和配置排查。
