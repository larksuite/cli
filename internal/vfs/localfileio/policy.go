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

// fixedDenyRoots returns the deny roots that do not live under an account home
// directory, resolved once per process.
var fixedDenyRoots = sync.OnceValue(func() []policyEntry {
	return []policyEntry{
		newPolicyEntry("/etc", "/etc"),
		newPolicyEntry("/proc", "/proc"),
		newPolicyEntry("/sys", "/sys"),
		newPolicyEntry("/dev", "/dev"),
		newPolicyEntry("/root", "/root"),
		newPolicyEntry("/var/run", "/var/run"),
	}
})

// homeDenyGroups returns one group per candidate home directory. The CLI
// config directory is covered from every candidate at once, because deny roots
// only ever accumulate: adding an environment-derived location can protect a
// second copy of the credentials, and can never unprotect the real one.
var homeDenyGroups = sync.OnceValue(func() []*homeDenyGroup {
	var groups []*homeDenyGroup
	for _, home := range candidateHomes() {
		groups = append(groups, newHomeDenyGroup(home))
	}
	return groups
})

// candidateHomes returns every directory that could be this account's home,
// most authoritative first and without duplicates.
func candidateHomes() []string {
	var homes []string
	seen := map[string]bool{}
	add := func(home string) {
		if home == "" || seen[home] {
			return
		}
		seen[home] = true
		homes = append(homes, home)
	}
	if home, ok := passwdHome(); ok {
		add(home)
	}
	if home, err := trustedHome(); err == nil {
		add(home)
	}
	if home, err := vfs.UserHomeDir(); err == nil {
		add(home)
	}
	if u, err := user.Current(); err == nil {
		add(u.HomeDir)
	}
	return homes
}

// denyName is one home-relative deny root before resolution. Label and literal
// path are known without touching the filesystem, which is all the name pass
// needs — and the name pass is what settles every spelling that addresses a
// deny root outright.
type denyName struct {
	label   string
	rel     string // slash-separated, relative to the home directory
	literal string
}

// homeDenyGroup holds the deny roots under one candidate home directory.
//
// Resolving those roots is what stats ~/.ssh, ~/.aws and their neighbours, and
// on a machine whose security tooling guards those paths every such stat is an
// alarm (#2726). So the group resolves nothing up front: the home directory
// itself is resolved, and the roots beneath it only when this target could
// actually be inside one — see rootsToCheck.
type homeDenyGroup struct {
	home  policyEntry
	names []denyName

	// Classification is one pass over the listings; the two selections derived
	// from it resolve their roots on first use and keep them.
	kindsOnce  sync.Once
	kinds      map[string]denyNameKind
	linkedOnce sync.Once
	linked     []policyEntry
	fileOnce   sync.Once
	files      []policyEntry

	// Both caches are shared by concurrent validations — download fan-out runs
	// them in parallel — and each has its own lock so that selecting a root
	// (which lists directories) never waits on resolving one.
	resolvedMu sync.Mutex
	resolved   map[string]policyEntry

	listMu   sync.Mutex
	listings map[string]listing
}

// listing is one directory read, kept whether it succeeded or not. Caching the
// failure matters as much as caching the entries: a home directory that denies
// enumeration while still allowing access by name would otherwise be re-read
// for every name on every validation, and under access control each attempt is
// another denial to log.
type listing struct {
	entries []os.DirEntry
	err     error
}

func newHomeDenyGroup(home string) *homeDenyGroup {
	g := &homeDenyGroup{
		home:     newPolicyEntry("the home directory", home),
		resolved: map[string]policyEntry{},
		listings: map[string]listing{},
	}
	for _, rel := range homeDenyNames {
		g.names = append(g.names, denyName{
			label:   "~/" + rel,
			rel:     rel,
			literal: filepath.Join(home, filepath.FromSlash(rel)),
		})
	}
	g.names = append(g.names, denyName{
		label:   "the CLI config directory",
		rel:     ".lark-cli",
		literal: filepath.Join(home, ".lark-cli"),
	})
	return g
}

