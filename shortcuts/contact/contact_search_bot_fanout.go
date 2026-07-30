// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// Bot fanout reuses the user fanout's query parsing, concurrency limit and
// response summary types.

type botFanoutResult struct {
	Index   int
	Query   string
	Bots    []searchBot
	HasMore bool
	Notice  string
	ErrMsg  string // empty = success
	Err     error  // original failure, kept for typed propagation
}

// runOneBotQuery converts one fanout request into either bots or an error summary.
func runOneBotQuery(ctx context.Context, runtime *common.RuntimeContext, index int, query string,
	filter *botSearchAPIFilter) botFanoutResult {
	// Pre-check ctx so queued workers see cancellation before issuing a request;
	// in-flight workers continue until DoAPI returns.
	if err := ctx.Err(); err != nil {
		return botFanoutErrorResult(index, query, err)
	}

	body := &botSearchAPIRequest{Query: query}
	if filter != nil {
		body.Filter = filter
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodPost,
		ApiPath:     botSearchURL,
		Body:        body,
		QueryParams: larkcore.QueryParams{"page_size": []string{strconv.Itoa(runtime.Int("page-size"))}},
	})
	if err != nil {
		return botFanoutErrorResult(index, query, err)
	}

	data, err := runtime.ClassifyAPIResponse(apiResp)
	if err != nil {
		return botFanoutErrorResult(index, query, err)
	}
	respData, err := decodeBotSearchAPIData(data)
	if err != nil {
		return botFanoutErrorResult(index, query, err)
	}

	return botFanoutResult{
		Index:   index,
		Query:   query,
		Bots:    projectBots(respData),
		HasMore: respData.HasMore,
		Notice:  respData.Notice,
	}
}

// botFanoutErrorResult records a failed fanout query without stopping other workers.
func botFanoutErrorResult(index int, query string, err error) botFanoutResult {
	if err == nil {
		return botFanoutResult{Index: index, Query: query}
	}
	return botFanoutResult{Index: index, Query: query, ErrMsg: contactFanoutErrorSummary(err), Err: err}
}

func botFanoutContextError(err error) error {
	subtype := errs.SubtypeNetworkTransport
	message := "bot search fanout cancelled"
	if errors.Is(err, context.DeadlineExceeded) {
		subtype = errs.SubtypeNetworkTimeout
		message = "bot search fanout deadline exceeded"
	}
	return errs.NewNetworkError(subtype, "%s", message).WithCause(err)
}

func botFanoutPanicError(query string, recovered any) error {
	err := errs.NewInternalError(errs.SubtypeUnknown,
		"bot search query %q panicked: %v", query, recovered)
	if cause, ok := recovered.(error); ok {
		return err.WithCause(cause)
	}
	return err
}

// Terminal failures invalidate the batch; API and network failures remain
// eligible for partial-success reporting.
func botFanoutTerminalError(results []botFanoutResult) error {
	for _, result := range results {
		if result.Err == nil {
			continue
		}
		if errors.Is(result.Err, context.Canceled) || errors.Is(result.Err, context.DeadlineExceeded) {
			return botFanoutContextError(result.Err)
		}
		problem, ok := errs.ProblemOf(result.Err)
		if !ok {
			return errs.NewInternalError(errs.SubtypeUnknown,
				"bot search query %q failed with an unclassified error: %v", result.Query, result.Err).
				WithCause(result.Err)
		}
		if problem.Category != errs.CategoryAPI && problem.Category != errs.CategoryNetwork {
			return result.Err
		}
	}
	return nil
}

type fanoutBot struct {
	searchBot
	MatchedQuery string `json:"matched_query"`
}

type botFanoutResponse struct {
	Bots    []fanoutBot    `json:"bots"`
	Queries []querySummary `json:"queries"`
	Notice  string         `json:"notice,omitempty"`
}

