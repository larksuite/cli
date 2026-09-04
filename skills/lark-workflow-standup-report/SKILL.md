---
name: lark-workflow-standup-report
description: "工作日报/周报/WBS 生成：综合飞书日历、视频会议、群聊、云文档、Codex 或 Claude Code 本地会话、项目知识记录、WBS Base 表结构等多源数据，生成含来源标注的日报/周报，或生成可写入研发 WBS 多维表格的条目草稿。用于生成今日日报、当天日报、工作日报、本周周报、工作简报、周工作总结、飞书周报，以及根据聊天和会议总结 WBS、填研发 WBS、按 WBS 表格要求整理周报；当用户希望直接产出飞书云文档而不是只要本地 Markdown 时，也必须使用这个 skill。"
---

# 工作日报 / 周报 / WBS

开始前必须先读取 `../lark-shared/SKILL.md`，并使用 `--as user`。当需要创建或更新飞书云文档时，同时读取 `../lark-doc/SKILL.md`。

## 模式选择

| 用户意图 | 模式 | 默认动作 |
| --- | --- | --- |
| 日报、今日、当天、今天总结 | 日报模式 | 先本地 Markdown，再按需创建飞书文档 |
| 周报、工作简报、周总结 | 周报简报模式 | 先本地 Markdown，再按需创建飞书文档 |
| WBS、研发 WBS、多维表格、按条目填表 | WBS 填报模式 | 先分析表结构，再生成 WBS 草稿 |
| 明确写入/新增/加到表格 | WBS 写入 | 先生成或读取写入计划，再写表并核验 |

日报默认时间范围为用户所在时区当天 `00:00:00` 到当前时刻。周报默认自然周。日报只覆盖当天，除非用户明确要求补写其他日期。

## 跨平台脚本要求

Windows/PowerShell 使用 `.ps1` 脚本。macOS/Linux 使用 `.sh` 脚本，并要求 `bash`、`jq`、`python3`、`perl`、`lark-cli` 可用。

macOS 可用 Homebrew 补齐依赖：`brew install jq python3`。脚本避免依赖 GNU-only 命令；若运行环境缺少 `timeout`，采集命令会降级为无外层超时，但仍会保留 lark-cli 自身错误输出。

## 日报强制流程

日报模式必须走脚本化采集和候选审查，避免漏翻页、复用旧证据或把他人群聊事项误归因为本人工作。

1. 确定日期、起止时间、输出目录：
   - 本地 Markdown：`outputs/工作日报-YYYY-MM-DD.md`
   - 工作目录：`work/daily_YYYY-MM-DD/`
2. 运行采集脚本采集飞书数据、分页、脱敏，并生成 `source_manifest.json`。
   - macOS/Linux 优先使用 `scripts/collect_lark_daily.sh`。
   - Windows/PowerShell 使用 `scripts/collect_lark_daily.ps1`。
   - 会议证据不能只停留在 `vc +search` 列表；脚本应优先补抓会议详情，以及可用的逐字稿/纪要/妙记元数据。
   - 妙记 AI 总结、待办、章节只可作为“候选线索”，不能直接当高可信主结论；只要拿到了逐字稿，后续必须以逐字稿为主，由 Agent 自己归纳。
   - 私聊证据不能在候选审查里退化成泛化的 `本人 p2p 消息`；后续审查和正文应尽量落到“与谁私聊”“哪个群”“哪场会议”“哪个文档”。
3. 运行 Agent 证据脚本采集 Codex、Claude Code 和本地项目证据，并生成 `agent_session_evidence_YYYY-MM-DD.md` 与 `agent_evidence_YYYY-MM-DD.json`。
   - macOS/Linux 优先使用 `scripts/collect_agent_evidence.sh`。
   - Windows/PowerShell 使用 `scripts/collect_agent_evidence.ps1`。
   - 本地 Codex / Claude Code 会话不是默认低一级证据。若事项本身是本地方案、代码、测试、文档、构建或知识沉淀产出，且有当天可验证文件，则本地 Agent 证据可作为该工作包主证据。
   - 只有在“引用会议内容下结论”时，同主题本地 Agent 会话才默认作为会议证据的补强，不能替代逐字稿、纪要正文或会议元数据。
