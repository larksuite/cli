# Dependencies

<!-- Generated: 2026-04-10 | Files scanned: 520 | Token estimate: ~600 -->

## Go Module Dependencies

### Core Dependencies

| Package | Version | Purpose | Critical |
|---------|---------|---------|----------|
| `github.com/larksuite/oapi-sdk-go/v3` | v3.5.3 | Lark/Feishu SDK | ✅ Yes |
| `github.com/spf13/cobra` | v1.10.2 | CLI framework | ✅ Yes |
| `github.com/spf13/pflag` | v1.0.9 | CLI flags | ✅ Yes |
| `github.com/gorilla/websocket` | v1.5.0 | WebSocket (event sub) | ✅ Yes |
| `github.com/zalando/go-keyring` | v0.2.8 | OS keychain (tokens) | ✅ Yes |
| `github.com/tidwall/gjson` | v1.18.0 | JSON parsing | ✅ Yes |
| `github.com/itchyny/gojq` | v0.12.17 | JQ-like JSON query | Yes |

### CLI/UX Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/charmbracelet/huh` | v1.0.0 | Spinner, progress |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | Styling, colors |
| `github.com/charmbracelet/bubbletea` | v1.3.6 | TUI framework |
| `github.com/atotto/clipboard` | v0.1.4 | Clipboard operations |
| `github.com/skip2/go-qrcode` | - | QR code generation |

### Utility Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/google/uuid` | v1.6.0 | UUID generation |
| `github.com/gofrs/flock` | v0.8.1 | File locking |
| `golang.org/x/net` | v0.33.0 | Network utilities |
| `golang.org/x/sys` | v0.33.0 | System calls |
| `golang.org/x/term` | v0.27.0 | Terminal handling |
| `golang.org/x/text` | v0.23.0 | Text encoding |

### Testing Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/stretchr/testify` | v1.11.1 | Test assertions |
| `github.com/smartystreets/goconvey` | v1.8.1 | BDD testing |

---

## External Service Dependencies

### Feishu/Lark Open Platform

| API | Purpose | Auth Required |
|-----|---------|----------------|
| `/open-apis/auth/v3/*` | OAuth, token refresh | ✅ Yes |
| `/open-apis/im/v1/*` | Messaging, chat | ✅ Yes |
| `/open-apis/calendar/v4/*` | Calendar, events | ✅ Yes |
| `/open-apis/doc/v1/*` | Documents | ✅ Yes |
| `/open-apis/sheets/v3/*` | Spreadsheets | ✅ Yes |
| `/open-apis/bitable/v1/*` | Base (multidimensional tables) | ✅ Yes |
| `/open-apis/mail/v1/*` | Mail | ✅ Yes |
| `/open-apis/task/v1/*` | Tasks | ✅ Yes |
| `/open-apis/vc/v1/*` | Video meetings | ✅ Yes |
| `/open-apis/whiteboard/v1/*` | Whiteboards | ✅ Yes |
| `/open-apis/wiki/v2/*` | Wiki | ✅ Yes |
| `/open-apis/contact/v3/*` | Contacts, users | ✅ Yes |
| `/open-apis/minutes/v1/*` | Meeting minutes | ✅ Yes |
| `/open-apis/market/*` | App management | ✅ Yes |

### Event Subscription (WebSocket)

| Endpoint | Purpose | Protocol |
|----------|---------|----------|
| `wss://open.feishu.cn/open-apis/event/v1/` | Event stream | WebSocket |
| `wss://open.larksuite.com/open-apis/event/v1/` | Event stream (Lark) | WebSocket |

**Event Types**:
- `im.message.receive_v1` - New message
- `im.message.message_read_v1` - Message read
- `im.chat.member.bot.added_v1` - Bot added to chat
- `calendar.calendar.event.changed_v4` - Calendar event changed
- ... (19 common event types)

---

## Development Tools

### Build Tools
- **Go** 1.23+ - Compiler
- **make** - Build automation (Makefile)
- **go.mod** - Dependency management

### Package Management
- **npm** - For AI Agent Skills distribution
- **npx** - For skill installation (`npx skills add`)

