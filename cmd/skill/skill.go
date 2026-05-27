// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdSkill creates the top-level "skill" command with its subcommands.
func NewCmdSkill(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage and query AI agent skills bundled with lark-cli",
	}
	cmdutil.DisableAuthCheck(cmd)
	cmd.AddCommand(newCmdSkillReference(f))
	return cmd
}

// newCmdSkillReference creates the "skill reference" subcommand.
func newCmdSkillReference(f *cmdutil.Factory) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "reference <skill-name>",
		Short: "Print a skill reference document to stdout",
		Long: `Print the contents of a skill reference document to stdout.

Example:
  lark-cli skill reference lark-mail --name lark-mail-rule-reorder`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skillName := args[0]
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			content, err := readSkillReference(skillName, name)
			if err != nil {
				return err
			}
			fmt.Fprint(f.IOStreams.Out, content)
			return nil
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmdutil.SetRisk(cmd, "read")
	cmd.Flags().StringVar(&name, "name", "", "name of the reference document (without .md extension)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// readSkillReference reads <skill-name>/references/<name>.md from the skills
// directory, resolving the location relative to the running binary.
func readSkillReference(skillName, name string) (string, error) {
	// Sanitize inputs to prevent path traversal.
	if strings.ContainsAny(skillName, "/\\..") || strings.ContainsAny(name, "/\\..") {
		return "", fmt.Errorf("invalid skill or reference name")
	}

	relPath := filepath.Join("skills", skillName, "references", name+".md")

	for _, dir := range candidateDirs() {
		fullPath := filepath.Join(dir, relPath)
		data, err := os.ReadFile(fullPath)
		if err == nil {
			return string(data), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("reading skill reference: %w", err)
		}
	}

	return "", fmt.Errorf("skill reference not found: %s/%s (skill: %s, name: %s)",
		skillName, name, skillName, name)
}

// candidateDirs returns candidate root directories where the skills/ subtree
// may be located, in priority order.
//
// Lookup order:
//  1. LARKSUITE_CLI_SKILLS_DIR env override (testing / custom installs)
//  2. <binary_dir>/          — binary at repo root after `make build`
//  3. <binary_dir>/../       — binary in bin/, skills one level up (npm pkg layout)
func candidateDirs() []string {
	var dirs []string

	if env := os.Getenv("LARKSUITE_CLI_SKILLS_DIR"); env != "" {
		dirs = append(dirs, env)
	}

	exe, err := os.Executable()
	if err == nil {
		binDir := filepath.Dir(exe)
		dirs = append(dirs,
			binDir,
			filepath.Join(binDir, ".."),
		)
	}

	return dirs
}
