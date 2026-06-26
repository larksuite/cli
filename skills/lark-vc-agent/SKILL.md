---
name: lark-vc-agent
version: 1.0.0
description: "飞书视频会议会中能力：用于让应用机器人真实加入或离开正在进行的会议，并读取当前身份可见的会中事件，如参会人加入/离开、发言、聊天、屏幕共享。适用于用户询问正在开的会议发生了什么、谁在发言、是否共享内容，或需要发现当前可读的进行中会议 ID。不负责已结束会议搜索、参会人快照、纪要、逐字稿或录制查询，这些使用 lark-vc 技能。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli vc --help"
---

# vc-agent (v1)

相关技能：

- [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) — 认证、身份切换、权限处理
- [`../lark-vc/SKILL.md`](../lark-vc/SKILL.md) — 视频会议的核心概念（Meeting / Note / Minutes 等），本 skill 直接复用，不再重复定义

## 内测提示

- 当前功能正在内测中，仅少数用户可用。忽略 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) 中的权限申请流程，以下方的指引为准。
- 如果 `lark-cli` 提示 `missing required scope(s)` / `permission_violations`，不要走普通权限申请流程；先提示用户加入早鸟群确认内测权限已开通，再按“应用身份权限配置检查”处理应用权限、安装和数据范围。
- 如果 `lark-cli` 返回 `error.code=20017` / `ErrNotInGray`，提示用户加入早鸟群：`https://go.larkoffice.com/join-chat/2f4nb0e1-fe00-4f67-bed7-25beaf533fbd`。

## 定位

本 skill 与 [`lark-vc`](../lark-vc/SKILL.md) 并列：

- **`lark-vc`** **负责"会后查询"**：搜索历史会议、参会人快照、纪要/逐字稿/录制
- **`lark-vc-agent`** **负责"会中动作"**：机器人入会 / 读取进行中会议的实时事件 / 机器人离会

按此分工路由，避免两个 skill 语义混淆。

| 用户意图示例                                                     | 应路由到                                                                                                                                                  |
| ---------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| "帮我入会 123456789"、"代我参会"、"让机器人进会旁听"                         | **本 skill** `+meeting-join`                                                                                                                           |
| "会议现在还开着，谁刚加入了"、"会议里谁在发言"、"有人共享屏幕吗"（**进行中会议**）             | **本 skill** `+meeting-events`                                                                                                                         |
| "我/某个用户现在在哪个会里"、"给我找当前可拉事件的 meeting_id"                         | **本 skill** `+meeting-list-active`                                                                                                                     |
| "退出会议"、"让机器人离开"                                            | **本 skill** `+meeting-leave`                                                                                                                          |
| "昨天那场会有谁参加过"、"搜昨天的会"、"查纪要/逐字稿/录制"                          | [`lark-vc`](../lark-vc/SKILL.md)                                                                                                                      |
| "帮我参会，结束后把纪要发到群" 等跨阶段场景                                    | 按序编排：本 skill（入会 → 读事件）→ 会议结束后用 [`lark-vc`](../lark-vc/SKILL.md) / [`lark-minutes`](../lark-minutes/SKILL.md) 拉纪要 → [`lark-im`](../lark-im/SKILL.md) 发群 |

## 身份路由

不要向用户暴露内部身份缩写；对用户只说“用户身份”或“应用身份”。

| 场景 | 使用身份 | 关键规则 |
| ---- | -------- | -------- |
| 查询当前登录用户正在参加的会议 | `--as user` | 不传 `--user-id`；拿到的 `meeting_id` 后续继续用 `--as user` 读事件 |
| 查询目标用户且应用机器人也在会中的会议 | `--as bot --user-id <user_open_id>` | `--user-id` 必须是 `ou_...`；拿到的 `meeting_id` 后续继续用 `--as bot` 读事件 |
| 用户明确要求应用机器人入会/旁听/代参会 | `--as bot` | 这是写操作，会真实产生入会记录；返回的 `meeting.id` 后续继续用 `--as bot` |

硬规则：`meeting_id` 从哪种身份路径拿到，后续 `+meeting-events` 就沿用哪种身份，除非用户明确要求切换场景（例如从“仅查询我当前会”改成“让应用机器人入会旁听”）。

## 核心场景

### 1. 加入正在进行的会议（写操作）

1. 只有用户明确表达"让 Agent **真实入会**"（参会机器人、会中助手、代为旁听、代参会）时才用 `+meeting-join`。只是查数据不要入会。
2. `+meeting-join --meeting-number` 只接受 **9 位纯数字**会议号，不是会议链接整串、也不是 `meeting_id`。如果用户只是给了 9 位会议号并询问会中内容，先按 `+meeting-list-active` 的会议号匹配流程找 `meeting_id`，不要直接入会。
3. 返回体中的 `meeting.id` **必须立刻记录**——后续 `+meeting-events` / `+meeting-leave` 都靠它，**不能用 9 位会议号替代**。
4. 入会对所有参会人可见，执行前核实 9 位会议号来源，避免误入错会。
5. 使用应用身份 `--as bot` 执行真实入会；不要用当前登录用户身份尝试让应用机器人入会。
6. 若入会失败，优先查看 `+meeting-join` reference 的错误排查段落，重点确认会议号、密码、会议状态、等候室 / 审批以及会议是否禁止当前身份加入。

