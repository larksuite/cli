// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/vfs"
)

// hasTildePrefix reports whether s begins with `~` followed by end-of-string,
// `/`, or `\` — the form OpenClaw treats as home-relative.
func hasTildePrefix(s string) bool {
	if s == "" || s[0] != '~' {
		return false
	}
	if len(s) == 1 {
		return true
	}
	return s[1] == '/' || s[1] == '\\'
}

// joinTildeSuffix expands a tilde-prefixed string against a resolved home
// directory. Replaces only the leading `~` so the original separator
// (forward or back slash) and suffix bytes are kept verbatim, matching
// OpenClaw's `input.replace(/^~(?=$|[\\/])/, home)` semantics rather than
// going through filepath.Join (which would silently drop a literal `\` on
// POSIX). filepath.Clean is applied so `..` and duplicate separators are
// collapsed in the same way Node's path.resolve does on each platform.
//
// Caller must ensure hasTildePrefix(s) is true and home is non-empty.
func joinTildeSuffix(s, home string) string {
	if len(s) == 1 {
		return home
	}
	return filepath.Clean(home + s[1:])
}

// normalizeSentinel applies OpenClaw's normalize() helper to a single
// string: trims whitespace and treats the JS-flavoured literals
// "undefined" / "null" (along with empty/whitespace-only) as unset.
func normalizeSentinel(v string) string {
	v = strings.TrimSpace(v)
	if v == "undefined" || v == "null" {
		return ""
	}
	return v
}

// osHome returns the OS-level home directory by walking OpenClaw's
// resolution chain: HOME → USERPROFILE → OS user database (getpwuid on
// Unix / user32 on Windows, via os/user.Current). Each candidate is
// passed through normalizeSentinel so sentinel literals and blank
// strings fall through.
//
// Matches OpenClaw's resolveRawOsHomeDir env chain so the same tilde
// resolves against the same home under mixed shell environments and
// accidentally-stringified env values. Go's stdlib os.UserHomeDir on
// Unix only re-reads HOME and gives up; Node's os.homedir() still
// returns the account home via the user database, so the explicit
// user.Current() step is what keeps OpenClaw-authored `~/...` working
// in HOME-unset shells.
//
// Deliberate hybrid contract — neither a strict mirror of OpenClaw
// nor a strict reject-on-missing:
//
//   - OpenClaw's final fallback is cwd (via resolveRequiredHomeDir →
//     process.cwd()). We don't do that because requireAbsolutePath
//     exists precisely to reject cwd-dependent paths; routing
//     `~/secret` through cwd would defeat the audit invariant.
//
//   - We still go through user.Current() before giving up, even when
//     HOME is a sentinel literal ("undefined" / "null") and
//     USERPROFILE is unset. At that point OpenClaw would land on cwd,
//     and a strict implementation would reject; user.Current() lands
//     on the account home instead — cwd-independent and user-bound,
//     so it satisfies the audit's safety goal while still letting
//     ~/-authored configs resolve in a malformed-env shell.
//
//   - Only returns "" when the env chain AND user.Current() are all
//     unresolvable, at which point the caller surfaces a clean
//     "path must be absolute" error from the audit.
func osHome() string {
	if v := normalizeSentinel(os.Getenv("HOME")); v != "" {
		return v
	}
	if v := normalizeSentinel(os.Getenv("USERPROFILE")); v != "" {
		return v
	}
	if u, err := user.Current(); err == nil {
		return normalizeSentinel(u.HomeDir)
	}
	return ""
}

// explicitOpenClawHome reads OPENCLAW_HOME with OpenClaw's normalize()
// semantics applied.
func explicitOpenClawHome() string {
	return normalizeSentinel(os.Getenv("OPENCLAW_HOME"))
}

// absolutize returns p as an absolute path, resolving against the process
// cwd when p is relative. Returns "" when the cwd cannot be resolved.
// Wraps filepath.Abs semantics via vfs.Getwd because forbidigo bans
// filepath.Abs inside internal/ packages.
func absolutize(p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	wd, err := vfs.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(wd, p)
}

// openClawHome returns the home directory used to resolve `~`-relative paths
// authored against OpenClaw's config. Closely mirrors OpenClaw's
// home-resolution semantics so the same tilde resolves to the same
// absolute path here as inside OpenClaw runtime under all normal
// conditions.
//
// Resolution order:
//  1. OPENCLAW_HOME env var, when set (sentinel-normalised).
//  2. If OPENCLAW_HOME itself has a tilde prefix, expand it against the OS
//     home (see osHome); the result is empty when the OS home is
//     unresolvable.
//  3. Otherwise fall back to the OS home.
//
// The returned path is absolute (relative OPENCLAW_HOME values are
// absolutised against the process cwd, matching Node path.resolve in
// OpenClaw's pipeline).
//
// Returns "" when no home can be resolved. This is a deliberate
// divergence from OpenClaw, whose read pipeline would fall back to
// cwd via resolveRequiredHomeDir — see osHome for the rationale.
func openClawHome() string {
	raw := explicitOpenClawHome()
	switch {
	case raw == "":
		raw = osHome()
	case hasTildePrefix(raw):
		h := osHome()
		if h == "" {
			return ""
		}
		raw = joinTildeSuffix(raw, h)
	}
	return absolutize(raw)
}

// expandTildePath resolves a leading `~` or `~/...` prefix to OpenClaw's
// effective home directory (see openClawHome).
//
// Returns the input unchanged when it lacks a tilde prefix or when
// openClawHome cannot resolve a home directory. The latter case is a
// deliberate divergence from OpenClaw, whose read pipeline falls back
// to cwd — see osHome. Surfacing a "path must be absolute" error from
// the audit is preferable to silently routing a user-authored
// `~/secret` through cwd resolution.
//
// `~user` shell-style expansion is intentionally not supported (OpenClaw
// does not support it either).
func expandTildePath(p string) string {
	if !hasTildePrefix(p) {
		return p
	}
	home := openClawHome()
	if home == "" {
		return p
	}
	return joinTildeSuffix(p, home)
}
