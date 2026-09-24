---
name: lark-scan-report
version: 3.1.0
description: "Generate structured work reports from Feishu tasks and messages. Five report types: morning briefing, evening wrap-up, quick scan, full snapshot, weekly summary. Uses weekly log accumulation — no redundant re-fetching. All logic runs through lark-cli commands — no special runtime requirements. Triggers on: 'daily report', 'morning report', 'evening report', 'weekly summary', '日报', '晚报', '周报'."
metadata:
  requires:
    bins: ["lark-cli"]
    skills: ["lark-task", "lark-im", "lark-shared"]
---

# Lark Scan Report

Modular Feishu report system with **weekly log accumulation**. Morning and evening reports fetch data and append to a rolling weekly log. The weekly summary reads that log — no re-fetching.

All data collection and delivery runs through `lark-cli` commands. No code execution, no SDK dependency — any agent that can run shell commands can operate this skill.

**CRITICAL — Before starting, read [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) for auth and permission handling.**

## First-Time Setup

If `config.yaml` has empty whitelist/blacklist (or user says "setup daily report", "configure report"), run the interactive setup flow:

### Setup Flow

1. **Check auth**: Verify `lark-cli auth` is configured. If not, guide user through `lark-cli config init` + `lark-cli auth login --recommend`.

2. **Ask basic preferences**:
   - Language: zh or en? (default: en)
   - Timezone: (default: Asia/Shanghai)
   - Output format: text or card? (default: text)

3. **Scan all groups**: Run `lark-cli im +chat-search --member-ids "<user_open_id>" --page-size 100 --sort-by update_time_desc` to get the full list of groups the user is in.

4. **Generate interactive config page**: Create a local HTML file (like `lark-chat-config.html`) listing all groups with:
   - Dropdown: Auto / Whitelist / Blacklist
   - Note field for each group
   - Pre-fill suggestions: system groups (On Call, Help Desk, Assistant) as Blacklist
   - Open in browser for the user to configure

5. **Read user selections**: After user submits, read the JSON result and update `config.yaml` whitelist/blacklist accordingly.

6. **Check required scopes**: Test `+messages-search` and `+get-my-tasks`. If scope missing, guide user to run `lark-cli auth login --scope "<missing_scope>"`.

7. **Create report directories**: `mkdir -p reports/weekly-logs`

8. **Confirm**: Print summary of config — N whitelist, N blacklist, N auto groups. Ready to run.

### Re-configuration

User can say "reconfigure daily report" or "update report config" at any time to re-run steps 3-5 (group scan + selection). This is useful when the user joins new groups or wants to change whitelist/blacklist.

## Architecture

```
Mon-Fri morning/evening reports
  ├── 1. Fetch tasks (tasklists + subtasks)
  ├── 2. Scan messages (per config rules)
  ├── 3. Generate report → output to user
  └── 4. Append structured entry to weekly log

Sunday weekly summary
  ├── 1. Read weekly log (already accumulated)
  ├── 2. Scan weekend delta only (Sat 00:00 → Sun 15:00)
  └── 3. Generate weekly summary → output to user
```

### File Layout

```
{workspace}/reports/
├── morning_YYYY-MM-DD.md              # Archived morning reports
├── evening_YYYY-MM-DD.md              # Archived evening reports
├── weekly_YYYY-WNN.md                 # Archived weekly summaries
└── weekly-logs/
    └── week-YYYY-WNN.md              # Rolling weekly log (append-only)
```

## Report Types

| Type | Schedule (UTC+8) | Trigger | Template |
|------|-------------------|---------|----------|
| `morning` | 10:30 Mon-Fri | cron | [morning-report.md](templates/morning-report.md) |
| `evening` | 22:00 Mon-Fri | cron | [evening-report.md](templates/evening-report.md) |
| `weekly` | Sun 15:00 | cron | [weekly-summary.md](templates/weekly-summary.md) |
| `quick` | — | user | [quick-scan.md](templates/quick-scan.md) |
| `full` | — | user | [full-report.md](templates/full-report.md) |

### User-Triggered Reports

**Quick Scan** (`quick`): Lightweight delta check. Finds the last archived report timestamp, scans only messages since then, checks task changes. Minimal API calls — typically 2-3. Output is compact, ≤15 lines. Does not archive or append to weekly log.

