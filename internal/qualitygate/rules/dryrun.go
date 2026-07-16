// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/qualitygate/skillscan"
	"github.com/larksuite/cli/internal/vfs"
)

const dryRunTimeout = 20 * time.Second
const dryRunStdoutLimit = 256 * 1024

var errNoDryRunAPI = errors.New("dry-run output does not contain api request")

var placeholderValuePattern = regexp.MustCompile(`\b([a-z]{1,8})_x+\b`)
var dryRunAngleTokenPattern = regexp.MustCompile(`<([^>\n]+)>`)
var dryRunXMLTagNamePattern = regexp.MustCompile(`^[a-z][a-z0-9:_-]*$`)

func RunDryRuns(ctx context.Context, cliBin string, m manifest.Manifest, examples []skillscan.Example) ([]report.Diagnostic, []facts.CommandExample) {
	index := indexManifest(m)
	var diags []report.Diagnostic
	var out []facts.CommandExample
	for _, ex := range examples {
		fact := classifyExample(ex)
		if !fact.Executable {
			out = append(out, fact)
			continue
		}
		parsed, err := parseAgainstManifest(m, ex.Raw)
		if err != nil {
			if ex.HasPlaceholder || skillscan.HasPlaceholder(ex.Raw) {
				fact.Executable = false
				fact.SkipReason = "placeholder"
				out = append(out, fact)
				continue
			}
			if errors.Is(err, errUnknownCommand) && commandPathContainsPlaceholder(ex.Raw) {
				fact.Executable = false
				fact.SkipReason = "placeholder"
				out = append(out, fact)
				continue
			}
			diags = append(diags, parseWarning(ex, err))
			out = append(out, fact)
			continue
		}
		commandPath := parsed.CommandPath
		cmd := index.commands[commandPath]
		fact.CommandPath = commandPath
		if cmd == nil {
			fact.SkipReason = "unknown_command"
			fact.Executable = false
			out = append(out, fact)
			continue
		}
		if hasUnknownParsedFlag(index, parsed) {
			fact.SkipReason = "invalid_reference"
			fact.Executable = false
			out = append(out, fact)
			continue
		}
		runRaw := ex.Raw
		if ex.HasPlaceholder || skillscan.HasPlaceholder(ex.Raw) {
			materialized, ok := materializePlaceholderExample(ex.Raw, *cmd)
			if !ok {
				fact.Executable = false
				fact.SkipReason = "placeholder"
				out = append(out, fact)
				continue
			}
			runRaw = materialized.Raw
		}
		if skip := dryRunIdentitySkip(*cmd, runRaw); skip != "" {
			fact.SkipReason = skip
			fact.Executable = false
			out = append(out, fact)
			continue
		}
		if !index.hasFlag(commandPath, "dry-run") {
			fact.SkipReason = "no_dry_run_flag"
			fact.Executable = false
			out = append(out, fact)
			continue
		}
		argv, err := appendDryRunArg(runRaw)
		if err != nil {
			diags = append(diags, parseWarning(ex, err))
			out = append(out, fact)
			continue
		}
		result := runCommand(ctx, cliBin, argv)
		fact.ExitCode = result.ExitCode
		fact.StdoutBytes = len(result.Stdout)
		if result.TimedOut {
			fact.Executable = false
			fact.SkipReason = "timeout"
			diags = append(diags, dryRunFailureDiagnostic(ex, result))
			out = append(out, fact)
			continue
		}
		if result.Err != nil || result.ExitCode != 0 {
			diags = append(diags, dryRunFailureDiagnostic(ex, result))
			out = append(out, fact)
			continue
		}
		preview, apiCallCount, err := extractDryRunJSON(result.Stdout)
		if err != nil {
			if errors.Is(err, errNoDryRunAPI) {
				if fact.ExpectedRequest != nil {
					diags = append(diags, validateDryRunShape(fact)...)
					out = append(out, fact)
					continue
				}
				fact.SkipReason = "non_api_dry_run"
				out = append(out, fact)
				continue
			}
			if result.StdoutTruncated {
				fact.Executable = false
				fact.SkipReason = "stdout_truncated"
			}
			diags = append(diags, dryRunMalformedDiagnostic(ex, err, result))
			out = append(out, fact)
			continue
		}
		fact.APICallCount = apiCallCount
		fact.DryRun = &preview
		diags = append(diags, validateDryRunShape(fact)...)
		out = append(out, fact)
	}
	return diags, out
}

