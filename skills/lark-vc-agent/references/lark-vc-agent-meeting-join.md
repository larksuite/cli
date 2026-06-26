
# vc +meeting-join

通过 9 位会议号让应用机器人加入一场正在进行的视频会议。这是一次**写操作**，会实际让应用机器人加入会议。

本 skill 对应 shortcut：`lark-cli vc +meeting-join`（调用 `POST /open-apis/vc/v1/bots/join`）。

> **不要把 9 位会议号等同于入会意图。** 用户给出 9 位会议号并询问“会议讲了什么 / 查会中事件”时，先用 `+meeting-list-active` 查当前 active meetings 并按 `meeting_no` 匹配；只有用户明确要求“入会 / 让应用机器人旁听 / 代我参会”时才调用本命令。

## 命令

```bash
# 仅指定会议号（无密码）
lark-cli vc +meeting-join --as bot --meeting-number 123456789

# 指定会议号 + 密码
lark-cli vc +meeting-join --as bot --meeting-number 123456789 --password 8888

# 从邀请事件透传 call_id（参见「如何获取输入参数」）
lark-cli vc +meeting-join --as bot --meeting-number 123456789 --call-id a08e06bf-9a41-44e4-a89c-a7871899e783

# 输出格式
lark-cli vc +meeting-join --as bot --meeting-number 123456789 --format json

# 预览 API 调用（不实际加入会议）
lark-cli vc +meeting-join --as bot --meeting-number 123456789 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-number <no>` | 是 | 会议号，必须为 **9 位纯数字** |
| `--password <pw>` | 否 | 会议密码，仅在该会议设置了入会密码时传入 |
| `--call-id <id>` | 否 | 从 `vc.bot.meeting_invited_v1` 邀请事件透传的 `call_id`，原样回传即可。Agent 主动入会或无邀请事件来源时不传 |
| `--dry-run` | 否 | 预览 API 调用，不实际加入会议；会议号或身份不确定时先用它确认请求 |

## 核心约束

### 1. 使用应用身份

这是应用机器人入会能力，使用 `--as bot`。不要用当前登录用户身份尝试让应用机器人入会。

### 2. 会议号格式严格校验

`--meeting-number` 必须是 9 位纯数字，否则本地校验直接报错：
`--meeting-number must be exactly 9 digits`。

常见错误来源：
- 把会议链接整条粘进来（应仅取尾部的 9 位数字）
- 把 `meeting_id`（长数字 ID）当成会议号传入（两者不是同一个东西）

### 3. 会议必须已开始且允许入会

- 会议必须处于**进行中**状态，应用机器人无法加入尚未开始或已结束的会议。
- 若会议设置了**等候室 / 入会审批**，应用机器人可能需要主持人放行后才真正入会。
- 若返回 `HTTP 403: no permission`（错误码 `121003`），不要只理解成“账号没权限”。这类报错更常见的原因是：会议参数或会控配置当前不满足入会条件，例如会议号填错、密码未传或错误、会议尚未开始、等候室 / 入会审批未放行、会议禁止外部/特定身份加入等。应先确认这些配置项，再重试。

### 4. 机器人入会后对其他参会人可见

这是一次真实入会操作，机器人会立即出现在参会人列表中，其他参会人可见，并产生会议日志。误入错会的社交成本高于技术成本——执行前优先确认 9 位会议号的来源（用户输入 / 会议链接末尾），不要臆造。参数格式有疑问时可用 `--dry-run` 预览请求体。

## 输出结果

接口返回会议基本信息，字段视具体响应而定，常见字段：

| 字段 | 说明 |
|------|------|
| `meeting.id` | 会议 ID（可后续传给 `+meeting-leave --as bot --meeting-id`） |
| `meeting.meeting_no` | 会议号（与入参一致） |
| `meeting.topic` | 会议主题 |
| `meeting.start_time` | 会议开始时间 |

