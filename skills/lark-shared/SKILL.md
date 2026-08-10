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

1. **调用前先确认用法**：执行前读对应 reference 或跑 `--help`，别猜 flag 盲调。

2. **身份决定你代表谁操作**：`--as user` 代表用户本人（其日历 / 云空间 / 邮箱等），`--as bot` 代表应用自己，只能访问bot自己的资源 → [`identity-and-permissions`](references/lark-shared-identity-and-permissions.md)。

3. **`--format json`（默认）下，判断成功用 `ok == true`（或进程退出码 0），不要用 `code == 0`**：成功信封没有顶层 `code` / `msg` 字段，`code` 只出现在错误信封的 `error` 内。按 OpenAPI 老格式 `{"code": 0, "msg": "ok"}`判断会把所有成功调用误判为失败——封装写入类命令时尤其危险 → [`output-contract`](references/lark-shared-output-contract.md)。

4. **代表用户发起 `auth login` 授权时绝不阻塞**：走 split-flow（发起后交还控制权、下一轮再完成），别在同一轮阻塞等授权 → [`auth-split-flow`](references/lark-shared-auth-split-flow.md)。

5. **授权 / 配置类 URL 必须配二维码**：当命令输出 `verification_url`、`verification_uri_complete`、`console_url` 等 URL 字段时，必须用 `lark-cli auth qrcode` 生成并在回复中展示，URL 在前二维码在后；优先生成 PNG（`--output`），仅当用户明确要求时才使用 ASCII（`--ascii`）。URL 原样转发——不编解码、不加标点、不重拼 query，二维码和链接请一起展示给用户。


## 安全规则

1. **禁止输出密钥**（appSecret、accessToken等）到终端明文。

2. **写入/删除操作前必须确认用户意图**。

3. 目标命令支持 `--dry-run` 时，用 `--dry-run` 预览危险请求。

4. **退出码 10 是高风险确认门禁（`risk: "high-risk-write"`），不是错误**：停下 → **向用户确认**（展示 `action`、`risk` 和关键参数）→ 取得**用户显式同意**后，将 `hint` 指出的确认 flag **追加到你原始 argv 的末尾**后重试；**绝不**静默加确认 flag 绕过 → [`high-risk-approval`](references/lark-shared-high-risk-approval.md)。

5. **文件路径只接受相对路径**：`--file`、`--output`、`--output-dir`、`@file` 等路径参数只接受 cwd 下的相对路径，传绝对路径会报 `unsafe file path`。数据输入（`@file`、大 JSON）优先用 stdin 传入，避免路径和转义问题。


## Reference 强触发索引

命中任一触发条件时，**MUST 在执行下一步前读取对应 reference**。命中多条时按表中顺序读取，同一reference只读取一次。

| 强触发条件（命中任一即必读） | Reference |
|---|---|
| 查看自己是谁（user/bot）、获取当前身份详细字段信息、不理解`--as user/bot`的含义、登录态、认证、scope、授权、权限、`missing_scope` 或 `console_url` | [`identity-and-permissions`](references/lark-shared-identity-and-permissions.md) |
| Agent 准备发起或完成 `auth login`，或该命令输出 `verification_url`、`device_code` | [`auth-split-flow`](references/lark-shared-auth-split-flow.md) |
| 需要依赖 JSON 输出契约 判断成功 / 失败、读取 stdout / stderr，或为命令编写脚本与封装 | [`output-contract`](references/lark-shared-output-contract.md) |
| 准备执行high-risk-write(高风险操作)、判断命令风险等级、遇到退出码 exit 10、 `confirmation_required`、确认后重试 | [`high-risk-approval`](references/lark-shared-high-risk-approval.md) |
| 首次使用CLI需运行 `lark-cli config init` 完成应用配置、或 CLI 明确提示 `config init --new` | [`config-init`](references/lark-shared-config-init.md) |
| 用户询问 notice、CLI版本更新、或输出含 `_notice`（升级 / skills 落后 / 废弃命令提示）| [`update-notice`](references/lark-shared-update-notice.md) |
