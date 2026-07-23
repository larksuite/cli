# Calendar/VC/Doc 跨领域关联关系、领域知识和职责边界说明

本文档说明飞书日历（Calendar）、视频会议（VC）、云文档（Doc）三个域之间的关联关系，帮助理解跨域数据流转和产物依赖。

## Calendar 域

- **lark-calendar skill** 负责日历与日程管理，包括创建、查询、修改、删除日程等操作。
- **日程与会议的关系**：日程可以用于提前预约会议，确定会议时间、参与人、会议室、会议主题等信息。日程上可以关联飞书/Lark 视频会议。
- **并非所有会议都通过日程发起**：即时会议不经过日程预约，直接创建。因此，仅查询日程数据无法覆盖所有会议，搜索历史会议应优先使用 `vc +search`。
- **日程上的用户会议纪要**：用户可以在日程上绑定自己的会议纪要文档（MeetingNotes），用于手动记录会议相关信息。该文档与 AI 生成的智能纪要（`note_doc_token`）是不同的文档，相互独立。

> **路由规则**：查询过去已结束的会议 → `lark-vc`；查询未来日程/待开的会 → `lark-calendar`；查询"今天有哪些会议" → 两者结合（`vc +search` 查已结束 + `calendar` 查未开始）。

## VC 域

- **lark-vc skill** 负责视频会议管理，包括搜索历史会议、查询会议产物（智能纪要、逐字稿、妙记等）、查询参会人快照等操作。
- **会议类型**：会议可以是日程会议（由日程发起，有对应的 `calendar_event_id`），也可以是即时会议等其他类型。

### 会议产物

会议产物取决于会中开启的功能，分为两条独立链路：

#### 链路一：开启「AI 总结」

会中开启「AI 总结」功能后，产生以下产物：

