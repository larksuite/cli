# 设置发件人名单状态

仅限当前用户（`--as user`），固定邮箱 `me`。需要 `mail:user_mailbox.message:modify`。

```bash
lark-cli mail +sender-set --sender sender@example.com --type allow --dry-run
```

`--sender` 与 `--type allow|block` 必填。去掉 `--dry-run` 后执行单条设置，成功为 `data.status=configured`。重复设置、相反名单互斥由服务端处理；不先删后写。执行后用 `+sender-get` 查询目标类型。逐项失败返回非零退出码，保留 reason_code；成功响应不承诺新增计数。

地址示例须替换为用户指定的真实地址。参数/权限/接口错误均为非零退出码；按结构化错误提示修复后重试。操作授权与完整 set → get → delete → get 流程见域级 SKILL。
