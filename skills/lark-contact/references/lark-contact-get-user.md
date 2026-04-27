# +get-user

按 ID 取用户基本信息(姓名等)。

```bash
# 取自己
lark-cli contact +get-user --as user

# bot 按 ID 取他人
lark-cli contact +get-user --user-id ou_xxx --as bot

# 按 union_id / user_id 取(默认 open_id)
lark-cli contact +get-user --user-id <id> --user-id-type union_id --as bot
```

## 注意事项

- **user 身份按 ID 取他人请用 `+search-user --user-ids <id>`**,字段比本命令多(部门 / 邮箱 / 是否激活等)。本命令的 user 模式只回很少字段。
- **`--as bot` 必须传 `--user-id`**:不传会直接报错(只有 user 身份能省略 `--user-id` 取自己)。
