// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	maxFanoutQueries  = 20
	fanoutConcurrency = 5
)

// parseAndDedupQueries splits the raw CSV, trims whitespace, drops empty
// entries, and deduplicates case-sensitively while preserving first-occurrence
// order.
func parseAndDedupQueries(raw string) []string {
	parts := common.SplitCSV(raw)
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

type fanoutResult struct {
	Index   int
	Query   string
	Users   []searchUser
	HasMore bool
	ErrMsg  string // empty = success
	ErrCode int    // 0 = success or unknown; otherwise an HTTP status or Lark API code corresponding to the first error
}

// isFanoutSummaryFormat gates the per-fanout stderr summary line. Includes csv
// because that summary lives on stderr and never corrupts the csv stream on
// stdout — single-query mode keeps the narrower isHumanReadableFormat predicate
// for its refine hint, so adding csv here doesn't regress that path.
func isFanoutSummaryFormat(format string) bool {
	return format == "pretty" || format == "table" || format == "csv"
}

// runOneQuery converts every failure mode (transport, HTTP status, parse,
// API code) into an ErrMsg string instead of returning a Go error. The
// fanout dispatcher (Task 6) relies on this so a single failed query never
// short-circuits the remaining workers.
func runOneQuery(ctx context.Context, runtime *common.RuntimeContext, index int, query string,
	filter *searchUserAPIFilter) fanoutResult {
	// Pre-check ctx so queued workers see cancellation before issuing a
	// request; in-flight workers continue until DoAPI returns.
	if err := ctx.Err(); err != nil {
		return fanoutResult{Index: index, Query: query, ErrMsg: err.Error()}
	}

	body := &searchUserAPIRequest{Query: query}
	if filter != nil {
		body.Filter = filter
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     searchUserURL,
		Body:        body,
		QueryParams: larkcore.QueryParams{"page_size": []string{strconv.Itoa(runtime.Int("page-size"))}},
	})
	if err != nil {
		return fanoutResult{Index: index, Query: query, ErrMsg: err.Error()}
	}
	if apiResp.StatusCode != http.StatusOK {
		body := strings.TrimSpace(string(apiResp.RawBody))
		const maxBody = 200
		if len(body) > maxBody {
			body = body[:maxBody] + "..."
		}
		msg := fmt.Sprintf("HTTP %d %s", apiResp.StatusCode, http.StatusText(apiResp.StatusCode))
		if body != "" {
			msg = fmt.Sprintf("%s: %s", msg, body)
		}
		return fanoutResult{Index: index, Query: query,
			ErrMsg:  msg,
			ErrCode: apiResp.StatusCode}
	}

	var resp searchUserAPIEnvelope
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return fanoutResult{Index: index, Query: query,
			ErrMsg: fmt.Sprintf("parse response failed: %v", err)}
	}
	if resp.Code != 0 {
		return fanoutResult{Index: index, Query: query,
			ErrMsg:  fmt.Sprintf("API %d: %s", resp.Code, resp.Msg),
			ErrCode: resp.Code}
	}

	users, hasMore := projectUsers(resp.Data, runtime.Str("lang"), runtime.Config.Brand)
	return fanoutResult{Index: index, Query: query, Users: users, HasMore: hasMore}
}

type fanoutUser struct {
	searchUser
	MatchedQuery string `json:"matched_query"`
}

type querySummary struct {
	Query   string `json:"query"`
	Error   string `json:"error,omitempty"`
	HasMore bool   `json:"has_more"`
}

type fanoutResponse struct {
	Users   []fanoutUser   `json:"users"`
	Queries []querySummary `json:"queries"`
}

