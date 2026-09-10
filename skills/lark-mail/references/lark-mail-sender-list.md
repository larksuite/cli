# 列出用户黑白名单

仅限当前用户（`--as user`），固定邮箱 `me`。需要 `mail:user_mailbox.message:readonly`。

```bash
lark-cli mail +sender-list --type all --page-size 50
```

`--type` 支持 allow、block、all，默认为 all；`--page-size` 为 1–100，默认 50。自动遍历全部页，成功输出 `data.items`，每项有 `sender` 和 `list_type`。不支持手工跨名单游标，需单页原子操作时使用对应资源的 list 方法。任何页失败均返回错误，不输出不完整的成功列表。

地址示例须替换为用户指定的真实地址。参数/权限/接口错误均为非零退出码；按结构化错误提示修复后重试。操作授权与完整 set → get → delete → get 流程见域级 SKILL。
