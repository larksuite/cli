// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"encoding/json"
	"errors"
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

func TestBotFanoutTerminalContextOverridesPartialSuccess(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantSubtype errs.Subtype
	}{
		{name: "cancelled", err: context.Canceled, wantSubtype: errs.SubtypeNetworkTransport},
		{name: "deadline", err: context.DeadlineExceeded, wantSubtype: errs.SubtypeNetworkTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := []botFanoutResult{
				{Index: 0, Query: "会议", Bots: []searchBot{{OpenID: "ou_a"}}},
				botFanoutErrorResult(1, "日报", tt.err),
			}
			_, err := buildBotFanoutResponse([]string{"会议", "日报"}, results)
			if err == nil {
				t.Fatal("terminal context error must fail the batch after a partial success")
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("error must preserve %v as its cause: %v", tt.err, err)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != tt.wantSubtype {
				t.Fatalf("problem: got %+v, want network/%s", problem, tt.wantSubtype)
			}
		})
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
		searchBot:    searchBot{OpenID: "ou_a", Name: "会议助手", Description: strings.Repeat("长", 80)},
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

func TestBotFanoutPanicFailsBatch(t *testing.T) {
	factory, stdout, stderr, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	panicCause := errors.New("synthetic test panic")

	boom := botSearchStub(botSearchURL, "")
	boom.BodyFilter = func(b []byte) bool { return strings.Contains(string(b), `"boom"`) }
	boom.OnMatch = func(req *http.Request) { panic(panicCause) }
	registry.Register(boom)

	okStub := botSearchStub(botSearchURL, "")
	okStub.Reusable = true
	registry.Register(okStub)

	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--queries", "ok,boom,fine", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err == nil {
		t.Fatal("a panicking query must fail the batch")
	}
	if !errors.Is(err, panicCause) {
		t.Fatalf("panic cause must be preserved: %v", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("problem: got %+v, want internal/%s", problem, errs.SubtypeUnknown)
	}
	if stdout.Len() != 0 {
		t.Fatalf("terminal failure must not write a success envelope: %s", stdout.String())
	}
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

func TestBotFanoutPartialFailureKeepsNoticeAndSucceeds(t *testing.T) {
	factory, stdout, _, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())

	broken := botSearchStub(botSearchURL, "")
	broken.BodyFilter = func(b []byte) bool { return strings.Contains(string(b), `"日报"`) }
	broken.Status = 500
	broken.Body = map[string]interface{}{"reason": "boom"}
	registry.Register(broken)

	okStub := botSearchStub(botSearchURL, "")
	okStub.Reusable = true
	registry.Register(okStub)

	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--queries", "会议,日报", "--format", "json", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("one failing query must not fail the batch: %v", err)
	}

	var envelope struct {
		Data botFanoutResponse `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("response JSON: %v\n%s", err, stdout.String())
	}

	const wantNotice = "The query is too long and has been truncated to the first 50 characters for search."
	// Assert the notice itself, not just that some row survived: the surviving
	// query's server remark has to reach the caller both at the top level and in
	// its own sidecar entry.
	if envelope.Data.Notice != wantNotice {
		t.Errorf("top-level notice: got %q, want %q", envelope.Data.Notice, wantNotice)
	}
	if len(envelope.Data.Queries) != 2 {
		t.Fatalf("both queries must be enumerated: %+v", envelope.Data.Queries)
	}
	if envelope.Data.Queries[0].Notice != wantNotice {
		t.Errorf("surviving query notice: got %q, want %q", envelope.Data.Queries[0].Notice, wantNotice)
	}
	if envelope.Data.Queries[0].Error != "" {
		t.Errorf("surviving query must carry no error: %q", envelope.Data.Queries[0].Error)
	}
	if !strings.Contains(envelope.Data.Queries[1].Error, "500") {
		t.Errorf("failed query must carry the upstream status: %q", envelope.Data.Queries[1].Error)
	}
	// Only the surviving query contributes rows.
	if len(envelope.Data.Bots) != 1 || envelope.Data.Bots[0].MatchedQuery != "会议" {
		t.Fatalf("bots: %+v", envelope.Data.Bots)
	}
}

func TestBotFanoutCSVCarriesMatchedQueryAndSummary(t *testing.T) {
	factory, stdout, stderr, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	stub := botSearchStub(botSearchURL, "")
	stub.Reusable = true
	registry.Register(stub)

	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--queries", "会议,日报", "--format", "csv", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(stdout.String(), "matched_query") {
		t.Errorf("csv must expose matched_query so rows can be traced to a keyword: %s", stdout.String())
	}
	// csv is in the summary format set, so the batch counters belong on stderr.
	if !strings.Contains(stderr.String(), "2 queries, 2 total matches") || !strings.Contains(stderr.String(), "0 failed") {
		t.Errorf("stderr summary must report the batch counters: %s", stderr.String())
	}
	if strings.Contains(stderr.String(), "total bots") {
		t.Errorf("summary must count matches rather than imply unique bots: %s", stderr.String())
	}
}

func TestBotFanoutNDJSONKeepsStdoutClean(t *testing.T) {
	factory, stdout, stderr, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
	stub := botSearchStub(botSearchURL, "")
	stub.Reusable = true
	registry.Register(stub)

	err := mountAndRun(t, ContactSearchBot, []string{
		"+search-bot", "--queries", "会议,日报", "--format", "ndjson", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	// ndjson is a machine format outside the summary set: every stdout line must
	// parse, and the counters must not be mixed in.
	for i, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		var row map[string]interface{}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("stdout line %d is not JSON: %q", i, line)
		}
	}
	if strings.Contains(stderr.String(), "queries,") {
		t.Errorf("ndjson must not emit the summary line: %s", stderr.String())
	}
}

func TestBotFanoutCancelledContextFailsEveryQuery(t *testing.T) {
	results := make([]botFanoutResult, 0, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i, q := range []string{"会议", "日报"} {
		results = append(results, runOneBotQuery(ctx, nil, i, q, nil))
	}
	for _, r := range results {
		if r.ErrMsg == "" {
			t.Fatalf("a cancelled context must short-circuit before the request: %+v", r)
		}
	}
	_, err := buildBotFanoutResponse([]string{"会议", "日报"}, results)
	if err == nil {
		t.Fatal("all queries cancelled must surface as an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation cause must be preserved: %v", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("problem: got %+v, want network/%s", problem, errs.SubtypeNetworkTransport)
	}
}

func TestBotFanoutDryRunPreviewsOneRequestPerKeyword(t *testing.T) {
	cmd := newBotSearchTestCommand()
	setBotSearchFlag(t, cmd, "queries", "会议, 日报 ,会议")
	setBotSearchFlag(t, cmd, "chat-ids", "oc_a")
	setBotSearchFlag(t, cmd, "has-chatted", "true")
	runtime := common.TestNewRuntimeContext(cmd, botSearchDefaultConfig())

	raw, err := json.Marshal(ContactSearchBot.DryRun(context.Background(), runtime))
	if err != nil {
		t.Fatalf("marshal dry-run: %v", err)
	}
	var preview struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
			Body   struct {
				Query  string `json:"query"`
				Filter *struct {
					ChatIDs    []string `json:"chat_ids"`
					HasChatter bool     `json:"has_chatter"`
				} `json:"filter"`
			} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(raw, &preview); err != nil {
		t.Fatalf("decode dry-run: %v\n%s", err, raw)
	}

	// Deduped, so the repeated keyword previews once — the preview has to match
	// the requests Execute would actually issue.
	if len(preview.API) != 2 {
		t.Fatalf("expected one previewed request per deduped keyword, got %d: %s", len(preview.API), raw)
	}
	seen := make([]string, 0, len(preview.API))
	for i, call := range preview.API {
		if call.Method != "POST" || call.URL != botSearchURL {
			t.Errorf("api[%d]: got %s %s", i, call.Method, call.URL)
		}
		if call.Params["page_size"] != float64(20) {
			t.Errorf("api[%d] page_size: %v", i, call.Params["page_size"])
		}
		if _, ok := call.Params["page_token"]; ok {
			t.Errorf("api[%d] must not preview a page_token: %v", i, call.Params)
		}
		// The filter rides along with every keyword, not just the first.
		if call.Body.Filter == nil || !call.Body.Filter.HasChatter ||
			len(call.Body.Filter.ChatIDs) != 1 || call.Body.Filter.ChatIDs[0] != "oc_a" {
			t.Errorf("api[%d] filter: %+v", i, call.Body.Filter)
		}
		seen = append(seen, call.Body.Query)
	}
	if fmt.Sprint(seen) != fmt.Sprint([]string{"会议", "日报"}) {
		t.Errorf("previewed keywords: got %v, want [会议 日报]", seen)
	}
}

// The summary counts how many queries failed but never says which or why, and
// only json carries queries[].error. Without a per-query line on stderr an agent
// reading csv sees "1 failed" and cannot recover the keyword or the reason.
func TestBotFanoutFailedQueryIsNamedOnStderr(t *testing.T) {
	for _, format := range []string{"csv", "table", "pretty", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			factory, stdout, stderr, registry := cmdutil.TestFactory(t, botSearchDefaultConfig())
			broken := botSearchStub(botSearchURL, "")
			broken.BodyFilter = func(b []byte) bool { return strings.Contains(string(b), `"日报"`) }
			broken.Status = 500
			broken.Body = map[string]interface{}{"reason": "boom"}
			registry.Register(broken)
			okStub := botSearchStub(botSearchURL, "")
			okStub.Reusable = true
			registry.Register(okStub)

			if err := mountAndRun(t, ContactSearchBot, []string{
				"+search-bot", "--queries", "会议,日报", "--format", format, "--as", "user",
			}, factory, stdout); err != nil {
				t.Fatalf("one failing query must not fail the batch: %v", err)
			}
			for _, want := range []string{"日报", "500"} {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("%s: stderr must name the failed query and its reason (missing %q)\nstderr:\n%s",
						format, want, stderr.String())
				}
			}
		})
	}
}
