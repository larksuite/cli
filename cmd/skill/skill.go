// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skill implements the top-level `lark-cli skills` command group, which
// reads skill content embedded in the binary (injected via the Factory's
// SkillContent fs.FS) for AI agents. The package/dir name stays "skill"
// (internal); the user-facing verb is "skills".
package skill

import (
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

// newReader builds a Reader over the embedded skill tree carried by the Factory
// (wired by cmd.WithSkillContent). Builds that embed no skills leave it nil; the
// commands then return a typed internal error instead of panicking.
func newReader(f *cmdutil.Factory) (*skillcontent.Reader, error) {
	if f.SkillContent == nil {
		return nil, errs.NewInternalError(errs.SubtypeFileIO,
			"skill content not embedded in this build")
	}
	return skillcontent.New(f.SkillContent), nil
}

// readEnvelope is the --json shape for `skills read`. Guidance is present only
// when reading the main SKILL.md (omitted for reference files).
type readEnvelope struct {
	Skill    string `json:"skill"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Guidance string `json:"guidance,omitempty"`
}

// listEnvelope is the JSON shape for `skills list` (catalog form). "ok" is an
// explicit success marker. These are typed structs (not maps), so the automatic
// output.injectNotice _notice does not attach — that notice is a general
// binary/disk-skills update hint surfaced on every other command, and the
// embedded catalog is version-consistent by construction, so its absence here
// loses nothing.
type listEnvelope struct {
	OK     bool                     `json:"ok"`
	Skills []skillcontent.SkillInfo `json:"skills"`
	Count  int                      `json:"count"`
}

// listPathEnvelope is the JSON shape for `skills list <name[/sub]>` (the ls-style
// one-layer directory listing).
type listPathEnvelope struct {
	OK      bool                    `json:"ok"`
	Path    string                  `json:"path"`
	Entries []skillcontent.DirEntry `json:"entries"`
	Count   int                     `json:"count"`
}

// NewCmdSkill builds the `skills` command group.
func NewCmdSkill(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Read embedded skill content (list / read)",
		Long:  "Read skill content embedded in the CLI binary at build time. Content stays in sync with the CLI version.",
	}
	// Risk is set per leaf subcommand (GetRisk does not walk parents); the group
	// itself carries none, matching the config/service command groups. AuthCheck
	// is disabled on the group and propagates to children.
	cmdutil.DisableAuthCheck(cmd)
	cmd.AddCommand(newListCmd(f), newReadCmd(f))
	return cmd
}

func newListCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [name[/path]]",
		Short: "List skills, or list one layer under a skill path (like ls)",
		Example: `  lark-cli skills list                      # all skills: name, description, version
  lark-cli skills list lark-doc             # one layer under a skill (like ls)
  lark-cli skills list lark-doc/references  # one layer under a subdirectory`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"list takes at most 1 argument: [name[/path]]").
					WithHint("run 'lark-cli skills list --help'")
			}
			r, err := newReader(f)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				skills, err := r.List()
				if err != nil {
					return err
				}
				output.PrintJson(f.IOStreams.Out, listEnvelope{OK: true, Skills: skills, Count: len(skills)})
				return nil
			}
			// One-layer directory listing under args[0]; unknown skill / traversal /
			// non-directory → typed validation (exit 2).
			entries, listed, err := r.ListPath(args[0])
			if err != nil {
				return err
			}
			output.PrintJson(f.IOStreams.Out, listPathEnvelope{OK: true, Path: listed, Entries: entries, Count: len(entries)})
			return nil
		},
	}
	// list output is always JSON; accept --json as a no-op so it stays symmetric
	// with read (where --json is meaningful) and never surprises a caller with
	// cobra's "unknown flag" (exit 1) for a flag the sibling command accepts.
	cmd.Flags().Bool("json", false, "no-op (list output is always JSON)")
	cmdutil.SetRisk(cmd, "read")
	cmdutil.DisableAuthCheck(cmd)
	return cmd
}

func newReadCmd(f *cmdutil.Factory) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "read <name>[/<path>] [path]",
		Short: "Print a skill's SKILL.md, or a file under the skill (raw markdown by default)",
		Example: `  lark-cli skills read lark-doc                             # the skill's SKILL.md
  lark-cli skills read lark-doc references/lark-doc-fetch.md  # a file under the skill
  lark-cli skills read lark-doc/references/lark-doc-fetch.md  # same, slash form
  lark-cli skills read lark-doc --json                      # JSON envelope`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, relpath, err := parseReadTarget(args)
			if err != nil {
				return err
			}
			r, err := newReader(f)
			if err != nil {
				return err
			}

			var content []byte
			var pathOut string
			if relpath == "" {
				content, err = r.ReadSkill(name)
				pathOut = "SKILL.md"
			} else {
				content, pathOut, err = r.ReadReference(name, relpath)
			}
			if err != nil {
				return err
			}

			// Guidance is emitted only when reading the main SKILL.md — it nudges
			// the model to fetch this skill's own reference files via this command
			// (so they match the CLI version). Skipped for reference reads.
			isMain := pathOut == "SKILL.md"
			if asJSON {
				env := readEnvelope{Skill: name, Path: pathOut, Content: string(content)}
				if isMain {
					env.Guidance = readGuidance(name)
				}
				output.PrintJson(f.IOStreams.Out, env)
				return nil
			}
			// Raw mode: stdout is the SKILL.md bytes verbatim (so callers can treat
			// it as the file content). The guidance goes to stderr instead, keeping
			// stdout byte-identical while a human/agent still sees the tip.
			if _, err := f.IOStreams.Out.Write(content); err != nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "failed to write output: %v", err)
			}
			if isMain {
				fmt.Fprintln(f.IOStreams.ErrOut, readGuidance(name))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as a JSON envelope instead of raw markdown")
	cmdutil.SetRisk(cmd, "read")
	cmdutil.DisableAuthCheck(cmd)
	return cmd
}

// parseReadTarget resolves the read command's positional args into a skill name
// and an optional relative path. relpath "" means read the main SKILL.md.
//   - 2 args      → (args[0], args[1])
//   - 1 arg "a/b" → ("a", "b")   (only the first '/' splits)
//   - 1 arg "a"   → ("a", "")
func parseReadTarget(args []string) (name, relpath string, err error) {
	switch len(args) {
	case 1:
		name, relpath = skillcontent.SplitArg(args[0])
		return name, relpath, nil
	case 2:
		return args[0], args[1], nil
	default:
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"read requires 1 or 2 arguments: <name>[/<path>] [path]").
			WithHint("run 'lark-cli skills read --help'")
	}
}

// readGuidance is the one-line tip emitted for a skill's main SKILL.md — to
// stderr in raw mode, or the --json guidance field; never appended to stdout.
// It points the model at `skills read` for both this skill's own files and
// references to sibling skills: a "../lark-foo/..." reference is the same
// command with the leading "../" removed, keeping every hop version-consistent
// with the embedded tree (the path guard rejects literal "../", so the relative
// form must be rewritten to the sibling skill's name).
func readGuidance(name string) string {
	return fmt.Sprintf("> Tip: read this skill's own files (e.g. `references/...`) with "+
		"`lark-cli skills read %s <relative-path>` to keep them in sync with this CLI version. "+
		"A reference to another skill (`../lark-foo/...`) uses the same command with the "+
		"leading `../` removed: `lark-cli skills read lark-foo/...`.", name)
}
