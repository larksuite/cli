// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/qualitygate/skillscan"
)

func TestExtractDryRunJSONSkipsBanner(t *testing.T) {
	raw := "=== Dry Run ===\n{\n  \"api\": [{\"method\":\"GET\",\"url\":\"/open-apis/test\"}]\n}\n"
	got, apiCallCount, err := extractDryRunJSON([]byte(raw))
	if err != nil {
		t.Fatalf("extractDryRunJSON() error = %v", err)
	}
	if got.Method != "GET" {
		t.Fatalf("method = %q", got.Method)
	}
	if apiCallCount != 1 {
		t.Fatalf("apiCallCount = %d, want 1", apiCallCount)
	}
}

func TestExtractDryRunJSONSkipsBannerWithBraces(t *testing.T) {
	raw := "banner {not json}\n{\"api\":[{\"method\":\"GET\",\"url\":\"/open-apis/test\"}]}\n"
	got, apiCallCount, err := extractDryRunJSON([]byte(raw))
	if err != nil {
		t.Fatalf("extractDryRunJSON() error = %v", err)
	}
	if got.Method != "GET" || apiCallCount != 1 {
		t.Fatalf("got request=%#v apiCallCount=%d, want GET and count 1", got, apiCallCount)
	}
}

func TestRunCommandDisablesRemoteMetadata(t *testing.T) {
	result := runCommand(context.Background(), "env", nil)
	if result.Err != nil {
		t.Fatalf("runCommand(env) error = %v, stderr=%s", result.Err, result.Stderr)
	}
	stdout := string(result.Stdout)
	if !strings.Contains(stdout, "LARKSUITE_CLI_REMOTE_META=off") {
		t.Fatalf("dry-run child env missing remote meta off in %q", stdout)
	}
}

func TestRunCommandRemovesTemporaryConfigDir(t *testing.T) {
	result := runCommand(context.Background(), "sh", []string{"-c", "printf %s \"$LARKSUITE_CLI_CONFIG_DIR\""})
	if result.Err != nil {
		t.Fatalf("runCommand(sh) error = %v, stderr=%s", result.Err, result.Stderr)
	}
	configDir := string(result.Stdout)
	if configDir == "" {
		t.Fatalf("child did not print LARKSUITE_CLI_CONFIG_DIR")
	}
	if _, err := os.Stat(configDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary config dir %q still exists, stat error=%v", configDir, err)
	}
}

func TestExtractDryRunJSONReportsNonAPIRequest(t *testing.T) {
	_, _, err := extractDryRunJSON([]byte(`{"ok":true,"event":"subscribe"}`))
	if !errors.Is(err, errNoDryRunAPI) {
		t.Fatalf("error = %v, want errNoDryRunAPI", err)
	}
}

func TestExtractDryRunJSONReturnsAPICallCount(t *testing.T) {
	raw := `{"api":[{"method":"GET","url":"/open-apis/one"},{"method":"POST","url":"/open-apis/two"}]}`
	got, apiCallCount, err := extractDryRunJSON([]byte(raw))
	if err != nil {
		t.Fatalf("extractDryRunJSON() error = %v", err)
	}
	if got.URL != "/open-apis/one" || apiCallCount != 2 {
		t.Fatalf("got request=%#v apiCallCount=%d, want first request and count 2", got, apiCallCount)
	}
}

func TestClassifyExampleSkipsFileAndInteractiveInputs(t *testing.T) {
	cases := map[string]string{
		"lark-cli auth login":                                  "interactive",
		"lark-cli config init":                                 "local_state",
		"lark-cli docs +fetch --doc @doc.json":                 "file_input",
		"lark-cli im message send --content -":                 "stdin",
		"lark-cli drive file delete --file-token abc --yes":    "high_risk",
		"lark-cli docs +fetch --doc abc | jq -r '.data.title'": "stdin",
	}
	for raw, want := range cases {
		got := classifyExample(skillscan.Example{Raw: raw})
		if got.SkipReason != want {
			t.Fatalf("%s skip reason = %q, want %q", raw, got.SkipReason, want)
		}
	}
}

