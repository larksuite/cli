
# vc +meeting-events

查询一场正在进行的视频会议中的会中事件列表。该命令是**读操作**，必须沿用 `meeting_id` 的来源身份：用户身份发现的会议继续用用户身份读，应用身份发现或应用机器人入会得到的会议继续用应用身份读。对已结束会议，存在一个**结束后 5 分钟内的宽限窗口**；应用身份读取时，要求应用机器人曾经在这场会里出现过。

本 skill 对应 shortcut：`lark-cli vc +meeting-events`（调用 `GET /open-apis/vc/v1/bots/events`）。

可见性边界：

- `meeting_id` 来自 `+meeting-list-active --as user`：后续读取事件继续 `--as user`。
- `meeting_id` 来自 `+meeting-list-active --as bot --user-id <user_open_id>` 或 `+meeting-join --as bot`：后续读取事件继续 `--as bot`。
- 应用身份下，应用机器人必须在该会中或参会过；应用身份 active meeting 返回的是“目标用户在会中且应用机器人也在会中”的会议，不表示可以读取任意 `meeting_id`。

## 命令

```bash
# 默认用法：全量拉取当前身份可见事件；输出易读时间线
lark-cli vc +meeting-events --as <same_identity> --meeting-id <id> --page-all --format pretty

# 指定时间范围，并拉全该时间窗内当前可见事件
lark-cli vc +meeting-events --as <same_identity> --meeting-id <id> --start 2026-04-17T15:00:00+08:00 --end 2026-04-17T16:00:00+08:00 --page-all --format pretty

# 基于上一次保存的 page_token 继续查新增事件
lark-cli vc +meeting-events --as <same_identity> --meeting-id <id> --page-token <last_page_token> --page-all --format pretty
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-id <id>` | 是 | 会议 ID（长数字 ID，不是 9 位会议号） |
| `--start <time>` | 否 | 起始时间，支持 ISO 8601 / `YYYY-MM-DD` / Unix 秒 |
| `--end <time>` | 否 | 结束时间，支持 ISO 8601 / `YYYY-MM-DD` / Unix 秒 |
| `--page-token <token>` | 否 | 从指定分页游标继续拉取下一页 |
| `--page-size <n>` | 否 | 单页模式每页大小。CLI 会自动夹紧到 `20-100`；传 `--page-all` 时固定使用 `100` |
| `--page-all` | 否 | 自动分页，直到没有更多页面为止（内部有安全上限） |

## 核心约束

### 1. 输入必须是 meeting_id，不是 9 位会议号

`--meeting-id` 必须是会议的长数字 ID。它通常来自：
- `+meeting-join` 返回体中的 `meeting.id`
- `+meeting-list-active` 返回体中的 `meeting_id`
- `+search` 结果中的 `id`

**不要**把 9 位会议号（`--meeting-number`）传给这个命令。
如果 `meeting_id` 来自 `+meeting-list-active`，后续 `+meeting-events` 必须沿用同一身份；如果返回多个会议，先让用户选择具体 `meeting_id`。

如果用户提供的是 9 位会议号且没有明确要求应用机器人入会，先按当前场景身份查 active meetings 并按 `meeting_no` 匹配。匹配到唯一项后，取该项的长数字 `meeting_id`，再用同一身份调用本命令；匹配失败时不要自动入会，除非用户明确说“入会 / 让应用机器人旁听 / 代我参会”。

### 2. 身份来源是读取事件的权限锚点

- `+meeting-events` 支持 `--as user` 和 `--as bot`。
- 用户身份路径：用户身份发现的会议继续用用户身份读取。
- 应用身份路径：应用机器人必须在会中或参会过；不要拿任意 `meeting_id` 直接查。
- 不要在拿到 `meeting_id` 后随意切换身份。身份不一致时，常见结果是空列表、`no permission` 或 `bot is not in meeting`。

### 3. 应用身份的可见性窗口

