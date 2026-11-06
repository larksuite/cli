---
name: lark-contact
version: 1.0.0
description: "飞书 / Lark 通讯录:按姓名 / 邮箱解析成 open_id,或按 open_id 反查姓名 / 部门 / 邮箱 / 联系方式 / 个人状态 / 签名,以及按关键词搜索机器人 / 智能体(agent),可限定某个群。当用户提到一个名字要下一步发消息 / 排日程,拿到 open_id 想查具体信息,或要查某个群里的机器人时使用。不负责部门树遍历、按部门列员工、组织架构图,这类需求走原生 OpenAPI。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli contact --help"
---

开始前先读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)(认证、权限处理)。

## 选哪个命令

**user 和 bot 是两条独立路径**,先确定身份再选命令:

| 想做什么 | user 身份 | bot 身份 |
|---|---|---|
| 按姓名 / 邮箱搜员工拿 open_id | [`+search-user`](references/lark-contact-search-user.md) | 不支持 |
| 按关键词搜机器人 / 智能体 | [`+search-bot`](references/lark-contact-search-bot.md) | 不支持 |
| 只搜某个群里的机器人 | `+search-bot --query <词> --chat-ids <群ID>` | 不支持 |
| 已知 open_id(`ou_`)取他人资料 | `+search-user --user-ids <id>` | [`+get-user --user-id <id>`](references/lark-contact-get-user.md) |
| 已知的 ID 是 `on_`(union_id) 或 user_id | `+get-user --user-id <id> --user-id-type union_id`(是 user_id 就传 `user_id`) | 同左 |
| 要英文 / 其它语种下的展示名 | `+search-user --lang en_us`(可配 `--query` 或 `--user-ids`) | 不支持 |
| 查看自己 | `+get-user`(不带 `--user-id`) 或 `+search-user --user-ids me` | 不支持 |
| 查同事的个人状态 / 签名 | `user_profiles batch_query` | 不支持 |

已知 open_id 只是想发消息 / 排日程,不必经过 contact —— 直接 [`lark-im`](../lark-im/SKILL.md) / [`lark-calendar`](../lark-calendar/SKILL.md)。

名字没说清是人还是机器人时(如「和 reviewDuck 约个会」):含 bot / agent / AI / 助手 / 机器人 / 智能体 / assistant 等明显特征的先搜机器人;不确定的话两边都搜一下。

## 命令写法

```bash
lark-cli contact +search-user --query "张三" --as user   # 别默认带 --has-chatted 一类 filter,会把没聊过的人筛掉
lark-cli contact +search-bot --query '会议助手' --as user
lark-cli contact +search-bot --queries '会议助手,日报助手,审批助手' --as user
```

`bots: []` 不等于没有——只加在某个群里的机器人,要带 `--chat-ids <群ID>` 才搜得到。

批量查个人状态 / 个性签名:

```bash
lark-cli contact user_profiles batch_query \
  --params '{"user_id_type":"open_id"}' \
  --data '{"user_ids":["ou_xxx","ou_yyy"],"query_option":{"include_personal_status":true,"include_description":true}}' \
  --as user
```

搜索命中多条且后续操作有副作用(发消息、邀请会议等),把候选列给用户挑;不要擅自选第一条。

能力边界拿不准时,先跑一条 `+search-user` 看实际返回再据实回答,不要读完文档就断言不支持。

## 注意事项

- **跨租户用户**(`is_cross_tenant=true`)多数业务字段为空字符串,这是飞书可见性规则,下游做空值兜底。
- **ID 类型**:`--user-id-type` 归 `+get-user`,`--user-ids` / `me` 归 `+search-user`,跨命令配串报 unknown flag(但 `+get-user --user-id me` 是服务端报错)。`+search-bot` 只按关键词搜、不吃 ID,返回的机器人 open_id 也是 `ou_`。

## 不在本 skill 范围

- 部门树 / 按部门列员工 / 组织架构 → [`lark-openapi-explorer`](../lark-openapi-explorer/SKILL.md) 查找原生接口
- 查聊天记录 / 看群成员 → [`lark-im`](../lark-im/SKILL.md)
