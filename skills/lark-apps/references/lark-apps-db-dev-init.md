# apps +db-dev-init

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

> [!CAUTION] **不可逆操作**：应用数据库一旦从单库拆分为 `online` / `dev` 双环境，**无法撤销**。命令默认带确认关卡——不带 `--yes` 且非 `--dry-run` 时直接拒绝执行。**禁止在脚本 / 自动化场景下静默调用。**

为妙搭应用启用 `dev` 开发环境（将单一数据库拆分为 `online` 线上库与 `dev` 开发库）。

## 命令

```bash
# 启用 dev 环境（仅创建空的 dev 库，不复制线上数据）
lark-cli apps +db-dev-init --app-id app_xxx --yes

# 启用 dev 环境并把线上数据复制到 dev 库
lark-cli apps +db-dev-init --app-id app_xxx --sync-data --yes

# pretty 摘要
lark-cli apps +db-dev-init --app-id app_xxx --sync-data --yes --format pretty

# 预演（仅打印将发送的请求，不执行，无需 --yes）
lark-cli apps +db-dev-init --app-id app_xxx --sync-data --dry-run
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `--app-id <str>` | ✅ | — | 妙搭应用 ID |
| `--sync-data` | ❌ | `false` | 是否把线上库现有数据复制到新建的 dev 库 |
| `--yes` | ✅\* | — | 确认执行不可逆操作；\*非 `--dry-run` 时必填 |

## 返回值

**成功（JSON 默认 envelope）：**

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "status": "initialized",
    "environments": ["online", "dev"],
    "data_synced": true
  }
}
```

**成功（`--format pretty`）：**

```
✓ Multi-env initialized
Environments: online, dev
Data synced: yes
Note: structure changes in dev now need to be released to online.
```

**失败（未确认）：** 缺 `--yes` 时返回 `type: confirmation_required`（退出码 `10`）。这是给自动化识别"需人工确认"的信号——补 `--yes` 后重试：

```json
{
  "ok": false,
  "identity": "user",
  "error": {
    "type": "confirmation_required",
    "message": "apps +db-dev-init requires confirmation",
    "hint": "add --yes to confirm",
    "risk": { "level": "high-risk-write", "action": "apps +db-dev-init" }
  }
}
```

**失败（应用已启用多环境）：** 返回 `type: api_error`，`code` 与 `message` 由服务端给出：

```json
{
  "ok": false,
  "identity": "user",
  "error": { "type": "api_error", "code": 500002511, "message": "Multi-env is already initialized" }
}
```

## 字段语义

- `environments`：启用后存在的环境列表，固定为 `online`（线上）与 `dev`（开发）。
- `data_synced`：是否真的复制了线上数据，由 `--sync-data` 决定。
- 拆分双环境后，在 `dev` 改的结构需要**发布**到 `online` 才生效（pretty 摘要末行已提示）。
- 失败时优先转述 `error.hint`（为空则 `error.message`），不要原样复述 envelope JSON。

## 协同命令

| 场景 | 命令 |
|---|---|
| 拆库后在 dev 改结构 | `apps +db-sql --app-id <id> --env dev --query "ALTER TABLE ..."` |
| 查看某环境的表 | `apps +db-table-list --app-id <id> --env dev` |

## 参考

- [lark-apps](../SKILL.md)
- [lark-shared](../../lark-shared/SKILL.md)