func TestClassifyExampleDoesNotTreatQuotedPipeAsStdin(t *testing.T) {
	got := classifyExample(skillscan.Example{Raw: `lark-cli api GET /open-apis/test --jq '.data.items[] | select(.ok)'`})
	if !got.Executable || got.SkipReason != "" {
		t.Fatalf("quoted jq pipe should remain executable, got %#v", got)
	}
}

func TestDryRunIdentitySkipAllowsExplicitUser(t *testing.T) {
	cmd := manifest.Command{Path: "mail +send", Identities: []string{"user"}}
	if got := dryRunIdentitySkip(cmd, "lark-cli mail +send --as user"); got != "" {
		t.Fatalf("explicit user dry-run skip = %q, want executable", got)
	}
	if got := dryRunIdentitySkip(cmd, "lark-cli mail +send"); got != "requires_user_identity" {
		t.Fatalf("implicit user-only dry-run skip = %q, want requires_user_identity", got)
	}
}

func TestRunDryRunsMaterializesTypedPlaceholderFlagValues(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"GET","url":"/open-apis/im/v1/messages","params":{"chat_id":"oc_test123"}}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "im +chat-messages-list",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "chat-id", TakesValue: true, Usage: "chat ID (oc_xxx)"},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            "lark-cli im +chat-messages-list --chat-id <chat_id>",
		SourceFile:     "skills/lark-im/references/messages.md",
		Line:           12,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("placeholder example should be executable after materialization: %#v", facts)
	}
	if facts[0].Raw != ex.Raw {
		t.Fatalf("fact raw = %q, want original %q", facts[0].Raw, ex.Raw)
	}
	wantArgs := []string{"im", "+chat-messages-list", "--chat-id", "oc_test123", "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsIgnoresTrailingShellComment(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"GET","url":"/open-apis/docx/v1/documents/doccnxxxx"}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "docs +fetch",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "doc", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:        `lark-cli docs +fetch --doc doccnxxxx # inspect --params shape first`,
		SourceFile: "skills/lark-doc/SKILL.md",
		Line:       12,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("commented example should be executable: %#v", facts)
	}
	wantArgs := []string{"docs", "+fetch", "--doc", "doccnxxxx", "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsMaterializesPlaceholdersInsideJSONFlags(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"GET","url":"/open-apis/im/v1/messages","params":{"chat_id":"oc_test123","page_token":"page_test123"}}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "im messages list",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "params", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            `lark-cli im messages list --params '{"chat_id":"<chat_id>","page_token":"<PAGE_TOKEN>"}'`,
		SourceFile:     "skills/lark-im/references/messages.md",
		Line:           20,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("JSON placeholder example should be executable after materialization: %#v", facts)
	}
	wantArgs := []string{"im", "messages", "list", "--params", `{"chat_id":"oc_test123","page_token":"page_test123"}`, "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsWarnsWhenStdoutTruncated(t *testing.T) {
	stdout := `{"api":[` + strings.Repeat(`{"method":"GET","url":"/open-apis/test"},`, 7000)
	cliBin, _ := fakeDryRunCLI(t, stdout)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "docs +fetch",
		Runnable: true,
		Flags:    []manifest.Flag{{Name: "dry-run"}},
	}}}
	ex := skillscan.Example{Raw: "lark-cli docs +fetch", SourceFile: "skills/lark-doc/SKILL.md", Line: 10}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 1 || diags[0].Action != report.ActionWarning {
		t.Fatalf("truncated stdout should warn, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].Executable || facts[0].SkipReason != "stdout_truncated" {
		t.Fatalf("truncated stdout fact should be skipped, got %#v", facts)
	}
}

