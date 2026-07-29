// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestBotFanoutErrorResultNilErrorIsSuccess(t *testing.T) {
	r := botFanoutErrorResult(3, "会议助手", nil)
	if r.ErrMsg != "" || r.Err != nil {
		t.Fatalf("nil error must stay a success result: %+v", r)
	}
	if r.Index != 3 || r.Query != "会议助手" {
		t.Fatalf("index/query must survive: %+v", r)
	}
}

func TestBotFanoutAssembleOrderAndShape(t *testing.T) {
	results := []botFanoutResult{
		{Index: 1, Query: "日报", Bots: []searchBot{{OpenID: "ou_b"}}, HasMore: true},
		{Index: 0, Query: "会议", Bots: []searchBot{{OpenID: "ou_a1"}, {OpenID: "ou_a2"}}},
		{Index: 2, Query: "审批", ErrMsg: "API 1: nope"},
	}
	resp, err := buildBotFanoutResponse([]string{"会议", "日报", "审批"}, results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Results are emitted in query order even though the workers finished out of
	// order, and a failed query contributes no rows.
	wantRows := []struct {
		openID, matched string
	}{{"ou_a1", "会议"}, {"ou_a2", "会议"}, {"ou_b", "日报"}}
	if len(resp.Bots) != len(wantRows) {
		t.Fatalf("bots length: got %d, want %d", len(resp.Bots), len(wantRows))
	}
	for i, w := range wantRows {
		if resp.Bots[i].OpenID != w.openID || resp.Bots[i].MatchedQuery != w.matched {
			t.Errorf("bots[%d]: got %+v, want %s/%s", i, resp.Bots[i], w.openID, w.matched)
		}
	}

	want := []querySummary{
		{Query: "会议"},
		{Query: "日报", HasMore: true},
		{Query: "审批", Error: "API 1: nope"},
	}
	if len(resp.Queries) != len(want) {
		t.Fatalf("queries length: got %d, want %d (every query is enumerated)", len(resp.Queries), len(want))
	}
	for i, w := range want {
		if resp.Queries[i] != w {
			t.Errorf("queries[%d]: got %+v, want %+v", i, resp.Queries[i], w)
		}
	}
}

func TestBotFanoutAssembleAllFailedReturnsTypedError(t *testing.T) {
	results := []botFanoutResult{
		{Index: 0, Query: "会议", ErrMsg: "API 99991663: rate limit", Err: errs.NewAPIError(errs.SubtypeRateLimit, "rate limit").WithCode(99991663)},
		{Index: 1, Query: "日报", ErrMsg: "HTTP 500 Internal Server Error"},
	}
	_, err := buildBotFanoutResponse([]string{"会议", "日报"}, results)
	if err == nil {
		t.Fatal("expected an error when every query fails")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed problem, got %T: %v", err, err)
	}
	// The first failure's classification must survive, so the caller can tell a
	// rate limit apart from a transport fault.
	if problem.Code != 99991663 || problem.Subtype != errs.SubtypeRateLimit {
		t.Errorf("problem: got %d/%s, want 99991663/%s", problem.Code, problem.Subtype, errs.SubtypeRateLimit)
	}
	// Agents grep the count and the first failure out of this message.
	for _, want := range []string{"all 2 queries failed", "rate limit"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message must contain %q; got %v", want, err)
		}
	}
}

func TestBotFanoutAssemblePartialFailureSucceeds(t *testing.T) {
	results := []botFanoutResult{
		{Index: 0, Query: "会议", Bots: []searchBot{{OpenID: "ou_a"}}},
		{Index: 1, Query: "日报", ErrMsg: "API 1: nope"},
	}
	resp, err := buildBotFanoutResponse([]string{"会议", "日报"}, results)
	if err != nil {
		t.Fatalf("one failure out of two must not fail the call: %v", err)
	}
	if len(resp.Bots) != 1 || resp.Queries[1].Error == "" {
		t.Fatalf("partial failure shape: %+v", resp)
	}
}

