---
name: lark-contact
version: 1.0.0
description: "飞书 / Lark 通讯录:按姓名 / 邮箱解析成 open_id,或按 open_id 反查姓名 / 部门 / 邮箱 / 联系方式 / 个人状态 / 签名,以及按关键词搜索当前用户可见的机器人。当用户提到某人姓名要下一步发消息 / 排日程,或拿到 open_id 想查具体信息,或需要查找机器人 open_id 时使用。不负责部门树遍历、按部门列员工、组织架构图,这类需求走原生 OpenAPI。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli contact --help"
---

## 选哪个命令

**user 身份和 bot 身份是两条完全独立的路径**。先确定当前身份,再按下表选命令:

| 想做什么 | user 身份 | bot 身份 |
|---|---|---|
| 按姓名 / 邮箱搜员工拿 open_id | [`+search-user`](references/lark-contact-search-user.md) | 不支持 |
| 按名称搜索当前用户可见的机器人 | [`+search-bot`](references/lark-contact-search-bot.md) | 不支持 |
| 已知 open_id 取他人资料 | `+search-user --user-ids <id>` | [`+get-user --user-id <id>`](references/lark-contact-get-user.md) |
| 查看自己 | `+get-user` 或 `+search-user --user-ids me` | 不支持 |
| 查同事的个人状态 / 签名 | `user_profiles batch_query` | 不支持 |

已知 open_id 只是想发消息 / 排日程,不必经过 contact —— 直接 [`lark-im`](../lark-im/SKILL.md) / [`lark-calendar`](../lark-calendar/SKILL.md)。

## 典型场景

找张三给他发消息:先搜,确认 open_id,再发:

```bash
lark-cli contact +search-user --query "张三" --has-chatted --as user
lark-cli im +messages-send --user-id ou_xxx --text "Hi!"
```

批量查同事的个人状态 / 个性签名(先用 schema 看参数)。

```bash
lark-cli schema contact.user_profiles.batch_query
lark-cli contact user_profiles batch_query \
  --params '{"user_id_type":"open_id"}' \
  --data '{"user_ids":["ou_xxx","ou_yyy"],"query_option":{"include_personal_status":true,"include_description":true}}' \
  --as user
```

搜索命中多条且后续操作有副作用(发消息、邀请会议等),把候选列给用户挑;不要擅自选第一条。

## 搜索机器人

`+search-bot` 使用 user 身份按关键词搜索当前用户可见的机器人,返回 `ou_` 开头的机器人 open_id。参数细节、输入归一化规则和输出结构见 [`lark-contact-search-bot.md`](references/lark-contact-search-bot.md)。

```bash
lark-cli contact +search-bot --query '会议助手' --as user
lark-cli contact +search-bot --queries '会议助手,日报助手,审批助手' --as user
```

`enable_join_group=true` 只表示该机器人允许被拉进群聊,**不代表你能用这里的 `open_id` 把它拉进群**。把机器人加入群聊需要应用的 `cli_` 开头 app_id,本命令不返回;直接用 `ou_` 开头的 open_id 调加群接口会被服务端放进 `invalid_id_list`,且没有 open_id 到 app_id 的查询接口。看到这个字段为真时,不要据此声称已把机器人加入群聊。

## 注意事项

- **41050 / Permission denied** 受当前身份的可见范围限制(两条命令都可能遇到)。换 bot 身份或让管理员调整可见范围,细节见 [`lark-shared`](../lark-shared/SKILL.md)。
- **跨租户用户**(`is_cross_tenant=true`)多数业务字段为空字符串,这是飞书可见性规则,下游做空值兜底。
- **ID 类型**:默认 `open_id`。`+get-user` 可改 `--user-id-type union_id|user_id`;`+search-user` 只接受 `open_id`。
- **`+search-bot` 的权限**:只支持 user 身份,缺权限时重新授权 `search:bot`。
- **`+search-bot` 的 ID 类型**:和 `+search-user` 一样只出 `open_id`。接口本身支持 `user_id_type`,但输出结构的字段名会随之说谎,所以 CLI 不暴露;确实要 union_id / user_id 时走 `lark-cli api` 直调。

## 不在本 skill 范围

- 发消息 / 查聊天记录 → [`lark-im`](../lark-im/SKILL.md)
- 排日程 / 邀请会议 → [`lark-calendar`](../lark-calendar/SKILL.md)
- 部门树 / 按部门列员工 / 组织架构 → [`lark-openapi-explorer`](../lark-openapi-explorer/SKILL.md) 查找原生接口
