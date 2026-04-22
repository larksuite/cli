// internal/event/transport/transport_unix.go
//go:build !windows

package transport

import (
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs"
)

// dialTimeout bounds how long Dial will block waiting for the server
// side to accept. Without this, an accept-queue-full or otherwise
// wedged bus would hang the caller indefinitely. Matches the Windows
// winio.DialPipe timeout for symmetric behaviour across platforms.
const dialTimeout = 5 * time.Second

type unixTransport struct{}

// New returns a Unix socket transport.
func New() IPC {
	return &unixTransport{}
}

func (t *unixTransport) Listen(addr string) (net.Listener, error) {
	if err := vfs.MkdirAll(filepath.Dir(addr), 0700); err != nil {
		return nil, err
	}
	return net.Listen("unix", addr)
}

func (t *unixTransport) Dial(addr string) (net.Conn, error) {
	return net.DialTimeout("unix", addr, dialTimeout)
}

// Address returns the bus socket path under the project's config dir.
// Using core.GetConfigDir rather than os.UserHomeDir keeps the event
// subsystem consistent with the rest of the CLI (credentials, config,
// lockfile all go through the same helper) and honours the
// LARKSUITE_CLI_CONFIG_DIR env override so container/tmpdir-based tests
// get real isolation.
func (t *unixTransport) Address(appID string) string {
	return filepath.Join(core.GetConfigDir(), "events", sanitizeAppID(appID), "bus.sock")
}

// sanitizeAppID guards path construction against a malformed AppID
// (config corruption, manual edit) that could escape the events dir
// via "..", separators, or NUL. Strips them defensively — a corrupt
// AppID becoming a wedged bus is acceptable; silently `listen`ing in
// a parent directory is not.
func sanitizeAppID(appID string) string {
	if appID == "" {
		return "_"
	}
	repl := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		"\x00", "_",
		"..", "_",
	)
	out := repl.Replace(appID)
	if out == "" || out == "." {
		return "_"
	}
	return out
}

func (t *unixTransport) Cleanup(addr string) {
	_ = vfs.Remove(addr)
}
