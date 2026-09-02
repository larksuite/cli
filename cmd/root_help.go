// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"

	"github.com/larksuite/cli/internal/surface"
)

// rootHelpFragment is one framework-owned root-help fragment. A fragment with
// a target is emitted only while that exact command remains referenceable in
// this build. Keeping the target next to the text prevents curated examples
// from becoming dead pointers in reduced distributions.
type rootHelpFragment struct {
	target surface.CommandID
	text   string
}

// rootHelpSection keeps a heading coupled to the target-aware entries it
// introduces. When projection removes every entry, the heading disappears
// with them instead of leaving an empty section in reduced builds.
type rootHelpSection struct {
	heading   string
	fragments []rootHelpFragment
}

const (
	rootHelpAPI            surface.CommandID = "api"
	rootHelpCalendarAgenda surface.CommandID = "calendar/+agenda"
	rootHelpMailList       surface.CommandID = "mail/user_mailbox.messages/list"
)

var rootLongSections = []rootHelpSection{
	{fragments: []rootHelpFragment{
		{text: `lark-cli — Lark/Feishu CLI tool.

AGENT QUICKSTART (driving this as an agent? start here):
    Browse commands:  lark-cli <domain> --help            # +shortcuts (preferred) and raw API resources`},
		{target: surface.CommandSchema, text: `
    Inspect a call:   lark-cli schema <service>.<resource>.<method>   # params, types, scopes, examples`},
		{text: `
    Prefer a +shortcut over the raw API resource when one matches the task.
    Risk: each command's --help shows read | write | high-risk-write;
          high-risk-write needs --yes, only after the user confirms.
    On any API call: --jq <expr> filters JSON output, --dry-run previews the request (runs nothing).`},
	}},
	{
		heading: "\n\nEXAMPLES (one per command style, in order of preference):",
		fragments: []rootHelpFragment{
			{target: rootHelpCalendarAgenda, text: `
    lark-cli calendar +agenda                                       # +shortcut — a high-level task, prefer these`},
			{target: rootHelpMailList, text: `
    lark-cli mail user_mailbox.messages list --user-mailbox-id me   # typed command for one API method`},
			{target: surface.CommandSchema, text: `
    lark-cli schema mail.user_mailbox.messages.list                 # inspect a method's params before calling`},
			{target: rootHelpAPI, text: `
    lark-cli api GET /open-apis/calendar/v4/calendars               # raw escape hatch — any endpoint by HTTP path`},
		},
	},
}

// rootLong is the fully-visible default text retained as a compatibility
// oracle. Reduced builds derive their text from the same typed fragments.
var rootLong = renderRootHelpSections(rootLongSections, nil)

func renderRootHelpSections(sections []rootHelpSection, plan *surface.Plan) string {
	var b strings.Builder
	for _, section := range sections {
		body := renderRootHelpFragments(section.fragments, plan)
		if body == "" {
			continue
		}
		b.WriteString(section.heading)
		b.WriteString(body)
	}
	return b.String()
}

func renderRootHelpFragments(fragments []rootHelpFragment, plan *surface.Plan) string {
	var b strings.Builder
	for _, fragment := range fragments {
		if fragment.target != "" && !plan.CanReference(fragment.target) {
			continue
		}
		b.WriteString(fragment.text)
	}
	return b.String()
}

var rootUsageSynopsis = []rootHelpFragment{
	{text: `Usage:
  lark-cli <command> [subcommand] [method] [flags]`},
	{target: rootHelpAPI, text: `
  lark-cli api <method> <path> [--params <json>] [--data <json>]`},
	{target: surface.CommandSchema, text: `
  lark-cli schema <service.resource.method>`},
}

const rootUsageTemplatePrefix = `{{if .HasParent}}Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{else}}`

// rootUsageTemplateSuffix is Cobra's default usage template after the root
// synopsis. Root-only framework affordances are assembled separately above
// and below it so each command reference carries an explicit target.
const rootUsageTemplateSuffix = `{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}`

// skillsSetupFooter is the root-help pointer at the human one-time skills
// setup. It is emitted only while skills/read remains referenceable.
const skillsSetupFooter = `{{if not .HasParent}}

Skills setup (one-time, humans): npx skills add larksuite/cli -g -y — https://github.com/larksuite/cli#agent-skills{{end}}`

var rootUsageTemplate = renderRootUsageTemplate(nil)

func renderRootUsageTemplate(plan *surface.Plan) string {
	var b strings.Builder
	b.WriteString(rootUsageTemplatePrefix)
	b.WriteString(renderRootHelpFragments(rootUsageSynopsis, plan))
	b.WriteString(rootUsageTemplateSuffix)
	if plan.CanReference(surface.CommandSkillsRead) {
		b.WriteString(skillsSetupFooter)
	}
	b.WriteByte('\n')
	return b.String()
}
