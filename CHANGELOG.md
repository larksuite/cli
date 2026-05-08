# Changelog

All notable changes to this project will be documented in this file.

## [v1.0.26] - 2026-05-08

### Features

- **im**: Add `message_app_link` to message outputs (#668)
- **auth**: Add scope hint for missing authorization errors (#776)

### Bug Fixes

- **base**: Clean error detail output (#783)
- **whiteboard**: Reclassify `+update` as `write` risk (#775)

### Documentation

- **mail**: Add data integrity and write-confirmation rules (#749)

## [v1.0.25] - 2026-05-07

### Features

- Add skills version drift notice and unify update flow (#723)

### Bug Fixes

- Remove misleading default value from `--as` flag help text (#769)
- Handle negative truncate lengths (#744)
- Reject invalid JSON pointer escapes (#741)
- Migrate task shortcut errors to structured `output.Errorf`/`ErrValidation` (#740)

### Documentation

- Clarify base `user_open_id` guidance (#763)

## [v1.0.24] - 2026-05-06

### Features

- **sheets**: Add sheet management shortcuts (#722)
- **base**: Support batch record get and delete (#630)
- **task**: Add upload task attachment shortcut (#736)
- **drive**: Pre-flight 10000-rune total cap for `+add-comment` `reply_elements` (#605)

### Bug Fixes

- **auth**: Handle missing scopes and device flow improvements (#752)
- Add url to markdown `+create` output (#753)

### Documentation

- Refine field update conversion guidance (#748)

## [v1.0.23] - 2026-04-30

### Features

- **drive**: Add `+pull` shortcut for one-way Drive → local mirror (#696)
- **drive**: Add `+push` shortcut for one-way local → Drive mirror (#709)
- **drive**: Add `+status` shortcut for content-hash diff (#692)
- **drive**: Support `--file-name` for drive export (#685)
- **base**: Add markdown output for record reads (#726)
- **minutes**: Add media upload shortcut (#725)
- **doc**: Warn when callout uses `type=` without `background-color` (#467)
- **cmdutil**: Support `@file` for params and data (#724)
- Add markdown shortcuts and skill docs (#704)

### Documentation

- **doc**: Guide lark-doc v2 usage (#710)
- **minutes**: Clarify minutes file-to-notes routing (#732)

## [v1.0.22] - 2026-04-29

### Features

- **task**: Add resource agent & `agent_task_step_info` (#693)
- **task**: Support app task members by id (#712)
- **contact**: Add `--queries` multi-name fanout to `+search-user` (#707)
- **slides**: Add slide templates with template-first skill guidance (#684)
- **mail**: Support calendar events in emails (#646)
- **install**: Honor `npm_config_registry` for binary URL resolution with npmmirror fallback (#690)

### Bug Fixes

- **install**: Make Windows zip extraction resilient (#713)
- **config/init**: Respect `--brand` flag in `--new` mode (#711)

### Documentation

- **base**: Clarify base search routing (#708)
- **base**: Align base skills and view config contracts (#653)

## [v1.0.21] - 2026-04-28

### Features

- **contact**: Add search filters and richer profile fields to `+search-user` (#648)
- **common**: Backfill resource URL when create APIs omit it (#680)
- **risk**: Add risk tiering for command sensitivity classification (#633)
- **okr**: Add progress records support (#574)
- **calendar**: Enhance event search and meeting room finding (#679)
- **event**: Add event subscription & consume system (#654)
- **drive**: Extend `+add-comment` to support slides targets (#674)
- **slides**: Add font management for slides (#681)

### Bug Fixes

- **cmdutil**: Default flag completions to disabled (#688)
- **e2e/wiki**: Pass `obj_type` when deleting wiki nodes in cleanup (#687)
- **readme**: Fix readme statistics (#691)

## [v1.0.20] - 2026-04-27

### Features

- **drive**: Add `+search` shortcut with flat filter flags (#658)
- **mail**: Support sharing emails to IM chats via `+share-to-chat` (#637)
- **calendar**: Add `+update` shortcut (#678)
- **im**: Add `--at-chatter-ids` filter to `+messages-search` (#612)
- **pagination**: Preserve pagination state on truncation and natural end (#659)
- **lark-im**: Add `chat.members.bots` to skill docs (#616)

### Bug Fixes

- **strict-mode**: Reject explicit `--as` instead of silently overriding it (#673)
- **whiteboard**: Manual disable edge case for svg compatibility (#661)

### Documentation

- **lark-drive**: Add missing import command examples (#669)
- **readme**: Add Project (Meegle) to Features table (#660)

## [v1.0.19] - 2026-04-24

### Features

- **mail**: Add read receipt support — `--request-receipt` on compose, `+send-receipt` / `+decline-receipt` for response
- **doc**: Add v2 API for `docs +create` / `+fetch` / `+update` (#638)
- **im**: Request thread roots for chat message list (#635)
- **drive**: Support wiki node targets in `+upload` (#611)
- **config**: Block `auth` / `config` when external credential provider is active (#627)
- **whiteboard**: Pin `whiteboard-cli` to `v0.2.10` in `lark-whiteboard` skill (#649)

## [v1.0.18] - 2026-04-23

### Features

- **base**: Support `.base` import and export for bitable (#599)
- **config**: Add `config bind` for per-Agent credential isolation (#515)
- **slides**: Add `+replace-slide` shortcut for block-level XML edits (#516)
- **wiki**: Add `+delete-space` shortcut with async task polling (#610)
- **doc**: Add `--from-clipboard` flag to `docs +media-insert` (#508)
- **minutes**: Unify minute artifacts output to `./minutes/{minute_token}/` (#604)
- Add configurable content-safety scanning (#606)
- **install**: Add SHA-256 checksum verification to `install.js` (#592)
- **whiteboard**: Pin `whiteboard-cli` to `^0.2.9` (#617)

### Bug Fixes

- **drive**: Escape angle brackets in comment text (#632)
- **im**: Unify `messages-search` pagination int flags (#446)
- **im**: Fix markdown URL rendering issues in post content (#206)

### Documentation

- **base**: Refine record cell value guidance (#636)

## [v1.0.17] - 2026-04-22

### Features

- **im**: Use `Content-Disposition` filename when downloading message resources (#536)
- **drive**: Add `+apply-permission` to request doc access (#588)
- Support record share link (#466)
- **whiteboard**: Add image support to `whiteboard-cli` skill (#553)
- **cmdutil**: Add `X-Cli-Build` header for CLI build classification (#596)

### Bug Fixes

- **base**: Add default-table follow-up hint to `base-create` (#600)
- Skip flag-completion registration outside completion path (#598)
- Add `record-share-link-create` in `SKILL.md` (#597)
- **mail**: Remove leftover conflict marker in skill docs (#594)

### Documentation

- **drive**: Clarify that comment listing defaults to unresolved comments only (#609)
- **doc**: Fix `--markdown` examples that teach literal `\n` (#602)
- **mail**: Remove `get_signatures` from skill reference, exposed via `+signature` instead (#545)

## [v1.0.16] - 2026-04-21

### Features

- **mail**: Support large email attachments (#537)
- **mail**: Add draft preview URL to draft operations (#438)
- **doc**: Add pre-write semantic warnings to `docs +update` (#569)
- **doc**: Add `--selection-with-ellipsis` position flag to `+media-insert` (#335)
- **calendar**: Support event share link and error details (#583)

### Bug Fixes

- **doc**: Preserve round-trip formatting in `+fetch` output (#469)
- **docs**: Validate `--selection-by-title` format early (#256)
- **whiteboard**: Register `+media-upload` shortcut and add whiteboard parent type

### Refactor

- Split `Execute` into `Build` + `Execute` with explicit IO and keychain injection (#371)
- **auth**: Simplify scope reporting in login flow (#582)

## [v1.0.15] - 2026-04-20

### Features

- **sheets**: Add float image shortcuts (#494)
- **approval**: Document `remind` and `initiated` methods in skill (#554)

### Bug Fixes

- **base**: Preserve attachment metadata on base uploads (#563)
- **base**: Fix role view and record default permission on edit (#530)
- **sheets**: Normalize single-cell range in `+set-style` and `+batch-set-style` (#548)
- **im**: Cap `basic_batch` user_ids at 10 per API limit (#551)
- **install**: Refine install wizard messages (#529)
- **whiteboard**: Deprecate old `lark-whiteboard-cli` skill (#547)

## [v1.0.14] - 2026-04-17

### Features

- **mail**: Add email priority support for compose and read (#538)
- **mail**: Support scheduled send (#534)
- **drive**: Support sheet cell comments in `+add-comment` (#518)
- **doc**: Add `--file-view` flag to `+media-insert` (#419)
- **base**: Auto grant current user for bot create and copy (#497)
- **base**: Add identity priority strategy and error handling (#505)
- **auth**: Improve login scope handling and messages (#523)
- Add OKR business domain (#522)

### Documentation

- **wiki**: Improve wiki skill docs and add wiki domain template (#512)
- **task**: Document `custom_fields` and `custom_field_options` API resources and permissions (#524)

### Refactor

- **skills**: Introduce `lark-doc-whiteboard.md` and streamline whiteboard workflow (#502)

## [v1.0.13] - 2026-04-16

### Features

- **im**: Support user access token for file, image, audio, and video upload, aligning upload and send identity with `--as` flag (#474)
- **drive**: Add `drive +create-folder` shortcut with root-folder fallback and bot-mode auto-grant (#470)
- **wiki**: Add bot-mode auto-grant support to `wiki +node-create` (#470)
- **doc**: Default `skip_task_detail` in `docs +fetch` to reduce unnecessary task detail expansion (#471)

### Bug Fixes

- **im**: Preserve original URL filename for uploaded file messages instead of generic `media.ext` names (#514)
- **whiteboard**: Use atomic overwrite API parameter for `whiteboard +update`, replacing read-then-delete approach (#483)

### Documentation

- **base**: Unify record batch write limit to 200 and enforce serial writes for continuous operations (#499)
- **base**: Remove redundant reference documentation and command grouping chapters from SKILL.md (#500)

### CI

- Consolidate workflows into layered CI pyramid with single `results` gate (#510)

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

[v1.0.26]: https://github.com/larksuite/cli/releases/tag/v1.0.26
[v1.0.25]: https://github.com/larksuite/cli/releases/tag/v1.0.25
[v1.0.24]: https://github.com/larksuite/cli/releases/tag/v1.0.24
[v1.0.23]: https://github.com/larksuite/cli/releases/tag/v1.0.23
[v1.0.22]: https://github.com/larksuite/cli/releases/tag/v1.0.22
[v1.0.21]: https://github.com/larksuite/cli/releases/tag/v1.0.21
[v1.0.20]: https://github.com/larksuite/cli/releases/tag/v1.0.20
[v1.0.19]: https://github.com/larksuite/cli/releases/tag/v1.0.19
[v1.0.18]: https://github.com/larksuite/cli/releases/tag/v1.0.18
[v1.0.17]: https://github.com/larksuite/cli/releases/tag/v1.0.17
[v1.0.16]: https://github.com/larksuite/cli/releases/tag/v1.0.16
[v1.0.15]: https://github.com/larksuite/cli/releases/tag/v1.0.15
[v1.0.14]: https://github.com/larksuite/cli/releases/tag/v1.0.14
[v1.0.13]: https://github.com/larksuite/cli/releases/tag/v1.0.13
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
