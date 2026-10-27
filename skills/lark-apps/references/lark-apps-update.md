# apps +update

部分更新妙搭应用元信息。运行时命令事实以 `lark-cli apps +update --help` 为准。

## 何时用

只更新应用展示元信息。用户要改代码、发布内容、可见范围或数据库时，不走 `+update`。

## 命令骨架

- 必填：`--app-id`。
- 至少提供一个：`--name`、`--description`、`--icon-url` 或 `--icon-file`。
- 只发送用户提供的字段，不会清空未提供字段。
- 改图标：`--icon-file` 与 `--icon-url` 互斥，二选一。

## 修改图标

- **优先 `--icon-file`**：传本地 png/jpeg（按文件内容识别类型，不看扩展名；≤2 MB）。CLI 先上传图片拿到托管的 `icon_url`，再写入应用。
- `--icon-url` 只在用户明确给出公网图片链接时使用，须为 http(s)。外部链接作图标时，飞书分享卡片可能不显示应用头像，这是已知行为；能拿到图片文件时改用 `--icon-file`。
- 改图标前 CLI 会先查询应用并确认类型支持改图标：全栈、HTML、前端（含创意模式）应用支持；OpenClaw 类应用及其子应用等类型不支持，会直接报错并提示去妙搭编辑器修改，不会上传图片。遇到该错误不要换参数重试，转述提示即可。
- 执行后成功响应的 `data.app.icon_url` 即新图标；若与写入值不一致，CLI 会在 stderr 打出 warning，此时用 `+get` 回读确认。

## 示例

```bash
lark-cli apps +update --app-id app_xxx --name "审批系统"
lark-cli apps +update --app-id app_xxx --description "用于部门审批流转"
lark-cli apps +update --app-id app_xxx --name "审批系统" --description "用于部门审批流转" --dry-run
lark-cli apps +update --app-id app_xxx --icon-file ./icon.png
lark-cli apps +update --app-id app_xxx --icon-url https://example.com/icon.png
```

## 输出契约

- 成功读取 `data.app`；响应是完整应用对象，不只是被修改字段。
- 缺 `--app-id`、四个字段一个都没提供、同时传 `--icon-url` 与 `--icon-file`、`--icon-url` 不是 http(s)、`--icon-file` 不可读 / 超过 2 MB / 不是 png 或 jpeg，都会在本地 validation 失败，不发请求。
- `--dry-run` 改图标时先展示 `GET /open-apis/spark/v1/apps/:app_id`（类型预检）；配 `--icon-file` 时再展示 `POST /open-apis/spark/v1/icon`（multipart 上传），最后是 `PATCH /open-apis/spark/v1/apps/:app_id`。

## Agent 规则

更新前复述要变更的字段；用户没有提到的字段不要补默认值。执行后只转述新的名称/描述/图标和 app_id，不需要展开原始响应。
