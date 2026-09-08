// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/larksuite/cli/internal/vfs"
)

// Built-in path access policy. Both lists are compiled into the binary; no
// flag and no config file names a root. Two environment inputs remain, and
// each is bounded: LARKSUITE_CLI_CONFIG_DIR only contributes a deny root, and
// $HOME decides where ~/files points on a system whose account database
// cannot name this uid's home (see trustedHome). Deny always wins over allow,
// the cwd root included.

// allowRootsLabel names the allowlist in rejection messages. The lists are
// public documentation, so the error message is allowed (and expected) to
// spell them out.
func allowRootsLabel() string {
	if runtime.GOOS == "windows" {
		return `the current working directory, the account's temp directory, or the "files" directory in the account's home`
	}
	return "the current working directory, /tmp, or ~/files"
}

// policyEntry is one list root in both its literal and realpath forms. Deny
// matching compares both forms so a root that is (or later becomes) a symlink
// cannot slip past a comparison done in only one namespace.
type policyEntry struct {
	label    string      // user-facing name for error messages, e.g. "~/.ssh"
	literal  string      // cleaned absolute literal form
	resolved string      // realpath form; equals literal when resolution fails
	info     os.FileInfo // nil when the root does not exist; used for identity comparison
}

func newPolicyEntry(label, path string) policyEntry {
	literal := filepath.Clean(path)
	resolved, err := resolveNearestAncestor(literal)
	if err != nil {
		resolved = literal
	}
	// A root that exists carries its FileInfo so containment can be settled by
	// file identity; one that does not is matched by name only, which is
	// sufficient because a directory that does not exist has no alternate
	// spellings to hide behind.
	info, err := vfs.Lstat(resolved)
	if err != nil {
		info = nil
	}
	return policyEntry{label: label, literal: literal, resolved: resolved, info: info}
}

// configDirDenyRoots returns the deny roots derived from
// LARKSUITE_CLI_CONFIG_DIR. They are computed per call, not cached with the
// fixed roots, because a relative value is resolved against the working
// directory — core.GetBaseConfigDir accepts relative values, so refusing to
// consider them here would leave that credential directory unprotected.
func configDirDenyRoots(cwd string) []policyEntry {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
		return []policyEntry{newPolicyEntry("the CLI config directory", dir)}
	}
	// With no override and no reachable home directory, core.GetBaseConfigDir
	// falls back to a bare ".lark-cli", which resolves inside the working
	// directory — an allow root. Mirroring that fallback here keeps the
	// credentials it holds out of reach in containers where the home lookup
	// fails.
	if home, err := vfs.UserHomeDir(); err != nil || home == "" {
		return []policyEntry{newPolicyEntry("the CLI config directory", filepath.Join(cwd, ".lark-cli"))}
	}
	return nil
}

// trustedHome returns the account's home directory from the most
// authoritative source available, preferring the account database over $HOME.
// Where the database names this uid, an invocation that controls the
// environment cannot move the ~/files allow root, nor make the real ~/.ssh,
// ~/.gnupg, ~/.aws and ~/.lark-cli deny roots disappear.
//
// Where it does not, that preference has nothing to prefer, so this is not a
// guarantee. Release binaries are built with CGO_ENABLED=0, and pure-Go
// os/user answers from $HOME for a uid the database does not list — distroless
// images and `--user 99999` containers among them — as long as $USER is set
// too; with $USER unset it returns an error instead and the ~/files root is
// dropped altogether. Such an invocation does choose where ~/files points.
// What that reaches is a directory named "files" under the path it names and
// nothing else: the home directory itself is not an allow root, and denyRoots
// covers every candidate home, so the credential directories stay protected
// either way.
var trustedHome = sync.OnceValues(func() (string, error) {
	if home, ok := passwdHome(); ok {
		return home, nil
	}
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	if u.HomeDir == "" {
		return "", fmt.Errorf("account has no home directory")
	}
	return u.HomeDir, nil
})

// passwdHome reads this process's home directory straight out of the account
// database, which no environment variable can influence. Reports false when
// the file is absent (Windows, distroless) or holds no entry for the uid
// (macOS keeps regular accounts in DirectoryService, not /etc/passwd).
func passwdHome() (string, bool) {
	if runtime.GOOS == "windows" {
		return "", false
	}
	f, err := vfs.Open("/etc/passwd")
	if err != nil {
		return "", false
	}
	defer f.Close()

	want := strconv.Itoa(os.Getuid())
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// name:passwd:uid:gid:gecos:home:shell
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 6 || fields[2] != want {
			continue
		}
		if fields[5] == "" {
			return "", false
		}
		return fields[5], true
	}
	return "", false
}

