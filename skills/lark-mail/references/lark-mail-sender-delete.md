# 删除发件人名单配置

仅限当前用户（`--as user`），固定邮箱 `me`。需要 `mail:user_mailbox.message:modify`。

```bash
lark-cli mail +sender-delete --sender sender@example.com --type block --dry-run
```

`--sender` 与 `--type allow|block` 必填。去掉 `--dry-run` 并加 `--yes` 确认后仅删除指定类型，成功为 `data.status=absent`。不存在记录仍成功；另一类型不受影响。执行后用 `+sender-get` 验证；逐项失败返回非零退出码，不能把 HTTP 成功当删除成功。

地址示例须替换为用户指定的真实地址。参数/权限/接口错误均为非零退出码；按结构化错误提示修复后重试。操作授权与完整 set → get → delete → get 流程见域级 SKILL。