**Full Report** (`full`): Comprehensive on-demand snapshot. Runs all scan rules, full task hierarchy, completion detection from both sources, conversation extraction. Archives to `reports/full_YYYY-MM-DD_HH-MM.md`. Appends to weekly log.

## Execution Flow

### Phase 0: Delta Timestamp

Before any data collection, resolve the scan time window:

```bash
# Read last scan timestamp
cat reports/.last-scan-ts
# → "2026-03-28T10:30:00+08:00"  (or empty if first run)
```

- If `.last-scan-ts` exists → use it as `--start`, current time as `--end`
- If missing → fall back to `delta.fallback_range` from config (default: last_24h)
- After report completes → write current time to `.last-scan-ts`

This ensures every message is fetched exactly once across runs. No duplication, no gaps.

### Phase 1: Data Collection

#### 1A. Task Collection

**Known API issue**: `tasklists list` returns error 1470500. `tasks list` and `+get-my-tasks` only return tasks where user is **assignee** — they miss tasks where user is **follower**. The workaround is a 3-step approach:

```bash
# Step 1: Get all "my tasks" (assignee role) with full detail
# This returns complete task objects with tasklists[].tasklist_guid
lark-cli task tasks list --page-all --format json
# → Extract unique tasklist_guid values from each task's `tasklists` array

# Step 2: For EACH discovered tasklist_guid, get ALL tasks (assignee + follower)
# This is the ONLY way to get follower tasks
lark-cli task tasklists tasks \
  --params '{"tasklist_guid":"<GUID>","completed":"false"}' \
  --page-all --format json
# → Open tasks (completed_at == "0")

lark-cli task tasklists tasks \
  --params '{"tasklist_guid":"<GUID>","completed":"true"}' \
  --page-all --format json
# → Completed tasks — filter by completed_at timestamp for "today" or "this week"

# Step 3: Get tasklist name for display
lark-cli task tasklists get \
  --params '{"tasklist_guid":"<GUID>"}' --format json
# → Returns { tasklist: { name: "TODO", ... } }

# Step 4: For tasks with subtask_count > 0, expand subtasks
lark-cli task subtasks list \
  --params '{"task_guid":"<TASK_GUID>"}' \
  --page-all --format json
```

**Merging logic**: Step 1 gives detailed task objects (custom_fields, description, origin). Step 2 gives section_guid grouping. Merge by task GUID — use Step 1 data for rich fields, Step 2 for section placement.

**Section (Guild) classification**: Each task belongs to a section (guild) within the tasklist. Section names are mapped in `config.yaml → tasks.section_names` (sections API returns 404, so hardcoded). To get a task's section_guid, use `tasks get` → `tasklists[].section_guid`.

Classification rules:
- **TODO**: Active work items the user is responsible for
- **Ongoing Project**: Multi-day/week projects the user is driving
- **Tracking**: Items owned by others that the user monitors — show separately, lower priority
- **Content Topic**: Personal notes/memos — **skip in reports entirely**

The `members[].role` field (assignee/follower) is less important than the section classification for this user. All tasks in TODO and Ongoing are "responsible" regardless of role.

#### 1B. Message Scanning (Delta-Based)

Execute scan phases from config. All phases use `--start` / `--end` from Phase 0.

```bash
# Phase 0: Discover ALL active conversations in delta window
# This single call finds every chat with activity — including new groups you were added to
lark-cli im +messages-search \
  --start "<last_scan_ts>" \
  --end "<now>" \
  --page-size 50 --page-all --format json
# → Returns messages with chat_id, sender, content. Extract unique chat_ids.

# Phase 1: Whitelist groups — full message pull (paginate all)
lark-cli im +chat-messages-list \
  --chat-id "oc_xxx" \
  --start "<last_scan_ts>" --end "<now>" \
  --page-size 50 --page-all --format json

# Phase 2: All discovered P2P chats — iterate each
lark-cli im +chat-messages-list \
  --user-id "ou_xxx" \
  --start "<last_scan_ts>" --end "<now>" \
  --page-size 50 --page-all --format json

# Phase 3: All other discovered groups — full pull
# (groups found in Phase 0 that are not in whitelist or blacklist)
lark-cli im +chat-messages-list \
  --chat-id "oc_yyy" \
  --start "<last_scan_ts>" --end "<now>" \
  --page-size 50 --page-all --format json

# Phase 4: @me catch-all (belt-and-suspenders)
lark-cli im +messages-search \
  --is-at-me \
  --start "<last_scan_ts>" --end "<now>" \
  --page-size 30 --format json

# Phase 5: Thread follow-up in whitelist groups
lark-cli im +threads-messages-list \
  --params '{"thread_id":"omt_xxx"}' \
  --format json
```

