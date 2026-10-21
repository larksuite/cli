# apps +features / +feature-enable

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)（认证 / 全局参数 / 安全）。

查询和启用应用业务能力。本期只有 `database`（应用的数据库，PostgreSQL）。运行时命令事实以 `lark-cli apps +features --help`、`lark-cli apps +feature-enable --help` 为准。

## 心智模型

- `full_stack` 是工程与发布契约，**不等于已有数据库**。应用可以先没有数据库，需要保存业务数据时再显式启用。
- `+features` 只读（`spark:app:read`）：每个能力只返回 `name` 和 `enabled`。不申请资源、不返回连接串；`enabled=true` 只表示服务端已开启，**不代表本地开发已就绪**。
- `+feature-enable --feature database` 是服务端写操作（声明 `spark:app:read` + `spark:app:write`）：只需 `--app-id` 和 `--feature database`（本期只支持 `database`），CLI 自动生成 `idempotent_key` 并只提交它。成功只表示**服务端能力已开启**，生效方式看输出的 `next_steps`：本地开发需 `+env-pull` 并重启完整本地开发进程；在妙搭沙箱里由平台自动生效，不需要 `+env-pull`。两种情况都要再做一次真实读写验证。
- 它不改本地业务代码、不拆已有数据库、不推代码、不发布，也不会重启任何本地进程。

## 何时用

- 新建或接手的应用要开始保存业务数据：先 `+features`，`database` 为 `enabled=false` 时，经用户确认数据库需求后执行 `+feature-enable --feature database`。
- 网页或另一台电脑已开通数据库：不需要 `+feature-enable`，直接 `+env-pull` 并重启本地进程。
- `+features` 没列出 `database`、命令报错，或 `+feature-enable` 被服务端拒绝：该应用不走本入口，转述错误信息；需要数据库时按 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md) 经妙搭云端会话添加。

## 示例

```bash
lark-cli apps +features --app-id <app_id>

lark-cli apps +feature-enable --feature database --app-id <app_id> --dry-run
lark-cli apps +feature-enable --feature database --app-id <app_id>

# 上一次失败/超时后，用错误提示里的原 idempotent_key（示例值需替换）续同一次请求或查询
lark-cli apps +feature-enable --feature database --app-id <app_id> --idempotent-key b3d93913-e777-4968-a00c-afd0a1c7ce44
lark-cli apps +features --app-id <app_id> --operation-ref b3d93913-e777-4968-a00c-afd0a1c7ce44
```

## 结果判定

