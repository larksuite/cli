# 高风险操作的审批协议（exit 10）

lark-cli 对高风险写操作（`risk: "high-risk-write"`）有强制确认门禁。当缺少命令要求的确认 flag（通常是 `--yes`）时，CLI 会退出码 `10`、并在 stderr 返回如下结构化 envelope：

```json
{
  "ok": false,
  "identity": "bot",
  "error": {
    "type": "confirmation",
    "subtype": "confirmation_required",
    "message": "drive +delete requires confirmation",
    "hint": "add --yes to confirm",
    "risk": "high-risk-write",
    "action": "drive +delete"
  }
}
```

**遇到这种情况，不要当普通错误放弃。** 按以下流程处理：

1. **识别**：看到子进程 exit code = `10` 且 stderr JSON 里 `error.type == "confirmation"`、`error.subtype == "confirmation_required"`
2. **向用户确认**：把 `error.action`、`error.risk` 和关键参数展示给用户，明确告知"这是高风险操作"，等待用户显式同意
3. **用户同意** → 按 `error.hint` 确定确认 flag，并追加到你**自己的原始 argv** 后重试。多数命令使用 `--yes`
4. **用户拒绝** → 终止流程，不要擅自改写参数或跳过门禁

**绝对不允许**：
- 看到 exit 10 就默认加确认 flag 静默重试（这等于禁用门禁）
- 把 `confirmation_required` 当网络错误/权限错误处理
- 在用户没明确同意的前提下追加确认 flag 重试
- 用 `sh -c` 等 shell 方式拼接命令重试——用参数数组（argv）形式传参，避免 shell 解析把用户参数当作语法

提前预判：想先让用户 review 危险操作的具体请求，且目标命令支持 `--dry-run` 时，调用时加 `--dry-run`——它不触发确认门禁，会打印完整请求详情（URL / body / params），你可以把这个预览给用户看过再去真正执行。

## 如何识别一条命令是高风险

- shortcut：`lark-cli <service> +<cmd> --help` 顶部会显示 `Risk: high-risk-write`
- service 命令：`lark-cli schema <service>.<resource>.<method> --format json` 的返回值里 `"risk": "high-risk-write"`
