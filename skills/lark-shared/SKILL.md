---
name: lark-shared
version: 1.0.0
description: "首次配置 lark-cli、运行 auth login、用 --as 切换 user/bot 身份、处理权限不足或 scope 错误、遇到高风险写操作的确认门禁（exit 10 / confirmation）、更新 lark-cli、或看到 JSON 输出里的 _notice 时使用。"
---

# lark-cli 共享规则

通过 lark-cli 操作飞书资源的通用规则。正文是常驻核心；以下细节按需读取（`lark-cli skills read lark-shared references/<file>`）：

- **遇到 exit 10 / confirmation 错误** → 读 [`references/lark-shared-high-risk-approval.md`](references/lark-shared-high-risk-approval.md)（错误形态、识别、按 hint 重试、如何识别高风险）
- **要帮用户做 user 身份授权** → 读 [`references/lark-shared-auth-split-flow.md`](references/lark-shared-auth-split-flow.md)（split-flow 完整步骤）
- **拿到 `/wiki/` 链接或 wiki token** → 读 [`references/lark-wiki-token-routing.md`](references/lark-wiki-token-routing.md)（token 解包与按底层对象路由）

## 认证与身份

### 身份类型

两种身份，通过 `--as` 切换：

| 身份 | 标识 | 获取方式 | 适用场景 |
|------|------|---------|---------|
| user 用户身份 | `--as user` | `lark-cli auth login` 等 | 访问用户自己的资源（日历、云空间/云盘等） |
| bot 应用身份 | `--as bot` | 自动，只需 appId + appSecret | 应用级操作，访问 bot 自己的资源 |

### 身份选择原则

输出的 `[identity: bot/user]` 代表当前身份。bot 与 user 表现差异很大，需确认身份符合目标需求：

- **Bot 看不到用户资源**：无法访问用户的日历、云空间文档、邮箱等个人资源。例如 `--as bot` 查日程返回 bot 自己的（空）日历。
- **Bot 无法代表用户操作**：发消息以应用名义发送，创建文档归属 bot。
- **Bot 权限**：只需在飞书开发者后台开通 scope，无需 `auth login`。
- **User 权限**：后台开通 scope + 用户通过 `auth login` 授权，两层都要满足。

### 权限不足处理

遇到权限相关错误时，**根据当前身份采取不同方案**。错误响应中的关键字段（注意区分来源）：

- 缺失的 scope：`permission_violations`（原始 API 错误块，元素形如 `{subject: "<scope>"}`）；CLI 结构化错误里则是已抽取好的 `missing_scopes`（scope 字符串数组）。
- `console_url`：飞书开发者后台的权限配置链接。
- `hint`：建议的修复命令。

- **Bot 身份**：将 `console_url` 提供给用户（按下方「安全规则」的 URL 规则展示），引导去后台开通 scope。**禁止**对 bot 执行 `auth login`。
- **User 身份**：
  ```bash
  lark-cli auth login --domain <domain>           # 按业务域授权
  lark-cli auth login --scope "<missing_scope>"   # 按具体 scope 授权（推荐，最小权限）
  ```
  auth login 必须指定范围（`--domain` 或 `--scope`）；多次 login 的 scope 会累积（增量授权）。

### Agent 代理发起认证

优先用 split-flow（`--no-wait` 发起 → 展示给用户 → 后续轮 `--device-code` 完成），避免同轮阻塞。三条铁律：① 不在同一轮展示 URL 后立刻阻塞轮询 `--device-code`（交还控制权，等用户回来）；② `--device-code` 由你亲自执行，不要让用户自己跑；③ 不缓存 `verification_url` / `device_code`，每次需要授权都重新发起。授权 URL 的二维码展示按「安全规则」。完整步骤详见 [`references/lark-shared-auth-split-flow.md`](references/lark-shared-auth-split-flow.md)。

## 配置初始化

首次使用运行 `lark-cli config init --new`：帮用户初始化时**非阻塞启动**该命令、**持续读取输出**、从中提取授权链接发给用户（URL 的二维码展示按「安全规则」）。

## 更新检查

命令执行后若检测到新版本，JSON 输出会包含 `_notice.update`（字段：`current`、`latest`、`message`、`command`）。

**看到 `_notice.update` 时，完成用户当前请求后，主动提议更新**：

1. 告知用户当前版本和最新版本号（也可用 `lark-cli update --check` 只检查不安装）。
2. 提议执行 `lark-cli update`（同时更新 CLI 和 AI Skills）。
3. 更新完成后提醒：**退出并重新打开 AI Agent** 以加载最新 Skills。

不要静默忽略更新提示，即使当前任务与更新无关，也应在完成请求后补充告知。

## 安全规则

- **禁止输出密钥**（appSecret、accessToken）到终端明文。
- **写入/删除操作前必须确认用户意图**。
- 用 `--dry-run` 预览危险请求。
- **文件路径只接受相对路径**：`--file`、`--output`、`--output-dir`、`@file` 等路径参数只接受 cwd 下的相对路径，传绝对路径会报 `unsafe file path`。数据输入（`@file`、大 JSON）优先用 stdin，避免路径和转义问题。
- **输出任何授权 / 配置类 URL（`verification_url` / `verification_uri_complete` / `console_url` 等）时**：必须用 `lark-cli auth qrcode` 生成并展示二维码（URL 在前、二维码在后，不可跳过）；URL 视为 opaque string，不改写（不编码/解码、不加空格标点、不重拼 query）。

## 高风险操作的确认门禁（exit 10）

高风险写操作（`risk: "high-risk-write"`）未确认时，CLI **退出码 `10`**，并返回确认 envelope（`type` 为 `confirmation` / `confirmation_required`）。

**遇到 exit 10：绝不当普通错误放弃，绝不静默加 `--yes`。**

1. **停下**，把这次高风险操作和关键参数讲给用户，等其**显式同意**。
2. 同意后，从 envelope 的 `hint` 读出确认 flag（`--yes` / `--force`），以 argv 数组**追加到原始命令**重试——不写死 `--yes`，不用 `sh -c` 拼接。
3. 用户拒绝则终止。

**错误形态识别、`action` 字段位置、如何判断高风险、`--dry-run` 预览 → 详见 [`references/lark-shared-high-risk-approval.md`](references/lark-shared-high-risk-approval.md)。**