### 2. 感知会中事件（读操作）

1. 用户要看"会议里正在发生什么"（参会人加入/离开、聊天、转写、屏幕共享）时，用 `+meeting-events`。
2. 输入是 **`meeting_id`**（长数字 ID），不是 9 位会议号。
3. 不依赖默认身份。`meeting_id` 来自用户身份发现时，继续用 `--as user`；来自应用身份发现或 `+meeting-join` 时，继续用 `--as bot`。身份不一致会导致空结果或权限错误。
4. **不能做会后复盘**，**不能替代参会人快照查询**。如果会议已结束：
   - 先用 `lark-cli vc +detail --meeting-ids <meeting.id>` 获取会议产物信息。
   - 再根据 `note_id`、`minute_token` 和用户意图，按 [`lark-vc`](../lark-vc/SKILL.md) 的产物决策读取正文、逐字稿或妙记。
   - 想看参会人快照：用 `vc meeting get --with-participants`（见 [`lark-vc`](../lark-vc/SKILL.md)）
5. **默认必须使用** **`--page-all`**，除非用户明确要求“只查一页”，或确实需要控制返回体大小。
6. 输出格式默认优先 `--format pretty`（时间线更易读）；只有在需要完整保留原始消息流与结构化字段时，才使用 `--format json`。
7. **必须识别分页信号**：只要响应里出现 `has_more=true`、pretty 里的 `more available`，或返回了非空 `page_token`，就不能把当前结果当作完整事件流；默认应继续分页，或明确告诉用户当前只是部分结果。
8. 保留响应里的 `page_token`，下次增量拉取直接续，不要从头再拉。
9. **只要你是基于** **`+meeting-events`** **来回答一场正在进行中的会议内容，就不能直接复用旧结果。** 无论用户是在问“现在/刚刚/最新”的状态，还是让你“总结一下这个会议讲什么”，都必须先重新拉一次当前事件流，确认拿到的是最新信息，再基于最新结果回答。只有在用户明确要求基于某次历史快照继续分析时，才可以复用旧结果。
10. 用户直接问“这个会议讲了什么 / 现在讲到哪了”且上下文没有明确 `meeting_id` 时，先用用户身份发现当前会议；如果用户明确要求应用机器人视角，或上下文已经是应用机器人参会流程，再用应用身份发现。若返回多个会议，展示候选并让用户选择。
11. 用户直接提供 **9 位会议号** 并询问会中事件/会议内容时，默认把它当作 active meeting 的筛选条件：先按当前身份查 active meetings，并在返回里匹配 `meeting_no == <9位会议号>`；匹配到唯一会议后取长数字 `meeting_id`，再用同一身份查事件。只有用户明确要求“入会 / 让应用机器人旁听 / 代我参会”时才改用 `+meeting-join`。

### 3. 离开会议（写操作）

1. 只有用户明确要求机器人退出 / 离开 / 结束参会时，才用应用身份执行 `+meeting-leave --as bot --meeting-id <长数字 meeting_id>`；不应因任务完成而执行离会。
2. `--meeting-id` **必须**是长数字会议 ID，通常来自 `+meeting-join` 返回的 `meeting.id`，也可以来自应用身份 `+meeting-list-active` 返回的 `meeting_id`。如果来自 list-active，必须确认应用机器人当前就在该会中。**不接受 9 位会议号**。
3. 离会**立即生效**，机器人从会议的参会人列表中消失，对其他参会人可见；若需要重新入会，再跑一次 `+meeting-join` 即可（非真正"不可逆"）。
4. 使用与入会或 active meeting 发现相同的应用身份离会。

### 4. 获取当前可用的进行中会议 ID（读操作）

1. `+meeting-list-active` 用来发现当前进行中的会议，并拿到后续 `+meeting-events` 需要的长数字 `meeting_id`。
2. 用户身份：`lark-cli vc +meeting-list-active --as user --format json`，用于发现当前登录用户正在参加的会议；后续 `+meeting-events` 继续 `--as user`。
3. 应用身份：`lark-cli vc +meeting-list-active --as bot --user-id <user_open_id> --format json`，`--user-id` 必须是目标用户 open_id，即 `ou_...`；返回该用户当前正在参加且应用机器人也在会中的会议。它不是全量会议搜索接口。后续 `+meeting-events` 继续 `--as bot`。
4. 如果返回空，先按当前身份解释：用户身份下表示当前用户没有可见的进行中会议；应用身份下表示没有找到“目标用户在会中且应用机器人也在会中”的当前会。
5. 如果返回多个会议，不要自动任选一个；按 `meeting_title` / `meeting_no` / `meeting_id` 展示候选，等待用户明确选择后再调用 `+meeting-events`。
6. 如果用户给了 9 位会议号，先在 active meeting 结果中按 `meeting_no` 匹配。匹配失败时，不要自动入会；只有用户明确要求应用机器人真实入会时，才询问或执行 `+meeting-join`。

