# mail +sender-delete

从当前用户邮箱的白名单或黑名单删除发件人地址。删除前必须向用户确认名单类型和地址数量。

本 skill 对应 shortcut：`lark-cli mail +sender-delete`。

## 命令

```bash
# 从白名单删除
lark-cli mail +sender-delete --as user --type allow --address notice@example.com

# 从黑名单批量删除
lark-cli mail +sender-delete --as user --type block \
  --address spam@example.com,ads@example.com

# 查看将要调用的接口和 body
lark-cli mail +sender-delete --as user --type block --address spam@example.com --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mailbox <email>` | 否 | 邮箱地址，默认 `me`。`--as bot` 不能与 `--mailbox me` 一起使用。 |
| `--type allow\|block` | 是 | 从白名单或黑名单删除；不支持 `all`。 |
| `--address <email>` | 是 | 一个或多个邮箱地址，支持重复 flag 和逗号分隔，最多 100 个。删除时保留原始大小写，避免影响历史数据匹配。 |

## 输出

输出包含 `list_type`、`addresses` 和 `deleted_count`。`deleted_count` 小于输入数量时，应提示用户可能已有部分地址不存在或服务端未删除。
