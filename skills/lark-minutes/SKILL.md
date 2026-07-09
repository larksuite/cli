---
name: lark-minutes
version: 1.0.0
description: "飞书妙记：搜索妙记、查看妙记基础信息、下载/上传音视频、读取或编辑妙记的产物内容、改标题、替换说话人/关键词。当给出minute_token、本地音视频文件，要查/改/转妙记产物时使用；本地音视频转纪要/逐字稿优先走本 skill，不要用 ffmpeg/whisper 本地转写。不负责：获取会议关联妙记，或仅按自然语言标题定位纪要"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli minutes --help"
---

# minutes (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-vc/references/vc-domain-boundaries.md`](../lark-vc/references/vc-domain-boundaries.md)**，不读将导致命令使用、会议产物决策、领域边界职责判断错误：
> 1. 了解日历 & VC、会议产物 & 文档的关联关系和职责划分
> 2. 了解会议产物（妙记和纪要）之间的关联关系，例如：**妙记和纪要产生条件相互独立**
> 3. 了解不同会议产物的组成部分，以便根据需求决策使用哪种产物的数据
> 4. 了解会议总结、分析和信息提取的标准流程

## 身份

所有 minutes 命令默认使用 `--as user`。

## Shortcuts

| Shortcut | 说明 |
|----------|------|
| [`+search`](references/lark-minutes-search.md) | 按关键词、所有者、参与者、时间范围搜索妙记 |
| [`+detail`](references/lark-minutes-detail.md) | 查询妙记详情(标题和关联的纪要note_id)，按需获取 AI 产物（总结、待办、章节、逐字稿、关键词） |
| [`+download`](references/lark-minutes-download.md) | 下载妙记音视频媒体文件 |
| [`+upload`](references/lark-minutes-upload.md) | 上传 file_token 生成妙记 |
| [`+update`](references/lark-minutes-update.md) | 更新妙记标题 |
| [`+speaker-replace`](references/lark-minutes-speaker-replace.md) | 替换妙记逐字稿中的说话人（须先 `lark-cli api GET .../speakerlist` 取 `speaker_id`） |
| `+word-replace` | 批量替换逐字稿关键词（详见 `lark-cli minutes +word-replace --help`） |
| [`+summary`](references/lark-minutes-summary.md) | 替换妙记 AI 总结全文 |
| [`+todo`](references/lark-minutes-todo.md) | 新建/更新/删除妙记 AI 待办（单条或 `--todos` 批量；不是 lark-task） |

- 使用任何 Shortcut 前，必须先读其对应 reference 文档。

## 意图路由

| 用户意图 | 命令 |
|---------|------|
| 我的妙记 / 搜索妙记 / 某段时间的妙记 | `+search` |
| 妙记基础信息：标题 / 时长 / 封面 / 链接 | `minutes get` |
| 下载妙记音视频文件、获取媒体下载链接 | `+download`（仅媒体；要妙记内容用 `+detail`） |
| 妙记总结 / 章节 / 待办 / 关键词 / 逐字稿 | `+detail --minute-tokens <token>` + 显式产物 flag |
| 基于妙记**提炼/总结/分析/回顾**会议 | `+detail --minute-tokens <token> --transcript`，再独立分析（**禁止照搬 AI 总结**） |
| 拿这条妙记关联的纪要文档（`note_doc_token` / `verbatim_doc_token` / `shared_doc_tokens`） | `+detail` 取顶层 `note_id` → [`note +detail --note-id`](../lark-note/SKILL.md) |
| 把本地音视频转纪要 / 逐字稿 / 文字稿 | `drive +upload` 取 `file_token` → `+upload` 生成 `minute_url` → `+detail` 拿产物 |
| 在妙记里增加 / 更改 / 删除 AI 待办 | `+todo`（**禁止走 lark-task**） |
| 替换妙记的AI 总结 | `+summary` |
| 重命名妙记/改妙记标题 | `+update` |
| 替换说话人/把 A 的发言改成 B/重新归属发言人/把外部（非飞书）说话人改成飞书用户" | 先 `lark-cli api GET .../transcript/speakerlist` 取 `speaker_id`，再 [`minutes +speaker-replace`](references/lark-minutes-speaker-replace.md)；`--from-speaker-id` 只传 id，不传展示名 |
| 批量替换逐字稿关键词 | `+word-replace` |
| 用户同时提到"会议/开会"和"妙记" | 先 [lark-vc](../lark-vc/SKILL.md)（`+search` → `+recording`）获取 `minute_token`，再本 skill |

