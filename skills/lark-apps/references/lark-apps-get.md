# apps +get

按 app_id 查询单个应用详情。运行时命令事实以 `lark-cli apps +get --help` 为准。

## 何时用

需要查看一个应用的类型、名称、描述、发布状态等详情时使用。如果只是按应用名模糊搜索定位 app_id，用 `+list --keyword`。

## 命令骨架

- 必填：`--app-id`。
- 返回应用的完整信息：`app_id`、`meta_token`、`meta_doc_type`、`app_type`、`name`、`description`、`icon_url`、`created_at`、`updated_at`、`is_published`。

## 示例

```bash
lark-cli apps +get --app-id app_xxx
lark-cli apps +get --app-id app_xxx --dry-run
lark-cli apps +get --app-id app_xxx -q '.data.app.app_type'
```

## 输出契约

- 成功读取 `data.app` 对象，包含以下字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `app_id` | string | 应用唯一标识 |
| `meta_token` | string | 权限、密级接口使用的资源 token；不能用 app_id 替代 |
| `meta_doc_type` | string | 权限、密级接口使用的资源类型（`apps` 或 `miaoda_apps`），以接口实际返回为准 |
| `app_type` | string | 应用类型（如 HTML、FRONTEND、FULL_STACK、MODERN_HTML） |
| `name` | string | 应用显示名称 |
| `description` | string | 应用功能说明 |
| `icon_url` | string | 应用图标 URL |
| `created_at` | string | 创建时间（ISO 8601 UTC） |
| `updated_at` | string | 最后更新时间（ISO 8601 UTC） |
| `is_published` | boolean | 是否已发布 |

- pretty 输出展示核心字段：`app_id`、`app_type`、`name`、`is_published`、`updated_at`。
- `meta_token`、`description`、`icon_url`、`badge_status` 在服务端提供非空值时也会显示在 pretty 输出中；`meta_doc_type` 从 JSON 或 `--jq` 获取，pretty 不展示。JSON 保留服务端返回字段，缺失字段不补默认值。
- `is_published=true` 只代表应用历史上有发布版本，不代表最新代码已部署。

## Agent 规则

- 用户已有 `app_id` 想查看详情时用 `+get`；只有应用名时用 `+list --keyword`。
- 不要把 `cli_` 开头的飞书应用 ID 传给 `+get`，只接受 `app_` 开头的应用 ID。

## 妙搭权限与密级的 token/type 解析

对 `/app/app_xxx` 链接，从路径提取 `app_xxx`，使用同一 profile 执行：

```bash
lark-cli apps +get --app-id app_xxx --as user --jq '.data.app'
```

读取结果中的 `meta_token` 和 `meta_doc_type`，再将它们分别传给 Drive shortcut 的 `--token` 和 `--type`。不传 `--jq` 时字段位于 `data.app`。例如查询资源权限设置：

```bash
lark-cli drive +permission-get-setting --token '<meta_token>' --type '<meta_doc_type>' --as user
```

这一步适用于 `drive +apply-permission`、`drive +permission-get-setting`、`drive +member-list`、`drive +member-add`、`drive +member-remove`、`drive +secure-label-update`。写操作仍遵守对应命令的确认规则。

- 不把 `app_id` 当成 `meta_token`，不把 `app_type` 当成 `meta_doc_type`；不要猜测 `apps` 与 `miaoda_apps` 的映射。
- 查询失败、字段缺失/为空或类型不支持时停止，不继续权限或密级操作。
- Drive shortcut 不会自动查询 Spark；直接传 `/app/app_xxx` URL 会返回 `validation/invalid_argument`，在 `hint` 中引导本流程。
- 此流程只识别妙搭 `/app/app_...`；其他 `/app/` 链接可能属于 BaseApp，不据此前缀直接路由到妙搭。
