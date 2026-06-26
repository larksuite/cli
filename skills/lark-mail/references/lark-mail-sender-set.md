# mail +sender-set

把发件人地址加入当前用户邮箱的白名单或黑名单。执行前应向用户确认名单类型和地址数量。

本 skill 对应 shortcut：`lark-cli mail +sender-set`。

## 命令

```bash
# 加入白名单
lark-cli mail +sender-set --as user --type allow --address notice@example.com

# 加入黑名单，支持重复 flag 或逗号分隔
lark-cli mail +sender-set --as user --type block \
  --address spam@example.com --address ads@example.com,bot@example.com

# 查看将要调用的接口和 body
lark-cli mail +sender-set --as user --type allow --address notice@example.com --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--mailbox <email>` | 否 | 邮箱地址，默认 `me`。`--as bot` 不能与 `--mailbox me` 一起使用。 |
| `--type allow\|block` | 是 | 写入白名单或黑名单；不支持 `all`。 |
| `--address <email>` | 是 | 一个或多个邮箱地址，支持重复 flag 和逗号分隔，最多 100 个。写入时会 trim 并转小写。 |

## 输出

输出包含 `list_type`、`addresses` 和 `failed_items`。如 `failed_items` 非空，逐项向用户说明失败地址和原因。
