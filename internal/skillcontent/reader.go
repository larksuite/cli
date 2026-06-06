// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skillcontent reads embedded skill content (SKILL.md bodies, files
// under a skill directory, and a skill inventory) from an injected fs.FS. The
// FS is rooted at the skill list (entries are "lark-calendar/SKILL.md", ...).
// It is pure logic — the embedding lives in the repo-root package main.
package skillcontent

import (
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"gopkg.in/yaml.v3"
)

// Reader reads skill content from fsys (rooted at the skill list).
type Reader struct {
	fsys fs.FS
}

// New returns a Reader backed by fsys.
func New(fsys fs.FS) *Reader { return &Reader{fsys: fsys} }

// SkillInfo describes one skill in the top-level list output.
type SkillInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Version     string         `json:"version,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// DirEntry is one child of a listed directory. Path is skill-name-prefixed
// (e.g. "lark-doc/references/x.md") so it can be passed straight to `read`.
type DirEntry struct {
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

// List returns every skill (top-level dir) with its description, version, and
// metadata (from SKILL.md frontmatter). Skills are sorted by name.
func (r *Reader) List() ([]SkillInfo, error) {
	entries, err := fs.ReadDir(r.fsys, ".")
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeFileIO, "failed to read embedded skills: %v", err)
	}
	out := make([]SkillInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Skip directories without a SKILL.md: they are not real skills, and a
		// blank entry in the catalog would be worse than an omission. Full
		// validation (name==dir, etc.) is enforced at build time, not here.
		if info, ok := r.skillInfo(e.Name()); ok {
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// skillInfo builds the SkillInfo for a skill directory from its SKILL.md
// frontmatter (description/version/metadata). The bool is false when the
// directory has no readable SKILL.md, so callers can skip non-skill dirs.
func (r *Reader) skillInfo(name string) (SkillInfo, bool) {
	data, err := fs.ReadFile(r.fsys, name+"/SKILL.md")
	if err != nil {
		return SkillInfo{}, false
	}
	desc, version, metadata := parseFrontmatter(data)
	return SkillInfo{Name: name, Description: desc, Version: version, Metadata: metadata}, true
}

// ListPath lists the direct children (one layer, no recursion) of the directory
// named by arg, which is "<name>" or "<name>/<subpath>". It returns the entries
// (sorted by path), the cleaned skill-prefixed path that was listed, and an
// error. Unknown skill, traversal, or a non-directory target → typed validation
// error.
func (r *Reader) ListPath(arg string) ([]DirEntry, string, error) {
	name, sub := SplitArg(arg)
	if err := r.ensureSkill(name); err != nil {
		return nil, "", err
	}
	dir := name
	if sub != "" {
		cleaned, err := cleanSubPath(sub)
		if err != nil {
			return nil, "", err
		}
		dir = name + "/" + cleaned
		info, err := fs.Stat(r.fsys, dir)
		if err != nil {
			return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"path %q not found in skill %q", sub, name).
				WithHint("run 'lark-cli skills list " + name + "' to see files in this skill")
		}
		if !info.IsDir() {
			return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"path %q is a file, not a directory; use 'lark-cli skills read %s/%s' to read it", sub, name, cleaned)
		}
	}
	entries, err := fs.ReadDir(r.fsys, dir)
	if err != nil {
		return nil, "", errs.NewInternalError(errs.SubtypeFileIO,
			"failed to read embedded skill content: %v", err)
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, DirEntry{Path: dir + "/" + e.Name(), IsDir: e.IsDir()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, dir, nil
}

// SplitArg splits "<name>/<rest>" at the first separator; an argument with no
// separator is a bare skill name (rest ""). It is the single splitter shared by
// `read <name>/<path>` and `list <name>/<sub>`.
func SplitArg(arg string) (name, rest string) {
	name, rest, _ = strings.Cut(arg, "/")
	return name, rest
}

// parseFrontmatter extracts the `description`, `version`, and `metadata` fields
// from a SKILL.md YAML frontmatter block. All are best-effort: missing or
// unparseable frontmatter yields ("", "", nil) — never an error.
func parseFrontmatter(skillMD []byte) (description, version string, metadata map[string]any) {
	lines := strings.Split(string(skillMD), "\n")
	if strings.TrimRight(lines[0], "\r") != "---" {
		return "", "", nil
	}
	block := make([]string, 0, len(lines))
	closed := false
	for _, ln := range lines[1:] {
		if strings.TrimRight(ln, "\r") == "---" {
			closed = true
			break
		}
		block = append(block, ln)
	}
	if !closed {
		return "", "", nil
	}
	var fm struct {
		Description string         `yaml:"description"`
		Version     string         `yaml:"version"`
		Metadata    map[string]any `yaml:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(strings.Join(block, "\n")), &fm); err != nil {
		return "", "", nil
	}
	return fm.Description, fm.Version, fm.Metadata
}

// ReadSkill returns the raw bytes of <name>/SKILL.md.
func (r *Reader) ReadSkill(name string) ([]byte, error) {
	if err := r.ensureSkill(name); err != nil {
		return nil, err
	}
	data, err := fs.ReadFile(r.fsys, name+"/SKILL.md")
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeFileIO,
			"failed to read embedded skill content: %v", err)
	}
	return data, nil
}

// ensureSkill validates that name is a single path segment naming an embedded
// skill directory. Returns a typed validation error otherwise.
func (r *Reader) ensureSkill(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return unknownSkill(name)
	}
	info, err := fs.Stat(r.fsys, name)
	if err != nil || !info.IsDir() {
		return unknownSkill(name)
	}
	return nil
}

func unknownSkill(name string) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "unknown skill %q", name).
		WithHint("run 'lark-cli skills list' to see available skills")
}

// cleanSubPath validates that relpath is a safe relative path within a skill
// directory and returns its cleaned form. Absolute paths and ".." escapes are
// rejected with a typed validation error. relpath must be non-empty — callers
// handle the empty (skill-root) case themselves.
func cleanSubPath(relpath string) (string, error) {
	cleaned := path.Clean(relpath)
	// path.Clean only treats '/' as a separator, so a Windows-style "..\" prefix
	// survives verbatim in cleaned; reject it explicitly alongside the "../" case.
	if relpath == "" || path.IsAbs(relpath) || cleaned == "." ||
		cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, `..\`) {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid path %q: must be a relative path without '..'", relpath)
	}
	return cleaned, nil
}

// ReadReference returns the raw bytes of <name>/<relpath> and the cleaned
// relative path. relpath must be a relative path within the skill dir; ".."
// segments, absolute paths, and escapes are rejected with a typed validation
// error and no content is returned.
func (r *Reader) ReadReference(name, relpath string) ([]byte, string, error) {
	if err := r.ensureSkill(name); err != nil {
		return nil, "", err
	}
	cleaned, err := cleanSubPath(relpath)
	if err != nil {
		return nil, "", err
	}
	full := name + "/" + cleaned
	info, err := fs.Stat(r.fsys, full)
	if err != nil {
		return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"reference %q not found in skill %q", relpath, name).
			WithHint("run 'lark-cli skills list " + name + "' to see files in this skill")
	}
	if info.IsDir() {
		return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"reference %q is a directory, not a file", relpath)
	}
	data, err := fs.ReadFile(r.fsys, full)
	if err != nil {
		return nil, "", errs.NewInternalError(errs.SubtypeFileIO,
			"failed to read embedded skill content: %v", err)
	}
	return data, cleaned, nil
}
