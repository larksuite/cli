// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// Bot fanout mirrors the user fanout in contact_search_user_fanout.go: same
// dedup, same concurrency cap, same per-query summary, same "fail only when every
// query fails" rule. It reuses parseAndDedupQueries, querySummary,
// isFanoutSummaryFormat and the contactFanout* error helpers rather than growing
// a second set.

type botFanoutResult struct {
	Index   int
	Query   string
	Bots    []searchBot
	HasMore bool
	Notice  string
	ErrMsg  string // empty = success
	Err     error  // original failure, kept for typed all-failed propagation
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

type fanoutBot struct {
	searchBot
	MatchedQuery string `json:"matched_query"`
}

type botFanoutResponse struct {
	Bots    []fanoutBot    `json:"bots"`
	Queries []querySummary `json:"queries"`
	Notice  string         `json:"notice,omitempty"`
}

// buildBotFanoutResponse flattens ordered fanout results and fails only when all
// queries fail.
func buildBotFanoutResponse(queries []string, results []botFanoutResult) (*botFanoutResponse, error) {
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

	filter, err := buildBotFanoutFilter(runtime)
	if err != nil {
		return err
	}

	results := make([]botFanoutResult, len(queries))
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
					results[i] = botFanoutResult{
						Index:  i,
						Query:  q,
						ErrMsg: fmt.Sprintf("internal error: %v", r),
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
		fmt.Fprintf(runtime.IO().ErrOut, "\n%d queries, %d total bots; %d failed, %d with has_more\n",
			len(queries), len(resp.Bots), failed, hasMoreCount)
	}
	return nil
}

// buildBotFanoutFilter reuses the single-search filter: --chat-ids and
// --has-chatted narrow every query in the fanout, exactly as the bool filters do
// for the user fanout.
func buildBotFanoutFilter(runtime *common.RuntimeContext) (*botSearchAPIFilter, error) {
	filter := &botSearchAPIFilter{}
	hasFilter := false

	chatIDs, err := parseBotSearchChatIDs(runtime)
	if err != nil {
		return nil, err
	}
	if len(chatIDs) > 0 {
		filter.ChatIDs = chatIDs
		hasFilter = true
	}
	if runtime.Cmd.Flags().Changed("has-chatted") && runtime.Bool("has-chatted") {
		filter.HasChatter = true
		hasFilter = true
	}

	if !hasFilter {
		return nil, nil
	}
	return filter, nil
}

func prettyBotFanoutRows(bots []fanoutBot) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(bots))
	for _, bot := range bots {
		rows = append(rows, map[string]interface{}{
			"matched_query":     bot.MatchedQuery,
			"name":              bot.Name,
			"description":       common.TruncateStr(bot.Description, 50),
			"has_chatted":       bot.HasChatted,
			"is_agent":          bot.IsAgent,
			"enable_join_group": bot.EnableJoinGroup,
			"open_id":           bot.OpenID,
		})
	}
	return rows
}
