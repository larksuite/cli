// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newBotSearchTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("chat-ids", "", "")
	cmd.Flags().Bool("has-chatted", false, "")
	cmd.Flags().Int("page-size", 20, "")
	return cmd
}

func botSearchDefaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test", AppSecret: "test", Brand: core.BrandFeishu,
		UserOpenId: "ou_self",
	}
}

func setBotSearchFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("set --%s=%q: %v", name, value, err)
	}
}

func assertBotSearchValidationProblem(t *testing.T, err error, wantParam string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem: got %s/%s, want %s/%s", problem.Category, problem.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if validationErr.Param != wantParam {
		t.Fatalf("param: got %q, want %q", validationErr.Param, wantParam)
	}
}

func TestValidateBotSearchErrors(t *testing.T) {
	chatIDs := make([]string, 101)
	for i := range chatIDs {
		chatIDs[i] = fmt.Sprintf("chat_%03d", i)
	}

	tests := []struct {
		name        string
		flags       map[string]string
		wantParam   string
		wantMessage string
	}{
		{
			name:        "query missing",
			wantParam:   "--query",
			wantMessage: "--query is required: --chat-ids and --has-chatted only narrow a keyword search, they cannot enumerate bots on their own (the API returns an empty list for filter-only requests)",
		},
		{
			name:        "query over 50 characters",
			flags:       map[string]string{"query": strings.Repeat("中", 51)},
			wantParam:   "--query",
			wantMessage: "--query: length must be between 1 and 50 characters",
		},
		{
			name:        "chat ids parse empty",
			flags:       map[string]string{"query": "x", "chat-ids": " , , "},
			wantParam:   "--chat-ids",
			wantMessage: "--chat-ids: no valid chat_id parsed from \", ,\" (separate entries with ',')",
		},
		{
			name:        "over 100 chat ids",
			flags:       map[string]string{"query": "x", "chat-ids": strings.Join(chatIDs, ",")},
			wantParam:   "--chat-ids",
			wantMessage: "--chat-ids: must be at most 100 entries",
		},
		{
			name:        "invalid chat id",
			flags:       map[string]string{"query": "x", "chat-ids": "bad"},
			wantParam:   "--chat-ids",
			wantMessage: "invalid chat ID format, should start with 'oc_' (e.g., oc_abc123)",
		},
		{
			name:        "has chatted false",
			flags:       map[string]string{"query": "x", "has-chatted": "false"},
			wantParam:   "--has-chatted",
			wantMessage: "--has-chatted: pass the flag to enable the filter; omit it to disable filtering (=false is rejected to prevent silent wrong results)",
		},
		{
			name:        "page size below one",
			flags:       map[string]string{"query": "x", "page-size": "0"},
			wantParam:   "--page-size",
			wantMessage: "--page-size: must be between 1 and 30",
		},
		{
			name:        "page size over 30",
			flags:       map[string]string{"query": "x", "page-size": "31"},
			wantParam:   "--page-size",
			wantMessage: "--page-size: must be between 1 and 30",
		},
		{
			name:        "chat ids without query",
			flags:       map[string]string{"chat-ids": "oc_a"},
			wantParam:   "--query",
			wantMessage: "--query is required: --chat-ids and --has-chatted only narrow a keyword search, they cannot enumerate bots on their own (the API returns an empty list for filter-only requests)",
		},
		{
			name:        "has chatted without query",
			flags:       map[string]string{"has-chatted": "true"},
			wantParam:   "--query",
			wantMessage: "--query is required: --chat-ids and --has-chatted only narrow a keyword search, they cannot enumerate bots on their own (the API returns an empty list for filter-only requests)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newBotSearchTestCommand()
			for name, value := range tt.flags {
				setBotSearchFlag(t, cmd, name, value)
			}
			runtime := common.TestNewRuntimeContext(cmd, botSearchDefaultConfig())
			err := validateBotSearch(runtime)
			assertBotSearchValidationProblem(t, err, tt.wantParam)
			if err.Error() != tt.wantMessage {
				t.Fatalf("message: got %q, want %q", err.Error(), tt.wantMessage)
			}
		})
	}
}