**Key**: `--page-all` ensures we get ALL messages in the delta window, not just the first page. Since we're only fetching the delta (not 24h), the volume per call is manageable.

#### 1C. Completion Detection

Completions from TWO sources — always check both:

1. **Task API**: `tasklists tasks` with `completed=true`, filter by `completed_at` timestamp within report window
2. **Chat messages**: Scan for completion signals in conversations:
   - User said: "done", "搞定", "已发", "finished", "deployed", "merged", "shipped"
   - User shared a deliverable (file, link, screenshot)
   - Counterpart acknowledged: "收到", "looks good", "approved", "thanks"

Cross-reference: if a chat message indicates completion but the task is still open, note it. If a task is marked complete but no chat evidence, still include it.

#### 1D. Group Discussion Digest

For morning/evening/full reports, generate a **group discussion overview** from ALL scanned group messages (including groups where user wasn't @'d):

- Per active group: summarize the main topic(s) being discussed in 1-2 lines
- If a specific message is noteworthy, quote it with attribution: `@person: "exact quote"`
- Flag discussions that may relate to user's scope even if user wasn't involved
- This section captures the "ambient awareness" that would otherwise be missed

This becomes the "各群动态" / "Group Activity" section in the report output.

### Phase 2: Intelligence Layer (Message → Action Extraction)

**This is the core value of the report.** Raw data from Phase 1 is useless without intelligence extraction. The report is NOT a message log — it is an actionable briefing.

#### 2A. Temporal Reasoning

All timestamps must be interpreted relative to the **report generation time**, not the message send time.

Rules:
- If someone said "明天开放" at 23:00 on 3/27, and the report runs at 10:30 on 3/28, that means "今天已开放" — NOT "明天开放"
- "下周" in a Monday message means this current week if the report runs on Tuesday
- "刚刚部署了" in a message 12 hours ago = already deployed, verify current status
- Always convert relative time references to absolute dates, then re-relativize to report time

#### 2B. Message → Task Mapping

For each message thread/conversation, attempt to match it to an existing task:

1. **Keyword match**: task summary ↔ message content (fuzzy match on project names, feature names, people names)
2. **People match**: task assignee/follower ↔ message sender
3. **Context match**: same topic group + similar domain language

When matched:
- Update the task's status description with chat evidence (e.g., "消息中确认已部署")
- Merge chat progress into the task row — don't show it separately
- If chat indicates completion but task is still open, flag as "疑似完成，任务未关闭"

When NOT matched:
- Extract as a standalone action item if it requires user response
- Classify as FYI/signal if no action needed

#### 2C. Action Item Extraction

From every message, extract one of:
- **Action required by user**: "你需要..." — becomes a TODO item
- **Decision needed from user**: "需要你定..." — becomes a decision item
- **Progress update**: "XXX 已完成" — becomes evidence for task progress
- **Blocker reported**: "被 XXX 阻塞" — becomes a blocker item
- **FYI / signal**: no action needed — goes to signals section only if notable

#### 2D. Report Organization Principle

The report answers: **"你现在需要做什么？"**

Structure priority (top to bottom):
1. **What you finished** (completed tasks + chat evidence) — positive momentum first
2. **What needs your action NOW** (decision items, review requests, replies needed)
3. **What's in progress** (tasks with recent activity, grouped by tasklist)
4. **What's blocked** (waiting on others, with duration and escalation plan)
5. **Key conversations** (topic → conclusion → your action, NOT raw messages)
6. **Signals** (ambient awareness, org changes, competitor moves)

**Anti-patterns to avoid:**
- ❌ "群里讨论了XXX" → ✅ "XXX 已定方案：令牌桶 → 你需要更新设计文档"
- ❌ "收到 3 条消息" → ✅ "3 条消息均为 FYI，无需回复"
- ❌ "吴郕说明天开放" → ✅ "飞书 CLI 今天已开放（吴郕 3/27 晚确认）"

### Phase 2.5: Report Generation

Apply the appropriate template (see `templates/` directory). Each template defines the exact output structure.

**Output format** — check `config.yaml → output.default_format`:
- `text`: Plain text with tab-aligned columns and Unicode box chars for structure. Suitable for direct Feishu message.
- `card`: Feishu interactive card JSON via `--msg-type interactive`.

