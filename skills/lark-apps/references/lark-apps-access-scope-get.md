# apps +access-scope-get

查看妙搭应用运行时可见范围。运行时命令事实以 `lark-cli apps +access-scope-get --help` 为准。

## 何时用

用于确认应用运行时对谁可见。它不表示谁能开发或管理应用；协作者、仓库权限不从这里判断。

## 命令骨架

- 必填：`--app-id`。
- 服务端返回枚举是 `All` / `Tenant` / `Range`。
- `Range` 下用户、部门、群分别在 `users` / `departments` / `chats` 数组中；CLI 不合并回 `targets`。

## 示例

```bash
lark-cli apps +access-scope-get --app-id app_xxx
```

## 输出契约

- 成功读取 `data.scope`：`All`、`Tenant`、`Range`。
- `scope=All` 时关注 `data.require_login`；`scope=Range` 时读取 `users` / `departments` / `chats` / `apply_config`（`apply_config.approvers` 仅含一个 user open_id）。

## Agent 规则

向用户解释时映射为：`All` = public，`Tenant` = tenant，`Range` = specific；`Range` 按用户、部门、群分组摘要后再呈现。用户要修改时转到 [`+access-scope-set`](lark-apps-access-scope-set.md)。
