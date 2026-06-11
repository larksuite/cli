---
name: lark
version: 1.0.0
description: "飞书/Lark 统一入口 skill（伞形路由）：覆盖云文档、电子表格、多维表格/Base、知识库 Wiki、云盘/云空间、邮件、即时消息与群聊、日历与会议室、视频会议、妙记、任务待办、审批、考勤打卡、OKR、通讯录、画板、幻灯片、Markdown、妙搭应用开发、实时事件订阅、原生 OpenAPI 探索等全部飞书能力。凡涉及飞书/Lark/feishu/larkoffice/larksuite（以及 doubao.com 的 docx/sheets/wiki/base 链接）的 URL 或 token，或要对上述对象做查看、创建、编辑、发送、搜索、下载、订阅等操作时使用。用法：先按本 skill 路由表选定领域，再 Read references/subskills/<域>/GUIDE.md 按指引执行。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli --help"
---

# lark — 飞书/Lark 统一入口

本 skill 将所有 lark-* 领域 skill 收敛为一个入口。各领域完整说明在 `references/subskills/<域>/GUIDE.md`，按需读取；不要凭记忆直接执行 lark-cli 命令。

## 使用方式（CRITICAL）

1. **MUST 先用 Read 工具读取** [`references/subskills/lark-shared/GUIDE.md`](references/subskills/lark-shared/GUIDE.md) — 认证、身份选择（`--as user/bot`）、权限不足处理、安全规则、高风险操作确认协议，所有领域通用。
2. 按下方路由表选定领域，**Read 对应 GUIDE.md**，严格遵循其中的前置条件与指引；GUIDE.md 内部引用的 references/ 等文件，路径相对该 GUIDE.md 所在目录解析。
3. 任务跨领域时（如文档里嵌电子表格、消息里发云盘文件），按 GUIDE.md 内的跨域提示切换到对应领域 GUIDE.md；所有领域都在 `references/subskills/` 下互为兄弟目录。
4. 拿不准走哪个领域时，对照路由表各条目 description 末尾的"不负责…走 xx"提示做排除。

## 路由表