若应用机器人已离会、未入会、或会议已经无法再判断身份，后端通常会报：
- `bot is not in meeting, no permission`

更精确地说，后端当前的判断规则是：

- **会议进行中**：要求应用机器人**当前仍在会中**
- **会议已结束后的 5 分钟内**：只要应用机器人**曾经在这场会中出现过**，仍可拉取事件
- **会议结束超过 5 分钟**：按会议结束处理，通常不再返回事件流
- **应用机器人从未真实入会过**：即使会议仍在进行或刚结束，也会返回 `10005 bot is not in meeting`

### 4. 自动分页规则

- **先分清两层默认值**：
  - shortcut 本身：不传 `--page-all` 时，只查 1 页。
  - 本 skill 的默认策略：除非用户明确要求只看一页，或你确实需要控制返回体大小，否则默认**必须主动带 `--page-all`**，把当前可见事件尽量一次拉全。
- 传 `--page-all`：开启自动分页，直到没有更多页面为止。
- `--page-all` 时，CLI 固定使用最大 `page_size=100`。

执行准则：

- **默认命令模板**：`lark-cli vc +meeting-events --as <same_identity> --meeting-id <id> --page-all --format pretty`
- 如果你发现自己执行成了不带 `--page-all` 的单页查询，而响应里又出现 `has_more=true` / `more available` / 非空 `page_token`，应立刻意识到这只是部分结果。
- 遇到上述情况，默认补救方式是继续使用返回的 `page_token` 续拉，例如：`lark-cli vc +meeting-events --as <same_identity> --meeting-id <id> --page-token <returned_page_token> --page-all --format pretty`
- 只有在用户明确要求“就看第一页”“先不要翻页”时，才不要默认带 `--page-all`
- 只要你是基于 `+meeting-events` 来回答一场**正在进行中的会议内容**，就不能直接复用上一次查询结果。无论用户是在问“现在是谁在说话”“刚刚发生了什么”“最新事件有哪些”，还是让你“总结一下这个会议讲什么”，都必须先重新执行一次 `+meeting-events`，确认拿到的是最新事件流，再回答用户。只有在用户明确要求基于某次历史快照继续分析时，才可以复用旧结果。

### 5. 输出格式差异

- `--format pretty`：默认推荐格式，输出当前身份和逐条时间线，适合快速理解“发生了什么”。
- `--format json`：结构化契约，顶层包含 `meeting`、`identity`、`events`、`has_more`、`page_token`。`identity` 表示当前读取身份；事件 actor 统一含 `participant_type`、`role`、`label`；每条事件保留 `payload` 便于追溯细节。
- `--format ndjson`：输出事件行，并带 metadata 行，适合流式消费。

**选型原则**：默认先用 `--format pretty`；仅当 `pretty` 缺少完成任务所必需的结构化字段时，才改用 `--format json`。用户明确要求 JSON 或规则明确要求结构化字段时可直接用 `--format json`；需要流式消费时用 `--format ndjson`。

> **JSON 成本**：JSON 保留完整 payload，输出通常远大于 `pretty`；长会全量拉取时会显著占用上下文空间。

> **注意**：pretty 输出中的正文文本会做单行转义，真实换行会显示为 `\n`，避免打乱时间线布局。

### 6. 内容理解模式：共享文档不能只看标题

当用户意图是：

- “总结这个会议”
- “这个会议讲了什么”
- “有哪些结论 / 待办 / 关键讨论”
- “共享文档里在讲什么”

不要只基于事件时间线直接回答。此时 `+meeting-events` 只是**线索发现器**，不是最终信息源。

执行准则：