// rootsToCheck returns the roots of this group whose resolved form still has to
// be compared against the target, and resolves exactly those. A root reachable
// by name alone is not among them: matchHomeDenyNames has already ruled on it
// without reading anything.
//
// A root can only match by being the target's real location or an ancestor of
// it, and that bounds the work to three kinds of root:
//
//   - Roots that sit under the home directory as named. Such a root can only be
//     an ancestor of a target that is itself under this home, and then it has to
//     be the very first component below it — so the one name the target could be
//     inside is its own first component (nameRoots).
//   - Roots reached through a symlink, junction or other reparse point, which
//     can put them anywhere at all (linkedRoots).
//   - Roots that could be the target file itself under a second name, which is
//     possible only when the target carries more than one (hardLinkRoots).
//
// Everything else is skipped, and skipping it is the point: downloading into
// the working directory no longer stats ~/.ssh and its neighbours, which is
// what set off the security tooling in #2726.
//
// Two limits come with resolving late rather than up front, both of them the
// price of not stat-ing credential paths a target has nothing to do with:
//
//   - An alias that leaves no trace in a directory listing and that symlink
//     resolution cannot see through — a bind mount of a credential directory,
//     say — is not matched when the target is addressed through the alias.
//   - Identities are read when a root is resolved rather than pinned at the
//     first validation, so a credential directory renamed between two
//     validations of one process is matched under its new name only by that
//     name. Every ordinary invocation is a fresh process, which never had the
//     pinning either.
//
// In both cases the named path stays denied, and in the strict tier the
// allowlist still has to accept the alias independently.
func (g *homeDenyGroup) rootsToCheck(resolved, absLiteral string, chain []ancestor) []policyEntry {
	return slices.Concat(
		g.linkedRoots(),
		g.hardLinkRoots(chain),
		g.nameRoots(resolved, absLiteral, chain),
	)
}

// hardLinkRoots returns the credential files that could be the target itself
// under another name. A hard link has no target to resolve and no mark in a
// directory listing, so neither name containment nor the listing can see that
// "~/report.txt" and "~/.npmrc" are one file — only identity can, and identity
// needs those roots resolved. The link count decides that, and it is read from
// the stat the ancestor walk already did (Windows keeps the count behind a
// handle, so there it opens the caller's own file, never a credential one).
func (g *homeDenyGroup) hardLinkRoots(chain []ancestor) []policyEntry {
	if len(chain) == 0 {
		return nil
	}
	leaf := chain[0]
	if !leaf.info.Mode().IsRegular() || !hasExtraHardLinks(leaf.path, leaf.info) {
		return nil
	}
	g.fileOnce.Do(func() { g.files = g.rootsOfKind(denyNameFile) })
	return g.files
}

// nameRoots resolves the roots that share a first component with the target's
// own position under this home. Comparison is by directory-entry name, which
// folds like the filesystem does, so an alternate spelling of ".ssh" selects
// the ~/.ssh root and then loses to it on file identity.
func (g *homeDenyGroup) nameRoots(resolved, absLiteral string, chain []ancestor) []policyEntry {
	var roots []policyEntry
	for _, head := range g.targetHeads(resolved, absLiteral, chain) {
		for _, n := range g.names {
			if sameEntryName(firstSegment(n.rel), head) {
				roots = append(roots, g.resolveRoot(n))
			}
		}
	}
	return roots
}

// targetHeads returns the first path component below this home directory for
// every reading of the target that lands under it: the real location, the
// literal one a caller keeps verbatim, and the location reached when the home
// directory itself is spelled differently (matched by file identity).
func (g *homeDenyGroup) targetHeads(resolved, absLiteral string, chain []ancestor) []string {
	var heads []string
	seen := map[string]bool{}
	add := func(base, target string) {
		head, ok := headUnder(base, target)
		if !ok || seen[head] {
			return
		}
		seen[head] = true
		heads = append(heads, head)
	}
	add(g.home.resolved, resolved)
	add(g.home.literal, resolved)
	add(g.home.literal, absLiteral)
	for _, a := range chain {
		if g.home.info != nil && os.SameFile(a.info, g.home.info) {
			add(a.path, resolved)
		}
	}
	return heads
}

// linkedRoots resolves the roots whose real location is not where their name
// says — those reached through a symlink, a junction or another reparse point.
// Which ones those are is read from directory listings, which name the entries
// without opening any of them: listing ~ reveals whether ".ssh" is a link
// without ever touching ~/.ssh.
func (g *homeDenyGroup) linkedRoots() []policyEntry {
	g.linkedOnce.Do(func() { g.linked = g.rootsOfKind(denyNameLinked) })
	return g.linked
}