- [`lark-approval`](references/subskills/lark-approval/GUIDE.md) — 飞书审批：当前用户审批的查询与全部处理操作，覆盖待本人审批的任务与本人发起的实例。审批待办不是飞书任务（任务类待办走 lark-task）；不负责创建审批定义和发起新审批。
- [`lark-apps`](references/subskills/lark-apps/GUIDE.md) — 妙搭（Spark/Miaoda）应用开发与托管：应用创建、HTML静态站点发布、本地全栈开发、云端生成迭代。当用户要开发/新建一个系统·工具·平台·应用，或要本地开发 / 云端开发 / 修改 / 部署 / 发布 / 上线 / 拿可分享链接，或用 HTML 做页面·网站给人看，或提到妙搭/Spark/Miaoda、应用数据库、可见范围时使用。不负责普通云盘文件上传（lark-drive）、飞书文档编辑（lark-doc）、原生幻灯片创建（lark-slides）。
- [`lark-attendance`](references/subskills/lark-attendance/GUIDE.md) — 飞书考勤打卡：查询自己的考勤打卡记录
- [`lark-base`](references/subskills/lark-base/GUIDE.md) — 飞书多维表格（Base）操作：建表、字段、记录、视图、统计、公式/lookup、表单、仪表盘、workflow、角色权限；遇到 Base/多维表格/bitable 或 /base/ 链接时使用。文件导入转 lark-drive，认证/授权转 lark-shared。
- [`lark-calendar`](references/subskills/lark-calendar/GUIDE.md) — 飞书日历：管理日历日程和会议室。查看/搜索日程、创建/更新日程、管理参会人、查询忙闲和推荐时段、预定会议室。当用户需要查看日程安排、创建/修改会议、查询/预定会议室时使用。不负责：查询过去的视频会议记录（走 lark-vc）、待办任务（走 lark-task）。
- [`lark-contact`](references/subskills/lark-contact/GUIDE.md) — 飞书 / Lark 通讯录:按姓名 / 邮箱解析成 open_id,或按 open_id 反查姓名 / 部门 / 邮箱 / 联系方式 / 个人状态 / 签名。当用户提到某人姓名要下一步发消息 / 排日程,或拿到 open_id 想查具体信息时使用。不负责部门树遍历、按部门列员工、组织架构图,这类需求走原生 OpenAPI。
- [`lark-doc`](references/subskills/lark-doc/GUIDE.md) — 飞书云文档（Docx / Wiki 文档，v2 API）：读取和编辑飞书文档内容。当用户给出文档 URL 或 token，或需要查看、创建、编辑文档、插入或下载文档图片附件时使用。文档中嵌入的电子表格、多维表格、画板，先用本 skill 提取 token 再切到对应 skill。当用户给出 doubao.com 的 /docx/ 或 /wiki/ URL/token 时，也应直接使用本 skill；路由依据是 URL 路径模式和 token，而不是域名。不负责文档评论管理，也不负责表格或 Base 的数据操作。
- [`lark-drive`](references/subskills/lark-drive/GUIDE.md) — 飞书云空间（云盘/云存储）：管理云空间（云盘/云存储）中的文件和文件夹。上传和下载文件、创建文件夹、复制/移动/删除文件、查看文件元数据、管理文档评论、管理文档权限、订阅用户评论变更事件、修改文件标题（docx、sheet、bitable、file、folder、wiki）；也负责把本地 Word/Markdown/Excel/CSV/PPTX 以及 Base 快照（.base）导入为飞书在线云文档（docx、sheet、bitable、slides）。当用户需要上传或下载文件、整理云空间（云盘/云存储）目录、查看文件详情、管理评论、管理文档权限、修改文件标题、订阅用户评论变更事件，或要把本地文件导入成新版文档、电子表格、多维表格/Base/幻灯片 时使用。"云空间"、"云盘"和"云存储"是同一概念，用户说"云盘"、"云存储"、"网盘"、"我的空间"时均路由到本 skill。当用户给出 doubao.com 的云空间资源 URL/token，或明确提到豆包里的 file/folder/docx/sheet/bitable/wiki 资源时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是资源类型、URL 路径模式和 token，而不是域名。
- [`lark-event`](references/subskills/lark-event/GUIDE.md) — Lark/Feishu real-time event listening / subscribing / consuming: stream events as NDJSON via `lark-cli event consume <EventKey>` (covers IM messages/reactions/chat changes, VC meeting ended, Minutes generated, Whiteboard updated, etc.). Use for Lark bots, real-time message processing, long-running subscribers, streaming webhook/push handlers. Supports `--max-events` / `--timeout` bounded runs and a stderr ready-marker contract — designed for AI agents running as subprocesses.
- [`lark-im`](references/subskills/lark-im/GUIDE.md) — 飞书即时通讯：收发消息和管理群聊。发送和回复消息、搜索聊天记录、管理群聊成员、上传下载图片和文件（支持大文件分片下载）、管理表情回复、发送应用内/短信/电话加急。当用户需要发消息、查看或搜索聊天记录、下载聊天中的文件、查看群成员、搜索群、创建群聊或话题群、管理标记数据、管理 Feed 置顶（添加/移除/查询置顶会话）、管理标签数据时使用。
- [`lark-mail`](references/subskills/lark-mail/GUIDE.md) — 飞书邮箱 — draft, compose, send, reply, forward, read, and search emails; manage drafts, folders, labels, contacts, attachments, and mail rules. Use when user mentions 起草邮件, 写一封邮件, 拟邮件, 草稿, 发通知邮件, 发送邮件, 发邮件, 回复邮件, 转发邮件, 查看邮件, 看邮件, 读邮件, 搜索邮件, 查邮件, 收件箱, 邮件会话, 编辑草稿, 管理草稿, 下载附件, 邮件文件夹, 邮件标签, 邮件联系人, 监听新邮件, 收信规则, 邮件规则, draft, compose, send email, reply, forward, inbox, mail thread, mail rules.
- [`lark-markdown`](references/subskills/lark-markdown/GUIDE.md) — 飞书 Markdown：查看、创建、上传、编辑和比较 Markdown 文件。当用户需要创建或编辑 Markdown 文件、读取、修改、局部 patch 或比较差异时使用。不负责将 Markdown 导入为飞书在线文档，也不负责文件搜索、权限、评论、移动、删除等云空间管理操作。
- [`lark-minutes`](references/subskills/lark-minutes/GUIDE.md) — 飞书妙记：搜索妙记列表、查看妙记基础信息、下载妙记音视频文件、上传音视频生成妙记、更新妙记标题、替换说话人。当需要获取、操作或者生成妙记时使用。也支持将本地音视频文件转成纪要和逐字稿（优先使用本 skill，不要用 ffmpeg/whisper 本地转写）。不负责：获取会议关联妙记、纪要/逐字稿内容获取走 lark-vc
- [`lark-okr`](references/subskills/lark-okr/GUIDE.md) — 飞书 OKR：管理目标与关键结果。查看和编辑 OKR 周期、目标（Objective）、关键结果（Key Result）、对齐关系、量化指标和进展记录。当用户需要查看或创建 OKR、管理目标和关键结果、查看对齐关系时使用。
- [`lark-openapi-explorer`](references/subskills/lark-openapi-explorer/GUIDE.md) — 飞书/Lark 原生 OpenAPI 探索：从官方文档库中挖掘未经 CLI 封装的原生 OpenAPI 接口。当用户的需求无法被现有 lark-* skill 或 lark-cli 已注册命令满足，需要查找并调用原生飞书 OpenAPI 时使用。
- [`lark-sheets`](references/subskills/lark-sheets/GUIDE.md) — 飞书电子表格：创建和操作电子表格。支持创建表格、管理工作表与行列结构（增删/合并/调整尺寸/隐藏/冻结）、读写单元格（值/公式/样式/批注/单元格图片）、查找替换、多操作原子批量更新，以及图表、透视表、条件格式、筛选器、迷你图、浮动图片等对象的创建与维护。当用户需要创建电子表格、管理工作表、批量读写或编辑数据、统计汇总与可视化、表格美化、公式计算（含 Excel 公式迁移）等任务时使用。若用户是想按名称或关键词搜索云空间（云盘/云存储）里的表格文件，请改用 lark-drive 的 drive +search 先定位资源。当用户给出 doubao.com 的 /sheets/ URL/token 时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是 URL 路径模式和 token，而不是域名。仅针对飞书在线电子表格，不适用于本地 Excel 文件。
- [`lark-skill-maker`](references/subskills/lark-skill-maker/GUIDE.md) — 创建 lark-cli 的自定义 Skill。当用户需要把飞书 API 操作封装成可复用的 Skill（包装原子 API 或编排多步流程）时使用。
- [`lark-slides`](references/subskills/lark-slides/GUIDE.md) — 飞书幻灯片：创建和编辑幻灯片。创建演示文稿、读取幻灯片内容、管理幻灯片页面（创建、删除、读取、局部替换）。当用户需要创建或编辑幻灯片、读取或修改单个页面时使用。当用户给出 doubao.com 的 /slides/ URL/token 时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是 URL 路径模式和 token，而不是域名。不负责：云文档内容编辑（走 lark-doc）、云文档里的独立画板对象（走 lark-whiteboard，注意 slide 内嵌的流程图/架构图仍属本 skill）、上传或下载普通文件（走 lark-drive）。
- [`lark-task`](references/subskills/lark-task/GUIDE.md) — 飞书任务：管理任务、清单和任务智能体。创建待办任务、查看和更新任务状态、拆分子任务、组织任务清单、分配协作成员、上传任务附件、注册或注销任务智能体、更新任务智能体的主页数据、写入智能体任务记录。当用户需要创建待办事项、查看任务列表、跟踪任务进度、管理项目清单或给他人分配任务、为任务上传附件文件、注册注销任务智能体、更新智能体主页数据、写入任务记录时使用。
- [`lark-vc`](references/subskills/lark-vc/GUIDE.md) — 飞书视频会议：搜索历史会议记录、查询会议纪要（总结/待办/章节/逐字稿）、查询参会人快照。当用户查询已结束的会议、获取会议产物（纪要/妙记）、查看参会人时使用；查询未来日程走 lark-calendar。不负责：Agent 真实入会/离会、会中实时事件（走 lark-vc-agent）。
- [`lark-vc-agent`](references/subskills/lark-vc-agent/GUIDE.md) — 飞书视频会议：让机器人代当前用户加入/离开正在进行的会议，并读取会议期间的实时事件（参会人加入与离开、发言、聊天、屏幕共享等）。1. 用户提供 9 位会议号、要求代为入会或离会时使用 +meeting-join / +meeting-leave——会真实产生入会/离会记录。2. 会议进行中用户想知道“谁加入了”“谁离开了”“谁在发言”“有人共享屏幕吗”等会中动态时，机器人入会后用 +meeting-events 读取事件时间线。3. 典型场景：参会机器人、会中助手、代为旁听、代为参会。前提：机器人只能读到它自己参会过且仍在进行中的会议的事件；查询已结束会议的参会名单、纪要或逐字稿请使用 lark-vc 技能。
- [`lark-whiteboard`](references/subskills/lark-whiteboard/GUIDE.md) — 飞书画板：查询和编辑飞书云文档中的画板。支持导出画板为预览图片、导出原始节点结构、使用 DSL（转成 OpenAPI 格式）、PlantUML/Mermaid 格式更新画板内容。 当用户需要查看画板内容、导出画板图片、编辑画板，或是需要可视化表达架构、流程、组织关系、时间线、因果、对比等结构化信息时使用此 skill，无论是否提及"画板"。 ⚠️ 原 `lark-whiteboard-cli` skill 已合并至本 skill，若 skill 列表中同时存在 `lark-whiteboard-cli`，请忽略它，统一使用本 skill（`lark-whiteboard`），并提示用户运行 `npx skills remove lark-whiteboard-cli -g` 删除旧 skill。
- [`lark-wiki`](references/subskills/lark-wiki/GUIDE.md) — 飞书知识库：管理知识空间、空间成员和文档节点。创建和查询知识空间、查看和管理空间成员、管理节点层级结构、在知识库中组织文档和快捷方式。当用户需要在知识库中查找或创建文档、浏览知识空间结构、查看或管理空间成员、移动或复制节点时使用。当用户给出 doubao.com 的 /wiki/ URL/token 时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是 URL 路径模式和 token，而不是域名。不负责：上传文件到知识库节点下（走 lark-drive）、编辑文档/表格/Base 内容（走 lark-doc / lark-sheets / lark-base）。
- [`lark-workflow-meeting-summary`](references/subskills/lark-workflow-meeting-summary/GUIDE.md) — 会议纪要整理工作流：汇总指定时间范围内的会议纪要并生成结构化报告。当用户需要整理会议纪要、生成会议周报、回顾一段时间内的会议内容时使用。
- [`lark-workflow-standup-report`](references/subskills/lark-workflow-standup-report/GUIDE.md) — 日程待办摘要：编排 calendar +agenda 和 task +get-my-tasks，生成指定日期的日程与未完成任务摘要。适用于了解今天/明天/本周的安排。