> **重要**：拿到 `meeting.id` 后务必保留，退出会议（`+meeting-leave`）需要使用它，而不是会议号。

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|
| `meeting-number` | 会议号由主持人分享；也可从会议链接尾部解析 9 位数字 |
| `password` | 若会议设置了入会密码，由主持人提供 |
| `call-id` | 由 `vc.bot.meeting_invited_v1` 邀请事件的 `call_id` 字段携带，Agent 收到事件时透传过来；无邀请事件场景（如 Agent 主动入会）不传 |

## Agent 组合场景

### 场景 1：加入会议 → 监听会中事件

```bash
# 第 1 步：加入会议，记录返回的 meeting.id
lark-cli vc +meeting-join --as bot --meeting-number 123456789

# 第 2 步：使用返回的 meeting.id 查询会中事件
lark-cli vc +meeting-events --as bot --meeting-id <meeting.id> --page-all --format pretty
```

如果 bot 已经在会中，也可以通过 active meeting 找回 `meeting_id`：

```bash
lark-cli vc +meeting-list-active --as bot --user-id <user_open_id> --format json
```

### 场景 2：加入会议 → 会后进入 lark-vc 获取会议产物信息

```bash
# 第 1 步：加入并参会
lark-cli vc +meeting-join --as bot --meeting-number 123456789

# 第 2 步：会议结束后，先查询会议产物
lark-cli vc +detail --meeting-ids <meeting.id>
```

后续按 `lark-vc` 的产物决策处理：根据 `note_display_type`、`note_id`、`minute_token` 和用户意图选择纪要正文、逐字稿或妙记。

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--meeting-number must be exactly 9 digits` | 会议号不是 9 位纯数字 | 检查是否误传了会议链接或 meeting_id |
| 会议密码错误 | `--password` 错误或未提供 | 向主持人确认会议密码 |
| 会议不存在 / 已结束 | 会议号错误或会议未进行中 | 确认会议正在进行中 |
| `HTTP 403: no permission` / `121003` | 入会前置条件不满足，通常不是单纯 scope 问题 | 依次确认：1）会议允许智能体加入；2）会议号正确；3）如有密码，已正确传入 `--password`；4）会议已开始；5）等候室 / 入会审批已放行；6）会议未禁止当前身份加入（如限制外部、限制应用机器人、仅特定成员可入会）；确认后重试 |
| 应用身份权限不足 | 应用权限、租户安装、权限可访问的数据范围或 VC Agent privilege 未配置完整 | 不要执行 `auth login`。以 CLI 返回的 metadata / error envelope 为准确认缺失权限；检查应用发布/安装，以及开放平台“权限可访问的数据范围”：选择“按条件筛选”，条件为“会议的归属者 包含 与应用的可用范围一致”；仍失败再排查内测 privilege / 灰度 |
| 入会被拒绝 | 等候室 / 入会审批 / 限制外部入会 | 联系主持人放行或调整会议设置 |

## 提示

- 仅在 Agent 需要**真实加入**会议（例如参会机器人、会中助手）时使用；只拉取会议数据不需要入会。
- 入会会让机器人立即出现在参会列表；若用户要求退出 / 离开 / 结束参会，直接使用 `+meeting-leave --as bot --meeting-id <meeting.id>`。参数格式不确定时可选 `--dry-run` 预览，但不是必经步骤。
- 执行成功后，立即记录返回的 `meeting.id`，用于后续 `+meeting-leave` / `+meeting-events`。

## 参考

- [lark-vc-agent-meeting-leave](lark-vc-agent-meeting-leave.md) — 对应的离会命令
- [lark-vc-agent-meeting-list-active](lark-vc-agent-meeting-list-active.md) — 发现当前可读事件的进行中会议 ID
- [lark-vc-agent-meeting-events](lark-vc-agent-meeting-events.md) — 会中事件流
- [lark-vc-search](../../lark-vc/references/lark-vc-search.md) — 搜索历史会议记录
- [lark-vc-recording](../../lark-vc/references/lark-vc-recording.md) — 查询 minute_token
- [lark-vc-detail](../../lark-vc/references/lark-vc-detail.md) — 获取会议详情
- [lark-vc-agent](../SKILL.md) — Agent 参会能力（本 skill）
- [lark-vc](../../lark-vc/SKILL.md) — 视频会议原子域（Meeting / Note 等核心概念）
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