// rootsOfKind resolves every name the listings put in one class.
func (g *homeDenyGroup) rootsOfKind(want denyNameKind) []policyEntry {
	var roots []policyEntry
	for _, n := range g.names {
		if g.classifyAll()[n.rel] == want {
			roots = append(roots, g.resolveRoot(n))
		}
	}
	return roots
}

// classifyAll walks every deny name through the directory listings once per
// process. The listings are cached, failures included, so this never re-reads a
// directory however many validations follow.
func (g *homeDenyGroup) classifyAll() map[string]denyNameKind {
	g.kindsOnce.Do(func() {
		g.kinds = make(map[string]denyNameKind, len(g.names))
		for _, n := range g.names {
			g.kinds[n.rel] = g.classify(n.rel)
		}
	})
	return g.kinds
}

// resolveRoot resolves one deny root, once per process.
func (g *homeDenyGroup) resolveRoot(n denyName) policyEntry {
	g.resolvedMu.Lock()
	defer g.resolvedMu.Unlock()
	if e, ok := g.resolved[n.label]; ok {
		return e
	}
	e := newPolicyEntry(n.label, n.literal)
	g.resolved[n.label] = e
	return e
}

// denyNameKind is what directory listings can say about a deny name without
// opening it.
type denyNameKind int

const (
	denyNameMissing denyNameKind = iota // no such entry, so it contains nothing
	denyNameDir                         // a plain directory where its name says
	denyNameFile                        // a plain file where its name says
	denyNameLinked                      // reached through a link: can be anywhere
)

// classify walks rel from the home directory through directory listings alone.
// A directory it cannot list is reported as linked: an unreadable directory is
// a question the listing did not answer, and the fail-closed answer is to
// resolve the root and compare it properly.
func (g *homeDenyGroup) classify(rel string) denyNameKind {
	dir := g.home.resolved
	segments := strings.Split(rel, "/")
	for i, segment := range segments {
		entries, err := g.listing(dir)
		if err != nil {
			return denyNameLinked
		}
		entry, ok := findEntry(entries, segment)
		switch {
		case !ok:
			return denyNameMissing
		case entry.Type()&(os.ModeSymlink|os.ModeIrregular) != 0:
			return denyNameLinked
		case i == len(segments)-1:
			return finalKind(entry)
		case !entry.IsDir(): // a file cannot hold the rest of the path
			return denyNameMissing
		}
		dir = filepath.Join(dir, segment)
	}
	return denyNameMissing
}

func finalKind(entry os.DirEntry) denyNameKind {
	if entry.IsDir() {
		return denyNameDir
	}
	return denyNameFile
}

// listing reads dir once per process. Concurrent validations share the cache:
// download fan-out runs them in parallel.
//
// Reading the directory is what keeps the entries closed: Windows fills every
// attribute from the one directory query, and Unix from the dirent type. The
// exception is a filesystem that reports no dirent type (DT_UNKNOWN — some FUSE
// mounts, XFS made without ftype): os.ReadDir then lstats each entry itself, so
// on those the credential paths are stat'ed after all, as they were before this
// existed. It stays one listing per process either way.
func (g *homeDenyGroup) listing(dir string) ([]os.DirEntry, error) {
	g.listMu.Lock()
	defer g.listMu.Unlock()
	if cached, ok := g.listings[dir]; ok {
		return cached.entries, cached.err
	}
	entries, err := vfs.ReadDir(dir)
	g.listings[dir] = listing{entries: entries, err: err}
	return entries, err
}

// findEntry looks name up in a directory listing the way the filesystem itself
// would resolve it. Comparison is by Unicode simple case folding on the
// platforms whose default filesystems fold, which covers both ".SSH" on NTFS
// and APFS folding U+017F ("ſ") onto "s".
func findEntry(entries []os.DirEntry, name string) (os.DirEntry, bool) {
	for _, entry := range entries {
		if sameEntryName(entry.Name(), name) {
			return entry, true
		}
	}
	return nil, false
}