- 如果上下文没有明确 `meeting_id`，先按用户当前意图选择身份：问“我/当前用户所在会议”用 `lark-cli vc +meeting-list-active --as user --format json`；问“应用机器人可见的目标用户会议”用 `lark-cli vc +meeting-list-active --as bot --user-id <user_open_id> --format json`。返回多个会议时先让用户选择。
- 如果上下文只有 9 位会议号，先按当前身份执行 `+meeting-list-active` 并按 `meeting_no` 匹配；匹配到唯一会议后再查事件。不要为了总结会议而自动调用 `+meeting-join`。
- 确认 `meeting_id` 后，沿用其来源身份执行 `lark-cli vc +meeting-events --as <same_identity> --meeting-id <id> --page-all --format pretty` 拉取最新事件流。
- 如果事件流显示开始共享内容（JSON 事件类型为 `magic_share_started`，pretty 时间线显示“开始共享”），并包含文档标题或 URL 等线索，必须继续读取共享文档内容后再生成总结，不能只根据共享事件和文档标题概括会议内容。
- 若存在多个共享文档，按用户问题读取相关文档；处理某条文档上下文事件时必须按该 item 的 `share_id` 精确关联，不能用“最近一次共享”替代。
- 若文档读取失败，必须明确说明“以下总结仅基于会中事件流，未成功读取共享文档内容”。

### 7. 文档上下文事件消费

`document_context_changed` 是只读线索事件。需要根据该事件执行评论、章节或预览等后续处理时，必须用 `+meeting-events --page-all --format json` 读取 `share_id`、`comment_id`、`element_token` 等完整字段；仅向用户展示时间线时仍默认使用 pretty。`vc +meeting-events` 保留原始 payload，并按既有事件输出约定派生 actor 与 pretty timeline；它不会为单个事件类型扩张 JSON/NDJSON 公共 envelope，也不会查询评论、下载素材或写文件。后续 Drive/Docs 命令只能由 Agent 按下表显式选择。

#### 共享会话关联

`share_id` 标识一次共享会话。Agent 按事件时间顺序消费完整事件流，并维护共享会话状态：

1. 从 `payload.magic_share_started_items[]` 读取 `share_id` 和 `share_doc`，建立 `share_id -> share_doc` 映射并标记会话开始。同一 `share_id` 重复携带相同文档时按幂等事件处理；若指向不同文档则停止解析，不覆盖旧映射。
2. `document_context_changed_items[]` 通过自己的 `share_id` 精确查找该映射。当前契约中 item 自带的 `share_doc` 不提供文档信息；只保留它的原始值，不作为 URL/title 来源，也不做冲突判定。
3. `payload.magic_share_ended_items[]` 使用相同 `share_id` 标记该会话结束。历史映射可保留用于解释本批次中结束前已发生的上下文事件，但不能再作为新的活动共享会话。
4. 增量拉取从会话中途开始且本地没有对应映射时，重新拉取包含 `magic_share_started` 的完整事件流；仍无法命中则标记未解析。禁止回退到当前文档、最近一次共享或其他 `share_id`。

#### 字段合同

| 路径 | 含义与处理 |
| --- | --- |
| `payload.magic_share_started_items[].share_id/share_doc` | 建立一次共享会话与文档 URL/title 的映射。缺 `share_id` 时不建立映射。 |
| `payload.magic_share_ended_items[].share_id` | 结束同一 `share_id` 的共享会话；不得结束其他映射。 |
| `payload.document_context_changed_items[]` | 结构化消费按原序读取；pretty timeline 沿用统一时间排序。每项恰有一个已知 context 才生成 pretty 条目，未知/歧义项只保留 raw。 |
| `item.operator` | 当前 item 的 actor；缺 ID/name 时不猜共享发起人。 |
| `item.share_id` | 当前上下文所属共享会话；用它精确查找 `magic_share_started` 建立的 `share_doc` 映射。 |
| `item.share_doc.url/title` | 当前不作为文档元信息来源；保留在 raw payload 以兼容未来扩展。文档 URL/title 只从同 `share_id` 的 `magic_share_started` 映射取得。 |
| `item.time` | Unix 毫秒字符串；缺失或非法时 timeline 回退到事件时间。 |
| `item.comment_focus.comment_id/focused` | `focused=true` 才精确查询一个 comment ID；`false` 是清除焦点，零查询。 |
| `item.section_location.parent_titles/title/level` | `section_path` 按 parent 原序再追加 title，trim 后丢弃空段，以 ` > ` 连接；`level` 仅作诊断，不参与截断或补层。 |
| `item.element_preview.action/element_type/element_token/block_id` | 只有 `open + image + token`、`open + whiteboard + token` 可在明确预览意图下路由；其他组合零调用。 |
| 事件公共 envelope | JSON/NDJSON 只使用既有 `event_id/event_type/event_time/actors/payload`；不新增顶层 `summary/section_path`，也不发明 `derived.document_context`。 |
| 事件 `payload` | 原始恢复面；未知字段保留，顶层空数组沿用所有会议事件共用的压缩规则，派生字段不会写回 payload。 |

