# lark-cli apps Git credential

用于为妙搭应用仓库初始化本机 Git HTTP 凭证。该能力只处理 Git credential，不负责 `apps setup`、环境变量拉取或仓库内容初始化。

## 命令

```bash
lark-cli apps +git-credential-init --app-id app_xxx
lark-cli apps +git-credential-list
lark-cli apps +git-credential-remove --app-id app_xxx
```

隐藏 helper 由 Git 自动调用，不要让用户手动执行：

```bash
lark-cli apps git-credential-helper --app-id app_xxx get
lark-cli apps git-credential-helper --app-id app_xxx store
lark-cli apps git-credential-helper --app-id app_xxx erase
```

## 身份和权限

- 统一使用 `--as user`；`+git-credential-init` 需要 user scope：`spark:app:read`。
- `+git-credential-list` / `+git-credential-remove` 只处理本机 metadata、keychain 和 Git config，不调用远端 API。
- init 缺少权限时，先引导用户执行：

```bash
lark-cli auth login --scope "spark:app:read"
```

- `spark:app:read` 只允许 CLI 调用签发 Git 凭证接口；原生 `git push` 使用服务端签发的妙搭仓库 PAT，是否可 push 由服务端按用户对 app 的开发/写入权限决定。
- PAT 是服务端签发的妙搭仓库短期凭证，有效期按服务端 24 小时策略控制；`+git-credential-init`、Git helper 和 `+git-credential-list` 都不展示具体过期时间；任何输出都不会展示 PAT。

## 本地存储和 Git 配置

`lark-cli apps +git-credential-init` 成功后会写入 app 维度的本地状态，位置在当前 CLI 配置目录下的妙搭 app storage：

```text
<LARKSUITE_CLI_CONFIG_DIR>/spark/<escaped-appID>/
```

其中包含：

- `git.json`：当前 app 的 Git 非密钥元数据，记录 app_id、仓库 URL、profile、登录用户、PAT 引用和内部过期时间，用于后续 Git helper 精确匹配和刷新。这个文件只描述当前 app，不是所有应用的集合。
- PAT 本体：复用 CLI 现有 `internal/keychain`，以 `service=lark-cli`、`account=PATRef` 存取。macOS 下 PAT 会被 master key 加密后写入 `~/Library/Application Support/lark-cli/app-git-pat_<hash>.enc`；系统钥匙串中保存的是 master key，不是 PAT 明文。PAT 不写入 JSON 明文，也不拼进 remote URL。

同时会写入 URL 粒度的 Git 全局配置：

```ini
[credential "https://example.com/git/u_abc/app.git"]
	helper = !lark-cli apps git-credential-helper --app-id 'app_xxx'
	useHttpPath = true
```

这里使用仓库 URL 粒度即可，因为妙搭 Git URL 已经是仓库唯一地址；Git 只认识 credential URL section，不支持把 profile、user_open_id 这类业务维度写进 section 名。`--app-id` 由 init 时固化进 helper 配置，并做 shell-safe quoting；业务维度由 app-scoped metadata 做二次校验。PAT 隔离维度是 user_open_id + app_id。

## Git 版本要求

先检查本机 Git 版本：

```bash
git --version
```

- 最低版本：Git `2.0.5` 或更高版本；该版本已经支持 credential helper 和 `credential.useHttpPath`，满足本流程的基础凭证注入与路径级隔离要求。
- 推荐版本：当前稳定版 Git；如果环境需要通过 `credential.interactive=false` 强化无交互体验，推荐 Git `2.48.0` 或更高版本。
- 低于 Git `2.0.5` 时，不推荐使用该能力；credential helper 与路径级凭证隔离能力不满足本流程假设。
- 无论版本如何，都不要把 PAT 拼进 remote URL；凭证必须通过 `lark-cli apps git-credential-helper` 和 URL scoped Git config 注入。

如果 `git --version` 提示未安装，或版本低于 Git `2.0.5`：

1. 按系统安装或升级 Git。
   - macOS：优先使用 `brew install git` 或 `brew upgrade git`。
   - Windows：从 <https://git-scm.com/download/win> 安装最新版 Git for Windows。
   - Linux：优先使用系统包管理器，例如 `apt install git`、`dnf install git` 或 `yum install git`；如果发行版源版本过旧，改用 Git 官方安装说明。
2. 重新打开终端，确认 `git --version` 输出为 `2.0.5` 或更高版本。
3. 再执行 `lark-cli apps +git-credential-init --app-id <app_id>` 初始化妙搭 Git 凭证。

## 初始化流程

已有 `app_id` 时：

```bash
lark-cli apps +git-credential-init --app-id app_xxx
```

成功输出会包含 `Repository URL` 和下一步 `git clone <url>`。后续必须使用原生 Git：

```bash
git clone <Repository URL>
git fetch
git pull
git push
```

如果用户要新建妙搭应用再本地开发，先创建应用拿到 `app_id`，再初始化 Git 凭证：

```bash
lark-cli apps +create --name "<name>" --app-type HTML
lark-cli apps +git-credential-init --app-id <app_id>
git clone <Repository URL>
```

如果用户要操作已有妙搭应用，优先让用户提供 `app_id` 或妙搭应用 URL；不要依赖应用枚举来猜测目标应用。

## 修复

Git 认证失败、密文缺失或本地配置损坏时，重新执行 init 覆盖本地配置：

```bash
lark-cli apps +git-credential-init --app-id app_xxx
```

## 查看

列出本机已经初始化过的所有妙搭 app Git credential：

```bash
lark-cli apps +git-credential-list
```

该命令不需要 `--app-id`，会自动扫描当前 CLI 配置目录下的所有 app storage：

```text
<LARKSUITE_CLI_CONFIG_DIR>/spark/<escaped-appID>/git.json
```

输出包含 `app_id`、仓库 URL 和状态。状态由本地 metadata 与 keychain PATRef 一起判断，可能为 `valid`、`refresh_required`、`invalidated`、`missing_secret` 或 `incomplete`。PAT 本体和过期时间不会出现在输出中。

## 删除

删除本地 app-scoped metadata、keychain PAT 和对应 Git config：

```bash
lark-cli apps +git-credential-remove --app-id app_xxx
```

找不到本地记录也视为清理完成。已签发的旧 PAT 不会被本地命令踢出，会在服务端过期时间到达后自然失效。
