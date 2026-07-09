# apps +update

部分更新妙搭应用元信息。运行时命令事实以 `lark-cli apps +update --help` 为准。

## 何时用

只更新应用展示元信息。用户要改代码、发布内容、可见范围或数据库时，不走 `+update`。

## 命令骨架

- 必填：`--app-id`。
- 至少提供一个：`--name` 或 `--description`。
- 只发送用户提供的字段，不会清空未提供字段。

## 示例

```bash
lark-cli apps +update --app-id app_xxx --name "审批系统"
lark-cli apps +update --app-id app_xxx --description "用于部门审批流转"
lark-cli apps +update --app-id app_xxx --name "审批系统" --description "用于部门审批流转" --dry-run
```

## 输出契约

- 成功读取 `data.app`；响应是完整应用对象，不只是被修改字段。
- 缺 `--app-id` 或没有提供 `--name` / `--description` 会在本地 validation 失败。

## Agent 规则

更新前复述要变更的字段；用户没有提到的字段不要补默认值。执行后只转述新的名称/描述和 app_id，不需要展开原始响应。
