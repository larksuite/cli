---
name: lark-suite
version: 1.0.0
description: "Use lark-cli to operate Lark/Feishu across messaging, docs, drive, sheets, base, calendar, mail, tasks, wiki, meetings, approvals, events, and more. This is the single entry skill; load domain docs only when needed."
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli --help"
---

# Lark/Feishu CLI

Use `lark-cli` to operate Lark/Feishu resources. This skill is the default entry point and keeps the installed skill list small. Domain-specific details remain available on demand through the CLI-embedded skill content, so agents should load only the domain they need for the current user request.

## Start Here

1. For setup, authentication, identity switching, scope errors, or `_notice` output, read:

   ```bash
   lark-cli skills read lark-shared
   ```

2. Pick the domain from the routing table below.

3. Read that domain before choosing flags or payload shapes:

   ```bash
   lark-cli skills read <domain-skill>
   ```

4. For detailed reference files mentioned by a domain skill, read them with:

   ```bash
   lark-cli skills read <domain-skill> references/<file>.md
   ```

## Domain Routing

| User intent | Read this domain |
| --- | --- |
| Configure app credentials, login, switch `--as user` / `--as bot`, fix permission or scope errors | `lark-shared` |
| Send/reply/search messages, manage chats, download IM resources, reactions, feed shortcuts/groups | `lark-im` |
| Create/read/edit docs, insert or download doc media, handle docx/wiki document URLs | `lark-doc` |
| Upload/download/search/move/copy/delete files, import local files, manage Drive metadata/comments/permissions | `lark-drive` |
| Create/edit native Markdown files stored in Drive, patch/diff Markdown | `lark-markdown` |
| Create/read/write spreadsheets, formulas, formatting, charts, pivots, filters, images | `lark-sheets` |
| Work with Base/bitable tables, fields, records, views, forms, dashboards, formulas, permissions | `lark-base` |
| Create/edit slides and presentation pages | `lark-slides` |
| Create/search/update calendar events, agenda, free/busy, time suggestions, meeting rooms | `lark-calendar` |
| Search users and resolve names, emails, phone numbers, or open_id values | `lark-contact` |
| Browse/search/read/send/reply/forward mail, drafts, folders, labels, rules, attachments | `lark-mail` |
| Create/update/query tasks, task lists, subtasks, task agents, task attachments | `lark-task` |
| Manage wiki spaces, members, and node hierarchy | `lark-wiki` |
| Search past meetings, fetch meeting notes/minutes/transcripts, participant snapshots | `lark-vc` |
| Join/leave active meetings as an agent, read live meeting events | `lark-vc-agent` |
| Search/download/upload/update Minutes media or metadata | `lark-minutes` |
| Query or update OKRs | `lark-okr` |
| Query approval tasks, approve/reject/transfer/cancel/CC approval instances | `lark-approval` |
| Query personal attendance check-in records | `lark-attendance` |
| Subscribe to real-time Lark events via WebSocket / NDJSON streams | `lark-event` |
| Export or update whiteboards, diagrams, Mermaid/PlantUML/DSL visual content | `lark-whiteboard` |
| Explore raw OpenAPI endpoints not covered by existing shortcuts | `lark-openapi-explorer` |
| Build a custom lark-cli skill from repeated API operations | `lark-skill-maker` |
| Summarize meetings over a date range | `lark-workflow-meeting-summary` |
| Produce agenda plus task standup summaries | `lark-workflow-standup-report` |

## Command Exploration

```bash
lark-cli <domain> --help
lark-cli <domain> +<shortcut> --help
lark-cli schema <service>.<resource>.<method>
lark-cli <service> <resource> <method> [flags]
```

Prefer documented shortcuts (`+<name>`) when a domain skill offers one. Use raw OpenAPI commands only after reading the schema.
