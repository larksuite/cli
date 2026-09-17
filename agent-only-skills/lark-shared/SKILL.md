---
name: lark-shared
version: 1.1.0
description: "Use for lark-cli setup/auth tasks: Agent Employee (AAT) configuration, identity diagnostics, automatic token renewal, missing scopes, or handling _notice JSON."
metadata:
  requires:
    bins: ["lark-cli"]
---

# lark-cli 共享规则

所有 `lark-*` skill 共享的底座：身份、认证、输出契约与高风险操作。

## 通用准则

1. **调用前先确认用法**：执行前读对应 reference 或跑 `--help`，别猜 flag 盲调。

2. **身份决定你代表谁操作**：本分发仅面向 Agent Employee，使用 AAT；它是特殊 user 主体，`--as user` 表示 Agent Employee 自身，资源范围由其权限决定。不是当前对话中的人类用户，也不是应用机器人。配置应用凭据即可使用，无需交互授权；不切换到 bot，也不使用独立的 agent 身份参数。身份和权限管理 → [`lark-shared-identity-and-permissions.md`](references/lark-shared-identity-and-permissions.md)。

3. **权限配置链接原样转发**：错误中的 `console_url` 应交给指定的智能体平台或租户管理员处理；不编解码、不重拼 query。

4. **`--format json`（默认）下，判断成功用 `ok == true`（或进程退出码 0），不要用 `code == 0`**：成功信封没有顶层 `code` / `msg` 字段，`code` 只出现在错误信封的 `error` 内。按 OpenAPI 老格式 `{"code": 0, "msg": "ok"}`判断会把所有成功调用误判为失败——封装写入类命令时尤其危险。JSON 输出契约 → [`lark-shared-output-contract.md`](references/lark-shared-output-contract.md)。


## 安全规则

1. **禁止输出密钥**（appSecret、accessToken等）到终端明文。

2. **写入/删除操作前必须确认用户意图**。

3. 目标命令支持 `--dry-run` 时，用 `--dry-run` 预览危险请求。

4. **退出码 10 是高风险确认门禁（`risk: "high-risk-write"`），不是错误**：停下 → **向用户确认**（展示 `action`、`risk` 和关键参数）→ 取得**用户显式同意**后，将 `hint` 指出的确认 flag **追加到你原始 argv 的末尾**后重试；**绝不**静默加确认 flag 绕过 → [`lark-shared-high-risk-approval.md`](references/lark-shared-high-risk-approval.md)。

5. **文件路径只接受相对路径**：`--file`、`--output`、`--output-dir`、`@file` 等路径参数只接受 cwd 下的相对路径，传绝对路径会报 `unsafe file path`。数据输入（`@file`、大 JSON）优先用 stdin 传入，避免路径和转义问题。


## Reference 强触发索引

命中任一触发条件时，**MUST 在执行下一步前读取对应 reference**。命中多条时按表中顺序读取，同一reference只读取一次。

| 强触发条件（命中任一即必读） | Reference |
|---|---|
| 查看 Agent Employee 主体、身份诊断、`--as`、AAT 状态或续期、scope 和权限管理、`missing_scopes` 或 `console_url` | [`lark-shared-identity-and-permissions.md`](references/lark-shared-identity-and-permissions.md) |
| 需要依赖 JSON 输出契约判断成功 / 失败、读取 stdout / stderr，或为命令编写脚本与封装 | [`lark-shared-output-contract.md`](references/lark-shared-output-contract.md) |
| 准备执行high-risk-write(高风险操作)、判断命令风险等级、遇到退出码 exit 10、`confirmation_required`、确认后重试 | [`lark-shared-high-risk-approval.md`](references/lark-shared-high-risk-approval.md) |
| 首次配置 Agent Employee 应用、凭据存储修复、主体重新绑定、或 CLI 提示重新执行 `config init` | [`lark-shared-config-init.md`](references/lark-shared-config-init.md) |
| 用户询问 notice、CLI版本更新、或输出含 `_notice`（升级 / skills 落后 / 废弃命令提示）| [`lark-shared-update-notice.md`](references/lark-shared-update-notice.md) |