#### 评论聚焦：只查一个 ID

先读取当前 item 的 `share_id` 和 `comment_focus.comment_id`，再按“共享会话关联”取得 `share_doc.url`。优先把完整 URL 传给现有 shortcut，由它解析实际 `file_token/file_type`（含 Wiki 解包）；如果上游只留下裸 token，则必须同时提供已解析且受支持的 `file_type`。

```bash
# 推荐：share_doc.url 完整可用
lark-cli drive +batch-query-comments \
  --as <same_identity> \
  --url "<share_doc.url>" \
  --comment-ids "<comment_focus.comment_id>" \
  --format json

# 只有已经可靠解析出裸 token/type 时使用
lark-cli drive +batch-query-comments \
  --as <same_identity> \
  --token "<file_token>" \
  --type "<file_type>" \
  --comment-ids "<comment_focus.comment_id>" \
  --format json
```

该 shortcut 对应 `drive.file.comments.batch_query`，请求体必须只有 `comment_ids:["<当前comment_id>"]`。响应处理规则：

1. 整个响应 `items` 长度必须恰为 1，且 `items[0].comment_id` 必须与请求 ID 完全相等。`items` 为空、多于 1 项或唯一项 ID 不同都停止；即使多项中恰有一项匹配，也不得挑选该项继续。失败时保留 `share_doc/comment_id`，禁止改用 `drive +list-comments` 扫描整篇文档。
2. `item.quote` 是引用位置；评论正文和回复在 `item.reply_list.replies`，其中第一条是根评论。
3. 完整性看命中评论卡片的 **`item.has_more`**，不是外层评论分页，也不是根据非空 `page_token` 猜测。`item.has_more=false` 时直接使用内嵌列表，零 `+list-replies` 调用。
4. `item.has_more=true` 时忽略截断列表，从**不带 `--page-token` 的第一页**开始重建完整 replies：

```bash
lark-cli drive +list-replies \
  --as <same_identity> \
  --url "<share_doc.url>" \
  --comment-id "<comment_focus.comment_id>" \
  --page-size 100 \
  --format json

lark-cli drive +list-replies \
  --as <same_identity> \
  --url "<share_doc.url>" \
  --comment-id "<comment_focus.comment_id>" \
  --page-size 100 \
  --page-token "<returned_page_token>" \
  --format json
```

第一页 `items[0]` 才是根评论；后续页的 `items[0]` 是普通回复。按页原序累积，直到页级 `has_more=false`。如果 `has_more=true` 但 `page_token` 为空、与已用 token 重复、API/权限失败或 comment ID 改变，立即停止并标记为 `partial`；保留已经取得的内容和原始标识，不循环、不重复根评论、不声称完整。

#### 章节定位

结构化消费直接读取当前 `section_location` item。pretty timeline 会按 `parent_titles` 原序追加 `title`，trim 后丢弃空段，并以 ` > ` 连接；多个 section item 分别展示，不选择其中一个覆盖事件级标量；标题全空时不生成 pretty 条目，只保留 raw。该路径是本地展示派生，不写回 JSON/NDJSON，也不需要或允许为它新增 API 查询。

