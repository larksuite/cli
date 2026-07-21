# apps Git credential

妙搭 Git 凭证用于本地原生 `git clone/pull/push`。运行时命令事实以 `lark-cli apps +git-credential-init --help`、`+git-credential-list --help`、`+git-credential-remove --help` 为准。

## 命令

```bash
lark-cli apps +git-credential-init --app-id app_xxx
lark-cli apps +git-credential-list
lark-cli apps +git-credential-remove --app-id app_xxx
```

## 输出契约

- `+git-credential-init` 成功后读取 `data.repository_url`；不要展示或保存其中的凭据细节，只用于下一步 `git clone`。响应还包含 `data.commit_author_name` 和 `data.commit_author_email`，这两个字段由 `+init` 内部消费，自动写入仓库 repo-local git config（`user.name` / `user.email`），agent 和用户无需手动配置。
- `+git-credential-list` 返回本地记录和状态；可用来判断是否需要重新 init。
- `+git-credential-remove` 只清本地配置；成功后告知不会删除云端应用或仓库。

## 行为规则

- `+git-credential-init` 返回 `repository_url`，并配置 URL-scoped Git credential helper。后续 clone/pull/push 使用原生 git。
- `+git-credential-list` 列出本地已配置的妙搭 Git 凭证，不需要 `--app-id`。
- `+git-credential-remove` 只移除本地凭证/helper，不删除云端应用或仓库。
- 看到 Repository URL 后继续：

```bash
git clone <repository_url>
cd <repo>
git checkout sprint/default
```

## Agent 规则

- 不要手动打印、保存或拼接 token。
- clone、pull、push、diff、log 等代码仓库操作都使用原生 `git`；不存在 `apps +pull` / `apps +push` / `apps code +read` 这类代码读写 shortcut，不要臆造。
- 不要 push/force-push `main`；`main` 是发布态快照，由 `apps +release-create` 成功后服务端推进，直推/force-push 会被服务端护栏拒绝。
- Git 认证失败、本地凭证损坏或 helper 缺失时，重新执行 `+git-credential-init --app-id <id>` 覆盖本地配置；不要让用户复制 token 到 remote URL。
