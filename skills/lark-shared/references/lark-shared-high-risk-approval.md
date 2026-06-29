# 确认门禁 envelope 参考（exit 10）

处理协议见 SKILL.md 正文准则。本文讲报错 JSON 的两种形态、字段位置，以及重试 / 预览的两个坑。

## 可靠信号是退出码 10，不是 type 字符串

仓库正从扁平式迁往 typed 式，过渡期两种并存——扁平式仍是 shortcut / service 命令的当前形态（多数高风险命令），typed 式是已迁移命令（如 `config bind`）的新形态。**别认 `type` 字符串（迁移中会变），认退出码 10**：

**扁平式：**

```json
{
  "ok": false,
  "error": {
    "type": "confirmation_required",
    "message": "drive +delete requires confirmation",
    "hint": "add --yes to confirm",
    "risk": { "level": "high-risk-write", "action": "drive +delete" }
  }
}
```

**typed 式：**

```json
{
  "ok": false,
  "error": {
    "type": "confirmation",
    "subtype": "confirmation_required",
    "risk": "high-risk-write",
    "action": "config bind --force",
    "hint": "若用户确认切换，附加 --force 重新运行：`lark-cli config bind --identity user-default --force`"
  }
}
```

识别条件：exit code = 10，且 `error` 命中任一形态——`type == "confirmation_required"`（扁平），或 `type == "confirmation" && subtype == "confirmation_required"`（typed）。只判 `type == "confirmation_required"` 会漏掉 typed 式。

## 字段位置速查

| 信息 | 扁平式 | typed 式 |
|------|--------|----------|
| 操作名 | `error.risk.action` | `error.action` |
| 风险级别 | `error.risk.level`（`risk` 是对象） | `error.risk`（字符串） |
| 确认 flag | `error.hint` | `error.hint` |

取操作名：typed 式看 `error.action`，没有再看扁平式的 `error.risk.action`（哪个有用哪个）。`hint` 是给你看的自然语言提示，里面写明了该加哪个确认 flag（扁平式如 "add --yes to confirm" → `--yes`；`config bind` 的 hint 提示 `--force`）。**提取那个 flag 加到你自己的原始命令上**，别照抄 hint 里的完整示例命令——示例不含用户的原始参数，照抄会丢参数。

## 先预览再执行（可选，不触发门禁）

想让用户先 review 危险请求，调用时加 `--dry-run`：它不触发确认门禁，会打印完整请求（URL / body / params），可把预览给用户看过再真正执行。

## 如何预判一条命令是高风险

- shortcut：`lark-cli <service> +<cmd> --help` 顶部显示 `Risk: high-risk-write`。
- service 命令：`lark-cli schema <service> <resource> <method> --format json` 返回值里 `"risk": "high-risk-write"`（schema 同时注入 `yes` 布尔字段标记需确认）。
- 注意：标注 `high-risk-write` ≠ 一定走 exit-10 门禁（如 `lark-cli update` 有 risk 标注但没有 `--yes` flag、不走该门禁）。以**实际 exit 10 + envelope** 为准，不要臆造 `--yes`。