// buildBotFanoutResponse flattens recoverable results in query order. Terminal
// errors fail the batch even when another query succeeded.
func buildBotFanoutResponse(queries []string, results []botFanoutResult) (*botFanoutResponse, error) {
	if err := botFanoutTerminalError(results); err != nil {
		return nil, err
	}

	indexed := make([]botFanoutResult, len(queries))
	for _, r := range results {
		indexed[r.Index] = r
	}

	out := &botFanoutResponse{
		Bots:    make([]fanoutBot, 0),
		Queries: make([]querySummary, 0, len(queries)),
	}
	failed := 0
	var firstErrMsg, firstErrQuery string
	var firstErr error
	for i, r := range indexed {
		out.Queries = append(out.Queries, querySummary{
			Query:   queries[i],
			Error:   r.ErrMsg,
			HasMore: r.HasMore,
			Notice:  r.Notice,
		})
		if r.ErrMsg != "" {
			failed++
			if firstErrMsg == "" {
				firstErrMsg = r.ErrMsg
				firstErrQuery = queries[i]
				firstErr = r.Err
			}
			continue
		}
		if out.Notice == "" {
			out.Notice = r.Notice
		}
		for _, b := range r.Bots {
			out.Bots = append(out.Bots, fanoutBot{searchBot: b, MatchedQuery: queries[i]})
		}
	}
	if failed == len(queries) && len(queries) > 0 {
		msg := fmt.Sprintf("all %d queries failed; first: %s (query=%q)",
			len(queries), firstErrMsg, firstErrQuery)
		return nil, contactFanoutAllFailedError(firstErr, msg)
	}
	return out, nil
}

func executeBotSearchFanout(ctx context.Context, runtime *common.RuntimeContext) error {
	queries := parseAndDedupQueries(runtime.Str("queries"))

	filter, err := buildBotSearchFilter(runtime)
	if err != nil {
		return err
	}

	results := make([]botFanoutResult, len(queries))
	var wg sync.WaitGroup
	sem := make(chan struct{}, fanoutConcurrency)

schedule:
	for i, q := range queries {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			for j := i; j < len(queries); j++ {
				results[j] = botFanoutErrorResult(j, queries[j], ctx.Err())
			}
			break schedule
		}
		wg.Add(1)
		go func(i int, q string) {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					err := botFanoutPanicError(q, r)
					results[i] = botFanoutResult{
						Index:  i,
						Query:  q,
						ErrMsg: contactFanoutErrorSummary(err),
						Err:    err,
					}
				}
			}()
			results[i] = runOneBotQuery(ctx, runtime, i, q, filter)
		}(i, q)
	}
	wg.Wait()

	resp, err := buildBotFanoutResponse(queries, results)
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

	runtime.OutFormat(resp, &output.Meta{Count: len(resp.Bots)}, func(w io.Writer) {
		if len(resp.Bots) == 0 {
			fmt.Fprintln(w, "No bots found.")
			return
		}
		output.PrintTable(w, prettyBotFanoutRows(resp.Bots))
	})

	if isFanoutSummaryFormat(runtime.Format) {
		fmt.Fprintf(runtime.IO().ErrOut, "\n%d queries, %d total matches; %d failed, %d with has_more\n",
			len(queries), len(resp.Bots), failed, hasMoreCount)
	}
	// The counts above say how many queries failed but not which, and only the
	// json envelope carries queries[].error / queries[].notice. Without this an
	// agent reading csv or a table sees "1 failed" with no way to learn the
	// keyword or the reason, and a notice disappears entirely.
	if !botSearchStdoutCarriesEnvelope(runtime.Format) {
		for _, qs := range resp.Queries {
			if qs.Error != "" {
				fmt.Fprintf(runtime.IO().ErrOut, "failed: %q — %s\n", qs.Query, qs.Error)
			}
			if qs.Notice != "" {
				fmt.Fprintf(runtime.IO().ErrOut, "notice: %q — %s\n", qs.Query, qs.Notice)
			}
			if qs.HasMore {
				fmt.Fprintf(runtime.IO().ErrOut, "has_more: %q — more matches exist; narrow this keyword\n", qs.Query)
			}
		}
	}
	return nil
}

func prettyBotFanoutRows(bots []fanoutBot) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(bots))
	for _, bot := range bots {
		rows = append(rows, map[string]interface{}{
			"matched_query":     bot.MatchedQuery,
			"name":              bot.Name,
			"description":       common.TruncateStr(bot.Description, 50),
			"is_agent":          bot.IsAgent,
			"enable_join_group": bot.EnableJoinGroup,
			"open_id":           bot.OpenID,
		})
	}
	return rows
}
