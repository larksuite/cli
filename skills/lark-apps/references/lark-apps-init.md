# apps +init

`+init` 初始化妙搭应用的本地开发仓库。运行时命令事实以 `lark-cli apps +init --help` 为准。

## 何时用

用于把妙搭全栈应用源码拉到本地并准备开发环境。用户只是要云端 Agent 生成应用时，不要初始化本地仓库。

## 命令骨架

- 必填：`--app-id`。
- 可选：`--dir`，clone 目标目录；省略时默认 `./<app-id>`。agent 应先给出目录选项让用户选，不要只要求用户手动输入路径。
- 可选：`--template`，空仓库脚手架模板；省略时当前回退 `nestjs-react-fullstack`。
- 固定 checkout 分支：`sprint/default`。
- `+init` 会初始化 Git 凭证、clone 仓库、切到工作分支并生成/同步本地项目。

## 示例

```bash
lark-cli apps +init --app-id app_xxx --dir ./my-app
lark-cli apps +init --app-id app_xxx --dir /absolute/path/my-app --template nestjs-react-fullstack
lark-cli apps +init --app-id app_xxx --dir ./my-app --dry-run
```

## 目录选择话术

用户没给目录时，不要让用户手动输入路径；直接给选择：

```text
接下来要把源码拉到本地，请选一个目录：
1. ./todo-app（推荐）
2. ./app_xxx（CLI 默认）
3. 自定义路径
```

用户选 1/2 后直接执行对应 `--dir`；选 3 后再请用户给路径。

## 输出契约

- 真跑时 stdout 是 JSON envelope；stderr 会有 `->` / `→` 进度行。成功读 stdout，失败解析 stderr 末尾的 JSON 错误。
- 成功普通初始化读取 `data.clone_path`、`branch`、`committed`、`pushed`；`repository_url` 已脱敏，不要当凭据使用。
- `scaffold=already_initialized` 表示已初始化 no-op；此时通常没有 `repository_url` / `branch`。
- `--dry-run` 只打印计划，不执行 git / npx；若输出含 `dir_error`，真跑前先让用户换目录。

## Agent 规则

- 不要在未确认目录时默认 clone 到当前工作区根；进入目录选择时给 2-3 个选项，推荐项放第一位，并保留“自定义路径”。例如：1. `./<app-name>`（推荐，语义清晰）；2. `./<app-id>`（CLI 默认）；3. 自定义路径。
- 如果客户端支持按钮/选择器，优先用可点击选项；否则用编号选项让用户回复序号即可。不要只问“你想 clone 到哪个目录？”让用户手动拼路径。
- 目标目录必须不存在、为空目录，或已含 `.spark/meta.json` 的已初始化仓库。
- 目标目录已含 `.spark/meta.json` 时，`+init` 会友好 no-op；告知用户“仓库已初始化，可直接开发”，不要误报失败或重复 clone。
- `+init` 输出没有必要原样复述；告诉用户 clone path、分支和下一步即可。
