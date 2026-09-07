# vc +meeting-countdown

设置、延长、提前结束或关闭会中倒计时窗口。

本 skill 对应 shortcut：`lark-cli vc +meeting-countdown`（调用 `POST /open-apis/vc/v1/bots/countdown`）。

## 适用场景

- 用户要求在正在进行中的会议里设置倒计时，例如“设置 5 分钟倒计时”。
- 用户要求延长当前倒计时，例如“再延长 2 分钟”。
- 用户要求提前结束或关闭当前倒计时。
- 只用于正在进行中的会议；已结束会议不支持。

## 身份规则

`meeting_id` 从哪种身份路径拿到，操作倒计时时就沿用哪种身份：

| meeting_id 来源 | 操作时身份 |
| --- | --- |
| `+meeting-list-active --as user` | `+meeting-countdown --as user` |
| `+meeting-list-active --as bot --user-id <user_open_id>` | `+meeting-countdown --as bot` |
| `+meeting-join --as bot` 返回的 `meeting.id` | `+meeting-countdown --as bot` |

不要把用户身份发现的 `meeting_id` 改用应用身份操作，也不要把应用身份发现的 `meeting_id` 改用用户身份操作，除非用户明确要求切换。

## 参数

| 参数 | 说明 |
| --- | --- |
| `--meeting-id` | 必填，长数字 `meeting_id`，不是 9 位会议号 |
| `--action` | 必填，`set`、`prolong`、`end_in_advance` 或 `close_window` |
| `--duration` | 倒计时时长，单位是分钟；`set` 和 `prolong` 必填 |
| `--need-play-audio-at-end` | 仅 `set` 可用，表示倒计时结束时播放提示音 |
| `--reminder-before-end` | 仅 `set` 可用，提醒点单位是分钟；只支持传一个值 |

`duration` 和 `reminder_before_end` 都是分钟；提醒时间必须大于 0 且小于 `duration`。

## 设置倒计时

```bash
lark-cli vc +meeting-countdown --as user \
  --meeting-id <meeting_id> \
  --action set \
  --duration 5 \
  --need-play-audio-at-end \
  --reminder-before-end 1
```

Dry-run 请求体示例：

```json
{
  "meeting_id": "<meeting_id>",
  "action": "set",
  "duration": 5,
  "need_play_audio_at_end": true,
  "reminder_before_end": 1
}
```

## 延长倒计时

```bash
lark-cli vc +meeting-countdown --as bot \
  --meeting-id <meeting_id> \
  --action prolong \
  --duration 2
```

## 提前结束或关闭倒计时

```bash
lark-cli vc +meeting-countdown --as user --meeting-id <meeting_id> --action end_in_advance
lark-cli vc +meeting-countdown --as user --meeting-id <meeting_id> --action close_window
```

提前结束或关闭倒计时窗口时不要传 `--duration`、`--need-play-audio-at-end` 或 `--reminder-before-end`。

## 9 位会议号处理

如果用户给的是 9 位会议号并要求操作倒计时：

1. 先按当前身份执行 `+meeting-list-active`。
2. 在返回结果中按 `meeting_no` 匹配该 9 位会议号。
3. 匹配到唯一会议后取长数字 `meeting_id`。
4. 用发现该会议时的同一身份执行 `+meeting-countdown`。

匹配失败时不要自动入会。只有用户明确要求“让应用机器人入会/旁听/代参会”时，才改用 `+meeting-join`。

## 权限和前置条件

- 用户身份：当前用户必须正在该会议中。
- 应用身份：应用机器人必须正在该会议中。
- 需要 `vc:meeting.interaction:write` 权限；应用身份还需要应用已安装、数据范围已配置。

应用身份权限错误时，不要引导用户反复 `auth login`。按主 skill 的“应用身份权限配置检查”处理。

## 相关

- [lark-vc-meeting-list-active](lark-vc-meeting-list-active.md) — 发现当前进行中会议 ID
- [lark-vc-meeting-events](lark-vc-meeting-events.md) — 读取会中事件
- [lark-vc-meeting-message-send](lark-vc-meeting-message-send.md) — 发送会中文本或 reaction
- [lark-vc-agent-meeting-join](lark-vc-agent-meeting-join.md) — 应用机器人入会
