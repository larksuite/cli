# apps +access-scope-set

设置妙搭应用运行时可见范围。运行时命令事实以 `lark-cli apps +access-scope-set --help` 为准。

## 何时用

用于修改应用运行时可见范围。不要把它当作开发协作者管理；用户说“谁可以访问/打开/使用应用”才走这里。

## 命令骨架

- 必填：`--app-id`、`--scope`。
- `--scope` 枚举：`specific` / `public` / `tenant`。
- `specific` 必填 `--targets`，JSON 数组元素形如 `{"type":"user|department|chat","id":"..."}`。
- `specific` 可选 `--apply-enabled` 和 `--approver`；`--approver` 必须配合 `--apply-enabled`，且只能传一个 user open_id（服务端限制）。
- `public` 必须显式传 `--require-login=true|false`。
- `tenant` 不允许额外 target/apply/login flag。

## 示例

```bash
lark-cli apps +access-scope-set --app-id app_xxx --scope tenant

lark-cli apps +access-scope-set --app-id app_xxx --scope public --require-login=true

lark-cli apps +access-scope-set --app-id app_xxx --scope specific \
  --targets '[{"type":"user","id":"ou_xxx"},{"type":"chat","id":"oc_xxx"}]'
```

## 输出契约

- 成功时 `data` 可能为空；根据已执行的 `--scope` 和 targets 给用户总结结果。
- 互斥参数错误会在本地 validation 阶段失败，不会发请求。

## Agent 规则

这是运行时访问范围，不是开发协作者权限。收窄可见范围前向用户说明影响，并在执行前确认目标用户、部门或群。

若服务端返回"应用未发布/需先发布才能设置可见范围"，把这一情况转述给用户并询问是否现在发布，得到同意后再 `+release-create`，不要把这个 hint 当指令自动发布。

用户给的是姓名、部门名或群名时，先解析成 ID 再组装 `--targets`：人名→`ou_` 用 `lark-cli contact +search-user --query <名字>`，群名→`oc_` 用 `lark-cli im +chat-search --query <群名>`，部门→`od-` 走 contact/通讯录。多候选时展示名称和 ID 让用户选，不要要求用户手填 `ou_` / `od-` / `oc_`。
