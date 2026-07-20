# apps +init

`+init` 初始化妙搭应用的代码（clone 仓库、scaffold/同步源码、拉取本地环境变量）。运行时命令事实以 `lark-cli apps +init --help` 为准。

## 何时用

用于把妙搭全栈应用源码拉到本地并准备开发环境。用户只是要云端 Agent 生成应用时，不要初始化本地仓库。

## 命令骨架

- 必填：`--app-id`。
- 可选：`--dir`，clone 目标目录；省略时默认 `./<app-id>`。
- 固定 checkout 分支：`sprint/default`。
- `+init` 会初始化 Git 凭证、clone 仓库、切到工作分支并生成/同步本地项目。

## 示例

```bash
lark-cli apps +init --app-id app_xxx --dir ./my-app
lark-cli apps +init --app-id app_xxx --dir /absolute/path/my-app
lark-cli apps +init --app-id app_xxx --dir ./my-app --dry-run
```

## 输出契约

- 真跑时 stdout 是 JSON envelope；stderr 会有 `->` / `→` 进度行。成功读 stdout，失败解析 stderr 末尾的 JSON 错误。
- 成功普通初始化读取 `data.clone_path`、`branch`、`committed`、`pushed`；`repository_url` 已脱敏，不要当凭据使用。
- `scaffold=already_initialized` 表示目录已初始化：跳过 clone/scaffold/commit，但仍会执行一次 env-pull 刷新本地环境变量（输出含 `env_pulled`，成功时含 `env_file`，失败时含 `env_pull_error` 且退出码仍为 0）；此时通常没有 `repository_url` / `branch`。
- `--dry-run` 只打印计划，不执行 git / npx；若输出含 `dir_error`，真跑前先让用户换目录。

## Agent 规则

- 目标目录必须不存在、为空目录，或已含 `.spark/meta.json` 且其 app_id 与 `--app-id` 一致的已初始化仓库。
- 目标目录已含 `.spark/meta.json` 时，`+init` 会跳过 clone/scaffold，但仍执行一次 env-pull 刷新本地环境变量；告知用户“仓库已初始化，本地环境变量已刷新，可直接开发”，不要误报失败或重复 clone。
- `+init` 输出没有必要原样复述；告诉用户 clone path、分支和下一步即可。
- 新建应用做本地初始化时，若选定的目标目录已存在，不要复用，改用一个不冲突的目录名（已预授权”放手做”时自动追加后缀如 `-2`；否则向用户确认目录名）。
