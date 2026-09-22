// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func flagListPage(t *testing.T, ids []string, deleted int, more bool, token string) *clie2e.Result {
	t.Helper()
	items := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]string{"item_id": id, "item_type": "0", "flag_type": "2"})
	}
	tombstones := make([]map[string]string, deleted)
	for i := range tombstones {
		tombstones[i] = map[string]string{"item_id": fmt.Sprintf("om_deleted_%d", i)}
	}
	raw, err := json.Marshal(map[string]any{
		"ok": true, "identity": "user",
		"data": map[string]any{"flag_items": items, "delete_flag_items": tombstones, "has_more": more, "page_token": token},
	})
	require.NoError(t, err)
	return &clie2e.Result{Stdout: string(raw)}
}

func TestScanFlagListContinuesAfterPageLimit(t *testing.T) {
	// A long-lived account can have a full batch of canceled flags before the
	// just-created message. An empty active list is not proof of absence.
	pages := []*clie2e.Result{
		flagListPage(t, nil, 1000, true, "next"),
		flagListPage(t, []string{"om_target"}, 0, false, ""),
	}
	calls := 0
	item, stats, err := scanFlagList(context.Background(), "om_target", func(_ context.Context, req clie2e.Request) (*clie2e.Result, error) {
		require.Equal(t, "user", req.DefaultAs)
		want := []string{"im", "+flag-list", "--page-size", "50", "--page-all", "--page-limit", "20", "--enrich-feed-thread=false"}
		if calls > 0 {
			want = append(want, "--page-token", "next")
		}
		require.Equal(t, want, req.Args)
		require.Less(t, calls, len(pages))
		page := pages[calls]
		calls++
		return page, nil
	})
	require.NoError(t, err)
	require.Equal(t, "om_target", item.Get("item_id").String())
	require.Equal(t, "0", item.Get("item_type").String())
	require.Equal(t, "2", item.Get("flag_type").String())
	require.Equal(t, 2, calls)
	require.Equal(t, 1000, stats.Deleted)
}

func TestScanFlagListAbsenceRequiresLastPage(t *testing.T) {
	calls := 0
	item, stats, err := scanFlagList(context.Background(), "om_target", func(_ context.Context, _ clie2e.Request) (*clie2e.Result, error) {
		calls++
		return flagListPage(t, []string{"om_unrelated"}, 0, calls == 1, "next"), nil
	})
	require.NoError(t, err)
	require.False(t, item.Exists())
	require.Equal(t, 2, calls)
	require.False(t, stats.HasMore)
}

func TestScanFlagListStopsWhenTargetIsFound(t *testing.T) {
	calls := 0
	item, stats, err := scanFlagList(context.Background(), "om_target", func(_ context.Context, _ clie2e.Request) (*clie2e.Result, error) {
		calls++
		return flagListPage(t, []string{"om_target"}, 0, true, "next"), nil
	})
	require.NoError(t, err)
	require.True(t, item.Exists())
	require.Equal(t, 1, calls)
	require.True(t, stats.HasMore, "finding the target is sufficient; proving absence is not")
}

func TestScanFlagListRejectsIncompleteScan(t *testing.T) {
	for _, tc := range []struct {
		name      string
		tokens    []string
		wantCalls int
	}{
		{"missing cursor", []string{""}, 1},
		{"unchanged cursor", []string{"a", "a"}, 2},
		{"cursor cycle", []string{"a", "b", "a"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			item, _, err := scanFlagList(context.Background(), "om_target", func(_ context.Context, _ clie2e.Request) (*clie2e.Result, error) {
				token := tc.tokens[calls]
				calls++
				return flagListPage(t, nil, 1, true, token), nil
			})
			require.ErrorContains(t, err, "scan incomplete")
			require.False(t, item.Exists())
			require.Equal(t, tc.wantCalls, calls)
		})
	}
	t.Run("batch budget", func(t *testing.T) {
		calls := 0
		_, stats, err := scanFlagList(context.Background(), "om_target", func(_ context.Context, _ clie2e.Request) (*clie2e.Result, error) {
			calls++
			return flagListPage(t, nil, 1, true, fmt.Sprint(calls)), nil
		})
		require.ErrorContains(t, err, "scan incomplete")
		require.Equal(t, flagScanMaxBatches, calls)
		require.True(t, stats.HasMore)
	})
}

func TestScanFlagListRejectsInvalidResults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *clie2e.Result
	}{
		{"nil", nil},
		{"command failure", &clie2e.Result{ExitCode: 1}},
		{"malformed JSON", &clie2e.Result{Stdout: "not JSON"}},
		{"failed envelope", &clie2e.Result{Stdout: `{"ok":false,"identity":"user","data":{"flag_items":[],"has_more":false}}`}},
		{"wrong identity", &clie2e.Result{Stdout: `{"ok":true,"identity":"bot","data":{"flag_items":[],"has_more":false}}`}},
		{"missing list", &clie2e.Result{Stdout: `{"ok":true,"identity":"user","data":{"has_more":false}}`}},
		{"missing has_more", &clie2e.Result{Stdout: `{"ok":true,"identity":"user","data":{"flag_items":[]}}`}},
		{"invalid item", &clie2e.Result{Stdout: `{"ok":true,"identity":"user","data":{"flag_items":[{}],"has_more":false}}`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := scanFlagList(context.Background(), "om_target", func(context.Context, clie2e.Request) (*clie2e.Result, error) { return tc.result, nil })
			require.Error(t, err, "invalid output must not establish absence")
		})
	}
}

func TestScanFlagListPropagatesExecutionError(t *testing.T) {
	want := errors.New("test transport failure")
	_, _, err := scanFlagList(context.Background(), "om_target", func(context.Context, clie2e.Request) (*clie2e.Result, error) { return nil, want })
	require.ErrorIs(t, err, want)
}

func TestScanFlagListHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	_, _, err := scanFlagList(ctx, "om_target", func(context.Context, clie2e.Request) (*clie2e.Result, error) {
		calls++
		cancel()
		return flagListPage(t, nil, 0, true, "next"), nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
}
