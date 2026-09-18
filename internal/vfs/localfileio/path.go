// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/larksuite/cli/internal/charcheck"
	"github.com/larksuite/cli/internal/vfs"
)

// SafeOutputPath validates a download/export target path for --output flags.
func SafeOutputPath(path string) (string, error) {
	return safePath(path, "--output")
}

// rejectMultiplyLinkedTarget refuses to approve writing over an existing file
// that carries more than one name. A hard link has no target to resolve, so
// name-based containment cannot see that the same inode is also reachable from
// outside the allowlist; a caller that truncates the approved path in place
// would rewrite that outside file's contents. Writers that commit through a
// temp file and rename are immune, but the check belongs here so it also covers
// the ones that write directly.
// A target that cannot be inspected, or that is not a regular file, carries no
// link count to judge: the write layer reports the real failure with proper
// typing, and a directory legitimately holds several names.
func rejectMultiplyLinkedTarget(flagName, raw, resolved string) error {
	info, err := vfs.Lstat(resolved)
	if err == nil && info.Mode().IsRegular() && hasExtraHardLinks(resolved, info) {
		return fmt.Errorf("%s %q has multiple hard links, so writing it would also rewrite the other names "+
			"(hint: remove the file first, or write to a new name)", flagName, raw)
	}
	return nil
}

// SafeInputPath validates an upload/read source path for --file flags.
// The baseline invariant asserted by drive sync, upload flags, and the CI
// quality gates is "reject everything outside the built-in allowlist"
// (cwd, /tmp, ~/files) — see safePath. Out-of-tree content still reaches
// flags via stdin ("-").
func SafeInputPath(path string) (string, error) {
	return safePath(path, "--file")
}

// LocalInputPath validates an input path in the process local filesystem
// namespace. It intentionally does not impose allowlist containment or
// canonicalize the returned path: absolute paths, parent-relative paths, and
// symlink traversal retain their normal OS semantics (the grandfathered
// apps-upload exception, see #2005). The built-in denylist still applies:
// even the relaxed tier may not reach protected directories. Character
// validation remains mandatory because paths are user-controlled and may
// appear in errors or progress output.
func LocalInputPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("local input path must not be empty")
	}
	if strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("local input path must not contain control characters")
	}
	if err := charcheck.RejectControlChars(path, "local input path"); err != nil {
		return "", err
	}
	if err := validateLocalInputPlatform(path); err != nil {
		return "", err
	}
	if err := denyCheckLocalInput(path); err != nil {
		return "", err
	}
	return path, nil
}

// denyCheckLocalInput applies the built-in denylist to the relaxed local
// input tier. Resolution is fail-closed like safePath, but the allowlist is
// deliberately not consulted here. This tier hands the path back verbatim, so
// every interpretation of it is checked — the caller opens the one the OS
// picks, which for "~/..." is a literal "~" entry in the working directory.
func denyCheckLocalInput(path string) error {
	cwd, err := vfs.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}
	interps, err := interpretations(path, cwd)
	if err != nil {
		return err
	}
	for _, abs := range interps {
		resolved, err := resolveReal(abs)
		if err != nil {
			return err
		}
		if err := checkDeny("local input path", path, abs, resolved, cwd); err != nil {
			return err
		}
	}
	return nil
}

