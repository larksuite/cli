# vc +meeting-message-send

发送会中文本消息或会中 reaction emoji。

本 skill 对应 shortcut：`lark-cli vc +meeting-message-send`（调用 `POST /open-apis/vc/v1/bots/message`）。

## 适用场景

- 用户要求“在会里发一句话”“提示大家”“给当前会议发消息”。
- 用户要求发送会中表情，例如“发个点赞”“发个 OK”“发个爱心”。
- 用户要求表达会中反馈，例如“听不到”“看不到”“声音清楚”“效果不错”。
- 只用于正在进行中的会议；已结束会议不支持。

## 身份规则

`meeting_id` 从哪种身份路径拿到，发送消息时就沿用哪种身份：

| meeting_id 来源 | 发送时身份 |
| --- | --- |
| `+meeting-list-active --as user` | `+meeting-message-send --as user` |
| `+meeting-list-active --as bot --user-id <user_open_id>` | `+meeting-message-send --as bot` |
| `+meeting-join --as bot` 返回的 `meeting.id` | `+meeting-message-send --as bot` |

不要把用户身份发现的 `meeting_id` 改用应用身份发送，也不要把应用身份发现的 `meeting_id` 改用用户身份发送，除非用户明确要求切换。

## 参数

| 参数 | 说明 |
| --- | --- |
| `--meeting-id` | 必填，长数字 `meeting_id`，不是 9 位会议号 |
| `--msg-type` | 可选，`text` 或 `reaction`；只传 `--text` 或只传 `--emoji-type` 时可自动推断 |
| `--text` | 文本消息内容 |
| `--emoji-type` | 会中 reaction emoji key，大小写敏感，必须从本文“完整 `emoji_type` 列表”中选择 |
| `--uuid` | 可选，幂等 key；不传则服务端生成 |

CLI 会把 `--text` 或 `--emoji-type` 统一映射到 OpenAPI 请求体的 `content` 字段；`meeting_id` 也在请求体中传递。

## 文本消息

```bash
lark-cli vc +meeting-message-send --as user --meeting-id <meeting_id> --text "稍等，我在看文档"
```

文本消息会出现在会议内的文本互动区。不要把它当成绑定群消息发送能力；如果用户明确要求发到群聊，路由到 `lark-im`。

## 会中表情

会中 reaction 支持普通 Feishu reaction emoji，也支持 4 个 VC 反馈 key。

常见语义：

| 用户表达 | 推荐 `emoji_type` |
| --- | --- |
| 点赞、赞一下、认可 | `THUMBSUP` |
| +1、加一、附议、同上 | `JIAYI` |
| OK、好的 | `OK` |
| 收到、了解 | `Get` |
| 爱心、红心 | `HEART` |
| 喜欢、爱了 | `LOVE` |
| 比心 | `FINGERHEART` |
| 看起来没问题、可以继续 | `LGTM` |
| 搞定、已完成 | `DONE` |
| -1、减一 | `MinusOne` |
| 不赞同、踩 | `ThumbsDown` |
| 听不到、没声音 | `VC_NoSound` |
| 看不到、画面有问题 | `VC_CanNotSee` |
| 声音清楚 | `VC_SoundsClear` |
| 会议画面效果不错、画面看起来可以 | `VC_LooksGood` |

```bash
lark-cli vc +meeting-message-send --as bot --meeting-id <meeting_id> --msg-type reaction --emoji-type LOVE
lark-cli vc +meeting-message-send --as bot --meeting-id <meeting_id> --msg-type reaction --emoji-type VC_NoSound
```

不要编造列表外的 `emoji_type`，也不要把 mixed-case 值改成全大写，例如 `EatingFood`、`CheckMark`、`StatusInFlight` 都要按原值传。

如果用户给的是自然语言语义，可以在下方列表中选择语义最接近的 key；如果不确定，先向用户确认。

### 完整 `emoji_type` 列表

以下列表与 IM reaction 官方 emoji 列表保持一致，并额外包含 VC 会中特定反馈 key：

