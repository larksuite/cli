// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillref

import (
	"errors"
	"fmt"
	"io/fs"
)

// ErrInvalidRemap identifies malformed, conflicting, or dangling explicit
// reference mappings. Callers classify it as an invalid SkillsOverlay.
var ErrInvalidRemap = errors.New("invalid skill reference remap")

// Mapping is one parsed remap. A source without Path remaps the whole skill
// name; a source with Path is an exact-reference override.
type Mapping struct {
	From Ref
	To   Ref
}

// Resolver is an immutable, build-local projection from canonical references
// to the composed runtime skill tree. Underlying skill files remain live, in
// line with the skill manifest contract, so Resolve verifies availability at
// read/help-render time as well as validating explicit targets at construction.
type Resolver struct {
	content fs.FS
	skills  map[string]Ref
	exact   map[string]Ref
}

// New validates mappings against content and returns an immutable resolver.
//
// Explicit targets are build-integrity declarations and must exist. In
// contrast, an unmapped canonical reference may be absent: presenters then
// omit the complete guidance fragment that owns it.
func New(content fs.FS, mappings []Mapping) (*Resolver, error) {
	r := &Resolver{
		content: content,
		skills:  make(map[string]Ref),
		exact:   make(map[string]Ref),
	}
	seen := make(map[string]bool, len(mappings))
	for _, mapping := range mappings {
		from, to := mapping.From, mapping.To
		if _, err := Parse(from.String()); err != nil {
			return nil, fmt.Errorf("%w: source %q: %w", ErrInvalidRemap, from.String(), err)
		}
		if _, err := Parse(to.String()); err != nil {
			return nil, fmt.Errorf("%w: target %q: %w", ErrInvalidRemap, to.String(), err)
		}
		key := from.String()
		if seen[key] {
			return nil, fmt.Errorf("%w: source %q is mapped more than once", ErrInvalidRemap, key)
		}
		seen[key] = true

		if from.Path == "" {
			if to.Path != "" {
				return nil, fmt.Errorf(
					"%w: whole-skill source %q requires a bare target skill, got %q",
					ErrInvalidRemap, key, to.String())
			}
			r.skills[from.Skill] = to
		} else {
			r.exact[key] = to
		}
		exists, err := probe(content, to)
		if err != nil {
			return nil, fmt.Errorf("%w: cannot inspect target %q: %w",
				ErrInvalidRemap, to.String(), err)
		}
		if !exists {
			return nil, fmt.Errorf("%w: target %q does not exist in the composed skill tree",
				ErrInvalidRemap, to.String())
		}
	}
	return r, nil
}

// Resolve projects canonical through the exact-reference map first, then the
// whole-skill map, and finally identity. It returns ok=false when the projected
// target is not currently readable.
func (r *Resolver) Resolve(canonical Ref) (target Ref, ok bool) {
	if r == nil {
		return Ref{}, false
	}
	if exact, found := r.exact[canonical.String()]; found {
		target = exact
	} else if renamed, found := r.skills[canonical.Skill]; found {
		target = Ref{Skill: renamed.Skill, Path: canonical.Path}
	} else {
		target = canonical
	}
	return target, readable(r.content, target)
}

// ResolveString is a convenience boundary for metadata that stores references
// in their canonical string form.
func (r *Resolver) ResolveString(canonical string) (string, bool) {
	ref, err := Parse(canonical)
	if err != nil {
		return "", false
	}
	target, ok := r.Resolve(ref)
	if !ok {
		return "", false
	}
	return target.String(), true
}

func readable(content fs.FS, ref Ref) bool {
	ok, err := probe(content, ref)
	return err == nil && ok
}

func probe(content fs.FS, ref Ref) (bool, error) {
	if content == nil || !ValidSkillName(ref.Skill) {
		return false, nil
	}
	info, err := fs.Stat(content, ref.StatPath())
	switch {
	case err == nil:
		return !info.IsDir(), nil
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid):
		return false, nil
	default:
		return false, err
	}
}
