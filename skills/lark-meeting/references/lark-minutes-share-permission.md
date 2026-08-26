# minutes +share-permission

将一条妙记的权限一键分享给会议参会人。**写操作**，只有用户明确要求“分享给参会人”“给这场会的人开放妙记权限”时才调用。

本 skill 对应 shortcut：`lark-cli minutes +share-permission`（调用 `POST /open-apis/minutes/v1/minutes/{minute_token}/permissions/share`）。支持 `--as user` / `--as bot`。

## 命令

```bash
# 以 user 身份将妙记权限分享给会议参会人
lark-cli minutes +share-permission --minute-token obcnxxxxxxxxxxxxxxxxxxxx --as user

# 以 bot 身份将妙记权限分享给会议参会人
lark-cli minutes +share-permission --minute-token obcnxxxxxxxxxxxxxxxxxxxx --as bot

# 预览 API 调用
lark-cli minutes +share-permission --minute-token obcnxxxxxxxxxxxxxxxxxxxx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--minute-token <token>` | 是 | 妙记 Token |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 权限语义

- 本命令只接收 `minute_token`。分享范围、权限含义和权限校验由服务端下游处理，CLI 不自行判断参会人范围。
- `--as user` 表示以当前登录用户身份发起一键分享。
- `--as bot` 表示以应用身份发起一键分享，前提是应用已具备对应能力和资源权限。
- 给指定成员授权时不要用本命令，使用 `drive +member-add`。
- 为当前调用身份向所有者申请查看或编辑权限时不要用本命令，使用 `minutes +apply-permission`。

## 所需权限

| 身份 | 所需权限 |
|------|---------|
| user / bot | `minutes:minutes` |

## 输出结果

```json
{
  "minute_token": "obcnxxxxxxxxxxxxxxxxxxxx",
  "shared": true
}
```

| 字段 | 说明 |
|------|------|
| `minute_token` | 妙记 Token |
| `shared` | API 调用成功后为 `true` |

## 如何获取 minute_token

| 来源 | 获取方式 |
|------|---------|
| 妙记 URL | 从 URL 末尾提取，如 `https://sample.feishu.cn/minutes/obcnxxxxxxxxxxxxxxxxxxxx` |
| 妙记搜索 | `lark-cli minutes +search --query "关键词"` |
| 会议产物查询 | `lark-cli vc +recording --meeting-ids <id>`，拿到 `minute_token`（沿用同一 `--as`） |

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--minute-token` 为空或格式不合法 | 缺少有效妙记 Token | 从妙记 URL、搜索结果或会议产物里重新取 token |
| `missing required scope(s)` | 当前身份缺少 `minutes:minutes` | user 身份用 `auth login --scope` 补权限；bot 身份去开发者后台开通 |
| `permission_denied` | 当前身份没有操作这条妙记的资源权限 | 请妙记所有者授权；不要通过切换身份绕过 |