func TestRunDryRunsRejectsNonTimeoutFailure(t *testing.T) {
	cliBin := fakeFailingCLI(t)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "docs +fetch",
		Runnable: true,
		Flags:    []manifest.Flag{{Name: "dry-run"}},
	}}}
	ex := skillscan.Example{Raw: "lark-cli docs +fetch", SourceFile: "skills/lark-doc/SKILL.md", Line: 10}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 1 || diags[0].Action != report.ActionReject {
		t.Fatalf("non-timeout dry-run failure should reject, got %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("non-timeout dry-run failure should remain executable evidence, got %#v", facts)
	}
}

func TestRunDryRunsKeepsNonJSONParamsPlaceholderSkipped(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "api",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "params", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            `lark-cli api GET /open-apis/im/v1/messages --params 'container_id=oc_xxx&page_token=<PAGE_TOKEN>'`,
		SourceFile:     "skills/lark-im/references/lark-im-chat-messages-list.md",
		Line:           111,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), "missing-cli", m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("non-JSON --params placeholder should skip without diagnostics, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].Executable || facts[0].SkipReason != "placeholder" {
		t.Fatalf("non-JSON --params placeholder should stay skipped: %#v", facts)
	}
}

func TestRunDryRunsMaterializesInlinePlaceholderFlagValues(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"GET","url":"/open-apis/im/v1/messages","params":{"chat_id":"oc_test123"}}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "im +chat-messages-list",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "chat-id", TakesValue: true, Usage: "chat ID (oc_xxx)"},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            "lark-cli im +chat-messages-list --chat-id=<chat_id>",
		SourceFile:     "skills/lark-im/references/messages.md",
		Line:           24,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("inline placeholder example should be executable after materialization: %#v", facts)
	}
	wantArgs := []string{"im", "+chat-messages-list", "--chat-id=oc_test123", "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsMaterializesNumericPlaceholderFlagValues(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"GET","url":"/open-apis/vc/v1/bots/events","params":{"meeting_id":"400000000001","page_size":50}}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "vc +meeting-events",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "meeting-id", TakesValue: true, Usage: "meeting ID to query; must be a long positive integer, not a 9-digit meeting number"},
			{Name: "page-size", TakesValue: true, Usage: "page size, 20-100 (default 50)", DefValue: "50"},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            "lark-cli vc +meeting-events --meeting-id <meeting_id> --page-size <page_size>",
		SourceFile:     "skills/lark-vc-agent/SKILL.md",
		Line:           120,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("numeric placeholder example should be executable after materialization: %#v", facts)
	}
	wantArgs := []string{"vc", "+meeting-events", "--meeting-id", "400000000001", "--page-size", "50", "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsMaterializesNumericPlaceholdersInsideJSONFlags(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"GET","url":"/open-apis/test","params":{"timestamp":"1893456000","count":"20"}}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "api GET",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "params", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            `lark-cli api GET /open-apis/test --params '{"timestamp":"<timestamp>","count":"<count>"}'`,
		SourceFile:     "skills/lark-demo/SKILL.md",
		Line:           20,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("JSON numeric placeholder example should be executable after materialization: %#v", facts)
	}
	wantArgs := []string{"api", "GET", "/open-apis/test", "--params", `{"timestamp":"1893456000","count":"20"}`, "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsMaterializesLarkDocumentURLPlaceholders(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"GET","url":"/open-apis/drive/v1/metas/batch_query"}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "drive +inspect",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "url", TakesValue: true, Usage: "Lark/Feishu document URL (docx, doc, sheet, bitable, wiki, file, folder, mindnote, slides)"},
			{Name: "format", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            "lark-cli drive +inspect --url '<url>' --format json",
		SourceFile:     "skills/lark-drive/references/lark-drive-workflow-permission-governance-commands.md",
		Line:           15,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("Lark URL placeholder example should be executable after materialization: %#v", facts)
	}
	wantArgs := []string{"drive", "+inspect", "--url", "https://example.feishu.cn/docx/doc_test123", "--format", "json", "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsMaterializesResourceIDPlaceholderFlagValues(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"GET","url":"/open-apis/wiki/v2/spaces/space_test123/nodes"}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "wiki +node-list",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "space-id", TakesValue: true, Usage: "wiki space ID"},
			{Name: "page-token", TakesValue: true, Usage: "page token"},
			{Name: "format", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            "lark-cli wiki +node-list --space-id <space_id> --page-token <PAGE_TOKEN> --format json",
		SourceFile:     "skills/lark-wiki/references/lark-wiki-node-list.md",
		Line:           24,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("resource ID placeholder example should be executable after materialization: %#v", facts)
	}
	wantArgs := []string{"wiki", "+node-list", "--space-id", "space_test123", "--page-token", "page_test123", "--format", "json", "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsMaterializesResourcePlaceholdersInsideJSONFlags(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"POST","url":"/open-apis/mail/v1/user_mailboxes/me/drafts/draft_test123/send"}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "mail user_mailbox.drafts send",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "params", TakesValue: true},
			{Name: "data", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            `lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"me","draft_id":"<draft_id>"}' --data '{"send_time":"<unix_timestamp>"}'`,
		SourceFile:     "skills/lark-mail/references/lark-mail-send.md",
		Line:           172,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("JSON resource placeholder example should be executable after materialization: %#v", facts)
	}
	wantArgs := []string{"mail", "user_mailbox.drafts", "send", "--params", `{"user_mailbox_id":"me","draft_id":"draft_test123"}`, "--data", `{"send_time":"1893456000"}`, "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsSkipsUnknownFlagsBeforeDryRun(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "im +chat-messages-list",
		Runnable: true,
		Flags:    []manifest.Flag{{Name: "chat-id", TakesValue: true}, {Name: "dry-run"}},
	}}}
	ex := skillscan.Example{
		Raw:            "lark-cli im +chat-messages-list --container-id <chat_id>",
		SourceFile:     "skills/lark-im/references/messages.md",
		Line:           31,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), "missing-cli", m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("unknown flags should be left to reference rules, got dry-run diagnostics %#v", diags)
	}
	if len(facts) != 1 || facts[0].Executable || facts[0].SkipReason != "invalid_reference" {
		t.Fatalf("unknown flag example should skip dry-run as invalid_reference: %#v", facts)
	}
}

