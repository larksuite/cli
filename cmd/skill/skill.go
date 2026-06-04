// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skill implements the top-level `lark-cli skills` command group, which
// reads embedded skill content (injected via ContentFS) for AI agents. The
// package/dir name stays "skill" (internal); the user-facing verb is "skills".
package skill

import (
	"fmt"
	"io/fs"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/skillcontent"
	"github.com/spf13/cobra"
)

// ContentFS is the embedded skill filesystem, rooted at the skill list
// ("lark-calendar/SKILL.md", ...). It is injected by the repo-root package
// main at init time. Nil in builds that do not embed skills (e.g. example
// plugin hosts) — commands then return an internal error.
//
// Tests mutate this package global (see skill_test.go), so tests that touch it
// must not call t.Parallel() — concurrent writes would race.
var ContentFS fs.FS

func newReader() (*skillcontent.Reader, error) {
	if ContentFS == nil {
		return nil, errs.NewInternalError(errs.SubtypeFileIO,
			"failed to read embedded skill content: not embedded in this build")
	}
	return skillcontent.New(ContentFS), nil
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
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"list takes at most 1 argument: [name[/path]]").
					WithHint("run 'lark-cli skills list --help'")
			}
			r, err := newReader()
			if err != nil {
				return err
			}
			// "ok" makes these recognized envelopes so output.injectNotice can attach
			// _notice (e.g. binary/skills update hints) — list is the AI's discovery
			// entry point, where a "run lark-cli update" hint matters most.
			if len(args) == 0 {
				skills, err := r.List()
				if err != nil {
					return err
				}
				output.PrintJson(f.IOStreams.Out, map[string]any{
					"ok": true, "skills": skills, "count": len(skills),
				})
				return nil
			}
			// One-layer directory listing under args[0]; unknown skill / traversal /
			// non-directory → typed validation (exit 2).
			entries, listed, err := r.ListPath(args[0])
			if err != nil {
				return err
			}
			output.PrintJson(f.IOStreams.Out, map[string]any{
				"ok": true, "path": listed, "entries": entries, "count": len(entries),
			})
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
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, relpath, err := parseReadTarget(args)
			if err != nil {
				return err
			}
			r, err := newReader()
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

			// Guidance is injected only when reading the main SKILL.md — it steers the
			// model to fetch reference files via this command (so they match the CLI
			// version) instead of opening them directly. Skipped for reference reads to
			// avoid repeating it on every file.
			isMain := pathOut == "SKILL.md"
			if asJSON {
				env := map[string]any{"skill": name, "path": pathOut, "content": string(content)}
				if isMain {
					env["guidance"] = readGuidance(name)
				}
				output.PrintJson(f.IOStreams.Out, env)
				return nil
			}
			if _, err := f.IOStreams.Out.Write(content); err != nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "failed to write output: %v", err)
			}
			if isMain {
				if _, err := fmt.Fprintf(f.IOStreams.Out, "\n\n%s\n", readGuidance(name)); err != nil {
					return errs.NewInternalError(errs.SubtypeFileIO, "failed to write output: %v", err)
				}
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
		a := args[0]
		for i := 0; i < len(a); i++ {
			if a[i] == '/' {
				return a[:i], a[i+1:], nil
			}
		}
		return a, "", nil
	case 2:
		return args[0], args[1], nil
	default:
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"read requires 1 or 2 arguments: <name>[/<path>] [path]").
			WithHint("run 'lark-cli skills read --help'")
	}
}

// readGuidance is the one-line tip appended to a skill's main SKILL.md output,
// directing the model to read referenced files via this command.
func readGuidance(name string) string {
	return fmt.Sprintf("> Tip: read this skill's referenced files with "+
		"`lark-cli skills read %s <path>` instead of opening them directly, "+
		"so the content stays in sync with this CLI version.", name)
}