#### 元素预览：显式白名单

只有用户或上层 Agent 明确要求预览，并且 item 命中下表时才执行。两个命令都会写入 `--output`，因此输出路径必须由本次调用显式选择；不得默认覆盖已有文件。

| action | element_type | token 条件 | 精确命令 |
| --- | --- | --- | --- |
| `open` | `image` | `element_token` 非空 | `lark-cli docs +media-preview --as <same_identity> --token "<element_token>" --output "<explicit-path>"` |
| `open` | `whiteboard` | `element_token` 非空 | `lark-cli docs +media-download --as <same_identity> --type whiteboard --token "<element_token>" --output "<explicit-path>"` |
| `close` | `image`/`whiteboard` | 任意 | 零调用；pretty 只记录预览关闭 |
| 未知 | 任意 | 任意 | 零调用；不生成 pretty 条目，只保留 raw |
| `open` | 未知/空 | 任意 | 零调用；禁止把原值透传到 `--type` |
| `open` | `image`/`whiteboard` | token 为空 | 零调用；保留 `block_id/element_type/action` 并提示缺 token |

#### 失败恢复

- parser 遇到未知字段、歧义 one-of 或单 item 缺字段：保留整个事件 `payload`、`event_id/event_type/event_time` 和可用 sibling；该 item 不生成 pretty 条目，也不合成通用描述。
- `share_id` 缺失、映射未命中或 `share_doc` 冲突：回显 `share_id`、可用的 `share_doc.url/title` 与 `comment_id`；必要时重新拉取完整事件流，仍无法关联则停止，不用最近一次共享兜底。
- `share_doc` 无法解析：回显 `share_id`、`share_doc.url/title` 与 `comment_id`，提示需要有效文档 URL 或已确认的 `file_token/file_type`；不要猜 type。
- Drive API/权限失败：保留精确 batch-query 命令与 `comment_id`，根据 CLI 的 `missing_scopes/hint` 恢复权限后重试；不要扫描全部评论。
- Docs 预览失败：保留 `action/element_type/element_token/block_id` 和用户选择的输出路径，修复权限或 token 后重试同一白名单命令；不要让 `meeting-events` 自动下载兜底。
- 未知 context/type/action：保留 raw 并说明当前 CLI 没有安全路由；不得自动调用 overwrite、download 或任何猜测的 shortcut。

### 8. 关于 `page_token` 的返回与续拉

- 不管这次是只查 1 页，还是通过 `--page-all` 已经把当前可见事件都拿完，都应把最后拿到的 `page_token` 一并保留下来并返回给用户。
- 只要响应里出现 `has_more=true`、pretty 里出现 `more available`，或返回了非空 `page_token`，就必须先判断当前结果是否完整；默认情况下，这意味着你还需要继续分页。
- 如果没有使用 `--page-all`，但出现了上述分页信号，默认应继续用返回的 `page_token` 拉下一页，而不是直接结束。只有在用户明确不要继续翻页时，才可以停止并明确说明当前结果不完整。
- 下次继续“查新增事件”时，应优先复用上一次保存的 `page_token`，而不是从头全量再拉一次。
- 只有在用户明确要求“从头回放全部事件”时，才忽略历史 `page_token`，重新从第一页开始。
- 但如果用户要你回答的是**当前这场会正在讲什么**，而不是“上一次之后新增了什么”，也要先做一次新的事件查询，再决定是否需要基于旧 `page_token` 继续补拉。

## 返回结构

常见顶层字段：

| 字段 | 说明 |
|------|------|
| `meeting` | 会议身份与时间状态，包含 `id/topic/meeting_no/start_time/end_time/status` |
| `identity` | 当前读取身份，包含 `id/name/participant_type/label` |
| `events` | 结构化事件列表；每条事件沿用 `event_id/event_type/event_time/actors/payload` 公共 envelope，事件专属数据保留在 `payload` |
| `warnings` | 非阻断告警列表；事件列表本身仍可使用 |
| `has_more` | 是否还有下一页 |
| `page_token` | 下一页游标 |

