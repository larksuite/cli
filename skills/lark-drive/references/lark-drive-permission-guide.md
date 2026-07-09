# Drive 权限与授权指南

> 前置条件：通用认证、scope 与 `--as` 规则见 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

## 何时读取

- 用户要修改文档公开权限，尤其是 `drive permission.public patch` 返回 `91009` / `91010` / `91011` / `91012`。
- 用户要给文档、文件、文件夹、Wiki 或 slides 增加协作者权限，或把访问权限授予当前应用（bot）自身。
- 用户遇到 `permission denied`，但错误表现更像租户对外分享、安全策略或密级拦截，而不是普通 scope 缺失。

如果用户只是想向文档 owner 申请访问权限，优先使用 [`lark-drive-apply-permission.md`](lark-drive-apply-permission.md)。

## 公开权限修改前门槛

公开权限修改是高风险写操作。执行 `drive permission.public patch --yes` 前同时确认：

| 条件 | 可执行信号 |
|------|------------|
| 具体目标 | 单个 URL/token，或用户确认过的资源列表 |
| 公开范围 | 用户明确选择组织内/互联网、可读/可编辑等具体 `link_share_entity` 档位 |
| 执行确认 | 用户在本轮确认按该目标和范围执行 |

“开放一下”“共享给大家”“让大家能看”只表达目标状态，不包含具体公开范围。先列出可选范围并停止等待用户选择；公开档位必须来自用户选择，CLI 的 `--yes` 只表示已获得用户对该档位的执行确认。

## 公开权限错误码

调用 `lark-cli drive permission.public patch` 更新文档公开权限失败时，如果返回以下错误码，按表格给用户明确下一步。不要把这些错误简单归类为缺少 scope；它们通常表示租户、对外分享或文档密级策略拦截。

| 错误码 | 含义 | 给用户的引导 |
|--------|------|--------------|
| `91009` | 对外分享被租户安全策略管控，当前用户无法开启 | 提示用户：对外分享能力被租户安全策略统一管控，无法通过 API 或当前用户直接开启；需要联系租户管理员调整组织级对外分享策略。 |
| `91010` | 文档对外分享未打开 | 提示用户：当前文档尚未打开对外分享，请先在文档权限设置中打开对外分享，再重试 `permission.public.patch`。 |
| `91011` | 对外分享被文档密级管控 | 提示用户：对外分享被密级策略拦截，需要打开目标文档，在文档内发起密级豁免或进行密级降级后再重试；回复中必须给出目标文档 URL。 |
| `91012` | 权限设置被文档密级管控 | 提示用户：该权限设置被密级策略拦截，需要打开目标文档，在文档内发起密级豁免或进行密级降级后再重试；回复中必须给出目标文档 URL。 |

当用户最初提供的是文档 URL，遇到 `91011` 或 `91012` 时直接把该 URL 原样返回给用户作为操作入口；如果上下文只有 token，需要先尽量通过已有上下文、搜索结果或元数据恢复目标文档 URL，再给出可点击的文档 URL。

## 授权当前应用访问文档

需要将文档权限授予当前应用（bot）自身时：

1. 先执行 `lark-cli api GET /open-apis/bot/v3/info --as bot`，从返回值取 `bot.open_id`。
2. 再调用 `lark-cli drive permission.members create`，用 `member_type=openid`、`member_id=<bot_open_id>` 授权。

```bash
lark-cli drive permission.members create \
  --params '{"token":"<doc_token>","type":"<resource_type>"}' \
  --data '{"member_type":"openid","member_id":"<bot_open_id>","perm":"view","type":"user"}'
```

此方式仅适用于授权给当前应用。授权给其他用户时，直接使用对方的 open_id，无需调用 bot info 接口。

`<resource_type>` 可选值：`doc`、`docx`、`sheet`、`bitable`、`file`、`folder`、`wiki`、`slides`。
