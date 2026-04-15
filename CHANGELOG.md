# Changelog

All notable changes to this project will be documented in this file.

## [v1.0.12] - 2026-04-15

### Features

- Add guided npm install flow that installs or upgrades the CLI, installs AI skills, and walks through app config and auth login (#464)
- **mail**: Add email signature support with `+signature`, `--signature-id` compose flags, and draft signature edit operations (#485)
- **mail**: Return recall hints for sent emails when recall is available (#481)
- **slides**: Add `+media-upload` and support `@path` image placeholders in `+create --slides` (#450)

### Documentation

- **mail**: Add recipient search guidance to the mail skill workflow (#437)
- **calendar/vc**: Route past meeting queries to `lark-vc` and clarify historical date matching in skills (#482, #480)

## [v1.0.11] - 2026-04-14

### Features

- **sheets**: Add dropdown shortcuts for data validation management (`+set-dropdown`, `+update-dropdown`, `+get-dropdown`, `+delete-dropdown`) (#461)
- **task**: Add task search, tasklist search, related-task, set-ancestor, and subscribe-event shortcuts (#377)
- Streamline interactive login by removing the extra auth confirmation step (#451)

### Bug Fixes

- **base**: Validate JSON object inputs for base shortcuts and reject `null` objects (#458)

### Documentation

- **sheets**: Document value formats for formulas and special field types (#456)
- **readme**: Add Attendance to the features table (#460)

## [v1.0.10] - 2026-04-13

### Features

- **im**: Support im oapi range download for large files (#283)
- **sheets**: Add filter view and condition shortcuts (#422)
- **wiki**: Add wiki move shortcut with async task polling (#436)
- **drive**: Add drive `+create-shortcut` shortcut (#432)
- **drive**: Add drive files patch metadata API (#444)
- **task**: Support `--section-guid` flag in tasklist-task-add shortcut (#430)

### Bug Fixes

- **base**: Support large base attachment uploads (#441)
- **config**: Clarify init copy for TTY, preserve original for AI (#448)
- **im**: Reject `--user-id` under bot identity for chat-messages-list (#340)
- **mail**: Add missing scopes for mail `+watch` shortcut (#357)
- **mail**: Restrict `--output-dir` to current working directory (#376)

### Documentation

- **wiki**: Add wiki member operations to lark-wiki skill (#417)
- **task**: Document sections API resources, permissions, and URL parsing (#430)
- **doc**: Clarify when markdown escaping is needed (#312)

## [v1.0.9] - 2026-04-11

### Features

- Add attendance `user_task.query` (#405)
- Support minutes search (#359)
- **slides**: Add slides `+create` shortcut with `--slides` one-step creation (#389)
- **slides**: Return presentation URL in slides `+create` output (#425)
- **sheets**: Add dimension shortcuts for row/column operations (#413)
- **sheets**: Add cell operation shortcuts for merge, replace, and style (#412)
- **drive**: Add drive folder delete shortcut with async task polling (#415)

### Documentation

- **drive**: Add guide for granting document permission to current bot (#414)

## [v1.0.8] - 2026-04-10

### Features

- Add `update` command with self-update, verification, and rollback (#391)
- Add `--file` flag for multipart/form-data file uploads (#395)
- Support file comment reply reactions (#380)
- **base**: Add `+dashboard-arrange` command for auto-arranging dashboard blocks layout and `text` block type with Markdown support (#388)
- **base**: Add record batch `+add` / `+set` shortcuts (#277)
- **base**: Add `+record-search` for keyword-based record search (#328)
- **base**: Add view visible fields `+get` / `+set` shortcuts (#326)
- **base**: Add record field filters (#327)
- **base**: Optimize workflow skills (#345)
- **calendar**: Add room find workflow (#403)
- **mail**: Add `--page-token` and `--page-size` to mail `+triage` (#301)
- **whiteboard**: Add `+query` shortcut and enhance `+update` with Mermaid/PlantUML support (#382)

### Bug Fixes

- Improve error hints for sandbox and initialization issues (#384)
- Fix markdown line breaks support (#338)
- Return raw base field and view responses (#378)
- **base**: Return raw table list response and clarify sort help (#393)
- **calendar**: Add default video meeting to `+create` (#383)
- **mail**: Replace `os.Exit` with graceful shutdown in mail watch (#350)

### Documentation

- **base**: Document Base attachment download via docs `+media-download` (#404)
- Reorganize lark-base skill guidance (#374)

## [v1.0.7] - 2026-04-09

### Features

- Auto-grant current user access for bot-created docs, sheets, imports, and uploads (#360)
- **mail**: Add `send_as` alias support, mailbox/sender discovery APIs, and mail rules API
- **vc**: Extract note doc tokens from calendar event relation API (#333)
- **wiki**: Add wiki node create shortcut (#320)
- **sheets**: Add `+write-image` shortcut (#343)
- **docs**: Add media-preview shortcut (#334)
- **docs**: Add support for additional search filters (#353)

### Bug Fixes

- **api**: Support stdin and quoted JSON inputs on Windows (#367)
- **doc**: Post-process `docs +fetch` output to improve round-trip fidelity (#214)
- **run**: Add missing binary check for lark-cli execution (#362)
- **config**: Validate appId and appSecret keychain key consistency (#295)

### Refactor

- Route base import guidance to drive `+import` (#368)
- Migrate mail shortcuts to FileIO (#356)
- Migrate drive/doc/sheets shortcuts to FileIO (#339)
- Migrate base shortcuts to FileIO (#347)

### Documentation

- **lark-doc**: Document advanced boolean and intitle search syntax for AI agents (#210)

### Chore

- Add depguard and forbidigo rules to guide FileIO adoption (#342)

## [v1.0.6] - 2026-04-08

### Features

- Improve login scope validation and success output (#317)
- **task**: Support starting pagination from page token (#332)
- Support multipart doc media uploads (#294)
- **mail**: Auto-resolve local image paths in all draft entry points (#205)
- **vc**: Add `+recording` shortcut for `meeting_id` to `minute_token` conversion (#246)

### Bug Fixes

- Resolve concurrency races in RuntimeContext (#330)
- **config**: Save empty config before clearing keychain entries (#291)
- Reject positional arguments in shortcuts (#227)
- Improve raw API diagnostics for invalid or empty JSON responses (#257)
- **docs**: Normalize `board_tokens` in `+create` response for mermaid/whiteboard content (#10)
- **task**: Clarify `--complete` flag help for `get-my-tasks` (#310)
- **help**: Point root help Agent Skills link to README section (#289)

### Documentation

- Clarify `--complete` flag behavior in `get-my-tasks` reference (#308)

### Refactor

- Migrate VC/minutes shortcuts to FileIO (#336)
- Migrate common/client/IM to FileIO and add localfileio tests (#322)

## [v1.0.5] - 2026-04-07

### Features

- **drive**: Support multipart upload for files larger than 20MB (#43)
- Add darwin file master key fallback for keychain writes (#285)
- Add strict mode identity filter, profile management and credential extension (#252)

### Bug Fixes

- **mail**: Restore CID validation and stale PartID lookup lost in revert (#230)
- **base**: Clarify table-id `tbl` prefix requirement (#270)
- Fix parameter constraints for LarkMessageTrigger (#213)

### Documentation

- Fix root calendar example (#299)
- Fix README auth scope and api data flag (#298)
- Clarify task guid for applinks (#287)
- Clarify lark task guid usage (#282)
- **lark-base**: Add `has_more` guidance for record-list pagination (#183)

### Tests

- Isolate registry package state in tests (#280)

### CI

- Add scheduled issue labeler for type/domain triage (#251)
- **issue-labels**: Reduce mislabeling and handle missing labels (#288)
- Map wiki paths in pr labels (#249)

## [v1.0.4] - 2026-04-03

### Features

- Support user identity for im `+chat-create` (#242)
- Implement authentication response logging (#235)
- Support im chat member delete and add scope notes (#229)

### Bug Fixes

- **security**: Replace `http.DefaultTransport` with proxy-aware base transport to mitigate MITM risk (#247)
- **calendar**: Block auto bot fallback without user login (#245)

### Documentation

- **mail**: Add identity guidance to prefer user over bot (#157)

### Refactor

- **dashboard**: Restructure docs for AI-friendly navigation (#191)

### CI

- Add a CLI E2E testing framework for lark-cli, task domain testcase and ci action (#236)

## [v1.0.3] - 2026-04-02

### Features

- Add `--jq` flag for filtering JSON output (#211)
- Add `+download` shortcut for minutes media download (#101)
- Add drive import, export, move, and task result shortcuts (#194)
- Support im message send/reply with uat (#180)
- Add approve domain (#217)

### Bug Fixes

- **mail**: Use in-memory keyring in mail scope tests to avoid macOS keychain popups (#212)
- **mail**: On-demand scope checks and watch event filtering (#198)
- Use curl for binary download to support proxy and add npmmirror fallback (#226)
- Normalize escaped sheet range separators (#207)

### Documentation

- **mail**: Clarify JSON output is directly usable without extra encoding (#228)
- Clarify docs search query usage (#221)

### CI

- Add gitleaks scanning workflow and custom rules (#142)

## [v1.0.2] - 2026-04-01

### Features

- Improve OS keychain/DPAPI access error handling for sandbox environments (#173)
- **mail**: Auto-resolve local image paths in draft body HTML (#139)

### Bug Fixes

- Correct URL formatting in login `--no-wait` output (#169)

### Documentation

- Add concise AGENTS development guide (#178)

### CI

- Refine PR business area labels and introduce skill format check (#148)

### Chore

- Add pull request template (#176)

## [v1.0.1] - 2026-03-31

### Features

- Add automatic CLI update detection and notification (#144)
- Add npm publish job to release workflow (#145)
- Support auto extension for downloads (#16)
- Remove useless files (#131)
- Normalize markdown message send/reply output (#28)
- Add auto-pagination to messages search and update lark-im docs (#30)

### Bug Fixes

- **base**: Use base history read scope for record history list (#96)
- Remove sensitive send scope from reply and forward shortcuts (#92)
- Resolve silent failure in `lark-cli api` error output (#85)

### Documentation

- **base**: Clarify field description usage in json (#90)
- Update Base description to include all capabilities (#61)
- Add official badge to distinguish from third-party Lark CLI tools (#103)
- Rename user-facing Bitable references to Base (#11)
- Add star history chart to readmes (#12)
- Simplify installation steps by merging CLI and Skills into one section (#26)
- Add npm version badge and improve AI agent tip wording (#23)
- Emphasize Skills installation as required for AI Agents (#19)
- Clarify install methods as alternatives and add source build steps

### CI

- Improve CI workflows and add golangci-lint config (#71)

## [v1.0.0] - 2026-03-28

### Initial Release

The first open-source release of **Lark CLI** — the official command-line interface for [Lark/Feishu](https://www.larksuite.com/).

### Features

#### Core Commands

- **`lark api`** — Make arbitrary Lark Open API calls directly from the terminal with flexible parameter support.
- **`lark auth`** — Complete OAuth authentication flow, including interactive login, logout, token status, and scope management.
- **`lark config`** — Manage CLI configuration, including `init` for guided setup and `default-as` for switching contexts.
- **`lark schema`** — Inspect available API services and resource schemas.
- **`lark doctor`** — Run diagnostic checks on CLI configuration and environment.
- **`lark completion`** — Generate shell completion scripts for Bash, Zsh, Fish, and PowerShell.

#### Service Shortcuts

Built-in shortcuts for commonly used Lark APIs, enabling concise commands like `lark im send` or `lark drive upload`:

- **IM (Messaging)** — Send messages, manage chats, and more.
- **Drive** — Upload, download, and manage cloud documents.
- **Docs** — Work with Lark documents.
- **Sheets** — Interact with spreadsheets.
- **Base** — Manage multi-dimensional tables.
- **Calendar** — Create and manage calendar events.
- **Mail** — Send and manage emails.
- **Contact** — Look up users and departments.
- **Task** — Create and manage tasks.
- **Event** — Subscribe to and manage event callbacks.
- **VC (Video Conference)** — Manage meetings.
- **Whiteboard** — Interact with whiteboards.

#### AI Agent Skills

Bundled AI agent skills for intelligent assistance:

- `lark-im`, `lark-doc`, `lark-drive`, `lark-sheets`, `lark-base`, `lark-calendar`, `lark-mail`, `lark-contact`, `lark-task`, `lark-event`, `lark-vc`, `lark-whiteboard`, `lark-wiki`, `lark-minutes`
- `lark-openapi-explorer` — Explore and discover Lark APIs interactively.
- `lark-skill-maker` — Create custom AI skills.
- `lark-workflow-meeting-summary` — Automated meeting summary workflow.
- `lark-workflow-standup-report` — Automated standup report workflow.
- `lark-shared` — Shared skill utilities.

#### Developer Experience

- Cross-platform support (macOS, Linux, Windows) via GoReleaser.
- Shell completion for Bash, Zsh, Fish, and PowerShell.
- Bilingual documentation (English & Chinese).
- CI/CD pipelines: linting, testing, coverage reporting, and automated releases.

[v1.0.12]: https://github.com/larksuite/cli/releases/tag/v1.0.12
[v1.0.11]: https://github.com/larksuite/cli/releases/tag/v1.0.11
[v1.0.10]: https://github.com/larksuite/cli/releases/tag/v1.0.10
[v1.0.9]: https://github.com/larksuite/cli/releases/tag/v1.0.9
[v1.0.8]: https://github.com/larksuite/cli/releases/tag/v1.0.8
[v1.0.7]: https://github.com/larksuite/cli/releases/tag/v1.0.7
[v1.0.6]: https://github.com/larksuite/cli/releases/tag/v1.0.6
[v1.0.5]: https://github.com/larksuite/cli/releases/tag/v1.0.5
[v1.0.4]: https://github.com/larksuite/cli/releases/tag/v1.0.4
[v1.0.3]: https://github.com/larksuite/cli/releases/tag/v1.0.3
[v1.0.2]: https://github.com/larksuite/cli/releases/tag/v1.0.2
[v1.0.1]: https://github.com/larksuite/cli/releases/tag/v1.0.1
[v1.0.0]: https://github.com/larksuite/cli/releases/tag/v1.0.0
