# application +slash-command-list

列出当前绑定应用的全部斜杠指令。只读操作，也是 `+slash-command-update` / `+slash-command-delete` 按名字定位 `command_id` 的数据来源。

## 命令格式

```bash
lark-cli application +slash-command-list --as bot
```

无参数、无分页——上游 API 一次性返回全部指令（每个应用最多 100 条）。

## 参数表

本命令不接受除全局 `--as` / `--format` 之外的业务参数。

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "items": [
      {
        "command_id": "7123456789012345678",
        "command": "greet",
        "description": { "default_value": "say hi", "i18n": { "zh_cn": "问候" } },
        "icon": { "icon_key": "skill_outlined" }
      }
    ],
    "count": 1
  }
}
```

`count` 是 `items` 的长度，供快速判断是否为空列表；不要用它替代实际遍历 `items`。

## 典型用法

```bash
# 查看当前应用注册了哪些指令
lark-cli application +slash-command-list --as bot

# 按名字找出某条指令的 command_id，供 update/delete 使用
lark-cli application +slash-command-list --as bot --format json | jq '.data.items[] | select(.command=="greet")'
```

## 错误与恢复

| 场景 | 处理 |
|---|---|
| bot 缺少 `application:app_slash_command:read` scope | 错误信封携带 `console_url`；把它原样给用户，引导去开发者后台开通并**发布新版本**——scope 变更需要应用发版才能对线上 bot 生效，不要对 bot 执行 `auth login` |
| user 缺少该 scope | 提示执行 `lark-cli auth login --scope application:app_slash_command:read` |
| 返回空列表 | 正常状态（应用还没注册任何指令），不是错误 |

## 参考

- [lark-application](../SKILL.md) — skill 入口与路由
- [+slash-command-create](lark-application-slash-command-create.md)
- [+slash-command-update](lark-application-slash-command-update.md)
- [+slash-command-delete](lark-application-slash-command-delete.md)