| 产物 | Token 字段 | 本质 | 说明 |
|------|-----------|------|------|
| 智能纪要 | `note_doc_token` | 飞书文档 | AI 生成的会议总结与待办 |
| 逐字稿 | `verbatim_doc_token` | 飞书文档 | 完整的逐句发言记录（含说话人、时间戳）— **仅 `note_display_type=normal` 时是可读的独立文档**；`unified` 纪要的逐字稿用 `note +transcript --note-id <note_id>` 拉取（见下方 [Note 域](#note-域)） |
| 共享文档 | `shared_doc_token` | 飞书文档 | 会中投屏共享的文档信息 |

> **授权特性**：智能纪要总结文档及其逐字稿文档（总结文档尾部会挂逐字稿链接与会中投屏共享文档链接）在会后**自动授权给参会人**，参会人通常可直接读取，无需额外申请。

此外，还存在**用户会议纪要（MeetingNotes）**，对应 `meeting_note` 字段。这是用户主动绑定到日程的纪要文档，通常用于会前记录会议相关内容，与智能纪要文档相互独立。仅通过 [`calendar +meeting --event-ids`](../../lark-calendar/references/lark-calendar-meeting.md) 路径返回。

#### 链路二：开启「录制」

会中开启「录制」功能后，产生**妙记产物**（`minute_token`）。注意：妙记不一定是会中产生的，用户上传音视频文件或录音也会产生妙记。妙记本身包含以下子产物：

| 子产物 | 说明 |
|--------|------|
| Summary（总结） | 对整场会议的智能总结 |
| Todo（待办） | 会议中识别出的待处理任务列表 |
| Chapter（章节） | 按讨论话题划分的核心内容摘要 |
| Transcript（文字记录） | 整场会议最原始的逐人发言记录 |

> **授权特性**：妙记带有**原始会议录制视频**，会后**不会自动授权给参会人**，需管理员主动授权或参会人主动申请后才能读取（含其 Summary/Todo/Chapter/Transcript 等产物）。因此当同一场会议既有智能纪要又有妙记时，参会人访问**智能纪要及其逐字稿**的门槛通常低于妙记。

#### 两条链路的独立性

- 智能纪要（AI 总结链路）和妙记（录制链路）**相互独立、互不影响**。
- 一场会议可能同时拥有两类产物，也可能只有其中一类，也可能都没有。
- 当两者都存在时，Summary/Todo 内容可能重叠，应根据用户意图选择优先读取哪个。

> **产物选择决策**：
> - **AI 产物 vs 原始记录**：智能总结、待办、章节都属于 AI 分析产物，可能只包含最终结论和关键信息。
>   - **用户要求"提炼/总结/重新总结/整理/回顾"会议内容时** → **内容总结必须从逐字稿/文字记录出发，基于原始对话独立分析**。禁止直接搬运 AI 纪要的总结作为最终输出——那只是对 AI 产物的重新排版，不是独立提炼。
>   - **用户要求查看待办或章节时** → **应参考 AI 产物的待办和章节**，因为 AI 产物的待办更友好（包含提出人和负责人），章节按话题划分更结构化。
>   - **用户只想直接看 AI 总结结果** → 使用 AI 产物的总结。
> - **智能纪要 vs 妙记的选择规则**（适用于总结、待办、逐字稿等重复产物，含逐字稿/原始记录）：
>   - **只存在一类产物** → 用存在的那一类。
>   - **两类都存在、用户明确指定了其中一类**（如"看妙记的逐字稿""用妙记总结"）→ **语义指向哪个就走哪个链路，不要自作主张改道**。
>   - **两类都存在、用户未指定** → **默认用智能纪要及其逐字稿**（智能纪要及逐字稿会后自动授权给参会人，访问门槛更低；妙记含原始录制视频、不自动授权，需申请）。


#### 逐字稿与文字记录的格式

智能纪要的逐字稿（`normal` 纪要的 `verbatim_doc_token` 文档、`unified` 纪要的 `note +transcript` 输出）和妙记的文字记录（Transcript）都记录了用户原始对话内容，格式一致：

```
发言人名称 相对时间戳
<发言内容>
```

示例：

```
张三 00:00:00.195
我们接下来讨论一下项目进度。
```

- 第一行为发言人信息，包含用户名称和发言的相对时间（从会议开始计算的偏移量）。
- 后续行为该发言人的发言内容，直到下一个发言人标记出现。

### 会议总结和分析流程

#### Step 1: 定位会议

根据关键字、组织者、参与人、会议室等条件搜索会议，获取会议列表。

> **不要把纪要标题当会议线索：** 如果用户说“查询 xx 纪要的逐字稿 / 原始记录 / 谁说了什么”，且没有 `meeting_id`、`calendar_event_id`、会议号、参会人或时间范围，先用 `drive +search --query <标题>` 搜索纪要文档，拿到 Docx URL/token 后再 `docs +fetch`。若返回 `<vc-transcribe-tab vc-node-id="...">`，提取 `note_id` 后进入 Note 域判断 `normal` / `unified`；若没有该 block，但有“文字记录/逐字稿” Docx 链接，直接用 `docs +fetch` 读取该链接。

```bash
lark-cli vc +search --start "<YYYY-MM-DD>" --end "<YYYY-MM-DD>" --format json
```

详细用法请阅读 [`lark-vc-search.md`](lark-vc-search.md)。

#### Step 2: 根据 meeting_id 查询产物

##### 获取会议产物

当用户提供 `meeting_id` 并需要会议产物时，先用 `vc +detail` 拿到 `note_id` 和 `minute_token`：

```bash
lark-cli vc +detail --meeting-ids '<meeting_id1>,<meeting_id2>'
```

详细用法请阅读 [`lark-vc-detail.md`](lark-vc-detail.md)。

**优先路径：通过 `note_id` 获取纪要产物**

如果用户未明确要求使用妙记，且返回了 `note_id`，**优先**使用 `note +detail` 获取纪要文档的 token 信息：

```bash
lark-cli note +detail --note-id <note_id>
```

可获取会议的所有产物信息，包括：
- 纪要标识（`note_id`）与展示类型（`note_display_type`：`unknown` / `normal` / `unified`）— 决定逐字稿走哪条路由
- 智能纪要（`note_doc_token`）— AI 生成的总结和待办信息
- 逐字稿（`verbatim_doc_token`）— 完整的会中发言记录（仅 `normal` 纪要可直接读取该文档）
- 共享文档（`shared_doc_token`）— 会中投屏共享的文档

拿到文档 token 后，再通过 Doc 域 `docs +fetch` 拉取文档正文内容（见 Step 3）。详细用法请阅读 [`lark-note-detail.md`](../../lark-note/references/lark-note-detail.md)。

**备选路径：通过 `minute_token` 获取妙记产物**

如果 `note_id` 为空，或用户明确要求使用妙记产物，则使用 `minutes +detail` 获取妙记的具体产物：

```bash
# 必须显式指定要获取的产物 flag，至少传一个；不传则不会返回任何产物内容
lark-cli minutes +detail --minute-tokens '<minute_token1>,<minute_token2>' \
  --summary --todo --chapter --keyword --transcript
```

> **注意**：`minutes +detail` 需要**手动指定**要获取的产物 flag，可选 `--summary`（总结）、`--todo`（待办）、`--chapter`（章节）、`--keyword`（关键词）、`--transcript`（文字记录）。**未传任何产物 flag 时不会返回产物内容**，请按用户诉求按需指定。详细用法请阅读 [`lark-minutes-detail.md`](../../lark-minutes/references/lark-minutes-detail.md)。

#### Step 3: 按 `note_display_type` 拉取正文 / 逐字稿

智能纪要（`note_doc_token`）是飞书文档，使用 `docs +fetch` 读取正文内容；**逐字稿的读取方式由 `note_display_type` 决定**：

```bash
# 纪要正文（两种展示类型都适用）
lark-cli docs +fetch --doc <note_doc_token> --doc-format markdown

# note_display_type=normal：逐字稿是独立文档
lark-cli docs +fetch --doc <verbatim_doc_token> --doc-format markdown

# note_display_type=unified：逐字稿不是独立文档，按 note_id 拉取
lark-cli note +transcript --note-id <note_id>
```

详细用法请参考 [lark-doc](../../lark-doc/SKILL.md) 与 [lark-note](../../lark-note/SKILL.md) skill。

#### Step 4: 判断用户需要的产物内容

- 根据用户诉求（总结/待办/章节/完整发言记录等），选择合适的产物进行分析和信息提取
- 如果两种产物都不存在或没有权限，需如实告知用户

## Note 域

- VC 只负责从 `meeting_id` 定位会议产物和 `note_id` / `minute_token`（[`vc +detail`](lark-vc-detail.md)）。
- 已知 `note_id` 后切到 [lark-note](../../lark-note/SKILL.md)；逐字稿路由以 `lark-note` 的 `note_display_type` 规则为准。
- 已知 `minute_token` 时，[`minutes +detail`](../../lark-minutes/references/lark-minutes-detail.md) 顶层会一并返回该妙记关联的 `note_id`（如有）；可直接传给 `note +detail` 取纪要文档 token，无需绕回 VC。
- 仅有日程 `event_id` 时，先走 [`calendar +meeting`](../../lark-calendar/references/lark-calendar-meeting.md) 拿到 `meeting_id` 或用户绑定的 `meeting_note`，再按上述路径继续。
- 只有自然语言纪要标题时，先走文档搜索与 `docs +fetch`；只有 `<vc-transcribe-tab vc-node-id="...">` 的 `vc-node-id` 可以进入 Note 域。
- `doc_token` / Docx URL 不是 `note_id`。没有 `vc-node-id` 时不要反推 Note，继续按 Doc 域读取正文或正文中明确给出的逐字稿文档。

## Doc 域

- **lark-doc skill** 负责飞书云文档管理，包括获取文档元信息、读取文档内容、创建和编辑文档等操作。
- **会议产物的文档本质**：智能纪要（`note_doc_token`）和 `normal` 纪要的逐字稿（`verbatim_doc_token`）都是飞书文档，需要通过 `lark-doc` 的 API（如 `docs +fetch`）查询其内容和元信息；`unified` 纪要的逐字稿不是独立文档，用 `note +transcript` 拉取（[lark-note](../../lark-note/SKILL.md)）。
- **文档元信息查询**：获取文档名称、URL 等基本信息时，使用 `drive metas batch_query`；获取文档正文内容时，使用 `docs +fetch`。

## 三域关联总览

```
Calendar (日程) ──── 发起预约 ────► VC (会议)
                                       │
                    ┌──────────────────┤
                    │                  │
              AI 总结链路           录制链路
                    │                  │
                    ▼                  ▼
            智能纪要 (Doc)        妙记 (Minutes)
            逐字稿 (Doc)         ├── Summary
            共享文档 (Doc)        ├── Todo
            用户纪要 (Doc)        ├── Chapter
                                └── Transcript
```

- Calendar 提供会议预约入口，但并非所有会议都来自日程。
- VC 是会议数据的中心，管理会议记录和产物关联。
- Doc 是会议产物的载体，智能纪要和逐字稿都以飞书文档形式沉淀，需通过 Doc 域 API 读取。