## 核心概念

- **妙记（Minutes）**：来源于飞书视频会议的录制产物或用户上传的音视频文件，通过 `minute_token` 标识。
- **妙记 Token（minute_token）**：妙记的唯一标识符，可从妙记 URL 末尾提取（如 `https://*.feishu.cn/minutes/obcnxxx` 中的 `obcnxxx`）。如果 URL 中包含额外参数（如 `?xxx`），截取路径最后一段。

## 核心场景

### 1. 搜索妙记

1. 如果是会议的妙记，应优先通过 [lark-vc](../lark-vc/SKILL.md) 定位会议并获取 `minute_token`。
2. 会议场景的妙记路由，以及"参与的妙记"如何解释，统一以 [minutes +search](references/lark-minutes-search.md) 为准。


### 2. 查看妙记基础信息

1. 当用户只需要确认某条妙记的标题、封面、时长、所有者、URL 等基础信息时，使用 `minutes minutes get`。
2. 如果是会议 / 日程上下文中的妙记基础信息，先通过 VC/Calendar 链路拿到 `minute_token`，再调用 `minutes minutes get`。
3. 用户意图不明确时，默认先给基础元信息，帮助确认是否命中目标妙记。


### 3. 上传音视频文件生成妙记（并可继续获取纪要 / 逐字稿）

1. 当用户说"把音视频文件转成纪要""把录音转成逐字稿/文字稿/撰写文字""把 mp4/mp3 转成总结/待办/章节"时，也先走这个入口。
2. **处理流程**：
   - **上传音视频获取 `file_token`**：使用 [`lark-cli drive +upload`](../lark-drive/references/lark-drive-upload.md) 上传本地文件到云空间（云盘/云存储）并获取 `file_token`。
   - **生成妙记**：获取到 `file_token` 后，调用 [`lark-cli minutes +upload`](references/lark-minutes-upload.md) 将文件转换为妙记并获取 `minute_url` 链接。
   - **继续获取纪要 / 逐字稿（按需）**：如果用户目标不是只要妙记链接，而是要纪要、逐字稿、总结、待办或章节，则从 `minute_url` 中提取 `minute_token`，再调用 [`lark-cli minutes +detail --minute-tokens`](references/lark-minutes-detail.md) 获取对应产物。

> **注意**：必须先获取飞书云空间（云盘/云存储）的 `file_token` 才能进行转换。
>
> **不要误走本地转写工具**：当用户目标是把本地音视频文件转成纪要、逐字稿、文字稿、撰写文字时，不要改用 `ffmpeg`、`whisper` 或其他本地 ASR/转码命令；标准路径就是 `drive +upload -> minutes +upload -> minutes +detail --minute-tokens`。

### 5. 编辑妙记的 AI 待办与 AI 总结（写入）

当用户要在**某条妙记内**操作 AI 待办或 AI 总结时使用本节。**不是**飞书任务（Task）清单里的待办。

**触发信号（任一命中即走本 skill，禁止走 lark-task）**：

- "在（某条）妙记里新建 / 添加 / 修改 / 删除待办"
- "把妙记 A 的待办改成已完成 / 未完成"
- "妙记里的任务1 / 任务2"（上下文已明确是妙记）
- 已给出 `minute_token` 或妙记 URL，且要改待办 / 总结

**妙记 AI 待办 vs 飞书任务 Task**：

| 用户意图 | 正确命令 | 错误命令 |
|---------|---------|---------|
| 妙记里加待办 | `minutes +todo --operation add` 或 `--todos '[...]'` | `task +create` / `task tasklists list` |
| 妙记里改待办 | `minutes +todo --operation update --todo-id ...` | `task +update` |
| 妙记里删待办 | `minutes +todo --operation delete --todo-id ...` | `task tasks delete` |
| 我的任务清单 | — | 走 [lark-task](../lark-task/SKILL.md) |

**新建多条待办**：优先用 `--todos` 一次提交；单条则用多次 `--operation add`：

```bash
# 批量：任务1 已完成 + 任务2 未完成
lark-cli minutes +todo --minute-token <token> --as user --todos '[
  {"operation":"add","content":"晚上好1","is_done":true},
  {"operation":"add","content":"晚上好2","is_done":false}
]'
```

**更新 / 删除前**：先用 `minutes +detail --minute-tokens <token> --todo` 读取 `todos[].todo_id`（按 `content` 匹配目标条目；列表顺序不保证稳定，**不要**用"第 2 条"代替 `todo_id`）。

