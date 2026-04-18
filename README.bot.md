# lark-cli - Claude Code Bot

> **Bot Integration**: lark-cli integrates with Claude Code to provide an AI-powered assistant through Feishu/Lark messaging.

---

## Overview

The `lark-cli bot` command enables Claude Code Bot functionality within lark-cli. Users can chat with Claude Code directly from Feishu/Lark:

- **Natural conversation**: Chat with Claude Code in Feishu
- **Multi-turn sessions**: Context persists across messages
- **Slash commands**: Support for `/run`, `/status` and other shortcuts
- **Multi-user**: Each chat maintains its own session

---

## Commands

### `lark-cli bot` subcommands

```bash
# Start the Bot
lark-cli bot start [--config] [--daemon]

# View status
lark-cli bot status

# Stop the Bot
lark-cli bot stop
```

### Core Features

- **Session management**: Per-chat `session_id` persistence
- **Claude Code integration**: Uses `claude -p --resume` for conversations
- **Command routing**: Supports slash commands and natural language
- **Production-ready**: Supports pm2/systemd daemon mode
- **Configuration**: YAML config file support

---

## Quick Start

### 1. Prerequisites

```bash
# lark-cli (Go 1.23+)
go install github.com/larksuite/cli@latest

# Claude Code CLI
npm install -g @anthropic-ai/claude-code
```

### 2. Configure Feishu App

```bash
# Initialize lark-cli config
echo "YOUR_APP_SECRET" | lark-cli config init --app-id "cli_xxx" --app-secret-stdin
```

### 3. Start the Bot

```bash
# Basic start
lark-cli bot start

# With config file
lark-cli bot start --config ~/.lark-cli/bot-config.yaml

# Daemon mode
lark-cli bot start --daemon
```

### 4. Usage in Feishu

```
You: Write a Python function to calculate Fibonacci
Bot: [Claude Code generated code and explanation]

You: This function has a bug, help me fix it
Bot: [Claude Code analyzes and fixes the bug]

You: /run tests
Bot: [Executes tests and returns results]
```

---

## Configuration

```yaml
# ~/.lark-cli/bot-config.yaml
claude:
  work_dir: ~/projects              # Claude Code working directory
  system_prompt: "You are a helpful assistant"
  max_sessions: 100                # Max concurrent sessions
  session_ttl: 24h                # Session TTL

lark:
  app_id: cli_xxx                 # Feishu app ID
  app_secret: xxx                 # Feishu app secret

features:
  enable_commands: true            # Enable slash commands
  enable_file_ops: true           # Enable file operations
  allowed_users:                  # Allowed user list
    - ou_xxx
    - ou_yyy

logging:
  level: info
  format: json
  output: /var/log/lark-bot.log
```

---

## Architecture

```
Feishu user message
    ↓
lark-cli event +subscribe (WebSocket long-lived connection)
    ↓
bot/handler.go (message processor)
    ↓
bot/router.go (command routing)
    ↓
bot/claude.go (Claude Code integration)
    ↓
bot/session.go (session management)
    ↓
lark-cli im +messages-send (reply to Feishu)
```

### Core Modules

| Module | File | Purpose |
|--------|------|---------|
| **Command entry** | `cmd/bot/` | bot subcommand definitions |
| **Message handling** | `shortcuts/bot/handler.go` | Message event processing |
| **Session management** | `shortcuts/bot/session.go` | session_id persistence |
| **Claude integration** | `shortcuts/bot/claude.go` | Claude Code invocation |
| **Command routing** | `shortcuts/bot/router.go` | Slash command routing |
| **Event subscription** | `shortcuts/bot/subscribe.go` | WebSocket event subscriber |

---

## Documentation

- [Bot Integration Plan](docs/bot-integration-plan.md) - Technical design document
- [Bot Test Guide](cmd/bot/TEST.md) - Testing instructions
- [Architecture CODEMAP](docs/CODEMAPS/architecture.md) - System architecture
- [Backend CODEMAP](docs/CODEMAPS/backend.md) - Backend components
- [lark-cli README](README.md) - Main project documentation

---

## License

MIT License (same as larksuite/cli)

---

**Version**: 0.1.0-alpha (development)
**Last Updated**: 2026-04-10
