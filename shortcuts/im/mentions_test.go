// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestMentionParseRepeatCommaDeduplicateAndAll(t *testing.T) {
	got, err := parseMentionValues(
		[]string{"ou_alpha,u_beta", "ou_alpha", "u_gamma"},
		true,
		true,
	)
	if err != nil {
		t.Fatalf("parseMentionValues() error = %v", err)
	}
	want := mentionRequest{
		IDs: []string{"ou_alpha", "u_beta", "u_gamma"},
		All: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseMentionValues() = %#v, want %#v", got, want)
	}
}

func TestMentionFlagsAreDeclaredOnSendAndReply(t *testing.T) {
	for _, shortcut := range []common.Shortcut{ImMessagesSend, ImMessagesReply} {
		types := map[string]string{}
		for _, flag := range shortcut.Flags {
			types[flag.Name] = flag.Type
		}
		if types["mention"] != "string_slice" || types["mention-all"] != "bool" {
			t.Fatalf("%s mention flags = %#v", shortcut.Command, types)
		}
	}
}

func TestMentionAffordanceExposesShortestPath(t *testing.T) {
	affordance.SetSource(os.DirFS("../../affordance"))
	t.Cleanup(func() { affordance.SetSource(nil) })

	for _, command := range []string{"+messages-send", "+messages-reply"} {
		raw, ok := affordance.For("im", command)
		if !ok {
			t.Fatalf("missing %s affordance", command)
		}
		parsed, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
		if !ok || len(parsed.Examples) != 1 || !strings.Contains(parsed.Examples[0].Command, "--mention") {
			t.Fatalf("%s affordance does not expose one structured mention example: %#v", command, parsed.Examples)
		}
	}
}

func TestMentionParseRejectsUnsafeValuesWithoutEcho(t *testing.T) {
	const marker = "MENTION_INJECTION_MARKER"
	tests := []struct {
		name    string
		values  []string
		changed bool
		message string
		hint    string
	}{
		{
			name:    "explicit empty",
			changed: true,
			message: "--mention requires a non-empty user_id or open_id",
			hint:    "Pass each user_id or open_id with --mention; repeat the flag or use comma-separated values.",
		},
		{
			name:    "empty csv item",
			values:  []string{"ou_ok,,u_ok"},
			changed: true,
			message: "--mention requires a non-empty user_id or open_id",
			hint:    "Pass each user_id or open_id with --mention; repeat the flag or use comma-separated values.",
		},
		{
			name:    "mention all alias",
			values:  []string{"@_all"},
			changed: true,
			message: "Use --mention-all instead of passing all to --mention",
			hint:    "Remove the all value from --mention and add --mention-all.",
		},
		{
			name:    "whitespace",
			values:  []string{"ou_bad value"},
			changed: true,
			message: "--mention contains an invalid user_id or open_id",
			hint:    "Pass each user_id or open_id without whitespace or tag/attribute delimiter characters.",
		},
		{
			name:    "tag injection",
			values:  []string{`ou_bad"><` + marker},
			changed: true,
			message: "--mention contains an invalid user_id or open_id",
			hint:    "Pass each user_id or open_id without whitespace or tag/attribute delimiter characters.",
		},
		{
			name:    "control",
			values:  []string{"ou_bad\n" + marker},
			changed: true,
			message: "--mention contains an invalid user_id or open_id",
			hint:    "Pass each user_id or open_id without whitespace or tag/attribute delimiter characters.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseMentionValues(tt.values, tt.changed, false)
			if err == nil {
				t.Fatal("parseMentionValues() error = nil, want validation error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v, want typed problem", err, err)
			}
			validationErr, ok := err.(*errs.ValidationError)
			if !ok {
				t.Fatalf("error = %T %v, want validation error", err, err)
			}
			if problem.Category != errs.CategoryValidation ||
				problem.Subtype != errs.SubtypeInvalidArgument ||
				validationErr.Param != "--mention" {
				t.Fatalf("problem = %#v", problem)
			}
			if problem.Message != tt.message {
				t.Fatalf("message = %q, want %q", problem.Message, tt.message)
			}
			if problem.Hint != tt.hint {
				t.Fatalf("hint = %q, want %q", problem.Hint, tt.hint)
			}
			if strings.Contains(err.Error(), marker) {
				t.Fatalf("error leaked unsafe mention value: %v", err)
			}
		})
	}
}

