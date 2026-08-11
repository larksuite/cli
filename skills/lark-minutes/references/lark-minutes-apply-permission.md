# minutes +apply-permission

向妙记所有者发起查看或编辑权限申请。**写操作**，只在用户明确要求申请权限时才调用；调用后不代表立即获得权限，只是提交了一条申请。

本 skill 对应 shortcut：`lark-cli minutes +apply-permission`（调用 `POST /open-apis/minutes/v1/minutes/{minute_token}/permissions/apply`）。支持 `--as user` / `--as bot`。

## 命令

```bash
# 以 user 身份申请查看权限
lark-cli minutes +apply-permission --minute-token obcnxxxxxxxxxxxxxxxxxxxx --perm view --as user

# 以 bot 身份申请编辑权限
lark-cli minutes +apply-permission --minute-token obcnxxxxxxxxxxxxxxxxxxxx --perm edit --as bot

# 预览 API 调用
lark-cli minutes +apply-permission --minute-token obcnxxxxxxxxxxxxxxxxxxxx --perm view --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--minute-token <token>` | 是 | 妙记 Token |
| `--perm <view\|edit>` | 是 | 申请的权限：`view`（查看）或 `edit`（编辑） |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## user / bot 身份与权限语义

- **user**：以当前登录用户身份向妙记所有者申请。所有者在飞书客户端收到申请通知，同意后该用户获得对应权限。
- **bot**：以应用身份向妙记所有者申请，代表"这个应用"而不是某个用户。同意后应用（bot）获得对应权限，不会让触发申请的用户本人获得权限。
- 两种身份的申请互不代表：user 身份申请通过后 bot 仍然无权限，反之亦然。

## 核心约束

### 1. 必须继承触发无权限错误的来源身份

`+apply-permission` 不是通用的"求权限"按钮：它申请的是**当前 `--as` 对应身份**的权限。如果是 `--as bot` 读取妙记时遇到无权限，就要用 `--as bot` 申请；如果是 `--as user` 遇到无权限，就用 `--as user` 申请。不要在申请时切换成另一个身份——那申请的是另一个主体的权限，解决不了原来那次调用的问题。

### 2. missing scope 与资源 ACL 是两类不同问题

- **missing scope**（当前身份完全没有 `minutes:permission:apply` / `minutes:minutes.basic:read` 等 scope）：这不是"没有这条妙记的权限"，`+apply-permission` 解决不了。`--as user` 用 `auth login --scope` 补权限；`--as bot` 去开发者后台开通，**禁止**对 bot 执行 `auth login`。完整规则见 [lark-shared](../../lark-shared/SKILL.md)。
- **资源 ACL**（scope 都有，但对**这一条具体妙记**没有查看/编辑权限）：这才是 `+apply-permission` 要解决的场景。

先看错误的 `error.subtype` 是 `missing_scope` 还是资源级别的权限拒绝，再决定要不要调用本命令。

### 3. 只有用户明确要求才发起申请

遇到无权限错误时，先把"当前身份对这条妙记没有权限"的事实告知用户；只有用户明确说"帮我申请查看/编辑权限"时才调用本命令。不要在检测到无权限后自动发起申请。

### 4. 禁止通过切换身份绕过资源权限

如果 `--as bot` 对某条妙记没有权限，不要改用 `--as user` 重新读取来"绕过"这个限制（除非用户明确同意切换身份继续任务）。申请权限和切换身份是两件不同的事：前者是解决 bot 自身权限不足，后者是换一个完全不同的主体去访问资源。

## 所需权限

| 身份 | 所需权限 |
|------|---------|
| user / bot | `minutes:permission:apply` |

## 输出结果

```json
{
  "minute_token": "obcnxxxxxxxxxxxxxxxxxxxx",
  "perm": "view"
}
```

| 字段 | 说明 |
|------|------|
| `minute_token` | 妙记 Token |
| `perm` | 申请的权限（`view` / `edit`） |

## 如何获取 minute_token

| 来源 | 获取方式 |
|------|---------|
| 妙记 URL | 从 URL 末尾提取，如 `https://sample.feishu.cn/minutes/obcnxxxxxxxxxxxxxxxxxxxx` |
| 妙记搜索 | `lark-cli minutes +search --query "关键词"` |
| 会议产物查询 | `lark-cli vc +recording --meeting-ids <id>`，拿到 `minute_token`（沿用同一 `--as`） |

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--perm` 不是 `view`/`edit` | 参数值不合法 | 只能传 `view` 或 `edit` |
| `missing required scope(s)` | 当前身份缺少 `minutes:permission:apply` | 见上方「missing scope 与资源 ACL」 |
| 申请后仍无权限 | 所有者尚未同意 | 这是异步申请，需等待所有者处理；不代表命令执行失败 |

## 参考

- [lark-minutes](../SKILL.md) — 妙记全部命令
- [minutes +detail](lark-minutes-detail.md) — 妙记内容与产物查询
- [lark-shared](../../lark-shared/SKILL.md) — 身份延续与权限恢复规则
