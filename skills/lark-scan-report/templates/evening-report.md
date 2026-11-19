# Evening Report Template

**Purpose**: Daily wrap-up — answers "今天做了什么？还有什么没收尾？"
**Schedule**: Mon-Fri 22:00 UTC+8
**Archive**: `reports/evening_YYYY-MM-DD.md`

## Core Question

> "今天完成了什么？有什么阻塞需要明天升级？群里有什么重要决策？"

## Scan Window

- Full day messages: morning report time (10:30) → now (22:00), or 00:00 → 22:00
- Tasks: all tasks + today's completed tasks (filter `completed_at` to today)
- Today's morning report: read for continuity — check if TOP 3 was achieved

## Completion Detection

From TWO sources:
1. **Task API**: `tasklists tasks` with `completed=true`, filter `completed_at` to today's timestamp range
2. **Chat signals**: "done", "搞定", "已发", "deployed", "merged" + deliverable shared + counterpart acknowledged

Cross-reference both. A task marked done + chat evidence = `[双]` source tag.

## Output Structure (Card Format)

```json
{
  "config": { "wide_screen_mode": true },
  "header": {
    "template": "indigo",
    "title": { "tag": "plain_text", "content": "🌙 晚报 — YYYY-MM-DD 周X" }
  },
  "elements": [
    {
      "tag": "markdown",
      "content": "**✅ 今日完成**\n• [任务] **Schema 更新** — 已合并到 main\n• [消息] 发终稿给 @设计师 — 对方确认收到\n• [双] **支付 bugfix** — 已部署 staging + 任务已关\n\n— 或：今天没有完成项（说明原因）"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**📌 需要你明天推动**\n• **回复 @PM 关于优先级** — 今天在 [#核心团队](deeplink) 讨论了但没定\n• **Review PR #42** — @张三 今天提交，等你 review\n\n— 或：无待推动事项"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**🔄 进行中**\n*清单: TODO*\n• **限流模块** [负责] — 70% · 下一步: 跑测试\n  ├ Redis 部署 ✅\n  └ 配置迁移 🔄\n• **AI Reddit** [跟进] — @某人 在推进 · 2 个子任务未完成"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**🚫 阻塞项**\n• **沙盒凭证** — 等 @供应商 · 已 3 天 · *明天再催，后天升级到 VP*"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**💬 重要对话**\n• [#核心团队](deeplink): 上线时间 → **延后一周做 QA** — 全员同意 · *你需要更新排期表*\n• 私聊 @PM: 入职流程 → **优先级改 P0** · *你需要重排 sprint*\n• [#架构组](deeplink): 限流方案 → **确定令牌桶** · *FYI，与你任务一致*"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**📡 信号**\n• @CTO 在 #leadership 提到组织架构调整可能\n• 竞品发布了类似功能（#market-watch 讨论）"
    },
    {
      "tag": "note",
      "elements": [{ "tag": "plain_text", "content": "22:00 UTC+8 · 扫描窗口 10:30→22:00" }]
    }
  ]
}
```

## Section Rules

| Section | Show when | Skip when |
|---------|-----------|-----------|
| ✅ 今日完成 | Always — lead with wins | Say "无完成项" if empty |
| 📌 需要明天推动 | Extracted actions from messages/tasks | Nothing pending |
| 🔄 进行中 | Has open tasks | — |
| 🚫 阻塞项 | Something blocked > 1 day | No blockers |
| 💬 重要对话 | Has discussions with decisions/actions | No notable conversations |
| 📡 信号 | Org/competitor/strategic signals found | Nothing notable |

## Attribution Rule

When mentioning someone's words/actions:
- Under a group heading (e.g. `**[增长&留存](link)** · 3/27`): do NOT repeat group name on each bullet — just `某人 时间：内容`
- Outside group context (🔴需要关注, 📡信号): MUST include `[群名](deeplink)` for source context
- Never redundantly repeat group name when already under that group's heading

## Morning Report Reconciliation

Compare with this morning's TOP 3:
- If completed: show in ✅ section with "✓ 早报 TOP1"
- If not completed: show in 🔄 with reason
- If new urgent item displaced TOP 3: note what changed and why

## After Generation

Append structured entry to weekly log (see skill.md Phase 3).