| 结果 | 含义 | 下一步 |
|---|---|---|
| `ok:true`，`feature.enabled=true` | 服务端已开启数据库（含早已开启、无 `operation` 的情况） | 按 `next_steps`：本地在项目里执行 `+env-pull`，停掉并重启 `npm run dev`；妙搭沙箱里平台自动生效，稍候即可。最后都验证一次真实读写 |
| `+env-pull` 报 dev 数据库环境未就绪（`next_action=db_env_create`） | 应用只有 online 单库，没有 dev | **不能当作本地数据库已就绪**，不要用线上库做本地开发；拆分 dev 是不可逆操作，只有用户明确确认后才按 [`lark-apps-db.md`](lark-apps-db.md) 先 `+db-env-create --environment dev --dry-run` |
| exit 1，`subtype=failed_precondition`，提示应用不是新契约应用 | 该应用不能通过 CLI 开数据库 | 按 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md) 经妙搭云端会话添加；重试本命令没有用 |
| exit 1，`subtype=failed_precondition`，提示 `idempotent_key` 已不可用 | 该 key 已作废：被后续操作取代、不属于当前调用者或不存在 | 先 `+features` 看当前状态，已开启就不用再开；仍需开启就不带 `--idempotent-key` 重新执行（CLI 生成新 key）；不要用同一个 key 重试 |
| exit 1，`subtype=conflict` | 能力被并发修改、key 属于其他请求，或另一次开启仍在进行 | 转述 `error.message` / `hint`；重新 `+features` 看当前状态再决定，CLI 不会自动覆盖 |
| exit 1，`subtype=rate_limit` | 被限流，本次请求未执行。首次请求被限流，或服务端已确认这个 key 没有操作时，没有可续跑的东西；网络重放或带原 key 续跑时被限流，这个 key 可能已有操作 | 稍等约一分钟后按 `hint` 处理：`hint` 要求不带 `--idempotent-key` 重新执行时就换新 key；`hint` 给出原 key 时，先用 `--operation-ref` 查询，再用原 `--idempotent-key` 续跑，不要换新 key |
| exit 1，`subtype=feature_not_available`，提示服务端没有运行应用能力接口 | 当前服务端还不支持应用能力：网关内部错误 2200，或网关还没有这条路由、直接回 HTTP 404（`404 page not found`），只读复查结果相同；只有开通请求被回 404 时也按这一行处理 | 不要重试；需要数据库时按 [`lark-apps-cloud-dev.md`](lark-apps-cloud-dev.md) 经妙搭云端会话添加，或等服务端升级 |
| exit 3，身份或授权失效（如 `token_expired`；妙搭沙箱里用户凭证过期返回 221801，凭证无效时代理直接回 HTTP 401） | 当前用户凭证不可用，任何 key 都不会成功 | 按 `hint` 让用户重新授权或登录后再执行；不要用同一个 `--idempotent-key` 反复重试 |
| exit 1 / 4 / 5，结果失败、未知或无法核实 | 请求可能已执行，也可能没有 | 按 `hint` 先用 `--operation-ref <idempotent_key>` 查询；要续同一次请求用原 `--idempotent-key`；结果未知时不要换新 key 重新申请 |

带 `--idempotent-key` 时，CLI 先按同一 key 查询服务端保留的操作：已成功、结果未知或冲突直接报告，进行中则等待，只有可重试的失败才用同一 key 重发；只有服务端明确答复不保留该 key（`operation_unavailable`）时，才报错并提示先看 `+features`、确有需要再发起新操作；查询本身失败（权限、服务端错误、网络）时保留实际错误，结果按未知处理，只能用同一 key 重试或查询，不要换新 key。两种情况都不会自动重发或换 key。

`running` 只在服务端确实异步接手时出现，CLI 最多等待 60 秒；`conflicted` 等终态立即返回，不会继续等待。超时或中断不会取消或回滚服务端操作。错误的 `hint` 会给出原 `idempotent_key` 与查询命令，不含连接信息。

## 本地生效

`+env-pull` 只按服务端返回的环境变量写 `.env.local`，不查询 `+features`，输出也**不代表** `database` 能力是否开启（业务开关只看本命令的 `enabled`）：

- 本地数据库集成被禁用：仅当返回 `FORCE_FRAMEWORK_DISABLE_DATAPASS="true"` 且 `SUDA_DATABASE_URL`、`FORCE_DB_CONNECT_URL` 两个键都存在且为空时，把这两个键与 `FORCE_DB_BRANCH` 写成空值；缺任一键不清除本地旧值。
- 已导出数据库连接：`FORCE_FRAMEWORK_DISABLE_DATAPASS="false"` 且非空 `SUDA_DATABASE_URL` 时 `FORCE_DB_BRANCH=dev`；不代表数据库真实可连通。
- 以上两种情况都输出 `restart_required=true`，需重启完整本地开发进程并单独验证一次真实读写；其他返回形态按原有行为合并，只固定 `FORCE_DB_BRANCH=dev`。
- 用户自定义变量和已有 Redis（`MIAODA_CACHE_URL`）保留；拉取失败时不改 `.env.local`。
- 输出含 `shell_conflicts` 时，当前 shell 导出了不同的平台值，会压过 `.env.local`：按 stderr 提示 `unset` 这些变量或换新 shell，再重启本地进程。只重写文件不代表已生效。

## 参考

- [lark-apps-local-dev](lark-apps-local-dev.md) — 本地开发端到端流程
- [lark-apps-env-pull](lark-apps-env-pull.md) — 拉取本地环境变量
- [lark-apps-db](lark-apps-db.md) — 数据库命令与多环境
