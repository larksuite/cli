# 精确查询发件人状态

仅限当前用户（`--as user`），固定邮箱 `me`。需要 `mail:user_mailbox.message:readonly`。

```bash
lark-cli mail +sender-get --sender sender@example.com
```

仅接受邮箱地址或域名的精确比较，比较时 trim 并忽略大小写，不把子串或域名中的某个邮箱当作精确命中。全页扫描两类名单且不使用 keyword 搜索缓存。未配置返回 `data.found=false`；命中返回 `found=true`、`sender`、`list_type`。双名单命中返回冲突错误；无效、重复或缺失分页 token 和接口失败不会返回 found=false。

地址示例须替换为用户指定的真实地址。参数/权限/接口错误均为非零退出码；按结构化错误提示修复后重试。操作授权与完整 set → get → delete → get 流程见域级 SKILL。