func TestRunDryRunsKeepsGenericTypePlaceholderSkipped(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "contact users list",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "user-id-type", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            "lark-cli contact users list --user-id-type <type>",
		SourceFile:     "skills/lark-contact/references/users.md",
		Line:           44,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), "missing-cli", m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("generic <type> placeholder should skip without dry-run diagnostics, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].Executable || facts[0].SkipReason != "placeholder" {
		t.Fatalf("generic <type> placeholder should stay skipped: %#v", facts)
	}
}

func TestRunDryRunsKeepsAmbiguousAppLikePlaceholderSkipped(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "approval tasks get",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "code", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            "lark-cli approval tasks get --code <approval_code>",
		SourceFile:     "skills/lark-approval/references/tasks.md",
		Line:           52,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), "missing-cli", m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("ambiguous app-like placeholder should skip without dry-run diagnostics, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].Executable || facts[0].SkipReason != "placeholder" {
		t.Fatalf("ambiguous app-like placeholder should stay skipped: %#v", facts)
	}
}

func TestRunDryRunsPreservesMarkupLiteralWhileMaterializingPlaceholder(t *testing.T) {
	cliBin, argsPath := fakeDryRunCLI(t, `{"api":[{"method":"PATCH","url":"/open-apis/docx/v1/documents/doc_test123","body":{"content":"<p>ok</p>"}}]}`)
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "docs +update",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "doc-token", TakesValue: true},
			{Name: "body", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            `lark-cli docs +update --doc-token <doc_token> --body '<p>ok</p>'`,
		SourceFile:     "skills/lark-doc/references/update.md",
		Line:           58,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), cliBin, m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("RunDryRuns() diagnostics = %#v", diags)
	}
	if len(facts) != 1 || !facts[0].Executable || facts[0].SkipReason != "" {
		t.Fatalf("markup literal should not prevent placeholder dry-run: %#v", facts)
	}
	wantArgs := []string{"docs", "+update", "--doc-token", "doc_test123", "--body", "<p>ok</p>", "--dry-run"}
	if gotArgs := readArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("fake CLI args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestRunDryRunsKeepsUnmaterializablePlaceholdersSkipped(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "apps +html-publish",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "app-id", TakesValue: true},
			{Name: "path", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            `lark-cli apps +html-publish --app-id "$APP" --path ./dist`,
		SourceFile:     "skills/lark-apps/references/html-publish.md",
		Line:           103,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), "missing-cli", m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("unmaterializable placeholder should skip without diagnostics, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].Executable || facts[0].SkipReason != "placeholder" {
		t.Fatalf("unmaterializable placeholder should stay skipped: %#v", facts)
	}
}