// allowRoots returns the built-in allowlist for cwd. Only the cwd entry is
// per-call; the fixed roots are resolved once per process (fixedAllowRoots),
// which both pins them against later filesystem changes and keeps batch
// commands from re-resolving every root for every file.
func allowRoots(cwd string) []policyEntry {
	return append([]policyEntry{newPolicyEntry("the current working directory", cwd)}, fixedAllowRoots()...)
}

var fixedAllowRoots = sync.OnceValue(func() []policyEntry {
	var roots []policyEntry
	if tmp := tmpRoot(); tmp != "" {
		roots = append(roots, newPolicyEntry(tmpRootLabel(), tmp))
	}
	// An untrusted home yields no ~/files root: failing closed here costs a
	// convenience root, while guessing from $HOME would hand out an
	// attacker-chosen one.
	//
	// Running as root is such a case by a different route: the home directory
	// is then /root, which is a deny root, and deny wins — so ~/files is
	// unavailable to root and only the working directory and /tmp remain. That
	// is a deliberate decision, not an oversight: keeping "deny always wins"
	// unconditional is worth more than a third allowed root in containers that
	// run as root, where /tmp already serves the same purpose. Carving out
	// /root/files would make the rule conditional; dropping /root from the
	// denylist would expose the rest of root's home.
	if home, err := trustedHome(); err == nil {
		roots = append(roots, newPolicyEntry("~/files", filepath.Join(home, "files")))
	}
	return roots
})

// tmpRoot is the temporary-directory allow root: literal /tmp on Unix-like
// systems (macOS resolves it to /private/tmp via realpath). Windows has no
// /tmp, and os.TempDir there consults TMP/TEMP, which the invocation may set —
// so the per-account temp directory is derived from the account record
// instead. An empty return drops the temp allow root altogether: falling back
// to os.TempDir would reinstate exactly the TMP/TEMP influence this avoids, so
// a home the policy cannot establish costs the root rather than loosening it.
//
// Known limit: an environment that redirects the account's temp directory
// elsewhere (folder redirection, a service account with %TEMP% pointed at
// C:\Windows\Temp) leaves those files outside the allowlist.
func tmpRoot() string {
	if runtime.GOOS != "windows" {
		return "/tmp"
	}
	if home, err := trustedHome(); err == nil {
		return filepath.Join(home, "AppData", "Local", "Temp")
	}
	return ""
}

func tmpRootLabel() string {
	if runtime.GOOS == "windows" {
		return "the account's temp directory"
	}
	return "/tmp"
}

// denyRoots returns the built-in denylist, resolved once per process. The CLI
// config directory is covered from every candidate home at once, because deny
// roots only ever accumulate:
// adding an environment-derived location can protect a second copy of the
// credentials, and can never unprotect the real one.
var denyRoots = sync.OnceValue(func() []policyEntry {
	entries := []policyEntry{
		newPolicyEntry("/etc", "/etc"),
		newPolicyEntry("/proc", "/proc"),
		newPolicyEntry("/sys", "/sys"),
		newPolicyEntry("/dev", "/dev"),
		newPolicyEntry("/root", "/root"),
		newPolicyEntry("/var/run", "/var/run"),
	}
	homes := map[string]bool{}
	if home, ok := passwdHome(); ok {
		homes[home] = true
	}
	if home, err := trustedHome(); err == nil {
		homes[home] = true
	}
	if home, err := vfs.UserHomeDir(); err == nil {
		homes[home] = true
	}
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		homes[u.HomeDir] = true
	}
	for home := range homes {
		for _, d := range homeDenyNames {
			entries = append(entries, newPolicyEntry("~/"+d, filepath.Join(home, d)))
		}
		entries = append(entries, newPolicyEntry("the CLI config directory", filepath.Join(home, ".lark-cli")))
	}
	return entries
})

// homeDenyNames lists what under the account home is refused. Entries are
// matched by containment, so naming a directory covers everything beneath it
// and naming a file covers that file — filepath.Rel reports "." for a path
// against itself, which isUnderDir accepts.
//
// The working directory is an allow root and running the CLI from the home
// directory is ordinary, so anything here that is not listed is readable by a
// relative path. That makes the list the whole of the protection, not a
// convenience: a credential store missing from it has none. Shell histories
// are included because they carry pasted keys and internal hostnames as
// reliably as a credential file does.
var homeDenyNames = []string{
	".ssh", ".gnupg", ".aws",
	".netrc", ".git-credentials", ".gitconfig",
	".kube", ".docker", ".azure", ".config/gh", ".config/gcloud",
	".npmrc", ".pypirc", ".gem/credentials", ".cargo/credentials", ".cargo/credentials.toml",
	".bash_history", ".zsh_history", ".sh_history", ".python_history", ".psql_history",
}