func TestValidateBotSearchPassingCases(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
	}{
		{name: "query only", flags: map[string]string{"query": "x"}},
		{name: "query and chat ids", flags: map[string]string{"query": "x", "chat-ids": "oc_a,oc_b"}},
		{name: "query and has chatted", flags: map[string]string{"query": "x", "has-chatted": "true"}},
		{name: "all filters", flags: map[string]string{"query": "x", "chat-ids": "oc_a,oc_b", "has-chatted": "true"}},
		{name: "page size upper boundary", flags: map[string]string{"query": "x", "page-size": "30"}},
		// An explicitly blank string flag reads as "no filter", matching how
		// +search-user treats --user-ids / --queries. Only a non-blank value that
		// parses to zero entries is an error.
		{name: "blank chat ids ignored", flags: map[string]string{"query": "x", "chat-ids": ""}},
		{name: "whitespace chat ids ignored", flags: map[string]string{"query": "x", "chat-ids": "   "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newBotSearchTestCommand()
			for name, value := range tt.flags {
				setBotSearchFlag(t, cmd, name, value)
			}
			runtime := common.TestNewRuntimeContext(cmd, botSearchDefaultConfig())
			if err := validateBotSearch(runtime); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateBotSearchQueryRuneBoundary(t *testing.T) {
	for _, tt := range []struct {
		name      string
		query     string
		wantError bool
	}{
		{name: "50 CJK characters", query: strings.Repeat("中", 50)},
		{name: "51 CJK characters", query: strings.Repeat("中", 51), wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newBotSearchTestCommand()
			setBotSearchFlag(t, cmd, "query", tt.query)
			runtime := common.TestNewRuntimeContext(cmd, botSearchDefaultConfig())
			err := validateBotSearch(runtime)
			if tt.wantError {
				assertBotSearchValidationProblem(t, err, "--query")
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildBotSearchBody(t *testing.T) {
	tests := []struct {
		name     string
		flags    map[string]string
		wantJSON string
	}{
		{name: "query only", flags: map[string]string{"query": "x"}, wantJSON: `{"query":"x"}`},
		{name: "chat ids", flags: map[string]string{"query": "x", "chat-ids": "oc_a,oc_b"}, wantJSON: `{"query":"x","filter":{"chat_ids":["oc_a","oc_b"]}}`},
		{name: "chat id URL normalized", flags: map[string]string{"query": "x", "chat-ids": "https://example.feishu.cn/foo/oc_a,oc_b"}, wantJSON: `{"query":"x","filter":{"chat_ids":["oc_a","oc_b"]}}`},
		{name: "has chatted", flags: map[string]string{"query": "x", "has-chatted": "true"}, wantJSON: `{"query":"x","filter":{"has_chatter":true}}`},
		{name: "all fields", flags: map[string]string{"query": "x", "chat-ids": "oc_a,oc_b", "has-chatted": "true"}, wantJSON: `{"query":"x","filter":{"chat_ids":["oc_a","oc_b"],"has_chatter":true}}`},
		// A blank --chat-ids must not materialize an empty filter object.
		{name: "blank chat ids omit filter", flags: map[string]string{"query": "x", "chat-ids": "   "}, wantJSON: `{"query":"x"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newBotSearchTestCommand()
			for name, value := range tt.flags {
				setBotSearchFlag(t, cmd, name, value)
			}
			runtime := common.TestNewRuntimeContext(cmd, botSearchDefaultConfig())
			body, err := buildBotSearchBody(runtime)
			if err != nil {
				t.Fatalf("build body: %v", err)
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			if string(raw) != tt.wantJSON {
				t.Fatalf("body: got %s, want %s", raw, tt.wantJSON)
			}
		})
	}
}

func TestParseBotDisplayInfo(t *testing.T) {
	tests := []struct {
		name            string
		raw             string
		openID          string
		wantName        string
		wantDescription string
		wantSegments    []string
	}{
		{name: "single live match", raw: "<h>会议助手</h>\n推送未接会议消息提醒", openID: "ou_a", wantName: "会议助手", wantDescription: "推送未接会议消息提醒", wantSegments: []string{"会议助手"}},
		{name: "multiple live matches", raw: "<h>会议</h>室<h>助手</h>\n你的专属会议室小管家", openID: "ou_b", wantName: "会议室助手", wantDescription: "你的专属会议室小管家", wantSegments: []string{"会议", "助手"}},
		{name: "empty live description", raw: "尚磊的智能<h>助手</h>\n", openID: "ou_c", wantName: "尚磊的智能助手", wantSegments: []string{"助手"}},
		{name: "mid-name live match", raw: "红黑榜小<h>助</h>手\n每天定时发送阻塞红黑榜Bug看板", openID: "ou_d", wantName: "红黑榜小助手", wantDescription: "每天定时发送阻塞红黑榜Bug看板", wantSegments: []string{"助"}},
		{name: "no newline", raw: "会议助手", openID: "ou_e", wantName: "会议助手", wantSegments: []string{}},
		{name: "empty", raw: "", openID: "ou_f", wantName: "ou_f", wantSegments: []string{}},
		{name: "fallback line", raw: "\n\n真名", openID: "ou_g", wantName: "真名", wantSegments: []string{}},
		// A blank first line must not make the description echo the name back and
		// swallow the real description on the line after it.
		{name: "blank first line keeps description", raw: "\n真名\n简介", openID: "ou_h", wantName: "真名", wantDescription: "简介", wantSegments: []string{}},
		{name: "blank first line without description", raw: "\n真名", openID: "ou_i", wantName: "真名", wantSegments: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, description, segments := parseBotDisplayInfo(tt.raw, tt.openID)
			if name != tt.wantName || description != tt.wantDescription {
				t.Fatalf("name/description: got %q/%q, want %q/%q", name, description, tt.wantName, tt.wantDescription)
			}
			if segments == nil {
				t.Fatal("match segments must be an empty slice, not nil")
			}
			if fmt.Sprint(segments) != fmt.Sprint(tt.wantSegments) {
				t.Fatalf("match segments: got %v, want %v", segments, tt.wantSegments)
			}
		})
	}
}

func TestProjectBotsMapsEveryField(t *testing.T) {
	data := &botSearchAPIData{Items: []botSearchAPIItem{
		{
			ID:          "ou_with_chat",
			DisplayInfo: "<h>会议助手</h>\n提醒助手",
			MetaData: botSearchAPIMeta{
				TenantID: "1", EnableJoinGroup: true, ChatID: "oc_p2p", IsAgent: true,
			},
		},
		{
			ID:          "ou_without_chat",
			DisplayInfo: "无会话机器人",
			MetaData:    botSearchAPIMeta{TenantID: "1"},
		},
	}}

	bots := projectBots(data)
	if len(bots) != 2 {
		t.Fatalf("bots: got %d, want 2", len(bots))
	}
	first := bots[0]
	if first.OpenID != "ou_with_chat" || first.Name != "会议助手" || first.Description != "提醒助手" ||
		first.P2PChatID != "oc_p2p" || !first.HasChatted || !first.EnableJoinGroup || !first.IsAgent || first.TenantID != "1" ||
		fmt.Sprint(first.MatchSegments) != "[会议助手]" {
		t.Fatalf("first bot mapping: %+v", first)
	}
	second := bots[1]
	if second.P2PChatID != "" || second.HasChatted {
		t.Fatalf("second bot chat fields: %+v", second)
	}
	raw, err := json.Marshal(searchBotResponse{Bots: bots})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(raw), `"p2p_chat_id":""`) {
		t.Fatalf("empty p2p_chat_id must be omitted: %s", raw)
	}
}

func TestProjectBotsEmptySerializesAsArray(t *testing.T) {
	bots := projectBots(&botSearchAPIData{Items: []botSearchAPIItem{}})
	if bots == nil {
		t.Fatal("bots must be an empty slice, not nil")
	}
	raw, err := json.Marshal(searchBotResponse{Bots: bots})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if string(raw) != `{"bots":[],"has_more":false}` {
		t.Fatalf("response: got %s", raw)
	}
}

func botSearchStub(url string, pageToken string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    url,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"notice":     "The query is too long and has been truncated to the first 50 characters for search.",
				"has_more":   true,
				"page_token": pageToken,
				"items": []interface{}{
					map[string]interface{}{
						"id":           "ou_bot",
						"display_info": "<h>会议助手</h>\n推送未接会议消息提醒",
						"meta_data": map[string]interface{}{
							"tenant_id": "1", "enable_join_group": true, "chat_id": "oc_p2p", "is_agent": false,
						},
					},
				},
			},
		},
	}
}

func TestBotSearchIntegrationRequestAndResponsePassThrough(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	stub := botSearchStub(botSearchURL+"?page_size=25", "cursor_out")
	registry.Register(stub)

	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--query", "助手", "--chat-ids", "oc_a,oc_b", "--has-chatted",
		"--page-size", "25", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var requestBody map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &requestBody); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if requestBody["query"] != "助手" {
		t.Fatalf("request query: got %v", requestBody["query"])
	}
	filter, ok := requestBody["filter"].(map[string]interface{})
	if !ok || filter["has_chatter"] != true || fmt.Sprint(filter["chat_ids"]) != "[oc_a oc_b]" {
		t.Fatalf("request filter: %#v", requestBody["filter"])
	}

	var envelope struct {
		Data searchBotResponse `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("response JSON: %v\n%s", err, stdout.String())
	}
	if envelope.Data.Notice != "The query is too long and has been truncated to the first 50 characters for search." || !envelope.Data.HasMore {
		t.Fatalf("response pass-through: %+v", envelope.Data)
	}
	if len(envelope.Data.Bots) != 1 || envelope.Data.Bots[0].OpenID != "ou_bot" || envelope.Data.Bots[0].P2PChatID != "oc_p2p" {
		t.Fatalf("bots: %+v", envelope.Data.Bots)
	}
	registry.Verify(t)
}

func TestBotSearchIntegrationNeverSurfacesPageToken(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	// The stub returns a token; the envelope must still not carry one, matching
	// +search-user, which decodes page_token and drops it.
	registry.Register(botSearchStub(botSearchURL+"?page_size=20", "cursor_out"))

	err := mountAndRun(t, ContactSearchBot, []string{"+search-bot", "--query", "助手", "--format", "json", "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("response JSON: %v", err)
	}
	data := envelope["data"].(map[string]interface{})
	if _, ok := data["page_token"]; ok {
		t.Fatalf("page_token must never be surfaced: %v", data)
	}
}

func TestBotSearchPrettyOutputAndPaginationHint(t *testing.T) {
	factory, stdout, stderr, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	registry.Register(botSearchStub(botSearchURL+"?page_size=20", "cursor_out"))

	err := mountAndRun(t, ContactSearchBot, []string{"+search-bot", "--query", "助手", "--format", "pretty", "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, column := range []string{"name", "description", "has_chatted", "is_agent", "enable_join_group", "open_id"} {
		if !strings.Contains(stdout.String(), column) {
			t.Errorf("pretty output missing %q: %s", column, stdout.String())
		}
	}
	for _, genericField := range []string{"bots", "has_more", "notice", "tenant_id", "p2p_chat_id", "match_segments"} {
		if strings.Contains(stdout.String(), genericField) {
			t.Errorf("pretty output exposed %q: %s", genericField, stdout.String())
		}
	}
	wantHint := "\nhint: more matches exist; narrow the search (e.g. add --chat-ids, --has-chatted, or a more specific --query)\n"
	if stderr.String() != wantHint {
		t.Fatalf("pretty stderr: got %q, want %q", stderr.String(), wantHint)
	}
}

func TestBotSearchTableUsesGenericFormatterLikeSearchUser(t *testing.T) {
	factory, stdout, stderr, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	registry.Register(botSearchStub(botSearchURL+"?page_size=20", "cursor_out"))

	err := mountAndRun(t, ContactSearchBot, []string{"+search-bot", "--query", "助手", "--format", "table", "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, field := range []string{"open_id", "tenant_id", "p2p_chat_id", "match_segments"} {
		if !strings.Contains(stdout.String(), field) {
			t.Errorf("table output missing %q: %s", field, stdout.String())
		}
	}
	wantHint := "\nhint: more matches exist; narrow the search (e.g. add --chat-ids, --has-chatted, or a more specific --query)\n"
	if stderr.String() != wantHint {
		t.Fatalf("table stderr: got %q, want %q", stderr.String(), wantHint)
	}
}

func TestBotSearchCSVAndNDJSONExposeFullFieldsWithoutPaginationHint(t *testing.T) {
	for _, format := range []string{"csv", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			factory, stdout, stderr, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
			registry.Register(botSearchStub(botSearchURL+"?page_size=20", "cursor_out"))

			err := mountAndRun(t, ContactSearchBot, []string{"+search-bot", "--query", "助手", "--format", format, "--as", "user"}, factory, stdout)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			for _, field := range []string{"open_id", "tenant_id", "p2p_chat_id", "match_segments"} {
				if !strings.Contains(stdout.String(), field) {
					t.Errorf("%s output missing %q: %s", format, field, stdout.String())
				}
			}
			if stderr.Len() != 0 {
				t.Fatalf("%s stderr: got %q, want empty", format, stderr.String())
			}
		})
	}
}

func TestBotSearchPrettyEmptyResult(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	registry.Register(&httpmock.Stub{
		Method: "POST",
		URL:    botSearchURL + "?page_size=20",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"items": []interface{}{}, "has_more": false},
		},
	})

	err := mountAndRun(t, ContactSearchBot, []string{"+search-bot", "--query", "none", "--format", "pretty", "--as", "user"}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(stdout.String(), "No bots found.") {
		t.Fatalf("pretty output: %q", stdout.String())
	}
}

func TestBotSearchDryRunMirrorsRequest(t *testing.T) {
	factory, stdout, _, _ := cmdutil.TestFactory(t, botSearchDefaultConfig())
	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--query", "助手", "--chat-ids", "oc_a", "--has-chatted",
		"--page-size", "25", "--dry-run", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var envelope struct {
		Data struct {
			API []struct {
				Method string                 `json:"method"`
				URL    string                 `json:"url"`
				Params map[string]interface{} `json:"params"`
				Body   botSearchAPIRequest    `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("dry-run JSON: %v", err)
	}
	if len(envelope.Data.API) != 1 {
		t.Fatalf("api calls: got %d, want 1", len(envelope.Data.API))
	}
	call := envelope.Data.API[0]
	if call.Method != "POST" || call.URL != botSearchURL || call.Params["page_size"] != float64(25) {
		t.Fatalf("dry-run call: %+v", call)
	}
	if call.Body.Query != "助手" || call.Body.Filter == nil || fmt.Sprint(call.Body.Filter.ChatIDs) != "[oc_a]" || !call.Body.Filter.HasChatter {
		t.Fatalf("dry-run body: %+v", call.Body)
	}
}

func TestDecodeBotSearchAPIDataMarshalFailureTyped(t *testing.T) {
	_, err := decodeBotSearchAPIData(map[string]interface{}{"bad": func() {}})
	if err == nil {
		t.Fatal("expected marshal failure")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem: %+v, ok=%v", problem, ok)
	}
}