func TestMentionStringSliceParsingDoesNotEchoUnsafeValue(t *testing.T) {
	const marker = "MENTION_CSV_INJECTION_MARKER"
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSlice("mention", nil, "")
	installMentionFlagParser(cmd)
	err := cmd.ParseFlags([]string{"--mention", `ou_bad"><` + marker})
	if err != nil {
		t.Fatalf("string_slice parser rejected unsafe value before fixed validation: %v", err)
	}
	runtime := &common.RuntimeContext{Cmd: cmd}
	_, err = mentionRequestFromRuntime(runtime)
	if err == nil {
		t.Fatal("mentionRequestFromRuntime() error = nil, want validation error")
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("mention validation leaked unsafe value: %v", err)
	}
}

func TestMentionHelpOmitsEmptySliceDefault(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSlice("mention", nil, "mention target")
	installMentionFlagParser(cmd)

	flag := cmd.Flags().Lookup("mention")
	if flag == nil {
		t.Fatal("mention flag is missing")
	}
	if flag.DefValue != "" {
		t.Fatalf("mention DefValue = %q, want empty so help omits the synthetic default", flag.DefValue)
	}
	if usage := cmd.Flags().FlagUsages(); strings.Contains(usage, "(default [])") {
		t.Fatalf("mention help exposes an implementation default: %q", usage)
	}
}

