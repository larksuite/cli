# vc +meeting-list-active

列出当前进行中的会议，用来发现 `+meeting-events` 需要的长数字 `meeting_id`。

本 skill 对应 shortcut：`lark-cli vc +meeting-list-active`（调用 `GET /open-apis/vc/v1/bots/user_active_meeting`）。

## 命令

```bash
# 查询当前登录用户正在参加的会议
lark-cli vc +meeting-list-active --as user --format json

# 查询指定用户当前参加、且应用机器人也在会中的会议
lark-cli vc +meeting-list-active --as bot --user-id ou_xxx --format json
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--user-id <id>` | 应用身份必填 | 目标用户 open_id，格式为 `ou_...`。用户身份不传；应用身份直接透传给接口，不接受 internal user_id 或数字 ID |

## 身份语义

不要向用户暴露内部身份缩写；对用户只说“用户身份”或“应用身份”。

| 身份 | 命令 | 返回范围 | 后续事件读取 |
| ---- | ---- | -------- | ------------ |
| 用户身份 | `--as user` | 当前登录用户正在参加的会议 | 继续 `+meeting-events --as user` |
| 应用身份 | `--as bot --user-id <user_open_id>` | 目标用户正在参加、且应用机器人也在会中的会议 | 继续 `+meeting-events --as bot` |

硬规则：`meeting_id` 从哪种身份路径拿到，后续 `+meeting-events` 就沿用哪种身份。不要把用户身份拿到的 `meeting_id` 改用应用身份查，也不要把应用身份拿到的 `meeting_id` 改用用户身份查，除非用户明确要求切换场景。

应用身份返回空，不代表目标用户不在任何会议中，只能说明没有找到“目标用户在会中且应用机器人也在会中”的当前会。

常见流程：

```bash
# 方式 1：先让应用机器人入会，直接从 join 响应拿 meeting.id
lark-cli vc +meeting-join --as bot --meeting-number 123456789 --format json
lark-cli vc +meeting-events --as bot --meeting-id <meeting.id> --page-all --format pretty

# 方式 2：应用机器人已经在会中时，用应用身份发现 meeting_id
lark-cli vc +meeting-list-active --as bot --user-id <user_open_id> --format json
lark-cli vc +meeting-events --as bot --meeting-id <meeting_id> --page-all --format pretty

# 方式 3：只回答当前登录用户所在会议发生了什么
lark-cli vc +meeting-list-active --as user --format json
lark-cli vc +meeting-events --as user --meeting-id <meeting_id> --page-all --format pretty
```

## 多会议选择

- 如果返回多个会议，不要自动挑第一个。
- 向用户展示每个候选的 `meeting_title` / `meeting_no` / `meeting_id`，等待用户选择。
- 选择后继续使用发现该会议时的同一身份调用 `+meeting-events`。

## 9 位会议号匹配

用户提供 9 位会议号但没有明确要求应用机器人入会时，把会议号当作 active meeting 的筛选条件，而不是写操作指令。

```bash
# 用户问“我当前这个会讲了什么”
lark-cli vc +meeting-list-active --as user --format json

# 用户问“让应用机器人所在/可见的这个会讲了什么”
lark-cli vc +meeting-list-active --as bot --user-id <user_open_id> --format json
```

匹配规则：

- 在返回会议中匹配 `meeting_no == <9位会议号>`。
- 匹配到唯一会议：取该项的长数字 `meeting_id`，后续用同一身份调用 `+meeting-events`。
- 匹配到多个会议：展示候选，让用户选择。
- 没有匹配：说明当前身份没有发现该会议号对应的 active meeting；不要自动调用 `+meeting-join`，除非用户明确要求应用机器人入会。

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--user-id is required when --as bot` | 应用身份未传目标用户 | 传入目标用户 open_id |
| 用户身份返回空列表 | 当前登录用户没有可见的进行中会议 | 确认用户是否在会中，或是否切错身份 |
| 用户身份不支持 | 当前接口不支持用用户身份访问 | 不要反复执行 `auth login`。改用应用身份流程：先拿目标用户 open_id，再执行 `+meeting-list-active --as bot --user-id <user_open_id>`；同时按应用身份权限配置检查应用权限、安装、数据范围和灰度 |
| 应用身份返回空列表 | 没有满足“目标用户在会中且应用机器人也在会中”的当前会 | 先让应用机器人入会，或确认 `user_id` 和会议状态 |
| `--user-id` 格式错误 | 传入了 internal user_id 或其他非 `ou_...` 值 | 改传目标用户 open_id |
| 应用身份权限不足 | 应用权限、租户安装、权限可访问的数据范围或 VC Agent privilege 未配置完整 | 不要执行 `auth login`。以 CLI 返回的 metadata / error envelope 为准确认缺失权限；检查应用发布/安装，以及开放平台“权限可访问的数据范围”：选择“按条件筛选”，条件为“会议的归属者 包含 与应用的可用范围一致”；仍失败再排查内测 privilege / 灰度 |

## 参考

- [lark-vc-agent-meeting-join](lark-vc-agent-meeting-join.md) — 让应用机器人真实入会并拿 `meeting.id`
- [lark-vc-agent-meeting-events](lark-vc-agent-meeting-events.md) — 使用 `meeting_id` 读取会中事件
