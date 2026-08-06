---
name: lark-shared
version: 1.1.0
description: "Use for lark-cli setup/auth tasks: auth login/status/logout, user vs bot identity, business-domain permissions (--domain, including all/docs/drive), missing scopes, revoking authorization, or handling _notice JSON."
metadata:
  requires:
    bins: ["lark-cli"]
---

# lark-cli 共享规则

所有 `lark-*` skill 共享的底座：身份、认证、输出契约与高风险操作。

## 通用准则
1. **调用前先弄懂用法**：不懂则读对应 reference 或跑 `--help`，别猜 flag 盲调。

2. **身份决定你代表谁操作**：`--as user` 代表用户本人（其日历 / 云空间 / 邮箱等），`--as bot` 代表应用自己，只能访问bot自己的资源。`--as bot` 碰用户资源**可能静默返空**而非报错，别把空结果当成"用户没有数据"。**不懂必读** → [`identity-and-permissions`](references/lark-shared-identity-and-permissions.md)。

3. **`--format json`（默认）下，判断成功用 `ok == true`（或进程退出码 0），不要用 `code == 0`**：成功信封没有顶层 `code` / `msg` 字段，`code` 只出现在错误信封的 `error` 内。按 OpenAPI 老格式判断会把所有成功调用误判为失败——封装写入类命令时尤其危险，误判会绕过幂等逻辑导致**重复创建**。**不懂必读** → [`output-contract`](references/lark-shared-output-contract.md)。

4. **代表用户发起 `auth login` 授权时绝不阻塞**：走 split-flow（发起后交还控制权、下一轮再完成），别在同一轮阻塞等授权。完整步骤 **执行前必读** → [`auth-split-flow`](references/lark-shared-auth-split-flow.md)。

5. **授权 / 配置类 URL 必须配二维码**：比如当命令输出 `verification_url`、`verification_uri_complete`、`console_url` 等 URL 字段时，必须用 `lark-cli auth qrcode` 生成并在回复中展示，URL 在前二维码在后；优先生成 PNG（`--output`），仅当用户明确要求时才使用 ASCII（`--ascii`）。URL 原样转发——不编解码、不加标点、不重拼 query，二维码和链接请一起展示给用户。


## 通用安全规则
1. **写入/删除操作前必须确认用户意图**。

2. **退出码 10 是高风险确认门禁（`risk: "high-risk-write"`），不是错误**：停下 → **向用户确认**（展示 `action`、`risk` 和关键参数）→ 取得**用户显式同意**后，按 `hint` 指出的确认 flag **追加到你原始 argv 的末尾**后重试（`hint` 可能是整条示例命令，只取其中的 flag，不要照跑示例、丢掉原有参数）；**绝不**静默加确认 flag 绕过。不懂必读 → [`high-risk-approval`](references/lark-shared-high-risk-approval.md)。

3. **文件路径只接受相对路径**：`--file`、`--output`、`--output-dir`、`@file` 等路径参数只接受 cwd 下的相对路径，传绝对路径会报 `unsafe file path`。数据输入（`@file`、大 JSON）优先用 stdin 传入，避免路径和转义问题。

4. **禁止输出密钥**：appSecret、accessToken 等不打到终端。

## 其他场景

- 首次配置 lark-cli（`config init`）→ [`config-init`](references/lark-shared-config-init.md)
- 认证、scope、业务域、登录态、退出登录态、撤销授权速查；权限不足 / `missing_scope` 恢复 → [`identity-and-permissions`](references/lark-shared-identity-and-permissions.md)
- 拿到 `/wiki/` 链接或 token → [`wiki-token-routing`](references/lark-wiki-token-routing.md)
- 输出含 `_notice`（升级 / skills 落后 / 废弃命令）→ [`update-notice`](references/lark-shared-update-notice.md)