事件 `event_type` 常见类型：

| event_type | 含义 |
|-----------|------|
| `participant_joined` | 有参会人加入会议 |
| `participant_left` | 有参会人离开会议 |
| `chat_received` | 收到会中聊天消息 |
| `transcript_received` | 收到转写文本 |
| `magic_share_started` | 开始共享内容 / 文档 |
| `magic_share_ended` | 结束共享 |
| `document_context_changed` | 评论聚焦、章节定位或元素预览上下文变化 |
| `countdown_changed` | 会中倒计时被设置、延长、提前结束、关闭窗口，或自然结束、临近提醒 |

### Forwarding meeting chat and reactions to IM

转发到 IM 时，Agent 必须先用 `+meeting-events --format json` 的结构化事件构造完整 Feishu `post` 内容，再调用 IM 发送 shortcut。不要解析 pretty/Markdown 输出，也不要先生成纯文本或 Markdown 后再期望 IM 侧二次识别 reaction。

对 `event_type == "chat_received"` 的事件逐项处理 `payload.chat_received_items`：

- `message_type == 3` 是会中 reaction；构造 IM `post` 内容时，以 [`lark-im` reaction emoji 列表](../../lark-im/references/lark-im-reactions.md) 作为 IM `emotion` 白名单。白名单内的 key 写成 `{"tag":"emotion","emoji_type":"<content>"}`，例如 `JIAYI`、`THUMBSUP`、`OK`。
- 对不在 IM reaction emoji 白名单内的 reaction key，保留原始 key 但写成文本节点，例如 `{"tag":"text","text":"[<content>]"}`；不应直接写入 `emotion.emoji_type`，否则 IM 发送会失败。
- 不要大小写归一化或猜测映射；`content` 是原始 reaction key，必须原样判断。
- 其他聊天消息写成文本节点：`{"tag":"text","text":"<content>"}`。
- 最终调用 `im +messages-send --msg-type post --content '<post-json>'`，其中 `<post-json>` 应混合使用可渲染 `emotion` 节点和文本 fallback；不要用 `--markdown` 承载会中 reaction。
- 如果 IM 返回 `message_content_emotion_tag's emoji_type is invalid`，只降级非法 reaction key，不要把整条消息退化成纯文本。
- 如果用户原始请求已经明确“发给我 / 推送给我 / 发到我的聊天框 / 发到我的单聊”，这已经覆盖本次收件人、内容和发送动作，直接发送给当前用户，不要再二次询问“是否发送”。
- 默认用应用身份 `--as bot` 发送；只有用户明确要求“用本人身份 / 用户身份发送”时才切到 `--as user`。
- 如果用户要求发给某个群或其他人但收件人不可唯一确定，只询问缺失的收件人信息。

```bash
lark-cli vc +meeting-events \
  --as <same_identity> \
  --meeting-id <id> \
  --page-all \
  --format json
```

如果用户已经要求“发给我”，`<open_id>` 使用当前用户的 open_id；需要解析时先用用户查询能力获取当前用户信息。构造 IM post 时只发送用户请求范围内的会中内容，不要把前一条自然语言预览当作发送内容。

## pretty 输出示例

