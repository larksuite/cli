# 高风险操作的确认门禁（exit 10）

lark-cli 对高风险写操作（`risk: "high-risk-write"`）有强制确认门禁。不带确认标志调用这类命令时，CLI 退出码 `10`，并在 stderr 返回结构化 envelope。

> 正文已给出安全默认（停下、绝不静默 `--yes`、从 `hint` 取 flag 追加到原始 argv 重试）。本文件是机制细节，遇到 exit 10 时按需读取。

## 关键：可靠信号是退出码 10，不是 type 字符串

仓库正在从「扁平错误」迁移到「typed 错误」，同一个门禁可能以两种形态出现，但**都以 exit 10 为信号**：

- **扁平式**（service / shortcut 命令，旧形态）：

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

- **typed 式**（如 `config bind`，新形态）：

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

只用 `error.type == "confirmation_required"` 判断会**漏掉 typed 式**。正确识别：**子进程 exit code = 10**，且 `error` 命中任一：

- `type == "confirmation_required"`（扁平），或
- `type == "confirmation" && subtype == "confirmation_required"`（typed）。

## 处理流程

1. **识别**：exit code = 10 且命中上述任一形态。
2. **向用户确认**：把操作名和关键参数展示给用户，明确告知"这是高风险操作"，等待显式同意。
   - 操作名位置随形态而异：typed 在 `error.action`；扁平在 `error.risk.action`。取 `error.action || error.risk.action`。
   - 注意 `error.risk` 形态也不同：扁平是对象 `{level, action}`，typed 是字符串（如 `"high-risk-write"`）。
3. **用户同意 → 从 `hint` 读出确认 flag，追加到原始 argv 后重试**。`hint` 是给 Agent 看的自然语言提示，写明了该用哪个 flag——**提取那个 flag（如 `--yes` / `--force`）追加到你的原始命令**，不要写死 `--yes`，也**不要照抄 hint 里的示例命令**（示例不含用户原始参数，照抄会丢参数）：
   - 扁平式：`hint = "add --yes to confirm"` → 原始 argv 末尾追加 `--yes`。
   - typed 式（bind）：`hint` 提示用 `--force` → 原始 argv 末尾追加 `--force`。
4. **用户拒绝 → 终止流程**，不擅自改写参数或跳过门禁。

## 绝对不允许

- 看到 exit 10 就默认加 `--yes` 静默重试（等于禁用门禁）。
- 把 `confirmation` / `confirmation_required` 当网络错误 / 权限错误处理。
- 用户没明确同意就追加确认 flag 重试。
- 用 `sh -c` 等 shell 方式拼接命令重试——用 `exec.Command(argv...)` 参数数组形式，避免 shell 解析把用户参数当作语法。

## 提前预览（不触发门禁）

想先让用户 review 危险操作的具体请求，调用时加 `--dry-run`——它不触发门禁，会打印完整请求详情（URL / body / params），可把预览给用户看过再真正执行。

## 如何识别一条命令是高风险

- shortcut：`lark-cli <service> +<cmd> --help` 顶部显示 `Risk: high-risk-write`。
- service 命令：`lark-cli schema <service>.<resource>.<method> --format json` 返回值里 `"risk": "high-risk-write"`（同时 schema 会注入 `yes` 字段标记需确认）。
- 注意：被标注 `high-risk-write` ≠ 一定走 exit-10 门禁。例如 `lark-cli update` 标了 risk 但没有 `--yes` flag、不走该门禁——以**实际 exit 10 + envelope** 为准，不要臆造 `--yes`。
