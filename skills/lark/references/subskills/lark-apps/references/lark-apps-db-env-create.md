# apps +db-env-create

把存量单库应用初始化为 `dev` / `online` 多环境数据库。运行时命令事实以 `lark-cli apps +db-env-create --help` 为准。

## 何时用

仅用于存量单库应用需要拆成 `dev` / `online` 两套数据库的场景。普通查看表、查 schema、执行 SQL 不需要先初始化。注意：通过 `+create --app-type full_stack` 新建的应用通常已自带多环境，无需再初始化（重复初始化会返回「已初始化」错误）。

## 命令骨架

- 必填：`--app-id`。
- `--env`：要创建的环境，由调用方传入，目前只支持 `dev`（默认 `dev`）。
- `--sync-data`：bool 开关，传 `--sync-data` 则把现有 online 数据复制到新环境；不传则不复制（默认）。
- risk 是 `high-risk-write`；单库拆成 dev/online 后不可逆。

## 示例

```bash
lark-cli apps +db-env-create --app-id app_xxx --env dev --dry-run
lark-cli apps +db-env-create --app-id app_xxx --env dev --sync-data --yes
```

## 输出契约

- 成功读取 `data.status`、`data.environments`、`data.data_synced`；pretty 会提示是否初始化、多环境列表、是否同步数据。
- 未确认时返回 `confirmation_required` / exit 10；按 lark-shared 询问用户后再补 `--yes` 重试。
- 如果服务端提示已启用多环境（`Multi-env is already initialized`），转述状态即可，不要重复初始化。

## Agent 规则

不要静默追加 `--yes`。遇到 confirmation_required 时，按 `lark-shared` 的 exit-10 协议向用户确认不可逆风险；用户明确同意后才在原 argv 末尾追加 `--yes` 重试。