func TestRunDryRunsKeepsCommandTemplatePlaceholdersSkipped(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "approval", Runnable: true, Flags: []manifest.Flag{{Name: "dry-run"}}}}}
	ex := skillscan.Example{
		Raw:            "lark-cli approval <resource> <method> [flags]",
		SourceFile:     "skills/lark-approval/SKILL.md",
		Line:           42,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), "missing-cli", m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("command template placeholder should skip without diagnostics, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].Executable || facts[0].SkipReason != "placeholder" {
		t.Fatalf("command template placeholder should stay skipped: %#v", facts)
	}
}

func TestRunDryRunsKeepsUnparseablePlaceholderExamplesSkipped(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "docs +update",
		Runnable: true,
		Flags: []manifest.Flag{
			{Name: "doc-token", TakesValue: true},
			{Name: "body", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
	ex := skillscan.Example{
		Raw:            `lark-cli docs +update --doc-token <doc_token> --body '{"content":`,
		SourceFile:     "skills/lark-doc/references/update.md",
		Line:           77,
		HasPlaceholder: true,
	}

	diags, facts := RunDryRuns(context.Background(), "missing-cli", m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("unparseable placeholder should skip without diagnostics, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].Executable || facts[0].SkipReason != "placeholder" {
		t.Fatalf("unparseable placeholder should stay skipped: %#v", facts)
	}
}

func TestDryRunRequestShapeMismatchRejects(t *testing.T) {
	fact := facts.CommandExample{
		Raw:             "lark-cli api GET /open-apis/test --dry-run",
		ExpectedRequest: &facts.DryRunRequest{Method: "POST", URL: "/open-apis/test"},
		DryRun:          &facts.DryRunRequest{Method: "GET", URL: "/open-apis/test"},
		APICallCount:    1,
	}
	diags := validateDryRunShape(fact)
	if len(diags) != 1 || diags[0].Rule != "example_dry_run_request_shape" {
		t.Fatalf("expected request-shape reject, got %#v", diags)
	}
}

func TestDryRunRequestShapeMismatchRejectsUnexpectedParamsOrBody(t *testing.T) {
	for _, fact := range []facts.CommandExample{
		{
			Raw:             "lark-cli svc items list --dry-run",
			ExpectedRequest: &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items"},
			DryRun:          &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items", Params: map[string]any{"unexpected": "1"}},
			APICallCount:    1,
		},
		{
			Raw:             "lark-cli svc items get --dry-run",
			ExpectedRequest: &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items/1"},
			DryRun:          &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items/1", Body: jsonRaw(`{"unexpected":true}`)},
			APICallCount:    1,
		},
	} {
		diags := validateDryRunShape(fact)
		if len(diags) != 1 || diags[0].Rule != "example_dry_run_request_shape" {
			t.Fatalf("expected request-shape reject for %#v, got %#v", fact, diags)
		}
	}
}

func TestDryRunRequestShapeMismatchRejectsMultipleAPICalls(t *testing.T) {
	fact := facts.CommandExample{
		Raw:             "lark-cli svc items get --dry-run",
		ExpectedRequest: &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items/1"},
		DryRun:          &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items/1"},
		APICallCount:    2,
	}
	diags := validateDryRunShape(fact)
	if len(diags) != 1 || diags[0].Rule != "example_dry_run_request_shape" {
		t.Fatalf("expected multiple-api request-shape reject, got %#v", diags)
	}
}

func TestDryRunRequestShapeMismatchRejectsMissingAPICall(t *testing.T) {
	fact := facts.CommandExample{
		Raw:             "lark-cli svc items get --dry-run",
		ExpectedRequest: &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items/1"},
		APICallCount:    0,
	}
	diags := validateDryRunShape(fact)
	if len(diags) != 1 || diags[0].Rule != "example_dry_run_request_shape" {
		t.Fatalf("expected missing-api request-shape reject, got %#v", diags)
	}
}

func TestDryRunRequestShapeMismatchRejectsQueryMismatch(t *testing.T) {
	fact := facts.CommandExample{
		Raw:             "lark-cli svc items get --dry-run",
		ExpectedRequest: &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items", Query: map[string][]string{"page_size": {"20"}}},
		DryRun:          &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items", Query: map[string][]string{"page_size": {"200"}}},
		APICallCount:    1,
	}
	diags := validateDryRunShape(fact)
	if len(diags) != 1 || diags[0].Rule != "example_dry_run_request_shape" {
		t.Fatalf("expected query request-shape reject, got %#v", diags)
	}
}

func TestDryRunRequestShapeMismatchRejectsParamMismatch(t *testing.T) {
	fact := facts.CommandExample{
		Raw:             "lark-cli svc items list --params '{\"page_size\":1}' --dry-run",
		ExpectedRequest: &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items", Params: map[string]any{"page_size": float64(1)}},
		DryRun:          &facts.DryRunRequest{Method: "GET", URL: "/open-apis/svc/v1/items", Params: map[string]any{"page_size": float64(20)}},
		APICallCount:    1,
	}
	diags := validateDryRunShape(fact)
	if len(diags) != 1 || diags[0].Rule != "example_dry_run_request_shape" {
		t.Fatalf("expected request-shape reject, got %#v", diags)
	}
}

func TestDryRunFailureWarnsOnTimeout(t *testing.T) {
	ex := skillscan.Example{SourceFile: "skills/lark-doc/SKILL.md", Line: 10}
	diag := dryRunFailureDiagnostic(ex, commandResult{ExitCode: 1, TimedOut: true})
	if diag.Action != report.ActionWarning {
		t.Fatalf("timeout should warn instead of reject, got %#v", diag)
	}
}

func TestDryRunMalformedWarnsWhenStdoutTruncated(t *testing.T) {
	ex := skillscan.Example{SourceFile: "skills/lark-doc/SKILL.md", Line: 10}
	diag := dryRunMalformedDiagnostic(ex, context.Canceled, commandResult{StdoutTruncated: true})
	if diag.Action != report.ActionWarning {
		t.Fatalf("truncated stdout should warn instead of reject, got %#v", diag)
	}
}

func jsonRaw(raw string) json.RawMessage {
	return json.RawMessage(raw)
}

func TestAppendDryRunArgDoesNotDuplicate(t *testing.T) {
	got, err := appendDryRunArg("lark-cli docs +fetch --dry-run --doc abc")
	if err != nil {
		t.Fatalf("appendDryRunArg() error = %v", err)
	}
	var count int
	for _, arg := range got {
		if arg == "--dry-run" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("--dry-run count = %d, want 1 in %#v", count, got)
	}
}

func TestAppendDryRunArgForcesJSONFormat(t *testing.T) {
	got, err := appendDryRunArg("lark-cli vc +meeting-events --meeting-id 400000000001 --format pretty")
	if err != nil {
		t.Fatalf("appendDryRunArg() error = %v", err)
	}
	want := []string{"vc", "+meeting-events", "--meeting-id", "400000000001", "--format", "json", "--dry-run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendDryRunArg() = %#v, want %#v", got, want)
	}
}

func TestAppendDryRunArgForcesInlineJSONFormat(t *testing.T) {
	got, err := appendDryRunArg("lark-cli vc +meeting-events --meeting-id 400000000001 --format=pretty --dry-run")
	if err != nil {
		t.Fatalf("appendDryRunArg() error = %v", err)
	}
	want := []string{"vc", "+meeting-events", "--meeting-id", "400000000001", "--format=json", "--dry-run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendDryRunArg() = %#v, want %#v", got, want)
	}
}

func TestAppendDryRunArgPreservesNonPrettyFormat(t *testing.T) {
	for _, raw := range []string{
		"lark-cli mail +watch --format data --dry-run",
		"lark-cli export +events --format=ndjson --dry-run",
		"lark-cli docs +fetch --format table",
	} {
		got, err := appendDryRunArg(raw)
		if err != nil {
			t.Fatalf("appendDryRunArg(%q) error = %v", raw, err)
		}
		for _, arg := range got {
			if arg == "--format=json" {
				t.Fatalf("appendDryRunArg(%q) unexpectedly rewrote inline format: %#v", raw, got)
			}
		}
		for i, arg := range got {
			if arg == "--format" && i+1 < len(got) && got[i+1] == "json" {
				t.Fatalf("appendDryRunArg(%q) unexpectedly rewrote split format: %#v", raw, got)
			}
		}
	}
}

func TestAppendDryRunArgForcesDryRunWhenExplicitlyDisabled(t *testing.T) {
	got, err := appendDryRunArg("lark-cli docs +fetch --dry-run=false --doc abc")
	if err != nil {
		t.Fatalf("appendDryRunArg() error = %v", err)
	}
	want := []string{"docs", "+fetch", "--dry-run=false", "--doc", "abc", "--dry-run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendDryRunArg() = %#v, want %#v", got, want)
	}
}