### Phase 3: Weekly Log Append (morning & evening only)

After generating the report, append a structured entry to the weekly log. This is the key mechanism that makes weekly summaries efficient.

```markdown
<!-- Append to reports/weekly-logs/week-YYYY-WNN.md -->

## YYYY-MM-DD evening

### Completed
- [task] Finished API migration for user service
- [chat] Sent final design spec to @designer (confirmed received)

### In Progress
- [task] Payment integration — waiting for sandbox credentials
- [chat] Discussing rate limit strategy in #arch-group

### Discussions & Decisions
- #core-team: Agreed to delay launch by 1 week for QA
- @PM: Changed priority of onboarding flow to P0

### Blockers
- Sandbox credentials from payment provider (3 days waiting)

### Signals & Notes
- @CTO mentioned potential org restructure in #leadership
- New competitor feature spotted in #market-watch discussion
```

**Log rules**:
- One `## YYYY-MM-DD morning/evening` section per report run
- Use `[task]` or `[chat]` prefix to indicate the source
- Keep entries atomic and factual — no commentary
- Deduplicate across morning/evening for the same day
- This file is append-only during the week; never rewrite earlier entries

### Phase 4: Delivery

```bash
# Text format
lark-cli im +messages-send \
  --user-id "ou_xxx" \
  --text "<report_content>"

# Card format
lark-cli im +messages-send \
  --user-id "ou_xxx" \
  --msg-type interactive \
  --content '<card_json>'
```

## Weekly Summary: Diff-Based Approach

The Sunday weekly summary does NOT re-scan the entire week. Instead:

1. **Read the weekly log** (`reports/weekly-logs/week-YYYY-WNN.md`) — this already contains all Mon-Fri data
2. **Scan weekend delta only** — messages from Friday 22:00 → Sunday 15:00
3. **Synthesize** — merge the log entries into a cohesive weekly narrative

This means the weekly summary is fast and never makes redundant API calls.

## Classification Logic

When categorizing items from tasks and messages:

| Signal | Category |
|--------|----------|
| User is the owner/assignee | TODO / In Progress |
| Someone else owns it, user is waiting | Tracking |
| User-led, multi-day/week effort | Ongoing Project |
| Explicitly completed (task API or chat signal) | Completed |
| Mentioned but no action required | FYI / Signal |

## Formatting Rules

- `**bold**` for key items and names
- `*italic*` for next steps and notes
- **Attribution rule**: When mentioning someone's words/actions, the reader must know WHERE it was said. Rules:
  - If the item is already under a group heading (e.g. `**[增长&留存](link)** · 3/27`), do NOT repeat the group name on each bullet — just write `某人 时间：内容`
  - If the item appears OUTSIDE its group context (e.g. in 🔴需要关注 or 📡信号), MUST include `[群名](deeplink)` so reader knows the source
  - Never redundantly repeat `[群名]` when already scoped under that group's heading
- Group links as deeplinks: `[group-name](https://applink.feishu.cn/client/chat/open?openChatId=oc_xxx)`
- Task links from API `url` field
- Completed subtasks: `~~*subtask name*~~ ✅` (strikethrough + italic + checkmark)
- In-progress subtasks: `🔄 subtask name`, indented with `\u2003\u2003`
- Status indicators: done / in-progress / blocked / waiting

## Cron Configuration Examples

```json
[
  {
    "name": "lark-morning-report",
    "schedule": { "kind": "cron", "expr": "30 2 * * 1-5", "tz": "UTC" },
    "payload": {
      "kind": "systemEvent",
      "text": "Generate morning report. Read lark-scan-report skill, load config.yaml, execute report_type=morning."
    }
  },
  {
    "name": "lark-evening-report",
    "schedule": { "kind": "cron", "expr": "0 14 * * 1-5", "tz": "UTC" },
    "payload": {
      "kind": "systemEvent",
      "text": "Generate evening report. Read lark-scan-report skill, load config.yaml, execute report_type=evening."
    }
  },
  {
    "name": "lark-weekly-summary",
    "schedule": { "kind": "cron", "expr": "0 7 * * 0", "tz": "UTC" },
    "payload": {
      "kind": "systemEvent",
      "text": "Generate weekly summary. Read lark-scan-report skill, load config.yaml, execute report_type=weekly."
    }
  }
]
```