func TestBotFanoutResponseHasNoTopLevelHasMore(t *testing.T) {
	resp, err := buildBotFanoutResponse([]string{"会议"}, []botFanoutResult{{Index: 0, Query: "会议", HasMore: true}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// has_more is per query in the sidecar; a single top-level flag would hide
	// which keyword was truncated.
	if _, ok := envelope["has_more"]; ok {
		t.Fatalf("fanout must not surface a top-level has_more: %s", raw)
	}
	if !envelope["queries"].([]interface{})[0].(map[string]interface{})["has_more"].(bool) {
		t.Fatalf("per-query has_more lost: %s", raw)
	}
}

func TestBotFanoutEmptyBotsSerializesAsArray(t *testing.T) {
	resp, err := buildBotFanoutResponse([]string{"会议"}, []botFanoutResult{{Index: 0, Query: "会议"}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"bots":[]`) {
		t.Fatalf("empty bots must serialize as [], not null: %s", raw)
	}
}

func TestPrettyBotFanoutRowsLeadWithMatchedQuery(t *testing.T) {
	rows := prettyBotFanoutRows([]fanoutBot{{
		searchBot:    searchBot{OpenID: "ou_a", Name: "会议助手", Description: strings.Repeat("长", 80), HasChatted: true},
		MatchedQuery: "会议",
	}})
	if len(rows) != 1 {
		t.Fatalf("rows: %d", len(rows))
	}
	if rows[0]["matched_query"] != "会议" {
		t.Errorf("matched_query missing: %+v", rows[0])
	}
	if got := rows[0]["description"].(string); len([]rune(got)) > 51 {
		t.Errorf("description must be truncated like the single-search table: %d runes", len([]rune(got)))
	}
}

func TestBotFanoutValidationRejectsQueryAndQueriesTogether(t *testing.T) {
	cmd := newBotSearchTestCommand()
	setBotSearchFlag(t, cmd, "query", "会议")
	setBotSearchFlag(t, cmd, "queries", "会议,日报")
	runtime := common.TestNewRuntimeContext(cmd, botSearchDefaultConfig())

	err := validateBotSearch(runtime)
	if err == nil {
		t.Fatal("expected mutual-exclusion error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem: %+v ok=%v", problem, ok)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("message: %v", err)
	}
}

func TestBotFanoutValidationLimits(t *testing.T) {
	tests := []struct {
		name      string
		queries   string
		wantParam string
	}{
		{name: "nothing parses", queries: " , , ", wantParam: "--queries"},
		{name: "over the entry cap", queries: strings.TrimSuffix(strings.Repeat("q%d,", maxFanoutQueries+1), ","), wantParam: "--queries"},
		{name: "entry too long", queries: strings.Repeat("会", maxBotSearchQueryChars+1), wantParam: "--queries"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries := tt.queries
			if strings.Contains(queries, "%d") {
				parts := make([]string, 0, maxFanoutQueries+1)
				for i := 0; i <= maxFanoutQueries; i++ {
					parts = append(parts, fmt.Sprintf("q%d", i))
				}
				queries = strings.Join(parts, ",")
			}
			cmd := newBotSearchTestCommand()
			setBotSearchFlag(t, cmd, "queries", queries)
			runtime := common.TestNewRuntimeContext(cmd, botSearchDefaultConfig())
			assertBotSearchValidationProblem(t, validateBotSearch(runtime), tt.wantParam)
		})
	}
}

// --queries alone is enough: the single-search "--query is required" rule must not
// leak into fanout mode.
func TestBotFanoutValidationQueriesAloneIsValid(t *testing.T) {
	cmd := newBotSearchTestCommand()
	setBotSearchFlag(t, cmd, "queries", "会议助手,日报助手")
	runtime := common.TestNewRuntimeContext(cmd, botSearchDefaultConfig())
	if err := validateBotSearch(runtime); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBotFanoutFilterAppliedToEveryQuery(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	stub := botSearchStub(botSearchURL+"?page_size=20", "")
	stub.Reusable = true
	registry.Register(stub)

	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--queries", "会议,日报", "--has-chatted", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(stub.CapturedBodies) != 2 {
		t.Fatalf("expected one request per query, got %d", len(stub.CapturedBodies))
	}
	seen := make(map[string]bool, len(stub.CapturedBodies))
	for i, raw := range stub.CapturedBodies {
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("unmarshal req %d: %v", i, err)
		}
		seen[fmt.Sprint(body["query"])] = true
		filter, ok := body["filter"].(map[string]interface{})
		if !ok || filter["has_chatter"] != true {
			t.Fatalf("filter must ride along with every query: %#v", body)
		}
	}
	for _, q := range []string{"会议", "日报"} {
		if !seen[q] {
			t.Fatalf("query %q never issued; saw %v", q, seen)
		}
	}
}

func TestBotFanoutMatchedQueryFidelityAndDedup(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	dedupStub := botSearchStub(botSearchURL+"?page_size=20", "")
	dedupStub.Reusable = true
	registry.Register(dedupStub)

	// " 会议 " and "会议" collapse to one query; the duplicate must not double the
	// requests or the rows.
	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--queries", " 会议 ,会议", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var envelope struct {
		Data botFanoutResponse `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("response JSON: %v\n%s", err, stdout.String())
	}
	if len(envelope.Data.Queries) != 1 || envelope.Data.Queries[0].Query != "会议" {
		t.Fatalf("dedup failed: %+v", envelope.Data.Queries)
	}
	for _, bot := range envelope.Data.Bots {
		if bot.MatchedQuery != "会议" {
			t.Fatalf("matched_query fidelity: %+v", bot)
		}
	}
}

func TestBotFanoutConcurrencyCap(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())

	var inFlight, peak int32
	stub := botSearchStub(botSearchURL+"?page_size=20", "")
	stub.Reusable = true
	stub.OnMatch = func(req *http.Request) {
		cur := atomic.AddInt32(&inFlight, 1)
		defer atomic.AddInt32(&inFlight, -1)
		for {
			p := atomic.LoadInt32(&peak)
			if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	registry.Register(stub)

	queries := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--queries", strings.Join(queries, ","), "--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if peak > fanoutConcurrency {
		t.Errorf("concurrency peak = %d, want <= %d", peak, fanoutConcurrency)
	}
	if peak < 2 {
		t.Errorf("concurrency peak = %d, want >= 2 so the test actually observes parallelism", peak)
	}
}

func TestBotFanoutPanicIsContainedPerQuery(t *testing.T) {
	factory, stdout, stderr, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())

	boom := botSearchStub(botSearchURL, "")
	boom.BodyFilter = func(b []byte) bool { return strings.Contains(string(b), `"boom"`) }
	boom.OnMatch = func(req *http.Request) { panic("synthetic test panic") }
	registry.Register(boom)

	ok := botSearchStub(botSearchURL, "")
	ok.Reusable = true
	registry.Register(ok)

	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--queries", "ok,boom,fine", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("one panicking query must not bubble out of the batch; got %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("response JSON: %v\n%s", err, stdout.String())
	}
	queries := got["data"].(map[string]interface{})["queries"].([]interface{})
	failed := queries[1].(map[string]interface{})
	if msg, _ := failed["error"].(string); !strings.HasPrefix(msg, "internal error:") {
		t.Errorf("queries[1].error: want an 'internal error:' prefix, got %q", failed["error"])
	}
	// A recovered panic must not dump a stack trace at the user.
	for _, marker := range []string{"goroutine ", ".go:", "runtime."} {
		if strings.Contains(stderr.String(), marker) {
			t.Errorf("stderr leaked stack-trace marker %q: %s", marker, stderr.String())
		}
	}
}

func TestBotFanoutAllQueriesFailingExitsNonZero(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	registry.Register(&httpmock.Stub{
		Method:   "POST",
		URL:      botSearchURL,
		Reusable: true,
		Status:   500,
		Body:     map[string]interface{}{"reason": "boom"},
	})

	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--queries", "会议,日报", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err == nil {
		t.Fatal("every query failing must surface as a command error")
	}
	if _, ok := errs.ProblemOf(err); !ok {
		t.Fatalf("expected a typed problem, got %T: %v", err, err)
	}
	// The first failure's upstream status and the all-failed mode must both survive,
	// so a caller can classify instead of seeing a generic internal error.
	for _, want := range []string{"500", "all 2 queries failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message must contain %q; got %v", want, err)
		}
	}
}
