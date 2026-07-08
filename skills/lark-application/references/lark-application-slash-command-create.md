# application +slash-command-create

在当前绑定应用上注册一个新的斜杠指令。

## 命令格式

```bash
lark-cli application +slash-command-create --command <name> --description <text> [flags]
```

## 参数表

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `--command` | string | 是 | 指令名，**不带前导 `/`**（服务端按应用维度校验唯一，最多 100 条指令） |
| `--description` | string | 是 | 客户端指令面板默认展示的描述（写入 `description.default_value`） |
| `--description-i18n` | string_array（可重复） | 否 | 多语言描述，格式 `<lang>=<text>`（如 `zh_cn=发送问候`），可重复多次传不同语言；语言码原样透传给服务端，不做校验 |
| `--icon-key` | string | 否 | 图标 key；不传时服务端默认 `skill_outlined`；非法 key 会被服务端拒绝（code `40000031`） |
| `--force` | bool | 否 | 撞名时不报错，改为解析出已存在指令的 `command_id` 并对其执行 PATCH（等价于幂等重跑，类似 `gh label create --force`） |

> **CAUTION**：`--force` 撞名转 PATCH 时，`description`（含 `i18n` map）按整体替换处理，不是增量合并。若重跑命令时不带上完整的 `--description-i18n`，指令已有的多语言翻译会被静默清空。要做真正幂等的重跑，必须把之前设置过的所有语言连同新值一起重新传入 `--description-i18n`。

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "command_id": "7123456789012345678",
    "command": "greet",
    "description": { "default_value": "say hi", "i18n": { "zh_cn": "问候" } },
    "icon": { "icon_key": "skill_outlined" },
    "action": "created"
  }
}
```

`action` 为 `created` 或 `updated`（后者只在 `--force` 命中撞名分支时出现）。命令执行成功后 stderr 会打印一行客户端缓存提示，不是错误。

## 典型用法

```bash
# 创建一个新指令，附带中文本地化描述
lark-cli application +slash-command-create \
  --command greet --description "say hi" --description-i18n zh_cn=问候 --as bot

# 幂等重跑：如果 greet 已存在则直接更新它，而不是报错退出
# 注意：若 greet 已有 --description-i18n（如 zh_cn=问候），这里不带上就会被清空
lark-cli application +slash-command-create \
  --command greet --description "say hi v2" --description-i18n zh_cn=问候 --force --as bot
```

## 错误与恢复

| 场景 | 识别 | 处理 |
|---|---|---|
| 指令名已存在 | 错误 `code=40000000`，message 含 `command already exists` | 未加 `--force` 时直接报错并提示改用 `+slash-command-update` 或加 `--force` 重跑；已加 `--force` 时 CLI 会自动列出全部指令解析 `command_id` 后转为 PATCH，无需人工介入 |
| `--icon-key` 非法 | 错误 `code=40000031`，原样透传服务端 message | 换成合法 icon key，或省略该参数使用默认值 |
| bot 缺少 `application:app_slash_command:write`（`--force` 撞名分支还需 `:read`） | 错误信封携带 `console_url` | 把 `console_url` 给用户，引导去开发者后台开通对应 scope 并**发布新版本**；不要对 bot 执行 `auth login` |
| user 缺少对应 scope | 错误提示缺失 scope | `lark-cli auth login --scope application:app_slash_command:write`（`--force` 场景还需 `:read`） |

## 参考

- [lark-application](../SKILL.md) — skill 入口与路由，含"改名=删除+创建"等关键语义
- [+slash-command-list](lark-application-slash-command-list.md)
- [+slash-command-update](lark-application-slash-command-update.md)