func isWindowsNonLocalNamespace(path string) bool {
	normalized := strings.ReplaceAll(path, "/", `\`)
	return strings.HasPrefix(normalized, `\\`) || strings.HasPrefix(normalized, `\??\`)
}

// SafeLocalFlagPath validates a flag value as a local file path.
// Empty values and http/https URLs are returned unchanged without validation.
func SafeLocalFlagPath(flagName, value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	if _, err := SafeInputPath(value); err != nil {
		return "", fmt.Errorf("%s: %w", flagName, err)
	}
	return value, nil
}

// SafeEnvDirPath validates an environment-provided application directory path.
// It requires an absolute path, rejects control characters, normalizes the
// input, and resolves symlinks through the nearest existing ancestor.
func SafeEnvDirPath(path, envName string) (string, error) {
	if err := charcheck.RejectControlChars(path, envName); err != nil {
		return "", err
	}

	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", envName, path)
	}

	resolved, err := resolveNearestAncestor(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve symlinks: %w", err)
	}
	return resolved, nil
}

// safePath is the shared implementation for SafeOutputPath and SafeInputPath.
// A path is accepted when its real location falls inside the built-in
// allowlist (cwd, /tmp, ~/files) and outside the built-in denylist; deny wins
// over allow, cwd included. Both lists are compiled in (policy.go), which
// also documents the two bounded environment inputs that remain.
func safePath(raw, flagName string) (string, error) {
	isOutputFlag := flagName == "--output"
	if err := charcheck.RejectControlChars(raw, flagName); err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%s must not be empty", flagName)
	}
	if err := validatePathPlatform(raw); err != nil {
		return "", fmt.Errorf("%s: %w", flagName, err)
	}
	if err := rejectForeignAbsolute(raw, flagName); err != nil {
		return "", err
	}

	cwd, err := vfs.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	// Every reading of the argument must pass, not just the one this function
	// returns: callers that keep the original string (SafeLocalFlagPath does)
	// open the location the OS computes, and for a "~/..." argument that is a
	// literal "~" entry in the working directory rather than the home
	// directory. A path is only safe when both readings are.
	interps, err := interpretations(raw, cwd)
	if err != nil {
		return "", fmt.Errorf("%s: %w", flagName, err)
	}
	var primary string
	for i, abs := range interps {
		resolved, err := resolveReal(abs)
		if err != nil {
			return "", fmt.Errorf("%s: %w", flagName, err)
		}
		if err := checkDeny(flagName, raw, abs, resolved, cwd); err != nil {
			return "", err
		}
		if err := checkAllow(flagName, raw, resolved, cwd); err != nil {
			return "", err
		}
		if err := checkRelativeStaysInCwd(flagName, raw, resolved, cwd); err != nil {
			return "", err
		}
		if isOutputFlag {
			if err := rejectMultiplyLinkedTarget(flagName, raw, resolved); err != nil {
				return "", err
			}
		}
		if i == 0 {
			primary = resolved
		}
	}
	return primary, nil
}

// checkRelativeStaysInCwd holds a relative path to the working directory even
// when a wider allow root would accept where it lands. Naming a full path is a
// deliberate act and the allowlist is the right judge of it; climbing out with
// ".." is not, and the two cannot share one verdict once an allow root is big
// enough to contain the working directory. A process running under /tmp — CI
// runners, containers and agent sandboxes commonly do — would otherwise reach
// a sibling session's files with "../", which the allowlist alone reads as
// still inside /tmp.
func checkRelativeStaysInCwd(flagName, raw, resolved, cwd string) error {
	if filepath.IsAbs(raw) || raw == "~" || strings.HasPrefix(raw, "~/") {
		return nil
	}
	if matchResolved(resolved, newPolicyEntry("the current working directory", cwd)) {
		return nil
	}
	return fmt.Errorf("%s %q resolves outside the current working directory "+
		"(hint: a relative path has to stay inside it; give a full path to reach another allowed root)",
		flagName, raw)
}

// interpretations returns every location this platform could open the argument
// at, most-intended first. They differ only for a leading "~": the first entry
// expands it to the home directory (what a caller using the returned path
// gets), the second keeps it literal (what the OS does with the original
// string).
func interpretations(raw, cwd string) ([]string, error) {
	abs, err := absolutize(raw, cwd)
	if err != nil {
		return nil, err
	}
	out := []string{abs}
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		literal := filepath.Clean(filepath.Join(cwd, raw))
		if literal != abs {
			out = append(out, literal)
		}
	}
	return out, nil
}

// absolutize expands a leading ~/ to the user home directory and roots
// relative paths at cwd, exactly as this platform's own path rules would. The
// result is Cleaned but not symlink-resolved.
//
// Joining is decided by filepath.IsAbs alone, so the location computed here is
// the location the OS will actually open. A shape that only looks absolute
// under foreign rules (`C:\x` or `\x` on Unix) is a relative path here, and
// resolving it as such is what lets the denylist see through it — a cwd-local
// symlink named `C:` would otherwise carry it anywhere.
//
// raw is used as given: trailing and leading spaces are legal in filenames, so
// trimming them here would silently address a different file than the caller
// named (emptiness is screened separately, before this call).
func absolutize(raw, cwd string) (string, error) {
	p := raw
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := trustedHome()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p), nil
}

// rejectForeignAbsolute refuses a path that is absolute only under another
// platform's rules. The strict tier rejects the shape outright rather than
// silently treating it as a relative name, which is a confusing way to grant
// access; the relaxed tier keeps such paths verbatim by contract and instead
// gets the denylist applied to the location the OS would really open.
func rejectForeignAbsolute(raw, flagName string) error {
	if isAbsolutePath(raw) && !filepath.IsAbs(raw) {
		return fmt.Errorf("%s %q is not a valid path on this platform", flagName, raw)
	}
	return nil
}

// resolveReal canonicalizes abs fail-closed: an existing target resolves
// through EvalSymlinks; a missing target (output files that do not exist yet)
// resolves through the nearest existing ancestor; any other Lstat failure is
// an error — the policy never guesses when the filesystem cannot be
// inspected. ENOTDIR counts as missing: a component of the path is a regular
// file, so the target cannot exist and the write layer will surface the real
// error with proper typing.
func resolveReal(abs string) (string, error) {
	_, lerr := vfs.Lstat(abs)
	switch {
	case lerr == nil:
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("cannot resolve symlinks: %w", err)
		}
		return resolved, nil
	case os.IsNotExist(lerr) || errors.Is(lerr, syscall.ENOTDIR):
		resolved, err := resolveNearestAncestor(abs)
		if err != nil {
			return "", fmt.Errorf("cannot resolve symlinks: %w", err)
		}
		return resolved, nil
	default:
		return "", fmt.Errorf("cannot inspect path: %w", lerr)
	}
}

func resolveNearestAncestor(path string) (string, error) {
	var tail []string
	cur := path
	for {
		if _, err := vfs.Lstat(cur); err == nil {
			real, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			parts := append([]string{real}, tail...)
			return filepath.Join(parts...), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			parts := append([]string{cur}, tail...)
			return filepath.Join(parts...), nil
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

// isAbsolutePath reports whether the path is rooted under this platform's
// rules or under another's. It does not trim: " /etc/passwd" is a relative
// path whose first component is a space, and calling it absolute would make
// this disagree with filepath.IsAbs — the disagreement itself was a bypass,
// since a path judged absolute here but relative by the OS gets validated
// against one location and opened at another.
func isAbsolutePath(path string) bool {
	if path == "" {
		return false
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) {
		return true
	}
	if len(path) >= 3 && path[1] == ':' && (path[2] == '/' || path[2] == '\\') {
		drive := path[0]
		return ('A' <= drive && drive <= 'Z') || ('a' <= drive && drive <= 'z')
	}
	return false
}

func isUnderDir(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// RejectControlChars delegates to charcheck.RejectControlChars.
// Kept as a package-level alias for backward compatibility with callers
// that import localfileio directly.
var RejectControlChars = charcheck.RejectControlChars

// IsDangerousUnicode delegates to charcheck.IsDangerousUnicode.
var IsDangerousUnicode = charcheck.IsDangerousUnicode