func TestAppendDryRunArgForcesDryRunWhenLastValueDisablesIt(t *testing.T) {
	got, err := appendDryRunArg("lark-cli docs +fetch --dry-run --doc abc --dry-run=0")
	if err != nil {
		t.Fatalf("appendDryRunArg() error = %v", err)
	}
	want := []string{"docs", "+fetch", "--dry-run", "--doc", "abc", "--dry-run=0", "--dry-run"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendDryRunArg() = %#v, want %#v", got, want)
	}
}

func TestDryRunIdentitySkipRequiresExplicitBotForDualIdentity(t *testing.T) {
	cmd := manifest.Command{Path: "mail +triage", Identities: []string{"user", "bot"}}
	if got := dryRunIdentitySkip(cmd, "lark-cli mail +triage"); got != "identity_auto_requires_state" {
		t.Fatalf("skip = %q, want identity_auto_requires_state", got)
	}
	if got := dryRunIdentitySkip(cmd, "lark-cli mail +triage --as bot"); got != "" {
		t.Fatalf("skip with --as bot = %q, want empty", got)
	}
}

func fakeDryRunCLI(t *testing.T, stdout string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	cliPath := filepath.Join(dir, "fake-cli.sh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellSingleQuote(argsPath) + "\nprintf '%s\\n' " + shellSingleQuote(stdout) + "\n"
	if err := os.WriteFile(cliPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake cli: %v", err)
	}
	return cliPath, argsPath
}

func fakeFailingCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cliPath := filepath.Join(dir, "fake-cli.sh")
	script := "#!/bin/sh\necho 'validation failed' >&2\nexit 2\n"
	if err := os.WriteFile(cliPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake cli: %v", err)
	}
	return cliPath
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func readArgs(t *testing.T, path string) []string {
	t.Helper()
	raw := strings.TrimSuffix(readFile(t, path), "\n")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}
