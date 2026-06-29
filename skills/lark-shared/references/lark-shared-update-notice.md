# 升级提示（_notice）

命令执行后 JSON 输出可能包含 `_notice`，其下三种通知的处置都是升级：

- `update`：CLI 有新版本（字段 `current` / `latest` / `message` / `command`）。
- `skills`：内置 AI Skills 落后于 CLI（字段 `current` / `target`）。
- `deprecated_command`：本次用了已废弃的命令别名（`replacement` 为新命令名）。

看到任一通知都**不要静默忽略**，即使与当前任务无关：完成用户当前请求后告知情况，主动提议执行 `lark-cli update`（同时更新 CLI 和 AI Skills；加 `--check` 可只检查不安装）。更新完成后提醒用户**退出并重新打开 AI Agent** 以加载最新 Skills。