func TestMentionApplyTextUsesCanonicalTags(t *testing.T) {
	content := `{"text":"please review"}`
	got, err := applyMentionRequest("text", content, "--text", mentionRequest{
		IDs: []string{"ou_alpha", "u_beta"},
		All: true,
	})
	if err != nil {
		t.Fatalf("applyMentionRequest() error = %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	want := `<at user_id="ou_alpha"></at> <at user_id="u_beta"></at> <at user_id="all"></at> please review`
	if payload["text"] != want {
		t.Fatalf("text = %q, want %q", payload["text"], want)
	}
}

func TestMentionApplyPostUsesIndependentParagraphForEveryLocale(t *testing.T) {
	content := `{
		"zh_cn":{"title":"标题","content":[[{"tag":"md","text":"## H2"}]]},
		"en_us":{"title":"Title","content":[[{"tag":"text","text":"body"}]]}
	}`
	got, err := applyMentionRequest("post", content, "--content", mentionRequest{
		IDs: []string{"ou_alpha"},
		All: true,
	})
	if err != nil {
		t.Fatalf("applyMentionRequest() error = %v", err)
	}

	var payload map[string]struct {
		Content [][]map[string]interface{} `json:"content"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("result is not JSON: %v", err)
	}
	for _, locale := range []string{"zh_cn", "en_us"} {
		paragraphs := payload[locale].Content
		if len(paragraphs) != 2 {
			t.Fatalf("%s paragraphs = %#v, want mention + original", locale, paragraphs)
		}
		if got := paragraphs[0][0]; got["tag"] != "at" || got["user_id"] != "ou_alpha" {
			t.Fatalf("%s first node = %#v, want individual at", locale, got)
		}
		last := paragraphs[0][len(paragraphs[0])-1]
		if last["tag"] != "at" || last["user_id"] != "all" {
			t.Fatalf("%s last mention node = %#v, want all at", locale, last)
		}
	}
	if got := payload["zh_cn"].Content[1][0]; got["tag"] != "md" || got["text"] != "## H2" {
		t.Fatalf("zh_cn markdown paragraph = %#v, want exclusive preserved md", got)
	}
}

func TestMentionManualAtValidationAndCompatibility(t *testing.T) {
	tests := []struct {
		name    string
		msgType string
		content string
		param   string
		request mentionRequest
		want    string
		message string
		hint    string
	}{
		{
			name:    "canonical manual at passes through without flags",
			msgType: "text",
			content: `{"text":"<at user_id=\"ou_alpha\"></at> hello"}`,
			param:   "--content",
			want:    `{"text":"<at user_id=\"ou_alpha\"></at> hello"}`,
		},
		{
			name:    "legacy text shape passes through without flags",
			msgType: "text",
			content: `"server-validated-text-shape"`,
			param:   "--content",
			want:    `"server-validated-text-shape"`,
		},
		{
			name:    "legacy post shape passes through without flags",
			msgType: "post",
			content: `{}`,
			param:   "--content",
			want:    `{}`,
		},
		{
			name:    "wrong id alias fails",
			msgType: "text",
			content: `{"text":"<at id=\"ou_alpha\"></at> hello"}`,
			param:   "--text",
			message: "Manual <at id> and <at open_id> tags are not supported",
			hint:    "Use --mention <user_id-or-open_id> instead.",
		},
		{
			name:    "wrong open id structured alias fails",
			msgType: "post",
			content: `{"zh_cn":{"content":[[{"tag":"at","open_id":"ou_alpha"}]]}}`,
			param:   "--content",
			message: "Manual <at id> and <at open_id> tags are not supported",
			hint:    "Use --mention <user_id-or-open_id> instead.",
		},
		{
			name:    "manual at conflicts with flag",
			msgType: "text",
			content: `{"text":"<at user_id=\"ou_alpha\"></at> hello"}`,
			param:   "--text",
			request: mentionRequest{IDs: []string{"ou_beta"}},
			message: "Do not combine mention flags with manual at tags",
			hint:    "Remove the manual at tags and pass each target with --mention or --mention-all.",
		},
		{
			name:    "structured at conflicts with flag",
			msgType: "post",
			content: `{"zh_cn":{"content":[[{"tag":"at","user_id":"ou_alpha"}]]}}`,
			param:   "--content",
			request: mentionRequest{All: true},
			message: "Do not combine mention flags with manual at tags",
			hint:    "Remove the manual at tags and pass each target with --mention or --mention-all.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyMentionRequest(tt.msgType, tt.content, tt.param, tt.request)
			if tt.message == "" {
				if err != nil {
					t.Fatalf("applyMentionRequest() error = %v", err)
				}
				if got != tt.want {
					t.Fatalf("applyMentionRequest() = %q, want exact pass-through %q", got, tt.want)
				}
				return
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("error = %T %v, want typed problem", err, err)
			}
			validationErr, ok := err.(*errs.ValidationError)
			if !ok {
				t.Fatalf("error = %T %v, want validation error", err, err)
			}
			if validationErr.Param != tt.param || problem.Message != tt.message || problem.Hint != tt.hint {
				t.Fatalf("problem = %#v", problem)
			}
		})
	}
}

func TestMentionRejectsNonTextPost(t *testing.T) {
	_, err := applyMentionRequest("file", `{"file_key":"file_xxx"}`, "--file", mentionRequest{All: true})
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed problem", err, err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("error = %T %v, want validation error", err, err)
	}
	if validationErr.Param != "--mention-all" ||
		problem.Message != "--mention and --mention-all support only text or post messages" ||
		problem.Hint != "Use --text, --markdown, or --content with --msg-type text|post; otherwise remove the mention flags." {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestMentionShortcutValidation(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		param    string
		message  string
		hint     string
	}{
		{
			name:     "empty mention",
			shortcut: ImMessagesSend,
			args:     []string{"--chat-id", "oc_test", "--text", "hello", "--mention", ""},
			param:    "--mention",
			message:  mentionEmptyMessage,
			hint:     mentionEmptyHint,
		},
		{
			name:     "all alias",
			shortcut: ImMessagesSend,
			args:     []string{"--chat-id", "oc_test", "--text", "hello", "--mention", "all"},
			param:    "--mention",
			message:  mentionAllMessage,
			hint:     mentionAllHint,
		},
		{
			name:     "non text post",
			shortcut: ImMessagesSend,
			args:     []string{"--chat-id", "oc_test", "--file", "file_test", "--mention-all"},
			param:    "--mention-all",
			message:  mentionTypeMessage,
			hint:     mentionTypeHint,
		},
		{
			name:     "manual at conflict",
			shortcut: ImMessagesReply,
			args: []string{
				"--message-id", "om_test",
				"--text", `<at user_id="ou_alpha"></at> hello`,
				"--mention", "ou_beta",
			},
			param:   "--text",
			message: manualAtConflictMessage,
			hint:    manualAtConflictHint,
		},
		{
			name:     "unsupported manual alias",
			shortcut: ImMessagesSend,
			args: []string{
				"--chat-id", "oc_test",
				"--text", `<at open_id="ou_alpha"></at> hello`,
			},
			param:   "--text",
			message: manualAtAliasMessage,
			hint:    manualAtAliasHint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newMentionCommandRuntime(t, tt.shortcut, tt.args)
			err := tt.shortcut.Validate(context.Background(), runtime)
			validationErr, ok := err.(*errs.ValidationError)
			if !ok {
				t.Fatalf("Validate() error = %T %v, want validation error", err, err)
			}
			if validationErr.Param != tt.param ||
				validationErr.Message != tt.message ||
				validationErr.Hint != tt.hint {
				t.Fatalf("validation error = %#v", validationErr)
			}
		})
	}
}

func TestMentionSendAndReplyDryRunUseSameRequestBuilder(t *testing.T) {
	sendRuntime := newMentionCommandRuntime(t, ImMessagesSend, []string{
		"--chat-id", "oc_test",
		"--text", "please review",
		"--mention", "ou_alpha,u_beta",
		"--mention", "ou_alpha",
		"--mention-all",
	})
	if err := ImMessagesSend.Validate(context.Background(), sendRuntime); err != nil {
		t.Fatalf("send Validate() error = %v", err)
	}
	assertMentionDryRunBody(t, ImMessagesSend.DryRun(context.Background(), sendRuntime),
		"text", `<at user_id="ou_alpha"></at> <at user_id="u_beta"></at> <at user_id="all"></at> please review`)

	replyRuntime := newMentionCommandRuntime(t, ImMessagesReply, []string{
		"--message-id", "om_test",
		"--markdown", "## H2",
		"--mention-all",
	})
	if err := ImMessagesReply.Validate(context.Background(), replyRuntime); err != nil {
		t.Fatalf("reply Validate() error = %v", err)
	}
	raw, err := json.Marshal(ImMessagesReply.DryRun(context.Background(), replyRuntime))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	content, _ := result.API[0].Body["content"].(string)
	var post map[string]struct {
		Content [][]map[string]interface{} `json:"content"`
	}
	if err := json.Unmarshal([]byte(content), &post); err != nil {
		t.Fatal(err)
	}
	paragraphs := post["zh_cn"].Content
	if len(paragraphs) != 2 ||
		paragraphs[0][0]["tag"] != "at" ||
		paragraphs[0][0]["user_id"] != "all" ||
		paragraphs[1][0]["tag"] != "md" ||
		paragraphs[1][0]["text"] != "## H2" {
		t.Fatalf("reply post = %#v", paragraphs)
	}
}

func TestMentionExecuteExposesReconciledEvidenceWithoutRawMentions(t *testing.T) {
	var requestBody map[string]interface{}
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &requestBody); err != nil {
			t.Fatal(err)
		}
		return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message_id":  "om_result",
				"chat_id":     "oc_result",
				"create_time": "1722168000000",
				"mentions": []interface{}{
					map[string]interface{}{"key": "@_user_1", "id": "ou_alpha", "id_type": "open_id"},
				},
			},
		}), nil
	}))
	runtime.Cmd = newMentionCommandRuntime(t, ImMessagesSend, []string{
		"--chat-id", "oc_test",
		"--text", "hello",
		"--mention", "ou_alpha",
	}).Cmd
	runtime.Format = "json"

	if err := ImMessagesSend.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	content, _ := requestBody["content"].(string)
	var payload map[string]string
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("request content is not JSON: %v", err)
	}
	if payload["text"] != `<at user_id="ou_alpha"></at> hello` {
		t.Fatalf("request body = %#v, want structured mention", requestBody)
	}
	stdout := runtime.Factory.IOStreams.Out.(*bytes.Buffer).String()
	if strings.Contains(stdout, `"mentions"`) ||
		!strings.Contains(stdout, `"@_user_1"`) ||
		!strings.Contains(stdout, `"ou_alpha"`) ||
		!strings.Contains(stdout, `"mention_result"`) ||
		!strings.Contains(stdout, `"status": "complete"`) {
		t.Fatalf("stdout did not expose only reconciled mention evidence:\n%s", stdout)
	}
}

func TestMentionReplyExecuteExposesOnlyAcceptedAllResult(t *testing.T) {
	var requestBody map[string]interface{}
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &requestBody); err != nil {
			t.Fatal(err)
		}
		return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message_id":  "om_reply",
				"chat_id":     "oc_result",
				"create_time": "1722168000000",
				"mentions": []interface{}{
					map[string]interface{}{"key": "@_all", "id": "all", "id_type": "user_id"},
				},
			},
		}), nil
	}))
	runtime.Cmd = newMentionCommandRuntime(t, ImMessagesReply, []string{
		"--message-id", "om_parent",
		"--text", "hello",
		"--mention-all",
	}).Cmd
	runtime.Format = "json"

	if err := ImMessagesReply.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	content, _ := requestBody["content"].(string)
	var payload map[string]string
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatalf("request content is not JSON: %v", err)
	}
	if payload["text"] != `<at user_id="all"></at> hello` {
		t.Fatalf("request body = %#v, want mention-all", requestBody)
	}
	stdout := runtime.Factory.IOStreams.Out.(*bytes.Buffer).String()
	if strings.Contains(stdout, `"mentions"`) ||
		strings.Contains(stdout, `"@_all"`) ||
		!strings.Contains(stdout, `"mention_result"`) ||
		!strings.Contains(stdout, `"status": "accepted_unverified"`) {
		t.Fatalf("stdout did not expose only accepted-unverified @all result:\n%s", stdout)
	}
}

func TestMessageSendWithoutMentionFlagsKeepsHistoricalOutput(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message_id":  "om_result",
				"chat_id":     "oc_result",
				"create_time": "1722168000000",
				"mentions": []interface{}{
					map[string]interface{}{"key": "@_user_1", "id": "ou_alpha", "id_type": "open_id"},
				},
			},
		}), nil
	}))
	runtime.Cmd = newMentionCommandRuntime(t, ImMessagesSend, []string{
		"--chat-id", "oc_test",
		"--text", "hello",
	}).Cmd
	runtime.Format = "json"

	if err := ImMessagesSend.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	stdout := runtime.Factory.IOStreams.Out.(*bytes.Buffer).String()
	if strings.Contains(stdout, `"mentions"`) || strings.Contains(stdout, `"mention_result"`) {
		t.Fatalf("no-flag output changed historical shape:\n%s", stdout)
	}
}

func newMentionCommandRuntime(t *testing.T, shortcut common.Shortcut, args []string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, flag := range shortcut.Flags {
		switch flag.Type {
		case "bool":
			cmd.Flags().Bool(flag.Name, flag.Default == "true", "")
		case "string_slice":
			cmd.Flags().StringSlice(flag.Name, nil, "")
		default:
			cmd.Flags().String(flag.Name, flag.Default, "")
		}
	}
	if shortcut.PostMount != nil {
		shortcut.PostMount(cmd)
	}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func assertMentionDryRunBody(t *testing.T, dryRun *common.DryRunAPI, msgType, wantText string) {
	t.Helper()
	raw, err := json.Marshal(dryRun)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.API) != 1 {
		t.Fatalf("api calls = %#v", result.API)
	}
	if result.API[0].Body["msg_type"] != msgType {
		t.Fatalf("msg_type = %#v, want %q", result.API[0].Body["msg_type"], msgType)
	}
	content, _ := result.API[0].Body["content"].(string)
	var payload map[string]string
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["text"] != wantText {
		t.Fatalf("text = %q, want %q", payload["text"], wantText)
	}
}