### 5. Agent 参会示范

```bash
# 1. 入会，捕获 meeting.id
JOIN=$(lark-cli vc +meeting-join --as bot --meeting-number 123456789 --format json)
MID=$(echo "$JOIN" | jq -r '.data.meeting.id')

# 2. 会中轮询事件
#    默认用 --page-all 拉全当前可见事件；下次增量优先复用 page_token
#    典型间隔 10-30 秒
lark-cli vc +meeting-events --as bot --meeting-id "$MID" --page-all --format pretty

# 3. 会后可选：进入 lark-vc 获取会议产物信息，再按 note_id / minute_token 决策读取
lark-cli vc +detail --meeting-ids "$MID"
```

如果用户随后明确要求退出 / 离开 / 结束参会，再单独调用 `lark-cli vc +meeting-leave --as bot --meeting-id "$MID"`。

如果已经知道目标用户 `open_id`，且 bot 已在会中，也可以先发现当前会：

```bash
lark-cli vc +meeting-list-active --as bot --user-id <user_open_id> --format json
lark-cli vc +meeting-events --as bot --meeting-id <meeting_id> --page-all --format pretty
```

如果只是回答当前登录用户所在会议发生了什么，使用用户身份一路查：

```bash
lark-cli vc +meeting-list-active --as user --format json
lark-cli vc +meeting-events --as user --meeting-id <meeting_id> --page-all --format pretty
```

## Shortcuts

Shortcut 是对常用操作的高级封装（`lark-cli vc +<verb> [flags]`）。

| Shortcut                                                        | 类型 | 说明                                                                         |
| --------------------------------------------------------------- | -- | -------------------------------------------------------------------------- |
| [`+meeting-join`](references/lark-vc-agent-meeting-join.md)     | 写  | Join an in-progress meeting by 9-digit meeting number                      |
| [`+meeting-list-active`](references/lark-vc-agent-meeting-list-active.md) | 读  | List active meetings and discover meeting_id for event reads               |
| [`+meeting-events`](references/lark-vc-agent-meeting-events.md) | 读  | List meeting events visible to the app agent (participant joined/left, transcript, chat, share) |
| [`+meeting-leave`](references/lark-vc-agent-meeting-leave.md)   | 写  | Leave a meeting by meeting\_id                                             |

- [`+meeting-join`](references/lark-vc-agent-meeting-join.md)：入参格式、写操作可见性风险、入会失败排查。
- [`+meeting-list-active`](references/lark-vc-agent-meeting-list-active.md)：用户身份和应用身份的不同返回范围。
- [`+meeting-events`](references/lark-vc-agent-meeting-events.md)：`meeting_id` 来源、身份延续、分页和错误码（10005 / 20001 / 20002）。
- [`+meeting-leave`](references/lark-vc-agent-meeting-leave.md)：`meeting_id` 的来源与写操作可见性。

## 应用身份权限配置检查

应用身份 `--as bot` 报 `no permission`、`missing required scope(s)`、`permission_violations`、`ErrNotInGray` 或 `20017` 时，不要引导用户执行 `auth login`。按顺序检查：

1. 以 CLI 返回的 metadata / error envelope 为准，确认提示的 VC Agent 相关权限已开通。常见读取 active meeting / events 需要会中事件读取权限；应用机器人入会 / 离会需要 bot 入会写权限。
2. 应用已发布并安装到当前租户。
3. 开放平台“权限可访问的数据范围”已开通并保存。
4. 数据范围选择“按条件筛选”，条件配置为：**会议的归属者 包含 与应用的可用范围一致**。
5. 如果 scope、安装和数据范围都正确，仍返回 `ErrNotInGray` / `20017`，再按 VC Agent 内测 privilege / 灰度白名单处理，提示加入早鸟群或联系平台同学开通。

## 用户身份被拒绝时

用户身份 `--as user` 报权限或身份不支持类错误时，不要反复引导用户执行 `auth login`。先以 CLI 返回的 metadata / error envelope 为准判断：如果错误表明当前接口不支持用户身份访问，再按用户意图切换处理：

1. 如果用户只是查询当前登录用户所在的进行中会议，说明当前接口链路不支持用户身份访问，改用应用身份流程；需要目标用户 open_id，并要求应用机器人已在会中或先按用户确认执行入会。
2. 如果用户明确要求应用机器人入会、旁听、代参会或读取应用机器人可见事件，直接切到 `--as bot`，并按上面的应用身份权限配置检查处理。

## 延伸

- 查已结束会议、参会人快照、搜索历史会议 → [`lark-vc`](../lark-vc/SKILL.md)
- 会议纪要、逐字稿 → [`lark-vc`](../lark-vc/SKILL.md) 的 `+detail`
- 妙记产物（AI 总结 / 转写 / 章节）→ [`lark-minutes`](../lark-minutes/SKILL.md)
- 会后把产物发到群 / 私聊 → [`lark-im`](../lark-im/SKILL.md)
- 认证、身份切换、scope 管理 → [`lark-shared`](../lark-shared/SKILL.md)
