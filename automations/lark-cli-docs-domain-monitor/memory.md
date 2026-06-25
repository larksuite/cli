# Lark CLI Docs Domain Monitor Memory

Last run: 2026-06-25 09:33:17 Asia/Shanghai

- Read required `~/.agents/agent.md`, repository `AGENTS.md`, GitHub skill docs, lark-doc shared/fetch/xml/update/update-workflow/style docs, and previous automation memory before writes.
- `CODEX_HOME` was empty in this shell; used existing automation memory path `automations/lark-cli-docs-domain-monitor/memory.md`, which was already untracked before this run.
- GitHub reads succeeded with `gh`: 25 open issues and 0 open PRs carrying exact `domain/doc` label. PR #1566 was separately checked because it cross-references #1555; it is open but labels are `domain/base`, `size/M`, so it was not included in the PR document.
- Lark fetches succeeded as user for both target wiki documents using `/Users/bytedance/go/bin/lark-cli docs +fetch --api-version v2 --as user` with notice env suppression.
- Existing Issue document had matching cards for 24 current open issues; only `issue-1555` was missing. All unchanged open issue cards were skipped because stored `GITHUB_UPDATED_AT` matched GitHub.
- Processed one new issue: #1555. Analysis concluded `base +record-upsert` is missing an optional `--user-id-type` flag and query param wiring for POST/PATCH record create/update, while underlying Base request helpers already support params.
- Updated Issue document with block-level replacements only for the `总览表` callout/table and run summary callout/table, then appended one new `issue-1555` analysis card. The overview now has 25 rows.
- Updated PR document run summary/table to state there are no open exact-label `domain/doc` PRs; no PR overview table was created.
- Acceptance checks passed after refetch: exactly one `总览表` before `本轮运行摘要`, overview columns are exactly `issue number`, `priority`, `title or URL`, `问题`, `recommended next action`, `updatedAt`, no `标签` column, all current open issue monitor keys are unique, and `issue-1555` contains code pointers plus a concrete validation plan.
- No repo-tracked files were modified; no local build/compile/unit-test commands were run.
