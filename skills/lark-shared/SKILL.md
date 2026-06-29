---
name: lark-shared
version: 1.1.0
description: "lark-cli 通用规则：user/bot 身份、认证授权、安全与高风险确认门禁。当首次配置 lark-cli、需要 auth login、遇到权限或 scope 错误、命令以退出码 10 要求确认、或输出包含 _notice 升级提示时使用。"
---

# lark-cli 共享规则

所有 lark-* skill 共享的底座：lark-cli 的身份、认证、安全与高风险操作通用规则。

## 通用准则

1. **调用前先懂用法**：执行 shortcut 前先读对应 reference 或跑 `-h` 弄懂用法，别猜 flag 盲调。

2. **身份决定你代表谁操作**：`--as user` 代表用户本人（能看到、也能操作其日历 / 云空间 / 邮箱等个人资源），`--as bot` 代表应用自己（只涉及 bot 的资源，发消息、建文档都归 bot）。用 `--as bot` 碰用户资源**可能静默返空**而非报错，别误判成"没有数据"。身份模型与权限恢复 → [`references/lark-shared-identity-and-permissions.md`](references/lark-shared-identity-and-permissions.md)。

3. **代表用户发起 `auth login` 授权时绝不阻塞**：走 split-flow（发起后交还控制权、下一轮再完成），别在同一轮阻塞等授权。完整步骤 **执行前必读** → [`references/lark-shared-auth-split-flow.md`](references/lark-shared-auth-split-flow.md)。

4. **授权 / 配置类 URL 必须配二维码**：用 `lark-cli auth qrcode` 生成、URL 在前二维码在后，URL 原样不改写。

5. **退出码 10 是高风险确认门禁，不是错误**：停下、取得用户**显式同意**后才按 `hint` 重试，**绝不**静默加确认 flag 绕过。机制 → [`references/lark-shared-high-risk-approval.md`](references/lark-shared-high-risk-approval.md)。

6. **路径参数只接受 cwd 相对路径**：绝对路径会被拒（`unsafe file path`），规划时就用相对路径。

7. **不输出密钥明文**（appSecret、accessToken）。

## 其他场景

- 首次配置 lark-cli（`config init`）→ [`references/lark-shared-config-init.md`](references/lark-shared-config-init.md)
- 拿到 `/wiki/` 链接或 wiki token → [`references/lark-wiki-token-routing.md`](references/lark-wiki-token-routing.md)
- 输出含 `_notice`（升级 / skills 落后 / 废弃命令提示）→ [`references/lark-shared-update-notice.md`](references/lark-shared-update-notice.md)
