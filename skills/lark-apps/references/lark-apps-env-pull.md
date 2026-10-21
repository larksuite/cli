# apps +env-pull

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)（认证 / 全局参数 / 安全）。

把妙搭应用 dev 启动期环境变量拉取到本地项目根的 `.env.local`。身份固定 `--as user`；scope `spark:app:read`。`--app-id` 必填，目标项目根默认当前工作目录（`--project-path` 可指定）。

这个命令是 dev-only 的本地恢复工具：内部固定 `POST env_vars`，body 为 `env=dev`。它没有 `--env` flag，也不管理线上环境变量。

## 何时别用（核心反模式）

**通常不需要手动跑**——脚手架的 `npm run dev` 在起本地开发时会自动后台拉取（非阻塞）。手动再跑会重复做同样的事，并用服务端返回值覆盖 `.env.local` 里的同名 key；本地无关行和注释会保留。

只在这些兜底场景用：

- 不通过 `npm run dev` 启动（直接跑 `node` / IDE debug）。
- `.env.local` 被改坏 / 删除，想重新同步。
- 刚用 `+feature-enable --feature database` 开了数据库，或网页 / 另一台电脑改了数据库能力：拉取后重启本地开发进程。

## 行为

- **合并、不清空**：写入 `.env.local` 时保留你手写的内容与注释——命中的 key 替换值，新 key 追加，不整体覆盖。
- **安全护栏**：返回的 envelope **不会回显任何 env value**（防止 token / 数据库凭据泄漏到日志或 CI 输出）。要看实际值请直接读 `.env.local`。
- **数据库**：输出只描述拉取到的环境变量，**不代表应用 `database` 能力是否开启**（旧应用的自定义配置可能带同名键）；业务开关只看 `+features` 的 `name` / `enabled`。
  - 本地数据库集成被禁用：仅当服务端返回 `FORCE_FRAMEWORK_DISABLE_DATAPASS="true"`，且 `SUDA_DATABASE_URL`、`FORCE_DB_CONNECT_URL` **两个键都存在且为空**时，才把这两个键和 `FORCE_DB_BRANCH` 写成空值，清掉残留连接。缺任一键都不算显式清除，`.env.local` 里的旧值保留。
  - 已导出数据库连接：`FORCE_FRAMEWORK_DISABLE_DATAPASS="false"` 且非空 `SUDA_DATABASE_URL` 时，`FORCE_DB_BRANCH` 固定为 `dev`。这只表示导出了连接，**不代表数据库真实可连通**。
  - 以上两种情况输出 `restart_required=true`；其他返回形态按原有行为合并，只固定 `FORCE_DB_BRANCH=dev`。
  - 应用只有 online 单库、没有 dev 数据库环境时，服务端直接报错（`next_action=db_env_create`），CLI 原样转述，不改 `.env.local`；不要用线上库做本地开发。
  - 用户自定义变量与已有 Redis（`MIAODA_CACHE_URL`）保留；任何失败都不改 `.env.local`。
- **写文件不等于生效**：已运行的本地进程不会读取新值，必须停掉并重启完整本地开发进程，再单独验证一次真实读写。输出含 `shell_conflicts`（只列 key）时，当前 shell 导出的同名平台变量会压过 `.env.local`，先 `unset` 或换新 shell 再重启。
- `+env-pull` 只读取已有能力，不会创建或重新开启数据库；需要数据库时见 [`lark-apps-features.md`](lark-apps-features.md)。

## 示例

```bash
lark-cli apps +env-pull --app-id <app_id>
```

## 失败处理

`missing_scope`（没拿到 `spark:app:read`）时，按 lark-shared 引导 `lark-cli auth login --domain apps`。其余失败优先转述 `error.hint` / `error.message`。

## 参考

- [lark-apps](../SKILL.md) — 妙搭应用全部命令 + 心智模型
- [lark-apps-local-dev](lark-apps-local-dev.md) — 本地应用开发端到端流程
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
