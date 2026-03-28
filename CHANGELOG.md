# Changelog

All notable changes to this project will be documented in this file.

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

[v1.0.0]: https://github.com/larksuite/cli/releases/tag/v1.0.0
