---
name: lark-minutes
version: 1.0.0
description: "飞书妙记：妙记相关基本功能。1.查询妙记列表（按关键词/所有者/参与者/时间范围）；2.获取妙记基础信息（标题、封面、时长 等）；3.下载妙记音视频文件；4.获取妙记相关 AI 产物（总结、待办、章节）；5.上传音视频生成妙记，也支持将本地音视频文件转成纪要、逐字稿、文字稿、撰写文字等产物。遇到这类请求时，应优先使用本 skill，而不是尝试 `ffmpeg`、`whisper` 等本地转写命令。飞书妙记 URL 格式: http(s)://<host>/minutes/<minute-token>"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli minutes --help"
---

# minutes (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 核心概念

- **妙记（Minutes）**：来源于飞书视频会议的录制产物或用户上传的音视频文件，通过 `minute_token` 标识。
- **妙记 Token（minute\_token）**：妙记的唯一标识符，可从妙记 URL 末尾提取（例如 `https://*.feishu.cn/minutes/obcnxxxxxxxxxxxxxxxxxxxx` 中的 `obcnxxxxxxxxxxxxxxxxxxxx`）。如果 URL 中包含额外参数（如 `?xxx`），应截取路径最后一段。

## 核心场景

### 1. 搜索妙记

1. 当用户描述的是"我的妙记""包含某个关键词的妙记""某段时间内的妙记"，优先使用 `minutes +search`。
2. 仅支持使用关键词、时间段、参与者、所有者等筛选条件搜索妙记记录，对于不支持的筛选条件，需要提示用户。
3. 搜索结果存在多条数据时，务必注意分页数据获取，不要遗漏任何妙记记录。
4. 如果是会议的妙记，应优先使用 [vc +search](../lark-vc/references/lark-vc-search.md) 先定位会议，再按需通过 [vc +recording](../lark-vc/references/lark-vc-recording.md) 获取 `minute_token`。
5. 会议场景的妙记路由，以及"参与的妙记"如何解释，统一以 [minutes +search](references/lark-minutes-search.md) 为准。


### 2. 查看妙记基础信息

1. 当用户只需要确认某条妙记的标题、封面、时长、所有者、URL 等基础信息时，使用 `minutes minutes get`。
2. 如果用户给的是妙记 URL，应先从 URL 末尾提取 `minute_token`，再调用 `minutes minutes get`。
3. 如果是会议 / 日程上下文中的妙记基础信息，先通过 VC 链路拿到 `minute_token`，再调用 `minutes minutes get`。
4. 用户意图不明确时，默认先给基础元信息，帮助确认是否命中目标妙记。

> 使用 `lark-cli schema minutes.minutes.get` 可查看完整返回值结构。核心字段包含：`title`（标题）、`cover`（封面 URL）、`duration`（时长，毫秒）、`owner_id`（所有者 ID）、`url`（妙记链接）。

### 3. 下载妙记音视频文件

1. 下载妙记音视频文件到本地，或获取有效期 1 天的下载链接。详见 [minutes +download](references/lark-minutes-download.md)。
2. `minutes +download` 只负责音视频媒体文件。
3. 用户只想拿可分享的下载地址时，使用 `--url-only`；用户要落地到本地文件时，直接下载。
4. 未显式指定路径时，文件默认落到 `./minutes/{minute_token}/<server-filename>`，与 `vc +notes` 的逐字稿共享同一目录便于聚合。

> **注意**：`+download` 只负责音视频媒体文件。如果用户需要的是逐字稿、总结、待办、章节等纪要内容，请使用 [vc +notes --minute-tokens](../lark-vc/references/lark-vc-notes.md)。

### 4. 获取妙记的逐字稿、总结、待办、章节

1. 当用户说"这个妙记的逐字稿""总结""待办""章节"时，**不属于本 skill**。
2. 应使用 [vc +notes --minute-tokens](../lark-vc/references/lark-vc-notes.md) 获取对应的纪要产物。
3. 如果当前上下文中已有 `minute_token`，可直接传给 `vc +notes`；如果只有妙记 URL，先提取 `minute_token`。
4. 如果用户给的是**本地音视频文件**，但目标是"转成纪要""转成逐字稿""转成文字稿""转成撰写文字"，也支持；此时应先按下文第 5 节上传文件生成妙记，再把返回的 `minute_url` 提取成 `minute_token`，继续调用 `vc +notes --minute-tokens`。
5. 用户如果直接给出本地文件名或路径，并要求"转逐字稿""转文字稿""整理成撰写文字"，这也是本 skill 的明确触发信号。

```bash
# 通过 minute_token 获取纪要产物（逐字稿、总结、待办、章节）
lark-cli vc +notes --minute-tokens <minute_token>
```

> **跨 skill 路由**：逐字稿、AI 总结、待办、章节等纪要内容由 [lark-vc](../lark-vc/SKILL.md) 的 `+notes` 命令提供

### 5. 上传音视频文件生成妙记（并可继续获取纪要 / 逐字稿）