```text
会议主题：张三的视频会议
会议时间：2026-04-17 15:28:52（进行中）

[00:00:33] 明日之虾BOE(ou_xxx) 加入了会议
[00:00:41] 张三(ou_xxx): [text] 6666
[00:00:44] 张三(ou_xxx) 开始共享《智能纪要：飞书20251022-140223 2026年3月9日》
           URL: https://...
[00:01:32] 张三(ou_xxx): [reaction] JIAYI
```

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|
| `meeting-id` | `+meeting-join` 返回的 `meeting.id`；或 `+meeting-list-active` 返回的 `meeting_id`；或 `+search` 结果中的 `id`。必须同时记录来源身份 |
| `start` / `end` | 用户给出的时间范围；如未给出则默认取全量可见事件 |
| `page-token` | 上一页或上一次查询结果中保存的 `page_token`；建议持久化保存，便于下次继续拉取新增事件 |

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--meeting-id is required` | 未传入 `--meeting-id` | 传入长数字 `meeting.id` |
| `10005 bot is not in meeting` | 使用应用身份读取，但应用机器人从未真实入会该会议；或会议已结束但应用机器人从未在会中出现过 | 如果 `meeting_id` 来自用户身份发现，改回 `--as user`；如果确实要应用身份读取，先让应用机器人入会或确认它曾参会后再用 `--as bot`。**如果只是想看参会人快照，改用 `lark-cli vc meeting get --params '{"meeting_id":"<meeting.id>","with_participants":true}'`** |
| 用户身份无权限 / 不可见 | 当前用户不是该会议的可见参与者，或 `meeting_id` 不是从用户身份路径获得 | 不要反复执行 `auth login`。先确认 `meeting_id` 是否来自 `+meeting-list-active --as user`；如果用户明确要切到应用身份，再通过 `+meeting-list-active --as bot --user-id <user_open_id>` 获取应用身份可读的 `meeting_id`，或在用户明确同意后让应用机器人入会，再用 `+meeting-events --as bot` 读取 |
| `20001 meeting_status_MEETING_END` | 会议已结束且已超出后端允许的 5 分钟宽限窗口 | 本接口不再适合继续拉取事件。先用 `lark-cli vc +detail --meeting-ids <meeting.id>` 获取会议产物信息，再根据 `note_display_type` / `note_id` / `minute_token` 和用户意图选择纪要正文、逐字稿或妙记；参会人请用 `lark-cli vc meeting get --params '{"meeting_id":"<meeting.id>"}' --with-participants` |
| `20002 meeting not exist` | `meeting_id` 错误，或会议实例当前已不可获取（常见于把 9 位会议号当 meeting_id 传） | 确认传入的是长数字 `meeting_id`，不是 9 位会议号 |
| 应用身份权限不足 | 应用权限、租户安装或权限可访问的数据范围未配置完整 | 不要执行 `auth login`。请应用开发者开通 `vc:meeting.bot.join:write`；再检查应用发布/安装和权限可访问的数据范围；配置正确仍失败时，保留错误码和 `log_id`，按服务端权限异常排查 |
| `HTTP 404` / `HTTP 500` | 服务端当前无法找到或处理该会议实例 | 换一个正在进行且 bot 可见的 meeting_id，或排查后端问题 |

## 提示

- 这是**会中事件流**查询，不适合拿来搜历史会议记录；搜历史会议请用 `+search`。
- 如果会议已经结束，不要卡在 `+meeting-events`：  
  - 先用 `lark-cli vc +detail --meeting-ids <meeting.id>` 获取会议产物信息。
  - 再根据 `note_display_type`、`note_id`、`minute_token` 和用户意图，按 `lark-meeting` 的产物决策读取纪要正文、逐字稿或妙记。
- 事件列表是否完整，取决于应用机器人何时入会、何时离会，以及后端当前可见的会中事件范围。对于已结束会议，通常只在**结束后 5 分钟内**、且应用机器人**曾经在会中**时还能继续拉到事件。
- 查询"谁参加过某会议"请用 `vc meeting get --params '{"meeting_id":"<id>","with_participants":true}'`——这是参会人**快照** API，不依赖 bot 是否参会，对已结束会议也可查；**不要** 用 `+meeting-events` 做参会人查询。

## 相关场景
- [会中事件与会中互动](../scenes/live-meeting-interact.md)
- [应用机器人参会与会中互动](../scenes/live-meeting-attend.md)
