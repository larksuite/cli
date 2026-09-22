// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Keep each CLI request bounded, but resume its cursor instead of assuming
// --page-all means the entire account history was returned. Flags are ordered
// oldest first, and canceled flags can also occupy pages before the new item.
const flagScanMaxBatches = 50 // 20 pages per batch; at most 1000 pages per scan.

type flagScanStats struct {
	Batches int
	Flags   int
	Deleted int
	HasMore bool
}

func waitForFlagState(t *testing.T, ctx context.Context, messageID string, present bool) gjson.Result {
	t.Helper()
	// Bound the calls themselves as well as the polling between complete scans.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	var item gjson.Result
	var stats flagScanStats
	err := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{Timeout: time.Minute}, func() (bool, error) {
		var err error
		item, stats, err = scanFlagList(ctx, messageID, func(ctx context.Context, req clie2e.Request) (*clie2e.Result, error) {
			return clie2e.RunCmdWithRetry(ctx, req, clie2e.RetryOptions{})
		})
		return item.Exists() == present, err
	})
	// Only emit pagination metadata, never unrelated message bodies from the list.
	require.NoError(t, err, "flag visibility: target=%s present=%t batches=%d flags=%d deleted=%d has_more=%t",
		messageID, present, stats.Batches, stats.Flags, stats.Deleted, stats.HasMore)
	return item
}

func scanFlagList(ctx context.Context, messageID string, run func(context.Context, clie2e.Request) (*clie2e.Result, error)) (gjson.Result, flagScanStats, error) {
	var stats flagScanStats
	token := ""
	seen := map[string]bool{"": true}
	for batch := 0; batch < flagScanMaxBatches; batch++ {
		if err := ctx.Err(); err != nil {
			return gjson.Result{}, stats, err
		}
		args := []string{"im", "+flag-list", "--page-size", "50", "--page-all", "--page-limit", "20", "--enrich-feed-thread=false"}
		if token != "" {
			args = append(args, "--page-token", token)
		}
		result, err := run(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
		stats.Batches++
		if err != nil {
			return gjson.Result{}, stats, fmt.Errorf("flag list batch %d: %w", stats.Batches, err)
		}
		if result == nil {
			return gjson.Result{}, stats, fmt.Errorf("flag list batch %d returned no result", stats.Batches)
		}
		if result.ExitCode != 0 {
			return gjson.Result{}, stats, fmt.Errorf("flag list batch %d failed: exit=%d log_id=%s", stats.Batches, result.ExitCode, gjson.Get(result.Stderr, "error.log_id").String())
		}
		var response struct {
			OK       bool   `json:"ok"`
			Identity string `json:"identity"`
			Data     struct {
				Items     []json.RawMessage `json:"flag_items"`
				Deleted   []json.RawMessage `json:"delete_flag_items"`
				HasMore   *bool             `json:"has_more"`
				PageToken string            `json:"page_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &response); err != nil {
			return gjson.Result{}, stats, fmt.Errorf("flag list batch %d returned invalid JSON", stats.Batches)
		}
		if !response.OK || response.Identity != "user" || response.Data.Items == nil || response.Data.HasMore == nil {
			return gjson.Result{}, stats, fmt.Errorf("flag list batch %d returned an invalid user list envelope", stats.Batches)
		}
		stats.Flags += len(response.Data.Items)
		stats.Deleted += len(response.Data.Deleted)
		stats.HasMore = *response.Data.HasMore
		for _, raw := range response.Data.Items {
			item := gjson.ParseBytes(raw)
			if !item.IsObject() || item.Get("item_id").String() == "" {
				return gjson.Result{}, stats, fmt.Errorf("flag list batch %d contains an invalid flag item", stats.Batches)
			}
			if item.Get("item_id").String() == messageID {
				return item, stats, nil
			}
		}
		if !stats.HasMore {
			return gjson.Result{}, stats, nil
		}
		token = response.Data.PageToken
		if token == "" || seen[token] {
			return gjson.Result{}, stats, fmt.Errorf("flag list scan incomplete: missing or repeated cursor after batch %d", stats.Batches)
		}
		seen[token] = true
	}
	return gjson.Result{}, stats, fmt.Errorf("flag list scan incomplete: reached %d batches with has_more=true", flagScanMaxBatches)
}