4. 运行候选审查脚本生成 `candidate_review_YYYY-MM-DD.md`。写日报前必须阅读该文件。
   - macOS/Linux 优先使用 `scripts/build_candidate_review.sh`。
   - Windows/PowerShell 使用 `scripts/build_candidate_review.ps1`。
5. 若会议拿到了逐字稿，写日报前必须先阅读逐字稿并自行做摘要；不得直接复述飞书内置 AI 的妙记摘要。若当天存在同主题 Codex / Claude Code 会话、本地调研文档或知识记录，只在会议事项归因环节把这些材料作为补强证据，与逐字稿交叉校验。
6. 按 `references/daily-report-rules.md` 生成 Markdown。自动化日报、飞书日报和用户未要求简版的日报正文必须使用五段式：
   - `今日口述摘要`
   - `今日推进明细`
   - `风险与阻塞`
   - `明日计划`
   - `数据来源说明`
   - 每个工作包、风险或计划默认只写一行 `来源：`，把飞书会议、群聊、云文档和本地证据合并到同一行，避免一条事项拆成多行来源。
7. 只有用户明确要求“简版/只要核心进展”时，才可省略 `今日口述摘要` 和 `数据来源说明`；省略原因仍写入执行记录、memory 或最终回复。
8. 若需要飞书文档，使用文档创建脚本创建到日报父 Wiki 节点，并保存响应到 `work/daily_YYYY-MM-DD/feishu_doc_create_daily_YYYY-MM-DD.json`。
   - macOS/Linux 优先使用 `scripts/create_daily_doc.sh`。
   - Windows/PowerShell 使用 `scripts/create_daily_doc.ps1`。
9. 在 `knowledge/` 和 automation memory 中记录文档 URL、覆盖范围、未完整获取的数据源、候选审查结论和未纳入的重要本地项目。

示例脚本调用：

macOS/Linux:

```bash
date="2026-06-18"
start="2026-06-18T00:00:00+08:00"
end="2026-06-18T23:00:00+08:00"
dir="work/daily_2026-06-18"
skill_dir="$HOME/.agents/skills/lark-workflow-standup-report"

bash "$skill_dir/scripts/collect_lark_daily.sh" \
  --date "$date" --start "$start" --end "$end" --out-dir "$dir"

bash "$skill_dir/scripts/collect_agent_evidence.sh" \
  --date "$date" --start "$start" --end "$end" --out-dir "$dir"

bash "$skill_dir/scripts/build_candidate_review.sh" \
  --date "$date" \
  --source-manifest-path "$dir/source_manifest.json" \
  --agent-evidence-json-path "$dir/agent_evidence_$date.json" \
  --out-file "$dir/candidate_review_$date.md"
```

Windows/PowerShell:

```powershell
$date = "2026-06-18"
$start = "2026-06-18T00:00:00+08:00"
$end = "2026-06-18T23:00:00+08:00"
$dir = "work/daily_2026-06-18"

& "C:\Users\Leo\.agents\skills\lark-workflow-standup-report\scripts\collect_lark_daily.ps1" `
  -Date $date -Start $start -End $end -OutDir $dir

& "C:\Users\Leo\.agents\skills\lark-workflow-standup-report\scripts\collect_agent_evidence.ps1" `
  -Date $date -Start $start -End $end -OutDir $dir

& "C:\Users\Leo\.agents\skills\lark-workflow-standup-report\scripts\build_candidate_review.ps1" `
  -Date $date `
  -SourceManifestPath "$dir/source_manifest.json" `
  -AgentEvidenceJsonPath "$dir/agent_evidence_$date.json" `
  -OutFile "$dir/candidate_review_$date.md"
```

