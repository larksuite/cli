// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/shortcuts/common"
)

type docsLegacyFlag struct {
	Name        string
	Replacement string
}

func docsAPIVersionCompatFlag() common.Flag {
	return common.Flag{
		Name:   "api-version",
		Desc:   "deprecated compatibility flag; ignored by docs shortcuts",
		Hidden: true,
	}
}

func docsOutputFormatCompatFlag() common.Flag {
	return common.Flag{
		Name:    "format",
		Default: "json",
		Desc:    "deprecated output-format compatibility flag",
		Hidden:  true,
		Enum:    []string{"json", "pretty", "table", "ndjson", "csv"},
	}
}

func docsJSONOutputCompatFlag() common.Flag {
	return common.Flag{
		Name:   "json",
		Type:   "bool",
		Desc:   "deprecated JSON-output compatibility flag",
		Hidden: true,
	}
}

func docsCreateLegacyFlags() []docsLegacyFlag {
	return []docsLegacyFlag{
		{Name: "markdown", Replacement: "use --content with --doc-format markdown"},
		{Name: "folder-token", Replacement: "use --parent-token"},
		{Name: "wiki-node", Replacement: "use --parent-token"},
		{Name: "wiki-space", Replacement: "use --parent-position my_library or a concrete parent position"},
	}
}

func docsFetchLegacyFlags() []docsLegacyFlag {
	return []docsLegacyFlag{
		{Name: "offset", Replacement: "use --scope outline/range/keyword/section for partial reads"},
		{Name: "limit", Replacement: "use --scope outline/range/keyword/section for partial reads"},
	}
}

func docsUpdateLegacyFlags() []docsLegacyFlag {
	return []docsLegacyFlag{
		{Name: "mode", Replacement: "use --command"},
		{Name: "markdown", Replacement: "use --content with --doc-format markdown"},
		{Name: "selection-with-ellipsis", Replacement: "use --command str_replace with --pattern"},
		{Name: "selection-by-title", Replacement: "fetch block ids first, then use --command block_replace/block_insert_after with --block-id"},
		{Name: "new-title", Replacement: "update the title through XML content in --content"},
	}
}

func docsLegacyFlagDefinitions(flags []docsLegacyFlag) []common.Flag {
	out := make([]common.Flag, 0, len(flags))
	for _, flag := range flags {
		out = append(out, common.Flag{
			Name:   flag.Name,
			Desc:   "deprecated compatibility flag; run the corresponding docs command with --help for the current interface",
			Hidden: true,
		})
	}
	return out
}

func validateDocsV2Only(runtime *common.RuntimeContext, shortcut string, legacyFlags []docsLegacyFlag) error {
	var used []string
	var replacements []string
	for _, flag := range legacyFlags {
		if !runtime.Changed(flag.Name) {
			continue
		}
		used = append(used, "--"+flag.Name)
		if flag.Replacement != "" {
			replacements = append(replacements, "--"+flag.Name+" -> "+flag.Replacement)
		}
	}
	if len(used) == 0 {
		return nil
	}

	detail := "the old v1 interface has been shut down; legacy v1 flag(s) " + strings.Join(used, ", ") + " are no longer supported"
	if len(replacements) > 0 {
		detail += "; " + strings.Join(replacements, "; ")
	}
	return docsV2OnlyError(runtime, shortcut, detail, used[0])
}

func docsV2OnlyError(runtime *common.RuntimeContext, shortcut, detail, param string) error {
	helpCommand := "lark-cli docs " + shortcut + " --help"
	err := errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"docs %s is v2-only; %s",
		shortcut,
		detail,
	)
	if param != "" {
		err = err.WithParam(param)
	}

	parts := []recovery.Part{
		recovery.Text(fmt.Sprintf("run `%s` for the latest command flags", helpCommand)),
	}
	if runtime != nil {
		if refs := runtime.ResolveAffordanceSkillReferences(); len(refs) > 0 {
			commands := make([]string, 0, len(refs))
			for _, ref := range refs {
				commands = append(commands, "`lark-cli skills read "+ref+"`")
			}
			parts = append(parts, recovery.Command(
				recovery.TargetSkillsRead,
				"read the version-matched embedded guidance before retrying: "+strings.Join(commands, ", ")+
					"; do not inspect another local SKILL.md copy",
			))
		}
	}
	return recovery.Attach(err, recovery.Join("; ", parts...))
}