func classifyExample(ex skillscan.Example) facts.CommandExample {
	fact := facts.CommandExample{Raw: ex.Raw, SourceFile: ex.SourceFile, Line: ex.Line, Executable: true}
	argv, _ := splitShellWords(ex.Raw)
	switch {
	case hasSubcommand(argv, "auth", "login"):
		fact.Executable, fact.SkipReason = false, "interactive"
	case hasSubcommand(argv, "config", "init"):
		fact.Executable, fact.SkipReason = false, "local_state"
	case hasAtFileArg(argv):
		fact.Executable, fact.SkipReason = false, "file_input"
	case hasExactArg(argv, "-") || hasExactArg(argv, "|"):
		fact.Executable, fact.SkipReason = false, "stdin"
	case strings.Contains(ex.Raw, "--help") || strings.Contains(ex.Raw, " -h"):
		fact.Executable, fact.SkipReason = false, "help"
	case strings.Contains(ex.Raw, "--yes") || strings.Contains(ex.Raw, "--force"):
		fact.Executable, fact.SkipReason = false, "high_risk"
	case hasAnyExactArg(argv, "&&", "||", ">", "2>", "<"):
		fact.Executable, fact.SkipReason = false, "shell_operator"
	}
	return fact
}

type materializedExample struct {
	Raw string
}

type placeholderContext struct {
	FlagName    string
	FlagUsage   string
	FlagDefault string
}

func materializePlaceholderExample(raw string, cmd manifest.Command) (materializedExample, bool) {
	if strings.Contains(raw, "$") || strings.Contains(raw, "...") {
		return materializedExample{}, false
	}
	argv, err := splitShellWords(raw)
	if err != nil || len(argv) == 0 {
		return materializedExample{}, false
	}
	commandArgEnd := 1 + len(strings.Fields(cmd.Path))
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if isShellOperator(arg) {
			break
		}
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				flagName := name[:eq]
				flag := findManifestFlag(&cmd, flagName)
				value, ok := materializePlaceholderValue(name[eq+1:], placeholderContextForFlag(flagName, flag))
				if !ok {
					return materializedExample{}, false
				}
				argv[i] = "--" + flagName + "=" + value
				continue
			}
			flag := findManifestFlag(&cmd, name)
			if flag != nil && flag.TakesValue && i+1 < len(argv) {
				value, ok := materializePlaceholderValue(argv[i+1], placeholderContextForFlag(name, flag))
				if !ok {
					return materializedExample{}, false
				}
				argv[i+1] = value
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			name := strings.TrimPrefix(arg, "-")
			flag := findManifestFlag(&cmd, name)
			if flag != nil && flag.TakesValue && i+1 < len(argv) {
				value, ok := materializePlaceholderValue(argv[i+1], placeholderContextForFlag(flag.Name, flag))
				if !ok {
					return materializedExample{}, false
				}
				argv[i+1] = value
				i++
			}
			continue
		}
		if i >= commandArgEnd {
			value, ok := materializePlaceholderValue(arg, placeholderContext{})
			if !ok {
				return materializedExample{}, false
			}
			argv[i] = value
		}
	}
	materializedRaw := shellJoinArgs(argv)
	if hasUnresolvedDryRunPlaceholder(materializedRaw) {
		return materializedExample{}, false
	}
	return materializedExample{Raw: materializedRaw}, true
}

func placeholderContextForFlag(name string, flag *manifest.Flag) placeholderContext {
	ctx := placeholderContext{FlagName: name}
	if flag != nil {
		ctx.FlagUsage = flag.Usage
		ctx.FlagDefault = flag.DefValue
	}
	return ctx
}

