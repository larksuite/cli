# im reactions

> **Prerequisite:** Before executing this command, ensure [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) has been read once in the current task for authentication, global parameters, and safety rules. Do not reread it if already loaded.

Maps to `lark-cli im reactions <method>` (`create` / `list` / `delete` / `batch_query`) — raw API only, no typed shortcut. **Run `lark-cli schema im.reactions.<method> --format pretty` for the authoritative parameters, field shapes, and value ranges.** This file covers only what `schema` cannot.

> **Heads-up — don't reach for `batch_query` by default.** The four message-pulling shortcuts (`+messages-mget`, `+chat-messages-list`, `+messages-search`, `+threads-messages-list`) already call `im.reactions.batch_query` automatically and attach the result as a `reactions` block on each message (replies inside `thread_replies` included). Use those shortcuts for any "read reactions of messages I'm already pulling" task. Reach for the raw `batch_query` API only when you have a standalone `message_id` outside that pull flow. See the main [message enrichment](lark-im-message-enrichment.md) for the contract.

## Gotchas

- **Reaction APIs return reaction *records*, not just aggregated counts.** `list` yields one entry per reaction event (who, when, which emoji); aggregate counts only appear in `batch_query`'s `success_msg_reaction_counts`. Do not expect a count-only response from `list`.
- **`operator.operator_id` is not always a user ID.** When `operator_type=app` the value is the **app ID**, not an `ou_xxx` open_id. Only when `operator_type=user` does it follow the request's `user_id_type`.

## `batch_query` Pagination

- **A batch fragment is never a complete reaction set.** `batch_query` pages *per message* — each
  `queries[]` element has its own cursor and its own size ceiling, so one call returns a slice of each
  message's reactions, not all of them. When the *complete* list for one message is required, switch to
  `im reactions list` and exhaust its pagination. An empty or short fragment from `batch_query` does not
  mean the message has no more reactions.
- **Read the cursor and size limits off `queries[].page_token` and `page_size_per_message`** in
  `lark-cli schema im.reactions.batch_query --format pretty`. Both live under `--data`, and the size
  ceiling is narrower than typical list endpoints — do not carry over a conventional page-size default.

## `emoji_type` Field

The same emoji identifier appears under **different field names and nesting depths** across the four methods — the single most common source of malformed reaction payloads:

- `im.reactions.create` — request and response use `reaction_type.emoji_type`
- `im.reactions.list` — request filter uses flat `reaction_type`; response uses `reaction_type.emoji_type`
- `im.reactions.delete` — response uses `reaction_type.emoji_type`
- `im.reactions.batch_query` — request filter uses top-level `reaction_type`; detail results use `message_reaction_items[].emoji_type`; aggregated results use `reaction_count[].reaction_type`

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

`schema` links to the official emoji documentation page but does **not** enumerate the values inline, so the
list below is the only machine-readable copy available offline. Keys are case-sensitive as written.


The following list is synchronized from the official Feishu reaction emoji documentation:

- Source page: `https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message-reaction/emojis-introduce`
- Markdown source: `https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message-reaction/emojis-introduce.md`

Current count in the fetched source: `185`.

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
```

## References

- [lark-im](../SKILL.md) — all IM commands