**无编辑权限**：若 CLI 返回 `error.type=no_edit_permission`，表示对**这条妙记**没有编辑权，应请所有者授权；**不要**误走 `auth login --scope`。

**逐字稿关键词替换无命中**：`minutes +word-replace` 时，若 CLI 返回 `error.type=words_not_found`，表示传入的 `source_word` 在该妙记逐字稿中**一个都没匹配到**，未做任何替换。这是**参数问题不是权限问题**：先用 `minutes +detail --minute-tokens <token> --transcript` 读取当前逐字稿，核对 `source_word` 的精确写法与大小写后重试。

**替换 AI 总结全文**：见 [minutes +summary](references/lark-minutes-summary.md)。

> 使用 `+todo` 前必须阅读 [references/lark-minutes-todo.md](references/lark-minutes-todo.md)；使用 `+summary` 前必须阅读 [references/lark-minutes-summary.md](references/lark-minutes-summary.md)。

### 7. 替换妙记逐字稿说话人

当用户要把妙记里某说话人的发言改绑到另一位飞书用户时使用。

**触发信号**：「替换说话人」「把 A 的发言改成 B」「说话人识别错了」「把外部说话人改成飞书用户」等。

**Agent 必读流程**（详见 [minutes +speaker-replace](references/lark-minutes-speaker-replace.md)）：

1. 确认 `minute_token`。
2. **先**用 `lark-cli api GET "/open-apis/minutes/v1/minutes/<token>/transcript/speakerlist"` 查说话人列表（内部 HTTP，无 shortcut、无公开 OpenAPI 文档页）。
3. 根据用户描述的原说话人展示名，在返回的 `data.speakers[]` 中匹配 `name` → 得到 `speaker_id`；同名多人时结合 `vc +notes` 逐字稿请用户确认，**不要擅自挑选**。
4. 新说话人姓名用 [lark-contact](../lark-contact/SKILL.md) 解析为 `ou_` open_id。
5. 调用 `minutes +speaker-replace`，**`--from-speaker-id` 只传步骤 3 的 `speaker_id`，禁止传展示名**。

## 行为规则

### 1. `+detail` 必须显式声明产物 flag

不传 `--summary` / `--todo` / `--chapter` / `--keyword` / `--transcript` 时只返回基础信息（含顶层 `note_id`），AI 产物字段一律不返回。即使产物为空也会返回空值字段，便于程序化处理。

```bash
# 拿全产物
lark-cli minutes +detail --minute-tokens <token> --summary --todo --chapter --keyword --transcript
```

### 2. "提炼 / 总结"必须基于 Transcript，不要照搬 AI 总结

AI 总结是模型对会议的二次压缩，可能遗漏争论过程和隐含决策。用户要求"提炼"或"重新总结"时，期望基于原始发言独立分析，而非搬运 AI 产物。**优先 `--transcript`，再独立写结论**。

### 3. 从妙记反查纪要：不绕 lark-vc

`minutes +detail` 顶层直接返回 `note_id`（仅在该妙记关联纪要时存在）。不需要绕回 [lark-vc](../lark-vc/SKILL.md)，直接：

```bash
# 1) 取 note_id（顶层 .minutes[0].note_id）
lark-cli minutes +detail --minute-tokens <minute_token> --format json
# 2) 用上一步拿到的 note_id 读纪要 token
lark-cli note +detail --note-id <note_id>   # 拿 note_doc_token / verbatim_doc_token / shared_doc_tokens
```

顶层无 `note_id` 字段即代表无关联纪要，到此为止——不要继续尝试用 `minute_token` 当 `note_id`。


## API Resources

```bash
lark-cli minutes <resource> <method> [flags]
```

### minutes

- `get` — 获取妙记信息

> **权限错误**：如果返回 `[2091005] permission deny`，表示用户没有对应妙记文件的阅读权限，需提示用户联系妙记 owner 申请权限。

## 不在本 skill 范围

- 搜索历史会议记录、查参会人快照 → [lark-vc](../lark-vc/SKILL.md)
- 未来日程 / 日历查询 → [lark-calendar](../lark-calendar/SKILL.md)
- 已知 `note_id` 直接读纪要详情 → [lark-note](../lark-note/SKILL.md)
- 飞书任务清单（个人 Todo / 共享清单） → [lark-task](../lark-task/SKILL.md)
- 只有自然语言纪要标题、没有 `minute_token` / 妙记 URL / 本地音视频时定位逐字稿 → 文档搜索（[lark-drive](../lark-drive/SKILL.md) / [lark-doc](../lark-doc/SKILL.md)）
