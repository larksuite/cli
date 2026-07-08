# application +slash-command-update

更新一个已有斜杠指令的描述 / 多语言描述 / 图标。

## 命令格式

```bash
lark-cli application +slash-command-update (--command-id <id> | --command <name>) [flags]
```

## 参数表

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `--command-id` | string | 与 `--command` 二选一 | 目标指令的 `command_id`（来自 `+slash-command-list` 或 create 的输出） |
| `--command` | string | 与 `--command-id` 二选一 | 按名字定位（不带前导 `/`），CLI 会先调用 list 解析出 `command_id`，需要 read scope |
| `--description` | string | 否 | 新的默认描述（`description.default_value`） |
| `--description-i18n` | string_array（可重复） | 否 | 多语言描述，格式 `<lang>=<text>`；**整体替换**已有 i18n map（未传入的语言会被服务端丢弃，不是增量合并）；**必须伴随 `--description` 一起传**，否则报错 |
| `--icon-key` | string | 否 | 新图标 key；非法 key 服务端拒绝（code `40000031`） |

`--description` / `--description-i18n` / `--icon-key` 三者至少提供一个，否则报错"没有可更新字段"。

## 关键语义

- **`--command-id` 与 `--command` 互斥，且必须提供其中之一**——两个都不传或都传都会报错。
- **PATCH 是字段级局部更新**：未显式传入的字段（如只传了 `--icon-key`）在服务端保持原值不变。
- **`--description-i18n` 是整体替换，不是增量合并**：如果指令原本有 `zh_cn` + `ja_jp` 两个语言的描述，本次只传 `--description-i18n en_us=hi`，结果是只剩 `en_us` 一个语言，`zh_cn`/`ja_jp` 会丢失。要保留原有语言，需要把它们连同新值一起重新传入。
- **指令名本身不可改**（API 限制）：`+slash-command-update` 不支持修改 `command` 字段。要"重命名"，必须走 [+slash-command-delete](lark-application-slash-command-delete.md) + [+slash-command-create](lark-application-slash-command-create.md)（详见 [SKILL.md 关键语义](../SKILL.md#关键语义)），会产生一个全新的 `command_id`。

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": {
    "command_id": "7123456789012345678",
    "command": "greet",
    "description": { "default_value": "new text", "i18n": { "en_us": "hi" } },
    "icon": { "icon_key": "skill_outlined" },
    "action": "updated"
  }
}
```

## 典型用法

```bash
# 按名字更新描述（CLI 内部先 list 解析 command_id 再 PATCH）
lark-cli application +slash-command-update --command greet --description "new text" --as bot

# 只改图标，不动描述
lark-cli application +slash-command-update --command-id 7123456789012345678 --icon-key chat_outlined --as bot

# 同时替换默认描述和多语言描述（记得带上所有要保留的语言）
lark-cli application +slash-command-update --command greet \
  --description "say hi" --description-i18n zh_cn=你好 --description-i18n en_us=hi --as bot
```

## 错误与恢复

| 场景 | 识别 | 处理 |
|---|---|---|
| `--description-i18n` 未伴随 `--description` | 校验期报错，提示两者必须同传 | 补上 `--description`（哪怕值和原来一样），因为 PATCH 会整体替换 description 对象，缺 `default_value` 会导致其被清空 |
| 按 `--command` 找不到指令 | 报错"未找到"，hint 指向 list | 先跑 `+slash-command-list` 确认指令名拼写和是否存在 |
| `--icon-key` 非法 | 错误 `code=40000031` | 换合法 icon key |
| bot 缺少 `application:app_slash_command:write`（按名定位还需 `:read`） | 错误信封携带 `console_url` | 引导用户去开发者后台开通并发版；不要对 bot 执行 `auth login` |
| user 缺少对应 scope | 提示缺失 scope | `lark-cli auth login --scope application:app_slash_command:write`（按名定位还需 `--scope application:app_slash_command:read`） |

## 参考

- [lark-application](../SKILL.md) — skill 入口与路由
- [+slash-command-list](lark-application-slash-command-list.md)
- [+slash-command-delete](lark-application-slash-command-delete.md)
