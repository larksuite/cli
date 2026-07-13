# apps +release-create

为妙搭应用创建发布 release。运行时命令事实以 `lark-cli apps +release-create --help` 为准。

## 何时用

用于把全栈应用的代码分支推进到发布流程。它不是 HTML 静态发布入口；本地 `index.html` / `dist` 要读 [`lark-apps-html-publish.md`](lark-apps-html-publish.md)。

## 命令骨架

- 必填：`--app-id`。
- 可选：`--branch`；省略时服务端使用默认发布分支。
- 返回 `release_id` 和 `status`，后续用 `+release-get` 轮询。

## 示例

```bash
lark-cli apps +release-create --app-id app_xxx
lark-cli apps +release-create --app-id app_xxx --branch sprint/default --dry-run
```

## 输出契约

- 成功读取 `data.release_id`、`data.status` 和 `data.sync`；`release_id` 是后续 `+release-get` 的入参。
- `sync=true` 表示同步部署（服务端等待部署完成后才返回），`sync=false` 或缺失表示异步部署。
- `status=publishing` 表示发布仍在进行；继续用 `+release-get` 轮询，轮询间隔应该为 20s。应用发布平均耗时大约 2min，整体超时时间大约 5min。
- `status=finished` 表示部署已完成（同步部署时可能直接返回此状态）。
- `+release-create` 返回 release 只代表发布已发起。只有 `+release-get` 对同一个 `release_id` 返回 `finished` 后，才能说本轮最新版本已部署。

## Agent 规则

`+release-create` 部署的是远端 `sprint/default` 上已 push 的代码，不是本地工作区——本地若有你修改但未推送的改动，需要先 `git add` + `git commit` 并 `git push` 到 `sprint/default`，否则这些改动不会进入这次发布。发布后若 status 是 `publishing`，用 [`+release-get`](lark-apps-release-get.md) 查询。`+release-create` 部署上线属高影响动作——作为别的命令的连带前置时，按 SKILL.md「高影响动作：确认与预授权」先征得用户同意再发布。
