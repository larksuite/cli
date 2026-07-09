// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

const docsServiceHelpDefault = `Document and content operations.`

const docsSkillReadCommand = "lark-cli skills read lark-doc"
const docsXMLSkillReadCommand = "lark-cli skills read lark-doc references/lark-doc-xml.md"
const docsMDSkillReadCommand = "lark-cli skills read lark-doc references/lark-doc-md.md"
const docsContentSkillHelp = "AI agents MUST read " +
	docsXMLSkillReadCommand + " before writing any --content payload; " +
	"when using --doc-format markdown, also read " + docsMDSkillReadCommand + ". " +
	"Follow the latest rules there, and MUST NOT grep/open local SKILL.md files " +
	"to discover this guidance"

func docsSkillReadCommandForShortcut(shortcut string) string {
	switch strings.TrimPrefix(shortcut, "+") {
	case "create":
		return docsSkillReadCommand + " references/lark-doc-create.md"
	case "fetch":
		return docsSkillReadCommand + " references/lark-doc-fetch.md"
	case "update":
		return docsSkillReadCommand + " references/lark-doc-update.md"
	case "history-list", "history-revert", "history-revert-status":
		return docsSkillReadCommand + " references/lark-doc-history.md"
	default:
		return docsSkillReadCommand
	}
}

func docsHelpCommandForShortcut(shortcut string) string {
	switch strings.TrimPrefix(shortcut, "+") {
	case "create":
		return "lark-cli docs +create --help"
	case "fetch":
		return "lark-cli docs +fetch --help"
	case "update":
		return "lark-cli docs +update --help"
	case "history-list":
		return "lark-cli docs +history-list --help"
	case "history-revert":
		return "lark-cli docs +history-revert --help"
	case "history-revert-status":
		return "lark-cli docs +history-revert-status --help"
	default:
		return "lark-cli docs --help"
	}
}

// Shortcuts returns all docs shortcuts.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		DocsSearch,
		DocsCreate,
		DocsFetch,
		DocsUpdate,
		DocsHistoryList,
		DocsHistoryRevert,
		DocsHistoryRevertStatus,
		DocMediaInsert,
		DocMediaUpload,
		DocMediaPreview,
		DocMediaDownload,
		DocResourceDownload,
		DocResourceUpdate,
		DocResourceDelete,
	}
}

// ConfigureServiceHelp adds docs-specific guidance to the parent `docs` command.
func ConfigureServiceHelp(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	cmd.Long = docsHelpLong(docsServiceHelpDefault, docsSkillReadCommand)
}

func installDocsShortcutHelp(command string) func(*cobra.Command) {
	return func(cmd *cobra.Command) {
		cmd.Long = docsHelpLong(cmd.Short, docsSkillReadCommandForShortcut(command))
	}
}

func docsHelpLong(summary, skillReadCommand string) string {
	return strings.TrimSpace(fmt.Sprintf(`%s

Start here (required for AI agents):
  %s

  AI agents MUST read the matching embedded skill before choosing flags
  or running docs commands. Do not skip this step, and do not infer
  workflows from --help alone. MUST NOT grep/open local SKILL.md files
  to discover this guidance; use %s so content stays version-matched
  with this CLI. Skills ship with the CLI and include docs workflows,
  selector/block-id usage, XML/Markdown formats, and copy-paste examples.

  skills read lark-doc                Docs workflow guide
  skills read lark-doc <path>         Read a referenced docs skill file`, strings.TrimSpace(summary), skillReadCommand, skillReadCommand))
}