```text
OK, THUMBSUP, THANKS, MUSCLE, FINGERHEART, APPLAUSE, FISTBUMP, JIAYI
DONE, SMILE, BLUSH, LAUGH, SMIRK, LOL, FACEPALM, LOVE
WINK, PROUD, WITTY, SMART, SCOWL, THINKING, SOB, CRY
ERROR, NOSEPICK, HAUGHTY, SLAP, SPITBLOOD, TOASTED, GLANCE, DULL
INNOCENTSMILE, JOYFUL, WOW, TRICK, YEAH, ENOUGH, TEARS, EMBARRASSED
KISS, SMOOCH, DROOL, OBSESSED, MONEY, TEASE, SHOWOFF, COMFORT
CLAP, PRAISE, STRIVE, XBLUSH, SILENT, WAVE, WHAT, FROWN
SHY, DIZZY, LOOKDOWN, CHUCKLE, WAIL, CRAZY, WHIMPER, HUG
BLUBBER, WRONGED, HUSKY, SHHH, SMUG, ANGRY, HAMMER, SHOCKED
TERROR, PETRIFIED, SKULL, SWEAT, SPEECHLESS, SLEEP, DROWSY, YAWN
SICK, PUKE, BETRAYED, HEADSET, EatingFood, MeMeMe, Sigh, Typing
Lemon, Get, LGTM, OnIt, OneSecond, VRHeadset, YouAreTheBest, SALUTE
SHAKE, HIGHFIVE, UPPERLEFT, ThumbsDown, SLIGHT, TONGUE, EYESCLOSED, RoarForYou
CALF, BEAR, BULL, RAINBOWPUKE, ROSE, HEART, PARTY, LIPS
BEER, CAKE, GIFT, CUCUMBER, Drumstick, Pepper, CANDIEDHAWS, BubbleTea
Coffee, Yes, No, OKR, CheckMark, CrossMark, MinusOne, Hundred
AWESOMEN, Pin, Alarm, Loudspeaker, Trophy, Fire, BOMB, Music
XmasTree, Snowman, XmasHat, FIREWORKS, 2022, REDPACKET, FORTUNE, LUCK
FIRECRACKER, StickyRiceBalls, HEARTBROKEN, POOP, StatusFlashOfInspiration, 18X, CLEAVER, Soccer
Basketball, GeneralDoNotDisturb, Status_PrivateMessage, GeneralInMeetingBusy, StatusReading, StatusInFlight, GeneralBusinessTrip, GeneralWorkFromHome
StatusEnjoyLife, GeneralTravellingCar, StatusBus, GeneralSun, GeneralMoonRest, MoonRabbit, Mooncake, JubilantRabbit
TV, Movie, Pumpkin, BeamingFace, Delighted, ColdSweat, FullMoonFace, Partying
GoGoGo, ThanksFace, SaluteFace, Shrug, ClownFace, HappyDragon
VC_CanNotSee, VC_NoSound, VC_LooksGood, VC_SoundsClear
```

## 9 位会议号处理

如果用户给的是 9 位会议号并要求发送会中消息：

1. 先按当前身份执行 `+meeting-list-active`。
2. 在返回结果中按 `meeting_no` 匹配该 9 位会议号。
3. 匹配到唯一会议后取长数字 `meeting_id`。
4. 用发现该会议时的同一身份执行 `+meeting-message-send`。

匹配失败时不要自动入会。只有用户明确要求“让应用机器人入会/旁听/代参会”时，才改用 `+meeting-join`。

## 权限和前置条件

- 用户身份：当前用户必须正在该会议中。
- 应用身份：应用机器人必须正在该会议中。
- 会议需要开启会中智能体/Agent 能力开关。
- 需要 `vc:meeting.message:write` 权限；应用身份还需要应用已安装、数据范围已配置。

应用身份权限错误时，不要引导用户反复 `auth login`。按主 skill 的“应用身份权限配置检查”处理。

## 相关

- [lark-vc-agent-meeting-list-active](lark-vc-agent-meeting-list-active.md) — 发现当前进行中会议 ID
- [lark-vc-agent-meeting-events](lark-vc-agent-meeting-events.md) — 读取会中事件
- [lark-vc-agent-meeting-join](lark-vc-agent-meeting-join.md) — 应用机器人入会