## 日报归因红线

写日报前必须执行候选审查：

- 只写本人主导、本人明确推进、本人实际产出、本人需要继续跟进的事项。
- 全量群聊只作上下文；无本人发言/响应/会议参与/本地产出/用户显式归属时，不纳入正文。
- 他人原型更新、Bug 修复、技术讨论不得写成本人推进事项，只能作为背景或风险来源。
- 普通日报采集、整理、创建文档不作为 `今日推进明细` 的业务工作项，只能放在 `数据来源说明`、执行记录或 memory。
- 妙记 AI 摘要、待办、章节不是高可信主证据；如果逐字稿可用，必须以逐字稿为准，由 Agent 自己总结，不得把飞书 AI 的表述直接当作正式结论。
- 本地 Codex / Claude Code 会话只有在“会议事项归因”场景中才默认作为补强证据，不能替代逐字稿或会议元数据本身；若会议内容只靠本地会话单边推断，正文必须降级表述为“本地分析判断”而不是“会议已确认”。
- 若事项本身是本地 Agent 工作产出，且有当天 knowledge、代码、脚本、测试、文档、构建产物等可验证落地，本地 Codex / Claude Code 会话和产物可作为该工作包的主证据。
- AIEXCEL、Swimlane、Wardrobe 等历史重点项目即使未纳入，也要在证据或执行记录中写明原因。

详细规则见 `references/daily-report-rules.md`。

## 周报与 WBS

周报简报、WBS 草稿、WBS 写入护栏见 `references/weekly-wbs-rules.md`。

关键约束：

- WBS 草稿必须包含 `口述汇报摘要` 和 `候选 WBS 条目`，否则不能作为写表依据。
- 用户说“先在本地 MD 写一下”时禁止写表。
- 用户明确要求写入时，默认写入全部候选条目；若要减少条目，必须先列出跳过原因并等待用户认可。
- WBS 写入/推送到 Base 时，`任务级别` 必须统一写入 `二级`；即使草稿或候选条目里出现 `一级`，构造 `record-upsert` payload 前也必须覆盖为 `二级`。
- WBS 排除只影响候选条目和实际写表，不影响日报/周报正文，除非用户明确说“日报也不要写”。

## 飞书命令参考

优先使用本 skill 的脚本。脚本不可用或需要调试时，读取 `references/lark-source-commands.md`。

权限缺失时按 `../lark-shared/SKILL.md` 处理：

- 日历：`calendar:calendar.event:read`
- 视频会议搜索：`vc:meeting.search:read`
- 会议详情：`vc:meeting.meetingevent:read`
- 会议纪要：`vc:note:read`
- 妙记元信息/AI 产物（按需）：`minutes:minutes:readonly minutes:minutes.artifacts:read minutes:minutes.transcript:export`
- 群聊消息：`search:message`
- 群聊消息详情（user 身份）：基础权限 `im:message:readonly`（或 `im:message`）+ 补充权限 `im:message.group_msg:get_as_user im:message.p2p_msg:get_as_user`
- 当前用户信息（user 身份）：`contact:user.basic_profile:readonly`
- 云文档：`search:docs:read`

## 失败分支

