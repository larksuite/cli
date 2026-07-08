# application +slash-command-delete

删除当前绑定应用上的一个斜杠指令。**`Risk: high-risk-write`，不可逆。**

## 命令格式

```bash
lark-cli application +slash-command-delete (--command-id <id> | --command <name>) --yes
```

## 参数表

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `--command-id` | string | 与 `--command` 二选一 | 目标指令的 `command_id` |
| `--command` | string | 与 `--command-id` 二选一 | 按名字定位（不带前导 `/`），CLI 会先调用 list 解析出 `command_id`，需要 read scope |
| `--yes` | bool | 高风险操作确认必需 | 见下方"高风险确认协议" |

## 高风险确认协议（CRITICAL）

本命令 `Risk: high-risk-write`。不带 `--yes` 调用会以 exit code `10` 失败，stderr 返回 `error.type == "confirmation_required"` 的结构化 envelope——完整协议见 [lark-shared 高风险操作的审批协议（exit 10）](../../lark-shared/SKILL.md)，本节只补充本命令的领域特有部分。

**Agent 必须在追加 `--yes` 重试前，先把要删除的指令名 / `command_id` 明确展示给用户并取得同意**——不要因为 exit 10 是"标准协议"就默认自动重试。判断"是否已获得授权"时，用户必须在当前对话中明确点名了具体指令；仅泛泛说"清理一下指令"不构成授权。

## 返回值

```json
{
  "ok": true,
  "identity": "bot",
  "data": { "action": "deleted", "command_id": "7123456789012345678", "command": "greet" }
}
```

`command` 字段只在通过 `--command` 定位时才会出现（`--command-id` 直接删除时无法反查名字，字段缺省）。

## 典型用法

```bash
# 先不加 --yes 试探，看到 confirmation_required 后向用户确认
lark-cli application +slash-command-delete --command greet

# 用户明确同意后，原样追加 --yes 重试
lark-cli application +slash-command-delete --command greet --yes --as bot
```

## 错误与恢复

| 场景 | 识别 | 处理 |
|---|---|---|
| 未加 `--yes` | exit code `10`，`error.type == "confirmation_required"` | 展示待删除指令给用户确认，同意后在原始命令末尾追加 `--yes` 重试；不要静默重试 |
| 按 `--command` 找不到指令 | 报错"未找到"，hint 指向 list | 先跑 `+slash-command-list` 确认名字 |
| bot 缺少 `application:app_slash_command:write`（按名定位还需 `:read`） | 错误信封携带 `console_url` | 引导用户去开发者后台开通并发版；不要对 bot 执行 `auth login` |
| user 缺少对应 scope | 提示缺失 scope | `lark-cli auth login --scope application:app_slash_command:write`（按名定位还需 `--scope application:app_slash_command:read`） |
| 删除后客户端指令面板仍显示旧指令 | 正常现象，约 5 分钟客户端缓存 | 服务端已立即生效（`+slash-command-list` 会立刻反映删除），不要因客户端未刷新而重复删除 |

## 恢复注意

删除是不可逆操作，且**没有"撤销删除"**。如果需要恢复同名指令，只能重新 `+slash-command-create`，得到的是**全新的 `command_id`**（与原指令无关联）。

## 参考

- [lark-application](../SKILL.md) — skill 入口与路由
- [+slash-command-list](lark-application-slash-command-list.md)
- [+slash-command-create](lark-application-slash-command-create.md)