### Code Quality
- **gofmt** - Code formatting
- **go vet** - Static analysis
- **golint** - Linting (optional)

---

## Platform-Specific Dependencies

### macOS
- **Keychain**: OS-native credential storage
- **Framework**: Cocoa (via go-keyring)

### Windows
- **Windows Credential Manager**: wincred (via go-keyring)
- **Framework**: Win32 API

### Linux
- **Secret Service API**: DBus (via go-keyring)
- **Framework**: libsecret

---

## Optional Dependencies

### Claude Code Integration (Bot Feature)

| Tool | Purpose | Installation |
|------|---------|--------------|
| **claude** | Claude Code CLI | `npm install -g @anthropic-ai/claude-code` |
| **jq** | JSON parsing (alternative) | `brew install jq` |

### Deployment

| Tool | Purpose | Usage |
|------|---------|-------|
| **pm2** | Process manager (daemon mode) | `npm install -g pm2` |
| **systemd** | Linux service manager | Systemd unit files |
| **Docker** | Containerization | Dockerfile (TODO) |

---

## Dependency Updates

### Update Strategy
- **Lark SDK**: Track latest stable (v3.x)
- **Cobra**: Track stable (v1.x)
- **Go stdlib**: Track supported version (1.23+)

### Update Commands
```bash
# Update all dependencies
go get -u ./...

# Update specific dependency
go get -u github.com/larksuite/oapi-sdk-go/v3

# Tidy dependencies
go mod tidy
```

---

## Security Considerations

### Vulnerability Scanning
- Run `go list -json -m all` | npx snyk
- Check GitHub Security Advisories
- Monitor CVE databases

### Supply Chain Security
- Verify checksums for dependencies (go.sum)
- Use minimal dependency set
- Regular security audits

---

## Transitive Dependencies

### Notable Transitive Dependencies

| Package | Purpose | Why It Matters |
|---------|---------|----------------|
| `github.com/gorilla/websocket` | WebSocket | Event subscription |
| `github.com/gogo/protobuf` | Protobuf | SDK serialization |
| `github.com/mattn/go-isatty` | Terminal detection | Color output |
| `github.com/godbus/dbus/v5` | DBus | Linux keychain |

### Transitive Dependency Count
- **Direct**: 15 packages
- **Indirect**: ~80 packages
- **Total**: ~95 packages

---

## Version Constraints

### Minimum Versions
```
go 1.23.0
```

### Compatible Versions
| Dependency | Min Version | Max Version |
|------------|-------------|--------------|
| Go | 1.23.0 | - (track latest) |
| Lark SDK | 3.5.x | 3.x |
| Cobra | 1.10.x | 1.x |

---

## Dependency Health

### Maintenance Status
| Dependency | Last Updated | Status |
|------------|--------------|--------|
| Lark SDK | 2026-03 | ✅ Active |
| Cobra | 2025-11 | ✅ Active |
| go-keyring | 2023-07 | ✅ Stable |
| gjson | 2023-11 | ✅ Stable |

### Known Issues
- **None** - All dependencies actively maintained

---

## Alternatives Considered

### CLI Framework
- **Chosen**: Cobra
- **Alternatives**: urfave/cli, kingpin
- **Rationale**: Cobra most mature, best ecosystem

### Keyring
- **Chosen**: go-keyring
- **Alternatives**: keyring (Python), OS-specific solutions
- **Rationale**: Cross-platform, OS-native storage

### JSON Query
- **Chosen**: gojq + gjson
- **Alternatives**: jsonparser, tailing
- **Rationale**: gojq for JQ compatibility, gjson for speed

---

## Future Dependencies (Planned)

### Bot Feature
| Package | Purpose | Status |
|---------|---------|--------|
| **YAML parser** | Config file parsing | ⏳ Planned |
| **Process manager** | Daemon mode | ⏳ Planned (pm2/systemd) |

### Monitoring (Optional)
| Package | Purpose | Status |
|---------|---------|--------|
| **Prometheus client** | Metrics exposure | 🔮 Future |
| **OpenTelemetry** | Distributed tracing | 🔮 Future |