func materializePlaceholderValue(value string, ctx placeholderContext) (string, bool) {
	if !hasUnresolvedDryRunPlaceholder(value) {
		return value, true
	}
	if strings.Contains(value, "$") || strings.Contains(value, "...") {
		return "", false
	}
	if ctx.FlagName == "params" && !looksLikeJSONValue(value) {
		return "", false
	}
	out := placeholderValuePattern.ReplaceAllString(value, "${1}_test123")
	var ok bool
	out, ok = replaceAnglePlaceholders(out, ctx)
	if !ok {
		return "", false
	}
	return out, true
}

func looksLikeJSONValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func replaceAnglePlaceholders(value string, ctx placeholderContext) (string, bool) {
	var b strings.Builder
	for {
		start := strings.IndexByte(value, '<')
		if start < 0 {
			b.WriteString(value)
			return b.String(), true
		}
		end := strings.IndexByte(value[start+1:], '>')
		if end < 0 {
			return "", false
		}
		end += start + 1
		b.WriteString(value[:start])
		literal := value[start : end+1]
		if !hasUnresolvedDryRunPlaceholder(literal) {
			b.WriteString(literal)
			value = value[end+1:]
			continue
		}
		replacement, ok := fakeValueForPlaceholder(value[start+1:end], ctx)
		if !ok {
			return "", false
		}
		b.WriteString(replacement)
		value = value[end+1:]
	}
}

func fakeValueForPlaceholder(raw string, ctx placeholderContext) (string, bool) {
	name := normalizePlaceholderName(raw)
	if name == "" {
		return "", false
	}
	if value, ok := fakeNumericValueForPlaceholder(name, ctx); ok {
		return value, true
	}
	if value, ok := fakeContextualURLValueForPlaceholder(name, ctx); ok {
		return value, true
	}
	if value, ok := fakeValueFromPlaceholderName(name); ok {
		return value, true
	}
	if isGenericPlaceholderName(name) {
		return fakeValueFromContextHint(ctx)
	}
	return "", false
}

func fakeValueFromPlaceholderName(name string) (string, bool) {
	if isGenericPlaceholderName(name) || isLikelyEnumPlaceholder(name) {
		return "", false
	}
	tokens := placeholderTokenSet(name)
	switch {
	case hasPlaceholderToken(tokens, "chat", "container", "feed"):
		return "oc_test123", true
	case name == "open_id" || hasPlaceholderToken(tokens, "user", "owner", "participant", "approver", "speaker"):
		return "ou_test123", true
	case hasPlaceholderToken(tokens, "department", "dept"):
		return "od_test123", true
	case hasPlaceholderToken(tokens, "message"):
		return "om_test123", true
	case name == "file_key":
		return "file_test123", true
	case hasPlaceholderToken(tokens, "file") && hasPlaceholderToken(tokens, "token"):
		return "file_test123", true
	case hasPlaceholderToken(tokens, "folder") && hasPlaceholderToken(tokens, "token"):
		return "fld_test123", true
	case hasPlaceholderToken(tokens, "image", "img"):
		return "img_test123", true
	case hasPlaceholderToken(tokens, "app"):
		return "app_test123", true
	case hasPlaceholderToken(tokens, "draft"):
		return "draft_test123", true
	case hasPlaceholderToken(tokens, "label"):
		return "label_test123", true
	case hasPlaceholderToken(tokens, "share"):
		return "share_test123", true
	case hasPlaceholderToken(tokens, "doc", "document"):
		return "doc_test123", true
	case hasPlaceholderToken(tokens, "sheet", "spreadsheet"):
		return "shtcn_test123", true
	case hasPlaceholderToken(tokens, "base"):
		return "base_test123", true
	case hasPlaceholderToken(tokens, "space"):
		return "space_test123", true
	case hasPlaceholderToken(tokens, "table"):
		return "tbl_test123", true
	case hasPlaceholderToken(tokens, "view"):
		return "viw_test123", true
	case hasPlaceholderToken(tokens, "record"):
		return "rec_test123", true
	case hasPlaceholderToken(tokens, "field"):
		return "fld_test123", true
	case hasPlaceholderToken(tokens, "wiki", "node", "obj"):
		return "wiki_test123", true
	case hasPlaceholderToken(tokens, "meeting"):
		return "meeting_test123", true
	case hasPlaceholderToken(tokens, "minute"):
		return "obcn_test123", true
	case hasPlaceholderToken(tokens, "task"):
		return "task_test123", true
	case hasPlaceholderToken(tokens, "item"):
		return "item_test123", true
	case hasPlaceholderToken(tokens, "page") && hasPlaceholderToken(tokens, "token"):
		return "page_test123", true
	case hasPlaceholderToken(tokens, "date"):
		return "2026-01-02", true
	case hasPlaceholderToken(tokens, "time", "start", "end"):
		return "2026-01-02T00:00:00+08:00", true
	case hasPlaceholderToken(tokens, "url", "link"):
		return "https://example.test/resource", true
	default:
		return "", false
	}
}

