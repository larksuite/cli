---
name: lark-contact
version: 1.0.0
description: "飞书 / Lark 通讯录:按姓名 / 邮箱解析成 open_id,或按 open_id 反查姓名 / 部门 / 邮箱 / 联系方式 / 个人状态 / 签名。当用户提到某人姓名要下一步发消息 / 排日程,或拿到 open_id 想查具体信息时使用。不负责部门树遍历、按部门列员工、组织架构图,这类需求走原生 OpenAPI。"
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

## 注意事项

- **41050 / Permission denied** 受当前身份的可见范围限制(两条命令都可能遇到)。换 bot 身份或让管理员调整可见范围,细节见 [`lark-shared`](../lark-shared/SKILL.md)。
- **跨租户用户**(`is_cross_tenant=true`)多数业务字段为空字符串,这是飞书可见性规则,下游做空值兜底。
- **ID 类型**:默认 `open_id`。`+get-user` 可改 `--user-id-type union_id|user_id`;`+search-user` 只接受 `open_id`。

## 不在本 skill 范围

- 发消息 / 查聊天记录 → [`lark-im`](../lark-im/SKILL.md)
- 排日程 / 邀请会议 → [`lark-calendar`](../lark-calendar/SKILL.md)
- 部门树 / 按部门列员工 / 组织架构 → [`lark-openapi-explorer`](../lark-openapi-explorer/SKILL.md) 查找原生接口
