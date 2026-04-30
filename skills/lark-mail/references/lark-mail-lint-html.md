# mail +lint-html

本 skill 对应 shortcut：`lark-cli mail +lint-html`。

用途：在不写邮箱状态的情况下检查邮件 HTML 与飞书编辑器兼容性，并可返回自动修复后的 HTML。

## 常用命令

```bash
lark-cli mail +lint-html --body '<p>Hello <font color="red">team</font></p>'
lark-cli mail +lint-html --body-file ./draft.html --strict
```

`--body` 与 `--body-file` 二选一；`--body-file` 必须是运行目录内的相对路径。

## 输出契约

`lint_applied` / `original_blocked` 是写入链路也会返回的契约字段名，不要改名。

```json
{
  "ok": true,
  "data": {
    "warnings": [],
    "errors": [],
    "cleaned_html": "<p>clean</p>"
  }
}
```

## 写入链路

`+send`、`+draft-create`、`+reply`、`+reply-all`、`+forward` 和 `+draft-edit` 的 HTML 写入路径会复用同一套 lint 规则：

- warning 默认自动修复并写入 `lint_applied`
- error 默认删除或阻断危险片段并写入 `original_blocked`
- 如需紧急止血，可设置 `LARK_CLI_MAIL_LINT_MODE=warn-only`，仅输出报告不改写 HTML