1. 当用户需要通过上传本地音视频文件来生成妙记时使用。
2. 当用户说"把音视频文件转成纪要""把录音转成逐字稿/文字稿/撰写文字""把 mp4/mp3 转成总结/待办/章节"时，也先走这个入口。
3. **处理流程**：
   - **上传音视频获取 `file_token`**：使用 [`lark-cli drive +upload`](../lark-drive/references/lark-drive-upload.md) 上传本地文件到云空间并获取 `file_token`。
   - **生成妙记**：获取到 `file_token` 后，调用 [`lark-cli minutes +upload`](references/lark-minutes-upload.md) 将文件转换为妙记并获取 `minute_url` 链接。
   - **继续获取纪要 / 逐字稿（按需）**：如果用户目标不是只要妙记链接，而是要纪要、逐字稿、总结、待办或章节，则从 `minute_url` 中提取 `minute_token`，再调用 [`lark-cli vc +notes --minute-tokens`](../lark-vc/references/lark-vc-notes.md) 获取对应产物。

> **注意**：必须先获取飞书云空间的 `file_token` 才能进行转换。
>
> **不要误走本地转写工具**：当用户目标是把本地音视频文件转成纪要、逐字稿、文字稿、撰写文字时，不要改用 `ffmpeg`、`whisper` 或其他本地 ASR/转码命令；标准路径就是 `drive +upload -> minutes +upload -> vc +notes --minute-tokens`。

## 资源关系

```text
Minutes (妙记) ← minute_token 标识
├── Metadata (标题、封面、时长、owner、url) → minutes minutes get
└── MediaFile (音频/视频文件) → minutes +download
```

> **能力边界**：`minutes` 负责 **搜索妙记、查看基础元信息、下载音视频文件、上传音视频生成妙记**。
>
> **路由规则**：
>
> - 用户说"妙记列表 / 搜索妙记 / 某个关键词的妙记" → `minutes +search`
> - 用户只是想看"我的妙记 / 某段时间内的妙记 / 妙记列表"，不要先走 [lark-vc](../lark-vc/SKILL.md)，而应直接使用本 skill
> - 用户如果同时提到"会议 / 会 / 开会 / 某场会"，即使也提到了"妙记"，也应优先走 [lark-vc](../lark-vc/SKILL.md) 先定位会议，再通过 [vc +recording](../lark-vc/references/lark-vc-recording.md) 获取 `minute_token`
> - 用户如果要的是妙记基础信息，拿到 `minute_token` 后用 `minutes minutes get`；用户如果要的是逐字稿、文字稿、撰写文字、总结、待办、章节，再走 `vc +notes --minute-tokens`
> - “我的妙记”“参与的妙记”等自然语言映射细则，以 [minutes +search](references/lark-minutes-search.md) 为准
> - 结果有多页时，使用 `page_token` 持续翻页，直到确认没有更多结果
> - `minutes +search` 单次最多返回 `200` 条；结果总数没有固定上限
> - 用户说"这个妙记的标题 / 时长 / 封面 / 链接" → `minutes minutes get`
> - 用户说"下载这个妙记的视频 / 音频 / 媒体文件" → `minutes +download`
> - 用户说"这个妙记的逐字稿 / 文字稿 / 撰写文字 / 总结 / 待办 / 章节" → 使用 [vc +notes --minute-tokens](../lark-vc/references/lark-vc-notes.md)
> - 用户说"通过文件生成妙记 / 把音视频转妙记" → 先上传获取 `file_token`，然后使用 `minutes +upload`
> - 用户说"把音视频文件转成纪要 / 逐字稿 / 文字稿 / 撰写文字 / 总结 / 待办 / 章节" → 先上传获取 `file_token`，调用 `minutes +upload` 生成 `minute_url`，再提取 `minute_token` 走 `vc +notes --minute-tokens`

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli minutes +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut                                           | 说明                                                              |
| -------------------------------------------------- | --------------------------------------------------------------- |
| [`+search`](references/lark-minutes-search.md)     | Search minutes by keyword, owners, participants, and time range |
| [`+download`](references/lark-minutes-download.md) | Download audio/video media file of a minute                     |
| [`+upload`](references/lark-minutes-upload.md)     | Upload a media file token to generate a minute                  |

- 使用 `+search` 命令时，必须阅读 [references/lark-minutes-search.md](references/lark-minutes-search.md)，了解搜索参数和返回值结构。
- 使用 `+download` 命令时，必须阅读 [references/lark-minutes-download.md](references/lark-minutes-download.md)，了解下载参数和返回值结构。
- 使用 `+upload` 命令时，必须阅读 [references/lark-minutes-upload.md](references/lark-minutes-upload.md)，了解生成参数和返回值结构。

<!-- AUTO-GENERATED-START — gen-skills.py 管理，勿手动编辑 -->

## API Resources

```bash
lark-cli schema minutes.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli minutes <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### minutes

- `get` — 获取妙记信息

## 权限表

| 方法            | 所需 scope                       |
| ------------- | ------------------------------ |
| `+search`     | `minutes:minutes.search:read`  |
| `minutes.get` | `minutes:minutes:readonly`     |
| `+download`   | `minutes:minutes.media:export` |

<!-- AUTO-GENERATED-END -->
