# calendar +transfer

把一个日程的**组织者（organizer）**转让给另一个用户或机器人。用户和机器人之间可以任意互转。

## 命令

```bash
# 转让给某人（原组织者保留为参与人）
lark-cli calendar +transfer --event-id <event_id> --to-user-id ou_xxx --yes

# 转让并把原组织者从参与人中移除
lark-cli calendar +transfer --event-id <event_id> --to-user-id ou_xxx --remove-original-organizer --yes

# 指定日历
lark-cli calendar +transfer --calendar-id <calendar_id> --event-id <event_id> --to-user-id ou_xxx --yes

# 重复性日程：必须显式确认整个序列一起转让
lark-cli calendar +transfer --event-id <event_id> --to-user-id ou_xxx --transfer-series --yes

# 预览请求，不实际执行
lark-cli calendar +transfer --event-id <event_id> --to-user-id ou_xxx --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--event-id <id>` | **是** | 日程 ID（`uid_originalTime` 形式） |
| `--to-user-id <ou_...>` | **是** | 接收人 open_id，成为新组织者；用户和机器人都可以 |
| `--calendar-id <id>` | 否 | 日程所在日历 ID（省略则使用主日历） |
| `--remove-original-organizer` | 否 | 转让后把原组织者移出参与人；默认保留。日程在共享日历上时服务端一定会移除 |
| `--transfer-series` | 否 | 确认整个重复性序列一起转让；重复性日程必填 |
| `--yes` | **是**（非 dry-run） | 高敏写操作确认 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 转让方向

转出方和接收方是两个**互相独立**的参数，四种组合都支持：

- **转出方**由 `--as` 决定，必须是日程**当前组织者**的身份。bot 组织的日程用 `--as bot`，用户自己的日程用 `--as user`。用非组织者身份调用会返回 403。
- **接收方**由 `--to-user-id` 决定，传谁的 open_id 就转给谁，是人还是机器人不影响命令写法。

| 方向 | 命令 |
|------|------|
| user → user | `--as user --to-user-id <对方用户 open_id>` |
| user → bot | `--as user --to-user-id <bot 的 open_id>` |
| bot → user | `--as bot --to-user-id <用户 open_id>` |
| bot → bot | `--as bot --to-user-id <另一个 bot 的 open_id>` |

**取接收人 open_id**：

```bash
# 用户
lark-cli contact +search-user --query <姓名> --as user
# 机器人：从它所在群的成员列表里取 bots[] 中的 open_id
lark-cli im +chat-members-list --chat-id <chat_id> --member-types bot
```

机器人的 open_id 同样是 `ou_` 开头；不要传 `cli_` 开头的 app_id，那是应用 ID，不是日程参与人身份。

无论哪个方向，转让都要求转出方和接收方**同租户**，且接收方能通过高管模式的协作校验。

## 重复性日程

后端按 `uid` 定位日程，忽略 `original_time`，**无法只转让某一次实例**。因此传入任何一个实例或例外的 `event_id`，都会把整个序列（含所有例外）一起转让。

是重复性日程且未加 `--transfer-series` 时命令直接失败（`failed_precondition`），不会发出转让请求。收到这个错误时**先向用户确认"整个重复日程都转让"**，得到确认后再带 `--transfer-series` 重跑；不要自动重试。已确认时加 `--transfer-series` 会跳过这次预读。

## 返回中的 `original_organizer_removed`

**共享日历不属于任何组织者，转让时服务端会强制把原组织者移出日程；主日历则会把原组织者保留为参与人。** 转让接口成功时不返回这个结果，所以命令只在能确定时才输出该字段：

| 情况 | 返回 |
|------|------|
| 带 `--remove-original-organizer` | `original_organizer_removed: true` |
| 省略 `--calendar-id`（主日历） | `original_organizer_removed: false` |
| 传了 `--calendar-id` 且未传 `--remove-original-organizer` | **不返回该字段**，stderr 给一条 note 说明共享日历会强制移除 |

字段缺失时**不要**告诉用户"原组织者已保留为参与人"，也不要断言已被移除。需要确认就转让后读一次日程看参与人，或一开始就显式传 `--remove-original-organizer`。

## 提示

- 转让不可逆，且会连同日程上的会议纪要、笔记和附件一起移交给新组织者。
- 需要 `calendar:calendar.event:transfer` 权限；转让前的重复性预读需要 `calendar:calendar.event:read`（带 `--transfer-series` 时不读）。

## 参考

- [lark-calendar](../SKILL.md) -- skill 入口与路由
- [重复性日程操作规范](lark-calendar-recurring.md)