func fakeValueFromContextHint(ctx placeholderContext) (string, bool) {
	if value, ok := fakeNumericValueForPlaceholder("", ctx); ok {
		return value, true
	}
	if value, ok := fakeContextualURLValueForPlaceholder("", ctx); ok {
		return value, true
	}
	match := placeholderValuePattern.FindStringSubmatch(strings.ToLower(ctx.FlagUsage))
	if len(match) != 2 || !knownTokenPrefix(match[1]) {
		return "", false
	}
	return match[1] + "_test123", true
}

func fakeContextualURLValueForPlaceholder(name string, ctx placeholderContext) (string, bool) {
	nameTokens := placeholderTokenSet(name)
	flagName := strings.ReplaceAll(strings.ToLower(ctx.FlagName), "-", "_")
	flagTokens := placeholderTokenSet(flagName)
	if !hasPlaceholderToken(nameTokens, "url", "link") && !hasPlaceholderToken(flagTokens, "url", "link") {
		return "", false
	}
	usage := strings.ToLower(ctx.FlagUsage)
	if strings.Contains(usage, "lark") || strings.Contains(usage, "feishu") || strings.Contains(usage, "document url") {
		return "https://example.feishu.cn/docx/doc_test123", true
	}
	return "", false
}

func fakeNumericValueForPlaceholder(name string, ctx placeholderContext) (string, bool) {
	nameTokens := placeholderTokenSet(name)
	flagName := strings.ReplaceAll(strings.ToLower(ctx.FlagName), "-", "_")
	flagTokens := placeholderTokenSet(flagName)
	usage := strings.ToLower(ctx.FlagUsage)

	switch {
	case placeholderTokenPair(nameTokens, "meeting", "id") || placeholderTokenPair(flagTokens, "meeting", "id"):
		return "400000000001", true
	case placeholderTokenPair(nameTokens, "meeting", "ids") || placeholderTokenPair(flagTokens, "meeting", "ids"):
		return "400000000001", true
	case placeholderTokenPair(nameTokens, "meeting", "no") || placeholderTokenPair(flagTokens, "meeting", "no"):
		return "123456789", true
	case placeholderTokenPair(nameTokens, "meeting", "number") || placeholderTokenPair(flagTokens, "meeting", "number"):
		return "123456789", true
	case hasPlaceholderToken(nameTokens, "timestamp") || hasPlaceholderToken(flagTokens, "timestamp") || strings.Contains(usage, "unix timestamp"):
		return defaultPositiveInteger(ctx.FlagDefault, "1893456000"), true
	case placeholderTokenPair(nameTokens, "page", "size") || placeholderTokenPair(flagTokens, "page", "size"):
		return defaultPositiveInteger(ctx.FlagDefault, "20"), true
	case placeholderTokenPair(nameTokens, "page", "limit") || placeholderTokenPair(flagTokens, "page", "limit"):
		return defaultPositiveInteger(ctx.FlagDefault, "10"), true
	case numericPlaceholderName(nameTokens) || numericPlaceholderName(flagTokens) || numericUsageHint(usage):
		return defaultPositiveInteger(ctx.FlagDefault, "20"), true
	default:
		return "", false
	}
}