// buildFanoutResponse walks results by Index (input order), flattens users[]
// with matched_query, lists every input in queries[] (including successes),
// and returns an error only when every query failed. The error wraps the
// first failing query's ErrMsg so the CLI exits non-zero on full failure.
func buildFanoutResponse(queries []string, results []fanoutResult) (*fanoutResponse, error) {
	indexed := make([]fanoutResult, len(queries))
	for _, r := range results {
		indexed[r.Index] = r
	}

	out := &fanoutResponse{
		Users:   make([]fanoutUser, 0),
		Queries: make([]querySummary, 0, len(queries)),
	}
	failed := 0
	var firstErrMsg, firstErrQuery string
	var firstErrCode int
	for i, r := range indexed {
		out.Queries = append(out.Queries, querySummary{
			Query:   queries[i],
			Error:   r.ErrMsg,
			HasMore: r.HasMore,
		})
		if r.ErrMsg != "" {
			failed++
			if firstErrMsg == "" {
				firstErrMsg = r.ErrMsg
				firstErrQuery = queries[i]
				firstErrCode = r.ErrCode
			}
			continue
		}
		for _, u := range r.Users {
			out.Users = append(out.Users, fanoutUser{searchUser: u, MatchedQuery: queries[i]})
		}
	}
	if failed == len(queries) && len(queries) > 0 {
		msg := fmt.Sprintf("all %d queries failed; first: %s (query=%q)",
			len(queries), firstErrMsg, firstErrQuery)
		// Only the HTTP-status / Lark-API-code branches in runOneQuery populate
		// ErrCode; transport, parse, panic, and ctx-canceled stay at 0. Code 0
		// means success in the Lark protocol, so don't pretend it's an API error
		// when we have nothing structured to report.
		if firstErrCode != 0 {
			return nil, output.ErrAPI(firstErrCode, msg, "")
		}
		return nil, output.ErrWithHint(output.ExitInternal, "fanout", msg, "")
	}
	return out, nil
}

func executeSearchUserFanout(ctx context.Context, runtime *common.RuntimeContext) error {
	queries := parseAndDedupQueries(runtime.Str("queries"))

	filter, err := buildFanoutFilter(runtime)
	if err != nil {
		return err
	}

	results := make([]fanoutResult, len(queries))
	var wg sync.WaitGroup
	sem := make(chan struct{}, fanoutConcurrency)

	for i, q := range queries {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, q string) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					results[i] = fanoutResult{
						Index:  i,
						Query:  q,
						ErrMsg: fmt.Sprintf("internal error: %v", r),
					}
				}
			}()
			results[i] = runOneQuery(ctx, runtime, i, q, filter)
		}(i, q)
	}
	wg.Wait()

	resp, err := buildFanoutResponse(queries, results)
	if err != nil {
		return err
	}

	failed, hasMoreCount := 0, 0
	for _, qs := range resp.Queries {
		if qs.Error != "" {
			failed++
		}
		if qs.HasMore {
			hasMoreCount++
		}
	}

	runtime.OutFormat(resp, &output.Meta{Count: len(resp.Users)}, func(w io.Writer) {
		if len(resp.Users) == 0 {
			fmt.Fprintln(w, "No users found.")
			return
		}
		output.PrintTable(w, prettyFanoutUserRows(resp.Users))
	})

	if isFanoutSummaryFormat(runtime.Format) {
		fmt.Fprintf(runtime.IO().ErrOut, "\n%d queries, %d total users; %d failed, %d with has_more\n",
			len(queries), len(resp.Users), failed, hasMoreCount)
	}
	return nil
}

func buildFanoutFilter(runtime *common.RuntimeContext) (*searchUserAPIFilter, error) {
	filter := &searchUserAPIFilter{}
	hasFilter := false
	for _, bf := range searchUserBoolFilters {
		if runtime.Cmd.Flags().Changed(bf.Flag) && runtime.Bool(bf.Flag) {
			bf.Apply(filter)
			hasFilter = true
		}
	}
	if !hasFilter {
		return nil, nil
	}
	return filter, nil
}

func prettyFanoutUserRows(users []fanoutUser) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		rows = append(rows, map[string]interface{}{
			"matched_query":     u.MatchedQuery,
			"localized_name":    u.LocalizedName,
			"department":        common.TruncateStr(u.Department, 50),
			"enterprise_email":  u.EnterpriseEmail,
			"has_chatted":       u.HasChatted,
			"chat_recency_hint": u.ChatRecencyHint,
			"open_id":           u.OpenID,
		})
	}
	return rows
}
