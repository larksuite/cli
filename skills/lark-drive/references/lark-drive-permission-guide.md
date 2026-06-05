# Drive 权限与授权指南

> 前置条件：通用认证、scope 与 `--as` 规则见 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)。

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