// checkDeny rejects paths under any built-in deny root. absLiteral is the
// cleaned pre-resolution form; matching it alongside the realpath form closes
// the gap where the filesystem changes between entry resolution and this
// check (a deny root swapped for a symlink still matches by literal).
// raw is echoed instead of the resolved location: the caller's own argument is
// what it can act on, and stderr is routinely collected by automation, where a
// path expanded to include the OS account name and working directory would be
// gratuitous exposure.
func checkDeny(flagName, raw, absLiteral, resolved, cwd string) error {
	// slices.Concat, not append: denyRoots() is a process-cached slice with
	// spare capacity, and appending to it would write the per-call entries into
	// the shared backing array — a data race between concurrent validations
	// (download fan-out does run them in parallel) and a cross-call overwrite.
	roots := slices.Concat(denyRoots(), configDirDenyRoots(cwd))
	for _, e := range roots {
		if matchResolved(resolved, e) || isUnderDir(foldCase(absLiteral), foldCase(e.literal)) {
			return denyError(flagName, raw, e.label)
		}
	}
	// Name comparison alone cannot decide containment: case-insensitive and
	// case-folding filesystems (APFS folds U+017F to "s", so ".ſſh" opens
	// "~/.ssh"), Unicode normalization, and Windows short names all give the
	// same directory several spellings. Ask the kernel instead — file identity
	// has exactly one answer per directory.
	if label, ok := matchByFileIdentity(resolved, roots); ok {
		return denyError(flagName, raw, label)
	}
	return nil
}

func denyError(flagName, raw, label string) error {
	return fmt.Errorf("%s %q is inside %s, which is protected by the built-in denylist", flagName, raw, label)
}

// matchByFileIdentity walks resolved upwards and reports the first root that
// is the very same directory as one of the ancestors, compared by device and
// inode rather than by name. Missing ancestors are skipped: a target that does
// not exist yet is decided by its nearest existing parent.
func matchByFileIdentity(resolved string, roots []policyEntry) (string, bool) {
	p := resolved
	for {
		if fi, err := vfs.Lstat(p); err == nil {
			for _, e := range roots {
				if e.info != nil && os.SameFile(fi, e.info) {
					return e.label, true
				}
			}
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", false
		}
		p = parent
	}
}

// checkAllow accepts paths under any built-in allow root; everything else is
// rejected with the full allowlist spelled out. Only the resolved form of the
// input participates: matching the pre-resolution literal would grant access
// to any symlink placed inside an allow root, no matter where it points.
func checkAllow(flagName, raw, resolved, cwd string) error {
	roots := allowRoots(cwd)
	for _, e := range roots {
		if matchResolved(resolved, e) {
			return nil
		}
	}
	// Identity matching also settles the permissive direction: when an
	// ancestor is the very same directory as an allow root, the target really
	// is inside it, whatever spelling reached it.
	if _, ok := matchByFileIdentity(resolved, roots); ok {
		return nil
	}
	return fmt.Errorf("%s %q is outside the built-in allowlist; allowed roots are %s "+
		"(hint: save under one of the allowed roots; flags that support stdin can read an out-of-tree file via '-')",
		flagName, raw, allowRootsLabel())
}

// matchResolved reports whether the fully resolved input path falls under the
// entry in either of the entry's namespaces. Comparing the input's resolved
// form against the entry literal is safe in both directions: it only matches
// when the real filesystem location truly is under that literal path.
func matchResolved(resolved string, e policyEntry) bool {
	return isUnderDir(foldCase(resolved), foldCase(e.resolved)) ||
		isUnderDir(foldCase(resolved), foldCase(e.literal))
}

// foldCase normalizes case on platforms whose default filesystems compare
// case-insensitively: NTFS on Windows and APFS/HFS+ on macOS, where
// ~/.SSH/id_rsa and ~/.ssh/id_rsa are the same file and a byte-for-byte
// comparison would walk straight past a deny root. Folding can over-match on
// the rarer case-sensitive volumes of those platforms, which errs toward
// refusing access rather than granting it.
func foldCase(p string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(p)
	}
	return p
}