func sameEntryName(a, b string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// headUnder returns the first path component of target below base, reporting
// false when target is not inside base or is base itself. Containment is
// decided on the folded spellings, the same way isUnderDir decides it.
func headUnder(base, target string) (string, bool) {
	rel, err := filepath.Rel(foldCase(base), foldCase(target))
	if err != nil {
		return "", false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return firstSegment(filepath.ToSlash(rel)), true
}

// firstSegment returns the leading component of a slash-separated relative path.
func firstSegment(rel string) string {
	if i := strings.IndexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return rel
}

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
	// slices.Concat, not append: fixedDenyRoots() is a process-cached slice with
	// spare capacity, and appending to it would write the per-call entries into
	// the shared backing array — a data race between concurrent validations
	// (download fan-out does run them in parallel) and a cross-call overwrite.
	eager := slices.Concat(fixedDenyRoots(), configDirDenyRoots(cwd))
	if label, ok := matchRoots(resolved, absLiteral, eager); ok {
		return denyError(flagName, raw, label)
	}
	if label, ok := matchHomeDenyNames(resolved, absLiteral); ok {
		return denyError(flagName, raw, label)
	}
	// Name comparison alone cannot decide containment: case-insensitive and
	// case-folding filesystems (APFS folds U+017F to "s", so ".ſſh" opens
	// "~/.ssh"), Unicode normalization, and Windows short names all give the
	// same directory several spellings. Ask the kernel instead — file identity
	// has exactly one answer per directory.
	chain := ancestors(resolved)
	if label, ok := identityLabel(chain, eager); ok {
		return denyError(flagName, raw, label)
	}
	if label, ok := matchHomeDenyRoots(resolved, absLiteral, chain); ok {
		return denyError(flagName, raw, label)
	}
	return nil
}

// matchRoots reports the first root that contains the target in either of the
// two readings of the argument.
func matchRoots(resolved, absLiteral string, roots []policyEntry) (string, bool) {
	for _, e := range roots {
		if matchResolved(resolved, e) || isUnderDir(foldCase(absLiteral), foldCase(e.literal)) {
			return e.label, true
		}
	}
	return "", false
}

// matchHomeDenyNames applies the home-relative denylist by name only. This is
// the pass that answers "--file ~/.ssh/id_rsa", and it answers it without
// reading anything from disk.
func matchHomeDenyNames(resolved, absLiteral string) (string, bool) {
	for _, g := range homeDenyGroups() {
		for _, n := range g.names {
			if isUnderDir(foldCase(absLiteral), foldCase(n.literal)) ||
				isUnderDir(foldCase(resolved), foldCase(n.literal)) {
				return n.label, true
			}
		}
	}
	return "", false
}

// matchHomeDenyRoots settles the spellings a name cannot: each group decides
// how much of itself has to be resolved for this target (rootsToCheck) and the
// comparison then runs over that subset.
func matchHomeDenyRoots(resolved, absLiteral string, chain []ancestor) (string, bool) {
	for _, g := range homeDenyGroups() {
		roots := g.rootsToCheck(resolved, absLiteral, chain)
		if label, ok := matchRoots(resolved, absLiteral, roots); ok {
			return label, true
		}
		if label, ok := identityLabel(chain, roots); ok {
			return label, true
		}
	}
	return "", false
}

func denyError(flagName, raw, label string) error {
	return fmt.Errorf("%s %q is inside %s, which is protected by the built-in denylist", flagName, raw, label)
}

// ancestor is one existing directory on the way up from the target, kept with
// the path it was reached by so a match can be read back as a location.
type ancestor struct {
	path string
	info os.FileInfo
}

// matchByFileIdentity reports the first root that is the very same directory
// as one of the target's ancestors, compared by device and inode rather than
// by name.
func matchByFileIdentity(resolved string, roots []policyEntry) (string, bool) {
	return identityLabel(ancestors(resolved), roots)
}

// ancestors walks resolved upwards and returns every ancestor that exists,
// deepest first. Missing ancestors are skipped: a target that does not exist
// yet is decided by its nearest existing parent.
func ancestors(resolved string) []ancestor {
	var chain []ancestor
	p := resolved
	for {
		if fi, err := vfs.Lstat(p); err == nil {
			chain = append(chain, ancestor{path: p, info: fi})
		}
		parent := filepath.Dir(p)
		if parent == p {
			return chain
		}
		p = parent
	}
}

func identityLabel(chain []ancestor, roots []policyEntry) (string, bool) {
	for _, a := range chain {
		for _, e := range roots {
			if e.info != nil && os.SameFile(a.info, e.info) {
				return e.label, true
			}
		}
	}
	return "", false
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
