// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/larksuite/cli/internal/charcheck"
	"github.com/larksuite/cli/internal/vfs"
)

// Plugin-declared allow roots. A plugin compiled into the binary may widen the
// strict tier's allowlist through platform.AllowPaths; the bootstrap collects
// those declarations and installs them here, once, before any command runs.
// The declaration is fixed by the binary exactly as the built-in roots are:
// no flag, environment variable, or config file can add one at runtime.

// PluginAllowRoots pairs a plugin name with the allow roots it declared through
// platform.AllowPaths. It mirrors cmdpolicy.PluginRule and
// skillpolicy.PluginSkill so a conflict can be attributed to its owner.
type PluginAllowRoots struct {
	PluginName string
	Roots      []string
}

// ErrMultipleAllowPathsOwners reports that more than one plugin declared allow
// paths. Mirrors cmdpolicy.ErrMultipleRestricts: the union of two declarations
// is wider than either, so independent plugins must not be able to widen each
// other's file access.
var ErrMultipleAllowPathsOwners = errors.New("multiple plugins declared allow paths; only one plugin may own the path policy")

// declaredAllowRoots is the process-wide snapshot of plugin-declared allow
// roots, each labelled with the root as declared. It is written by
// ApplyDeclaredAllowRoots and read on every validation; the atomic pointer
// keeps the swap safe against the concurrent validations download fan-out
// performs. nil means no plugin declared paths.
var declaredAllowRoots atomic.Pointer[[]policyEntry]

// ApplyDeclaredAllowRoots installs the plugin-declared allow roots for the
// strict tier. The bootstrap calls it before any command runs; each call
// replaces the snapshot and an empty or rejected declaration clears it. The
// policy is process-wide like the built-in roots: plugins register at init,
// so every Build in a process resolves the same declaration. Roots must be
// absolute or "~/"-relative below the account home (never the home itself,
// which would let $HOME choose the whole subtree where the account database
// cannot name the uid, and never $HOME), declared once, and outside the
// built-in denylist, which still wins at runtime; a root inside a deny root
// is an error rather than a dead entry. Roots need not exist, matching
// ~/files.
func ApplyDeclaredAllowRoots(specs []PluginAllowRoots) error {
	owners := distinctAllowPathsOwners(specs)
	if len(owners) > 1 {
		declaredAllowRoots.Store(nil)
		return fmt.Errorf("%w: %v", ErrMultipleAllowPathsOwners, owners)
	}
	if len(owners) == 0 {
		declaredAllowRoots.Store(nil)
		return nil
	}
	var roots []string
	for _, s := range specs {
		roots = append(roots, s.Roots...)
	}
	entries, err := resolveDeclaredRoots(owners[0], roots)
	if err != nil {
		declaredAllowRoots.Store(nil)
		return err
	}
	declaredAllowRoots.Store(&entries)
	return nil
}

// distinctAllowPathsOwners returns the unique plugin names contributing roots,
// in first-seen order. Mirrors cmdpolicy.distinctOwners.
func distinctAllowPathsOwners(specs []PluginAllowRoots) []string {
	seen := map[string]bool{}
	owners := make([]string, 0, len(specs))
	for _, s := range specs {
		if !seen[s.PluginName] {
			seen[s.PluginName] = true
			owners = append(owners, s.PluginName)
		}
	}
	return owners
}

func resolveDeclaredRoots(owner string, roots []string) ([]policyEntry, error) {
	cwd, err := vfs.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine working directory: %w", err)
	}
	entries := make([]policyEntry, 0, len(roots))
	seen := make(map[string]bool, len(roots))
	for _, raw := range roots {
		entry, err := declaredRootEntry(owner, raw, cwd)
		if err != nil {
			return nil, err
		}
		if seen[entry.literal] {
			return nil, fmt.Errorf("plugin %q allow path %q is declared more than once", owner, raw)
		}
		seen[entry.literal] = true
		entries = append(entries, entry)
	}
	return entries, nil
}

// declaredRootEntry validates one declared root and builds its policy entry.
// The entry's label is the root as declared, so rejection messages echo the
// plugin author's spelling rather than an expanded account path.
func declaredRootEntry(owner, raw, cwd string) (policyEntry, error) {
	subject := fmt.Sprintf("plugin %q allow path", owner)
	if err := charcheck.RejectControlChars(raw, subject); err != nil {
		return policyEntry{}, err
	}
	if strings.TrimSpace(raw) == "" {
		return policyEntry{}, fmt.Errorf("%s must not be empty", subject)
	}
	if filepath.Clean(raw) == "~" {
		return policyEntry{}, fmt.Errorf("%s %q names the whole home directory; declare a directory below it", subject, raw)
	}
	p := raw
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := trustedHome()
		if err != nil {
			return policyEntry{}, fmt.Errorf("%s %q cannot expand ~: %w", subject, raw, err)
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	// filepath.IsAbs is this platform's rule, so a shape that is absolute only
	// elsewhere (C:\x on Unix, /x on Windows) is refused by the same test.
	if !filepath.IsAbs(p) {
		return policyEntry{}, fmt.Errorf("%s %q must be an absolute path on this platform or start with ~/", subject, raw)
	}
	entry := newPolicyEntry(raw, p)
	if err := checkDeny(subject, raw, entry.literal, entry.resolved, cwd); err != nil {
		return policyEntry{}, err
	}
	return entry, nil
}