func numericPlaceholderName(tokens map[string]bool) bool {
	if len(tokens) == 0 || hasPlaceholderToken(tokens, "token", "format", "type", "status", "mode") {
		return false
	}
	return hasPlaceholderToken(tokens,
		"amount", "count", "depth", "height", "index", "length", "limit", "max",
		"number", "revision", "size", "width",
	)
}

func numericUsageHint(usage string) bool {
	if usage == "" {
		return false
	}
	return strings.Contains(usage, "positive integer") ||
		strings.Contains(usage, "decimal integer") ||
		strings.Contains(usage, "number of ") ||
		strings.Contains(usage, "(number)")
}

func defaultPositiveInteger(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "-") || raw == "0" {
		return fallback
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return fallback
		}
	}
	return raw
}

func knownTokenPrefix(prefix string) bool {
	switch prefix {
	case "app", "base", "doc", "draft", "file", "fld", "img", "item", "label", "meeting", "obcn", "oc", "od", "om", "ou", "page", "rec", "share", "shtcn", "space", "task", "tbl", "token", "viw", "wiki":
		return true
	default:
		return false
	}
}

func isGenericPlaceholderName(name string) bool {
	switch name {
	case "code", "command", "id", "key", "method", "name", "resource", "service", "token", "type", "value":
		return true
	default:
		return false
	}
}

func isLikelyEnumPlaceholder(name string) bool {
	return strings.HasSuffix(name, "_type") ||
		strings.HasSuffix(name, "_name") ||
		strings.HasSuffix(name, "_mode") ||
		strings.HasSuffix(name, "_status")
}

func placeholderTokenSet(name string) map[string]bool {
	tokens := map[string]bool{}
	for _, token := range strings.FieldsFunc(name, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if token != "" {
			tokens[token] = true
		}
	}
	return tokens
}

func hasPlaceholderToken(tokens map[string]bool, wants ...string) bool {
	for _, want := range wants {
		if tokens[want] {
			return true
		}
	}
	return false
}

func placeholderTokenPair(tokens map[string]bool, first, second string) bool {
	return tokens[first] && tokens[second]
}

func hasUnresolvedDryRunPlaceholder(value string) bool {
	if skillscan.HasPlaceholder(value) {
		return true
	}
	for _, match := range dryRunAngleTokenPattern.FindAllStringSubmatch(value, -1) {
		if len(match) < 2 {
			continue
		}
		if isDryRunTemplateAngle(match[1], value) {
			return true
		}
	}
	return false
}

func isDryRunTemplateAngle(inner, raw string) bool {
	inner = strings.TrimSpace(inner)
	if inner == "" || strings.HasPrefix(inner, "/") || strings.HasPrefix(inner, "!") || strings.HasPrefix(inner, "?") {
		return false
	}
	name := inner
	if cut := strings.IndexAny(name, " \t/"); cut >= 0 {
		name = name[:cut]
	}
	lower := strings.ToLower(strings.TrimPrefix(name, "/"))
	if dryRunMarkupTag(lower) {
		return false
	}
	if strings.Contains(inner, "=") || strings.HasSuffix(strings.TrimSpace(inner), "/") {
		return false
	}
	if dryRunXMLTagNamePattern.MatchString(lower) && strings.Contains(strings.ToLower(raw), "</"+lower+">") {
		return false
	}
	return true
}

func dryRunMarkupTag(name string) bool {
	switch name {
	case "a", "b", "br", "code", "content", "div", "em", "h1", "h2", "h3", "h4", "h5", "h6",
		"i", "img", "li", "ol", "p", "span", "strong", "table", "tbody", "td", "th", "thead",
		"title", "tr", "ul":
		return true
	default:
		return false
	}
}

