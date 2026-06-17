# im reactions

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli schema im.reactions.*` (no typed shortcut). **Run `lark-cli schema im.reactions.create|list|delete|batch_query --format pretty` for the authoritative parameters, the `reaction_type` emoji-key set, and field shapes.** This file covers only what `schema` cannot.

## Gotchas

- **Caller must be in the conversation** for `create`, `list`, and `delete`. `batch_query` has no this constraint — it is the only reactions method where bot or user can query messages from conversations they are not currently in.
- **`delete` can only remove reactions added by itself** (the calling identity). You cannot delete another user's or another bot's reaction, even as bot admin.
- **`operator_id` is the `app_id` when `operator_type=app`**, not an `ou_xxx` open_id. The type of the user-operator ID is controlled by `user_id_type` (default: `open_id`).
- **Don't call `batch_query` directly for messages you are already pulling.** The four message shortcuts (`+messages-mget`, `+chat-messages-list`, `+messages-search`, `+threads-messages-list`) call `batch_query` automatically and attach the result as a `reactions` block on each message (including replies inside `thread_replies`). Use the raw `batch_query` only for standalone `message_id`s outside that pull flow. See [message enrichment](lark-im-message-enrichment.md) for the contract.
- **`batch_query` pagination is per-message, not global.** Each entry in `queries[]` carries its own `page_token`. To page a specific message, set `queries[].page_token` for that entry; other entries in the same request start fresh. `success_msg_reaction_details[].has_more` indicates whether a given message has more pages.
- **`batch_query` `page_size_per_message` is capped at 10** (range 1..10 per schema). For high-reaction messages, expect multiple round trips per message.
- **`list` `page_size` is capped at 50** (schema: `range: <=50`, default 20).
- **`reaction_type` filter naming asymmetry across methods:**
  - `create` / `delete` response: nested as `reaction_type.emoji_type`
  - `list` request filter: flat string param `reaction_type`; response: nested `reaction_type.emoji_type`
  - `batch_query` request filter: flat string `reaction_type`; detail items: `message_reaction_items[].emoji_type` (flat, not nested); aggregated counts: `reaction_count[].reaction_type` (flat)

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Full `emoji_type` key list (185 values):** schema only links to the Feishu docs page (`https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1/message-reaction/emojis-introduce`); it does not enumerate the values inline. Keys include: `OK THUMBSUP THANKS MUSCLE FINGERHEART APPLAUSE FISTBUMP JIAYI DONE SMILE BLUSH LAUGH SMIRK LOL FACEPALM LOVE WINK PROUD WITTY SMART SCOWL THINKING SOB CRY ERROR NOSEPICK HAUGHTY SLAP SPITBLOOD TOASTED GLANCE DULL INNOCENTSMILE JOYFUL WOW TRICK YEAH ENOUGH TEARS EMBARRASSED KISS SMOOCH DROOL OBSESSED MONEY TEASE SHOWOFF COMFORT CLAP PRAISE STRIVE XBLUSH SILENT WAVE WHAT FROWN SHY DIZZY LOOKDOWN CHUCKLE WAIL CRAZY WHIMPER HUG BLUBBER WRONGED HUSKY SHHH SMUG ANGRY HAMMER SHOCKED TERROR PETRIFIED SKULL SWEAT SPEECHLESS SLEEP DROWSY YAWN SICK PUKE BETRAYED HEADSET EatingFood MeMeMe Sigh Typing Lemon Get LGTM OnIt OneSecond VRHeadset YouAreTheBest SALUTE SHAKE HIGHFIVE UPPERLEFT ThumbsDown SLIGHT TONGUE EYESCLOSED RoarForYou CALF BEAR BULL RAINBOWPUKE ROSE HEART PARTY LIPS BEER CAKE GIFT CUCUMBER Drumstick Pepper CANDIEDHAWS BubbleTea Coffee Yes No OKR CheckMark CrossMark MinusOne Hundred AWESOMEN Pin Alarm Loudspeaker Trophy Fire BOMB Music XmasTree Snowman XmasHat FIREWORKS 2022 REDPACKET FORTUNE LUCK FIRECRACKER StickyRiceBalls HEARTBROKEN POOP StatusFlashOfInspiration 18X CLEAVER Soccer Basketball GeneralDoNotDisturb Status_PrivateMessage GeneralInMeetingBusy StatusReading StatusInFlight GeneralBusinessTrip GeneralWorkFromHome StatusEnjoyLife GeneralTravellingCar StatusBus GeneralSun GeneralMoonRest MoonRabbit Mooncake JubilantRabbit TV Movie Pumpkin BeamingFace Delighted ColdSweat FullMoonFace Partying GoGoGo ThanksFace SaluteFace Shrug ClownFace HappyDragon`
