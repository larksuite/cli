# apps +delete

删除妙搭应用。删除后是否进入 web 端「我的应用 → 回收站」取决于返回的 `data.in_trash`；只有 `data.in_trash=true` 时才可在保留期内恢复，`false` 表示永久删除。运行时命令事实以 `lark-cli apps +delete --help` 为准。

## 何时用

只在用户明确要求删除某个应用时使用。不要为了清理测试数据或在用户没提出时主动删除。

## 命令骨架

- 必填：`--app-id`（app_id 或 meta_token）、`--confirm-name`；真实执行还必须带 `--yes`，`--dry-run` 预览不需要。
- `--confirm-name` 必须与删除前 GET 返回的 `data.app.name` 完全一致（区分大小写，首尾空白忽略），否则拒绝删除。
- 名称只在 DELETE 前检查一次；如果应用在 GET 和 DELETE 之间被重命名，CLI 无法检测这段窗口，因为 DELETE API 没有名称、版本或 ETag 前置条件。
- `--dry-run` 只预览 GET + DELETE 两步请求，不删除。

```bash
lark-cli apps +get --app-id app_xxx --jq '.data.app.name'
# 向用户展示 app_id 与名称、说明删除后果，并取得用户对该应用的明确删除确认后再执行：
lark-cli apps +delete --app-id app_xxx --confirm-name "审批系统" --yes
```

## Agent 规则

- **CRITICAL**：执行前必须先 `+get`，向用户展示应用名和 app_id，说明删除后可能进入回收站，也可能永久删除；只有返回 `data.in_trash=true` 时才可在 web 端恢复。拿到用户对**这个应用**的明确删除确认后，才能带 `--confirm-name` 和 `--yes` 执行。“删掉 X”只确定了目标，不等于确认。
- **CRITICAL**：用户给的名称与 `+list` / `+get` 查到的名称不完全一致时（如用户说「X-正式版」，只查到「X」），视为用户还没确认这个应用。`--confirm-name` 只能用用户确认过的名称，不得用查到的名称替换后执行；先停下展示候选名称与 app_id，等用户确认。
- **CRITICAL**：返回 `failed_precondition` 且 `param` 为 `--confirm-name` 时，名称不一致。不要猜测或从别处推断名称后自动重试，回到上一条重新确认。

## 输出契约

- 成功读取 `data.app_id`、`data.name`、`data.deleted`。
- `data.in_trash=true`：告诉用户可以在 web 端「我的应用 → 回收站」于保留期内恢复。`data.in_trash=false`：明确告知已永久删除。
- 响应里没有 `data.in_trash` 时：只告知用户已删除，不承诺可以恢复，也不要因此重试。
- 删除状态异步生效，删除后短时间内 `+get` 仍可能查到该应用；不要据此判断删除失败，也不要重复删除。
- 删除生效后再 `+get`（或对同一应用再 `+delete`）会失败，错误码为 `40300` 或 `40400`：表示应用已删除，不是权限问题，不要据此让用户去申请权限。

## 错误

| type / subtype | 含义 | 处理 |
|---|---|---|
| `authorization/permission_denied` | 只有应用 owner 或创建者能删 | 转告用户，不要重试 |
| `failed_precondition`（hint 指向 web 端） | 该应用类型（如 OpenClaw）不能通过 lark-cli 删除 | 引导用户在 web 端删除 |
| `not_found` | 应用不存在、已删除或无权访问 | 用 `+list` 核对 app_id |
