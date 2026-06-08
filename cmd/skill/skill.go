// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skill implements the `lark-cli skills` command group, which serves
// binary-embedded skill content to AI agents. The package is "skill"; the
// user-facing verb is "skills".
package skill

import (
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

func newReader(f *cmdutil.Factory) (*skillcontent.Reader, error) {
	if f.SkillContent == nil {
		return nil, errs.NewInternalError(errs.SubtypeFileIO,
			"skill content not embedded in this build")
	}
	return skillcontent.New(f.SkillContent), nil
}

type readEnvelope struct {
	Skill    string `json:"skill"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Guidance string `json:"guidance,omitempty"`
}

type listEnvelope struct {
	OK     bool                     `json:"ok"`
	Skills []skillcontent.SkillInfo `json:"skills"`
	Count  int                      `json:"count"`
}

type listPathEnvelope struct {
	OK      bool                    `json:"ok"`
	Path    string                  `json:"path"`
	Entries []skillcontent.DirEntry `json:"entries"`
	Count   int                     `json:"count"`
}

func NewCmdSkill(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Read embedded skill content (list / read)",
		Long: "Read agent-readable skill content (SKILL.md and reference files) embedded in " +
			"the CLI binary at build time, so it stays in sync with the CLI version. " +
			"Machine resources such as assets/ and scripts/ are not embedded.",
	}
	// Risk is set on each leaf (GetRisk does not walk parents); the group has none.
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
			entries, listed, err := r.ListPath(args[0])
			if err != nil {
				return err
			}
			output.PrintJson(f.IOStreams.Out, listPathEnvelope{OK: true, Path: listed, Entries: entries, Count: len(entries)})
			return nil
		},
	}
	// --json is a no-op (list is always JSON), accepted only to stay symmetric with read.
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

			isMain := pathOut == "SKILL.md"
			if asJSON {
				env := readEnvelope{Skill: name, Path: pathOut, Content: string(content)}
				if isMain {
					env.Guidance = readGuidance(name)
				}
				output.PrintJson(f.IOStreams.Out, env)
				return nil
			}
			// Raw stdout stays byte-identical to the file; guidance goes to stderr.
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

// parseReadTarget maps 1-or-2 positional args to (name, relpath); a lone
// "<a>/<b>" splits on the first '/', and relpath "" reads the main SKILL.md.
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

// readGuidance routes cross-skill "../lark-foo/..." references back through
// `skills read lark-foo/...`: the path guard rejects a literal "../", so the
// relative form must be rewritten.
func readGuidance(name string) string {
	return fmt.Sprintf("> Tip: read this skill's own files (e.g. `references/...`) with "+
		"`lark-cli skills read %s <relative-path>` to keep them in sync with this CLI version. "+
		"A reference to another skill (`../lark-foo/...`) uses the same command with the "+
		"leading `../` removed: `lark-cli skills read lark-foo/...`.", name)
}
