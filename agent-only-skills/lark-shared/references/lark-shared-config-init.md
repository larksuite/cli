# 配置 Agent Employee

使用 Agent Employee 的 App ID 和 App Secret 执行 `lark-cli config init`。通过应用凭据完成认证，无需人类交互授权。先读 `lark-cli config init --help`；不要使用创建普通应用的 `--new` 流程。

```bash
# 通过安全输入通道将 App Secret 送入 stdin，不在命令行或回复中展开密钥。
lark-cli config init --name <profile> --app-id <app_id> --app-secret-stdin --brand feishu
```

品牌为 Lark 时改用 `--brand lark`。`--name` 创建或更新指定 profile；不指定名称的初始化会替换配置，已有多 profile 时应先明确目标。

初始化通过 client_credentials 获取 token，再由 Introspection 验证 Agent Employee 主体，保存 AAT、主体绑定、scope 与应用凭据。不能仅凭 App ID 或命令退出成功断言已经绑定 Agent Employee：网络等临时错误可能让首次配置保存为未分类普通应用并输出警告。此时恢复连接后重跑初始化，确认 AAT 身份前不要执行业务调用，也不要回退到应用机器人。

```bash
lark-cli auth status --json --verify
lark-cli whoami
```

确认正在使用目标 profile，检查返回的 `identity`、`credential`、`principal` 及验证结果：预期为 user 身份、AAT 凭据、Agent Employee 主体。不要输出 token 或 App Secret。

后续调用由 CLI 自动续期 AAT：重新获取 token 并验证主体仍与绑定一致，无需 refresh token。若凭据损坏、应用绑定变化或 CLI 明确要求重新配置，先核实目标 Agent Employee，再对目标 profile 重新执行初始化。既有 Agent Employee 的重新验证失败时，CLI 会保留原绑定，不应删除 profile 绕过验证。