| 触发条件 | 一线处理 | 仍失败时 |
| --- | --- | --- |
| `lark-cli` 或 Node 运行时不可用 | 读取 `references/lark-source-commands.md`，定位 CLI 或设置 `LARK_CLI_NODE` / `LARK_CLI_RUNJS` | 不创建飞书文档，只输出本地 Markdown，并在执行记录写明阻塞 |
| 飞书权限不足 | 按 `../lark-shared/SKILL.md` 使用最小 scope 重新授权 | 跳过该数据源，正文不编造结论，执行记录标明未获取 |
| 群聊/云文档分页失败 | 保留已获取页面，记录 `source_manifest.json` 的错误项 | 候选审查中降低置信度，不能把缺证据事项写成确定推进 |
| 会议纪要/妙记获取失败 | 先保留 `vc +search` 和 `meeting get` 结果，再单独记录 notes/minutes 错误 | 仍可生成日报，但必须在 `数据来源说明` 和执行记录中写明会议内容仅使用元数据/链接，未拿到逐字稿或纪要正文 |
| 只有妙记 AI 摘要、没有逐字稿 | 把 AI 摘要当候选线索，再用本人消息、云文档、本地 Agent 产物交叉验证 | 若仍缺逐字稿和交叉证据，不得把 AI 摘要写成确定结论；正文降级为背景、风险或待确认线索 |
| 群聊消息搜索超时 | 优先保留本人消息；macOS/Linux 用 `collect_lark_daily.sh --im-chunk-hours 1 --im-request-timeout-seconds 45 --im-max-failures-per-search 4`，Windows 用 `collect_lark_daily.ps1 -ImChunkHours 1 -ImRequestTimeoutSeconds 45 -ImMaxFailuresPerSearch 4` 分片采集并隔离失败 | 跳过超时的全量群聊分片，在 `数据来源说明` 标明缺口，不阻塞日报生成 |
| 本地 Agent 证据脚本无候选 | 检查 `CODEX_HOME`、Claude `projects`、项目根目录参数 | 在执行记录写“未发现当天本地证据”，不要复用旧证据 |
| 候选审查表缺失 | 先运行 `build_candidate_review.sh` 或 `build_candidate_review.ps1` | 停止写日报正文，直到审查表生成 |
| 飞书文档创建失败 | 保存失败响应，保留本地 Markdown | 最终回复给出本地路径、失败原因和重试命令 |

## 检查点

🔴 CHECKPOINT · 写入 WBS 前必须停下：只有用户明确要求“写入/新增/加到表格”，并且本地草稿存在 `候选 WBS 条目` 与 `实际写入计划`，才允许写 Base。

🔴 CHECKPOINT · 覆盖或更新已有飞书日报前必须停下：除非用户明确要求覆盖更新，否则默认创建新文档或只返回本地 Markdown。

## 反例黑名单

不要做这些事：

- 不要跳过 `candidate_review_YYYY-MM-DD.md` 直接写日报。
- 不要把私聊候选统一写成 `本人p2p消息` 这种不可读标签；优先写“与某某私聊”或“未命名私聊联系人”。
- 不要把他人群聊推进、机器人 Bug 通知、仅浏览文档写成本人工作。
- 不要把普通日报自动化采集/创建文档写成业务工作项。
- 不要复用旧的 Agent 证据文件当作当天完整证据。
- 不要展示裸 `message_id`、敏感凭据、token、连接串。
- 不要在未确认候选条目数量和跳过原因时写入 WBS。
- 不要把飞书来源集中堆到附录。
- 不要把一条事项拆成 3-5 行零散来源；默认合并成一行 `来源：链接 A；链接 B；本地证据 C`。
- 不要把妙记 AI 自动摘要、待办或章节原文直接抄进日报当正式结论。
- 不要在已有逐字稿时仍优先引用 AI 摘要；逐字稿优先，由 Agent 自己总结。
- 不要把本地 Codex / Claude Code 分析误写成“会议已确认”；它只能补强会议事项，不能替代会议原始证据。
- 不要把“会议事项的补强证据”泛化成“所有 Codex / Claude Code 会话都只能补强”；本地 Agent 实际产出有落地文件时可作为主证据。

## 输出与记录

每条正式结论必须包含：

1. 事项描述。
2. 背景/目标、价值或原因。
3. 直接内联来源标注。

禁止：

- 时间流水账。
- 无来源支撑的结论。
- 裸 `message_id`。
- 敏感凭据、token、连接串。
- 把来源集中放到附录。

日报飞书标题建议：`工作日报 YYYY-MM-DD 杜励承`。正文不要重复一级标题。

创建飞书文档后，必须保存创建响应，并在 workspace `knowledge/` 追加中文执行记录。