func normalizePlaceholderName(raw string) string {
	name := strings.TrimSpace(raw)
	if cut := strings.Index(name, "|"); cut >= 0 {
		name = name[:cut]
	}
	name = strings.TrimSpace(name)
	if cut := strings.IndexAny(name, " \t/"); cut >= 0 {
		name = name[:cut]
	}
	name = strings.Trim(name, "[]{}()")
	name = strings.ReplaceAll(name, "-", "_")
	return strings.ToLower(name)
}

func hasUnknownParsedFlag(index manifestIndex, parsed ParsedExample) bool {
	for _, flag := range parsed.Flags {
		if !index.hasFlag(parsed.CommandPath, flag) {
			return true
		}
	}
	return false
}

func shellJoinArgs(argv []string) string {
	out := make([]string, 0, len(argv))
	for _, arg := range argv {
		out = append(out, shellSingleQuote(arg))
	}
	return strings.Join(out, " ")
}

func shellSingleQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	if strings.IndexFunc(arg, func(r rune) bool {
		return !(r == '_' || r == '-' || r == '+' || r == '.' || r == '/' || r == ':' || r == '=' ||
			('0' <= r && r <= '9') || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z'))
	}) < 0 {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

func hasSubcommand(argv []string, parts ...string) bool {
	for i := 0; i+len(parts) <= len(argv); i++ {
		match := true
		for j, part := range parts {
			if argv[i+j] != part {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func hasExactArg(argv []string, want string) bool {
	for _, arg := range argv {
		if arg == want {
			return true
		}
	}
	return false
}

func hasAnyExactArg(argv []string, wants ...string) bool {
	for _, want := range wants {
		if hasExactArg(argv, want) {
			return true
		}
	}
	return false
}

func hasAtFileArg(argv []string) bool {
	for _, arg := range argv {
		if strings.HasPrefix(arg, "@") {
			return true
		}
	}
	return false
}

func dryRunIdentitySkip(cmd manifest.Command, raw string) string {
	if len(cmd.Identities) == 0 {
		return ""
	}
	as, hasAs := explicitAs(raw)
	if hasAs && as == "bot" && supportsIdentity(cmd.Identities, "bot") {
		return ""
	}
	if hasAs && as == "user" && supportsIdentity(cmd.Identities, "user") {
		return ""
	}
	if supportsIdentity(cmd.Identities, "user") && !supportsIdentity(cmd.Identities, "bot") {
		return "requires_user_identity"
	}
	if supportsIdentity(cmd.Identities, "user") && supportsIdentity(cmd.Identities, "bot") && !hasAs {
		return "identity_auto_requires_state"
	}
	if !supportsIdentity(cmd.Identities, "bot") {
		return "unsupported_identity"
	}
	return ""
}

func supportsIdentity(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func explicitAs(raw string) (string, bool) {
	argv, err := splitShellWords(raw)
	if err != nil {
		return "", false
	}
	for i, arg := range argv {
		if strings.HasPrefix(arg, "--as=") {
			return strings.TrimPrefix(arg, "--as="), true
		}
		if arg == "--as" && i+1 < len(argv) {
			return argv[i+1], true
		}
	}
	return "", false
}

func appendDryRunArg(raw string) ([]string, error) {
	argv, err := splitShellWords(raw)
	if err != nil {
		return nil, err
	}
	if len(argv) == 0 || argv[0] != "lark-cli" {
		return nil, fmt.Errorf("not a lark-cli command")
	}
	argv = truncateShellTail(argv)
	argv = forceDryRunJSONFormat(argv)
	hasDryRunArg := false
	dryRunEnabled := false
	for _, arg := range argv[1:] {
		if arg == "--dry-run" {
			hasDryRunArg = true
			dryRunEnabled = true
			continue
		}
		if strings.HasPrefix(arg, "--dry-run=") {
			hasDryRunArg = true
			dryRunEnabled = dryRunFlagExplicitlyTrue(arg)
		}
	}
	if hasDryRunArg && dryRunEnabled {
		return argv[1:], nil
	}
	return append(argv[1:], "--dry-run"), nil
}

func forceDryRunJSONFormat(argv []string) []string {
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--format" {
			if i+1 < len(argv) && argv[i+1] == "pretty" {
				argv[i+1] = "json"
			}
			return argv
		}
		if arg == "--format=pretty" {
			argv[i] = "--format=json"
			return argv
		}
	}
	return argv
}

func truncateShellTail(argv []string) []string {
	for i, arg := range argv {
		if i == 0 {
			continue
		}
		if isShellOperator(arg) {
			return argv[:i]
		}
	}
	return argv
}

func dryRunFlagExplicitlyTrue(arg string) bool {
	value, ok := strings.CutPrefix(arg, "--dry-run=")
	if !ok {
		return false
	}
	switch strings.ToLower(value) {
	case "1", "t", "true", "yes", "on":
		return true
	default:
		return false
	}
}

type commandResult struct {
	ExitCode        int
	Stdout          []byte
	Stderr          []byte
	Err             error
	TimedOut        bool
	StdoutTruncated bool
}

func runCommand(ctx context.Context, cliBin string, argv []string) commandResult {
	tempDir, err := vfs.MkdirTemp("", "lark-cli-quality-gate-")
	if err != nil {
		return commandResult{Err: err, ExitCode: 1}
	}
	defer vfs.RemoveAll(tempDir)

	runCtx, cancel := context.WithTimeout(ctx, dryRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, cliBin, argv...)
	cmd.Env = append(os.Environ(),
		"LARKSUITE_CLI_CONFIG_DIR="+tempDir,
		"LARKSUITE_CLI_APP_ID=dry-run",
		"LARKSUITE_CLI_APP_SECRET=dry-run",
		"LARKSUITE_CLI_BRAND=feishu",
		"LARKSUITE_CLI_REMOTE_META=off",
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1",
		"LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1",
	)
	stdout := limitedBuffer{N: dryRunStdoutLimit}
	stderr := limitedBuffer{N: 32 * 1024}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	result := commandResult{
		Stdout:          stdout.Bytes(),
		Stderr:          stderr.Bytes(),
		Err:             runErr,
		TimedOut:        runCtx.Err() == context.DeadlineExceeded,
		StdoutTruncated: stdout.Truncated(),
	}
	if runErr == nil {
		return result
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.ExitCode = 1
	}
	return result
}

type limitedBuffer struct {
	N         int
	buf       bytes.Buffer
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := b.N - b.buf.Len()
	if remaining <= 0 {
		if len(p) > 0 {
			b.truncated = true
		}
		return len(p), nil
	}
	if len(p) > remaining {
		b.truncated = true
		_, _ = b.buf.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

func (b *limitedBuffer) Truncated() bool {
	return b.truncated
}

func extractDryRunJSON(raw []byte) (facts.DryRunRequest, int, error) {
	start := bytes.IndexByte(raw, '{')
	if start < 0 {
		return facts.DryRunRequest{}, 0, fmt.Errorf("dry-run output does not contain JSON")
	}
	var firstErr error
	for start >= 0 {
		var preview struct {
			API  []facts.DryRunRequest `json:"api"`
			Data struct {
				API []facts.DryRunRequest `json:"api"`
			} `json:"data"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw[start:]))
		if err := dec.Decode(&preview); err == nil {
			api := preview.API
			if len(api) == 0 {
				api = preview.Data.API
			}
			if len(api) == 0 {
				if firstErr == nil {
					firstErr = errNoDryRunAPI
				}
			} else {
				return api[0], len(api), nil
			}
		} else if firstErr == nil {
			firstErr = err
		}
		next := bytes.IndexByte(raw[start+1:], '{')
		if next < 0 {
			break
		}
		start += next + 1
	}
	if firstErr != nil {
		return facts.DryRunRequest{}, 0, firstErr
	}
	return facts.DryRunRequest{}, 0, fmt.Errorf("dry-run output does not contain JSON")
}

func validateDryRunShape(f facts.CommandExample) []report.Diagnostic {
	if f.ExpectedRequest == nil {
		return nil
	}
	if f.DryRun == nil || f.APICallCount != 1 {
		return []report.Diagnostic{{
			Rule:       "example_dry_run_request_shape",
			Action:     report.ActionReject,
			File:       f.SourceFile,
			Line:       f.Line,
			Message:    fmt.Sprintf("dry-run emitted %d API requests for %q; expected exactly one", f.APICallCount, f.Raw),
			Suggestion: "update the example or command implementation so the request preview is unambiguous",
		}}
	}
	if f.ExpectedRequest.Method != f.DryRun.Method || f.ExpectedRequest.URL != f.DryRun.URL ||
		!reflect.DeepEqual(f.ExpectedRequest.Query, f.DryRun.Query) ||
		!expectedParamsMatch(f.ExpectedRequest.Params, f.DryRun.Params) ||
		!expectedBodyMatches(f.ExpectedRequest.Body, f.DryRun.Body) {
		return []report.Diagnostic{{
			Rule:       "example_dry_run_request_shape",
			Action:     report.ActionReject,
			File:       f.SourceFile,
			Line:       f.Line,
			Message:    fmt.Sprintf("dry-run request shape mismatch for %q", f.Raw),
			Suggestion: "update the example or expected endpoint metadata",
		}}
	}
	return nil
}

func expectedParamsMatch(expected, actual map[string]any) bool {
	if len(expected) == 0 {
		return len(actual) == 0
	}
	return reflect.DeepEqual(expected, actual)
}

func expectedBodyMatches(expected, actual json.RawMessage) bool {
	if len(expected) == 0 {
		return len(actual) == 0
	}
	return jsonEqual(expected, actual)
}

func jsonEqual(a, b json.RawMessage) bool {
	var av any
	var bv any
	if err := json.Unmarshal(a, &av); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

func dryRunFailureDiagnostic(ex skillscan.Example, result commandResult) report.Diagnostic {
	if result.TimedOut {
		return report.Diagnostic{
			Rule:       "example_dry_run",
			Action:     report.ActionWarning,
			File:       ex.SourceFile,
			Line:       ex.Line,
			Message:    fmt.Sprintf("example dry-run timed out after %s", dryRunTimeout),
			Suggestion: "inspect the command locally; timeout and local process hangs are not treated as deterministic example failures",
		}
	}
	message := fmt.Sprintf("example dry-run exited with code %d", result.ExitCode)
	if len(result.Stderr) > 0 {
		message += ": " + strings.TrimSpace(string(result.Stderr))
	}
	return report.Diagnostic{
		Rule:       "example_dry_run",
		Action:     report.ActionReject,
		File:       ex.SourceFile,
		Line:       ex.Line,
		Message:    message,
		Suggestion: "update the example so it can run locally with --dry-run, or mark placeholders explicitly",
	}
}

func dryRunMalformedDiagnostic(ex skillscan.Example, err error, result commandResult) report.Diagnostic {
	if result.StdoutTruncated {
		return report.Diagnostic{
			Rule:       "example_dry_run",
			Action:     report.ActionWarning,
			File:       ex.SourceFile,
			Line:       ex.Line,
			Message:    fmt.Sprintf("example dry-run output exceeded %d bytes and was truncated before JSON validation completed", dryRunStdoutLimit),
			Suggestion: "reduce dry-run preview size or inspect the command locally; truncated local output is not treated as a deterministic example failure",
		}
	}
	return report.Diagnostic{
		Rule:       "example_dry_run",
		Action:     report.ActionReject,
		File:       ex.SourceFile,
		Line:       ex.Line,
		Message:    fmt.Sprintf("example dry-run output is not valid dry-run JSON: %v", err),
		Suggestion: "ensure --dry-run prints a JSON request preview",
	}
}
