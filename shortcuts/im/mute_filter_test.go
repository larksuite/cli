// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestBuildMuteFilterHint(t *testing.T) {
	cases := []struct {
		name    string
		meta    MuteFilterMeta
		hasMore bool
		want    string
	}{
		{
			name:    "1 skipped bot identity",
			meta:    MuteFilterMeta{Applied: "exclude_muted", Skipped: true, SkipReason: SkipReasonBotIdentity},
			hasMore: false,
			want:    "--exclude-muted has no effect under bot identity (mute is a per-user setting, bots have no mute data); returned all results unfiltered. Use --as user to filter.",
		},
		{
			name:    "2 skipped all non-member, has_more",
			meta:    MuteFilterMeta{Applied: "exclude_muted", Skipped: true, SkipReason: SkipReasonAllNonMember},
			hasMore: true,
			want:    "All results on this page are non-member public groups; mute filter does not apply. Use --page-token to fetch more.",
		},
		{
			name:    "3 skipped all non-member, no more",
			meta:    MuteFilterMeta{Applied: "exclude_muted", Skipped: true, SkipReason: SkipReasonAllNonMember},
			hasMore: false,
			want:    "All results on this page are non-member public groups; mute filter does not apply. No more pages.",
		},
		{
			name:    "4 filtered>0 unknown=0 has_more",
			meta:    MuteFilterMeta{Applied: "exclude_muted", FetchedCount: 20, ReturnedCount: 17, FilteredCount: 3},
			hasMore: true,
			want:    "Filtered out 3 muted chat(s) on this page (17 remaining); use --page-token to fetch more.",
		},
		{
			name:    "5 filtered>0 unknown=0 no more",
			meta:    MuteFilterMeta{Applied: "exclude_muted", FetchedCount: 20, ReturnedCount: 17, FilteredCount: 3},
			hasMore: false,
			want:    "Filtered out 3 muted chat(s) on this page (17 remaining); no more pages.",
		},
		{
			name:    "6 filtered>0 unknown>0 has_more",
			meta:    MuteFilterMeta{Applied: "exclude_muted", FetchedCount: 20, ReturnedCount: 19, FilteredCount: 1, UnknownCount: 2},
			hasMore: true,
			want:    "Filtered out 1 muted chat(s) on this page (19 remaining, including 2 non-member public group(s)); use --page-token to fetch more.",
		},
		{
			name:    "7 filtered>0 unknown>0 no more",
			meta:    MuteFilterMeta{Applied: "exclude_muted", FetchedCount: 20, ReturnedCount: 19, FilteredCount: 1, UnknownCount: 2},
			hasMore: false,
			want:    "Filtered out 1 muted chat(s) on this page (19 remaining, including 2 non-member public group(s)); no more pages.",
		},
		{
			name:    "8 filtered=0 returns empty regardless of unknown/hasMore",
			meta:    MuteFilterMeta{Applied: "exclude_muted", FetchedCount: 5, ReturnedCount: 5, UnknownCount: 2},
			hasMore: true,
			want:    "",
		},
		{
			name:    "9 skipped with unrecognized reason returns empty",
			meta:    MuteFilterMeta{Applied: "exclude_muted", Skipped: true, SkipReason: "unknown_reason"},
			hasMore: false,
			want:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BuildMuteFilterHint(c.meta, c.hasMore)
			if got != c.want {
				t.Fatalf("BuildMuteFilterHint() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestBuildBatchGetMuteStatusBody(t *testing.T) {
	got := BuildBatchGetMuteStatusBody([]string{"oc_a", "oc_b"})
	want := map[string]interface{}{"chat_ids": []string{"oc_a", "oc_b"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildBatchGetMuteStatusBody() = %v, want %v", got, want)
	}
}

func TestParseBatchGetMuteStatusResponse(t *testing.T) {
	t.Run("happy path with mixed muted/non-muted/invalid", func(t *testing.T) {
		input := []string{"oc_a", "oc_b", "oc_c", "bad"}
		resp := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"chat_id": "oc_a", "is_muted": true},
				map[string]interface{}{"chat_id": "oc_b", "is_muted": false},
			},
			"invalid_id_list": []interface{}{
				map[string]interface{}{"id": "oc_c", "msg": "not_a_member"},
				map[string]interface{}{"id": "bad", "msg": "invalid_format"},
			},
		}
		muted, unknown := ParseBatchGetMuteStatusResponse(input, resp)
		wantMuted := map[string]bool{"oc_a": true, "oc_b": false}
		wantUnknown := []string{"oc_c", "bad"}
		if !reflect.DeepEqual(muted, wantMuted) {
			t.Fatalf("muted = %v, want %v", muted, wantMuted)
		}
		if !reflect.DeepEqual(unknown, wantUnknown) {
			t.Fatalf("unknown = %v, want %v", unknown, wantUnknown)
		}
	})

	t.Run("missing chat_ids fall through to unknown", func(t *testing.T) {
		input := []string{"oc_a", "oc_b"}
		resp := map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"chat_id": "oc_a", "is_muted": true},
			},
		}
		muted, unknown := ParseBatchGetMuteStatusResponse(input, resp)
		if !reflect.DeepEqual(muted, map[string]bool{"oc_a": true}) {
			t.Fatalf("muted = %v", muted)
		}
		if !reflect.DeepEqual(unknown, []string{"oc_b"}) {
			t.Fatalf("unknown = %v", unknown)
		}
	})

	t.Run("empty response yields all unknown", func(t *testing.T) {
		input := []string{"oc_a"}
		muted, unknown := ParseBatchGetMuteStatusResponse(input, map[string]interface{}{})
		if len(muted) != 0 {
			t.Fatalf("muted = %v, want empty", muted)
		}
		if !reflect.DeepEqual(unknown, []string{"oc_a"}) {
			t.Fatalf("unknown = %v", unknown)
		}
	})

	t.Run("skips nil entries and empty chat_id in items/invalid_id_list", func(t *testing.T) {
		input := []string{"oc_a", "oc_b"}
		resp := map[string]interface{}{
			"items": []interface{}{
				nil,
				map[string]interface{}{"chat_id": "", "is_muted": false},
				map[string]interface{}{"chat_id": "oc_a", "is_muted": true},
			},
			"invalid_id_list": []interface{}{
				nil,
				map[string]interface{}{"id": "oc_b", "msg": "not_a_member"},
			},
		}
		muted, unknown := ParseBatchGetMuteStatusResponse(input, resp)
		if !reflect.DeepEqual(muted, map[string]bool{"oc_a": true}) {
			t.Fatalf("muted = %v", muted)
		}
		if !reflect.DeepEqual(unknown, []string{"oc_b"}) {
			t.Fatalf("unknown = %v", unknown)
		}
	})
}

func TestApplyMuteFilter(t *testing.T) {
	chats := []map[string]interface{}{
		{"chat_id": "oc_a", "name": "alpha"},
		{"chat_id": "oc_b", "name": "beta"},
		{"chat_id": "oc_c", "name": "gamma"},
		{"chat_id": "oc_d", "name": "delta"},
	}

	t.Run("drops only is_muted=true", func(t *testing.T) {
		muted := map[string]bool{"oc_a": true, "oc_b": false, "oc_c": true, "oc_d": false}
		got, meta := ApplyMuteFilter(chats, "chat_id", muted, nil)
		if len(got) != 2 {
			t.Fatalf("len(got) = %d, want 2", len(got))
		}
		if got[0]["chat_id"] != "oc_b" || got[1]["chat_id"] != "oc_d" {
			t.Fatalf("got = %v, want [oc_b, oc_d]", got)
		}
		want := MuteFilterMeta{
			Applied: "exclude_muted", FetchedCount: 4, ReturnedCount: 2, FilteredCount: 2, UnknownCount: 0,
		}
		if meta != want {
			t.Fatalf("meta = %+v, want %+v", meta, want)
		}
	})

	t.Run("retains unknown chats and counts them", func(t *testing.T) {
		muted := map[string]bool{"oc_a": true, "oc_b": false}
		unknown := []string{"oc_c", "oc_d"}
		got, meta := ApplyMuteFilter(chats, "chat_id", muted, unknown)
		if len(got) != 3 {
			t.Fatalf("len(got) = %d, want 3 (oc_b + oc_c + oc_d)", len(got))
		}
		if meta.FilteredCount != 1 || meta.UnknownCount != 2 || meta.ReturnedCount != 3 {
			t.Fatalf("meta = %+v, want filtered=1 unknown=2 returned=3", meta)
		}
	})

	t.Run("preserves original order", func(t *testing.T) {
		muted := map[string]bool{"oc_b": true}
		got, _ := ApplyMuteFilter(chats, "chat_id", muted, []string{"oc_c", "oc_d"})
		gotIDs := []string{}
		for _, r := range got {
			gotIDs = append(gotIDs, r["chat_id"].(string))
		}
		want := []string{"oc_a", "oc_c", "oc_d"}
		if !reflect.DeepEqual(gotIDs, want) {
			t.Fatalf("ordering = %v, want %v", gotIDs, want)
		}
	})

	t.Run("missing chatIDKey treated as unknown but kept", func(t *testing.T) {
		bad := []map[string]interface{}{{"name": "no_id"}}
		got, meta := ApplyMuteFilter(bad, "chat_id", map[string]bool{}, nil)
		if len(got) != 1 {
			t.Fatalf("missing-id row should be retained, got len = %d", len(got))
		}
		if meta.UnknownCount != 1 || meta.FilteredCount != 0 || meta.ReturnedCount != 1 {
			t.Fatalf("meta = %+v, want unknown=1 filtered=0 returned=1", meta)
		}
	})

	t.Run("invariant fetched == returned + filtered", func(t *testing.T) {
		muted := map[string]bool{"oc_a": true, "oc_b": false}
		_, meta := ApplyMuteFilter(chats, "chat_id", muted, []string{"oc_c", "oc_d"})
		if meta.FetchedCount != meta.ReturnedCount+meta.FilteredCount {
			t.Fatalf("invariant broken: fetched=%d, returned=%d, filtered=%d",
				meta.FetchedCount, meta.ReturnedCount, meta.FilteredCount)
		}
	})
}

func TestExtractChatIDs(t *testing.T) {
	t.Run("dedupes and preserves order", func(t *testing.T) {
		chats := []map[string]interface{}{
			{"chat_id": "oc_a"},
			{"chat_id": "oc_b"},
			{"chat_id": "oc_a"},
			{"chat_id": ""},
			{"name": "no_id"},
			{"chat_id": "oc_c"},
		}
		got := ExtractChatIDs(chats, "chat_id")
		want := []string{"oc_a", "oc_b", "oc_c"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ExtractChatIDs() = %v, want %v", got, want)
		}
	})

	t.Run("empty input yields empty slice", func(t *testing.T) {
		got := ExtractChatIDs(nil, "chat_id")
		if len(got) != 0 {
			t.Fatalf("ExtractChatIDs(nil) = %v, want empty", got)
		}
	})
}

func TestMuteFilterMetaToMap(t *testing.T) {
	wantKeys := []string{"applied", "fetched_count", "returned_count", "filtered_count", "hint"}

	t.Run("active filter exposes exactly 5 fields", func(t *testing.T) {
		meta := MuteFilterMeta{
			Applied:      "exclude_muted",
			FetchedCount: 20, ReturnedCount: 19, FilteredCount: 1, UnknownCount: 2,
			Hint: "test hint",
		}
		got := MuteFilterMetaToMap(meta)
		if got["applied"] != "exclude_muted" ||
			got["fetched_count"] != 20 || got["returned_count"] != 19 ||
			got["filtered_count"] != 1 || got["hint"] != "test hint" {
			t.Fatalf("MuteFilterMetaToMap() = %v", got)
		}
		assertExactKeys(t, got, wantKeys)
	})

	t.Run("skipped path: hint carries the skip explanation, no extra fields", func(t *testing.T) {
		meta := MuteFilterMeta{
			Applied: "exclude_muted", Skipped: true, SkipReason: SkipReasonBotIdentity,
			FetchedCount: 5, ReturnedCount: 5, Hint: "skipped hint",
		}
		got := MuteFilterMetaToMap(meta)
		if got["hint"] != "skipped hint" {
			t.Fatalf("hint = %v, want \"skipped hint\"", got["hint"])
		}
		assertExactKeys(t, got, wantKeys)
	})
}

// assertExactKeys fails the test if got has any keys outside want, or is missing any.
func assertExactKeys(t *testing.T, got map[string]interface{}, want []string) {
	t.Helper()
	wantSet := make(map[string]struct{}, len(want))
	for _, k := range want {
		wantSet[k] = struct{}{}
		if _, ok := got[k]; !ok {
			t.Errorf("missing required key %q", k)
		}
	}
	for k := range got {
		if _, ok := wantSet[k]; !ok {
			t.Errorf("unexpected key %q in MuteFilterMetaToMap output (got %v)", k, got)
		}
	}
}

// runtimeForOrchestrator builds a minimal RuntimeContext for testing the
// branches of MaybeApplyMuteFilter that do NOT call the underlying API.
func runtimeForOrchestrator(t *testing.T) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func TestMaybeApplyMuteFilter_NotEnabled(t *testing.T) {
	rt := runtimeForOrchestrator(t)
	chats := []map[string]interface{}{{"chat_id": "oc_a"}}
	out, err := MaybeApplyMuteFilter(rt, MuteFilterInput{
		ExcludeMuted: false,
		Chats:        chats,
		ChatIDKey:    "chat_id",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(out.Chats) != 1 || out.Meta.Applied != "" {
		t.Fatalf("expected pass-through, got chats=%v meta.applied=%q", out.Chats, out.Meta.Applied)
	}
}

func TestMaybeApplyMuteFilter_BotIdentity(t *testing.T) {
	rt := runtimeForOrchestrator(t)
	chats := []map[string]interface{}{
		{"chat_id": "oc_a"},
		{"chat_id": "oc_b"},
	}
	out, err := MaybeApplyMuteFilter(rt, MuteFilterInput{
		ExcludeMuted: true,
		IsBot:        true,
		Chats:        chats,
		ChatIDKey:    "chat_id",
		HasMore:      false,
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(out.Chats) != 2 {
		t.Fatalf("bot skip should retain all chats, got %d", len(out.Chats))
	}
	if !out.Meta.Skipped {
		t.Fatalf("skipped should be true, got meta=%+v", out.Meta)
	}
	if out.Meta.SkipReason != SkipReasonBotIdentity {
		t.Fatalf("skip_reason = %v", out.Meta.SkipReason)
	}
	wantHint := "--exclude-muted has no effect under bot identity (mute is a per-user setting, bots have no mute data); returned all results unfiltered. Use --as user to filter."
	if out.Meta.Hint != wantHint {
		t.Fatalf("hint = %q", out.Meta.Hint)
	}
}

func TestMaybeApplyMuteFilter_PreSkipAllNonMember(t *testing.T) {
	rt := runtimeForOrchestrator(t)
	chats := []map[string]interface{}{
		{"chat_id": "oc_a"},
		{"chat_id": "oc_b"},
	}
	out, err := MaybeApplyMuteFilter(rt, MuteFilterInput{
		ExcludeMuted:  true,
		IsBot:         false,
		PreSkipReason: SkipReasonAllNonMember,
		Chats:         chats,
		ChatIDKey:     "chat_id",
		HasMore:       true,
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(out.Chats) != 2 {
		t.Fatalf("pre-skip should retain all chats, got %d", len(out.Chats))
	}
	if !out.Meta.Skipped || out.Meta.SkipReason != SkipReasonAllNonMember {
		t.Fatalf("meta = %+v", out.Meta)
	}
	wantHint := "All results on this page are non-member public groups; mute filter does not apply. Use --page-token to fetch more."
	if out.Meta.Hint != wantHint {
		t.Fatalf("hint = %q", out.Meta.Hint)
	}
}

func TestMaybeApplyMuteFilter_EmptyPage(t *testing.T) {
	rt := runtimeForOrchestrator(t)
	out, err := MaybeApplyMuteFilter(rt, MuteFilterInput{
		ExcludeMuted: true,
		Chats:        nil,
		ChatIDKey:    "chat_id",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(out.Chats) != 0 {
		t.Fatalf("expected empty out, got %v", out.Chats)
	}
	if out.Meta.Applied != "exclude_muted" {
		t.Fatalf("meta.applied = %q, want exclude_muted", out.Meta.Applied)
	}
	if out.Meta.FetchedCount != 0 || out.Meta.ReturnedCount != 0 || out.Meta.FilteredCount != 0 {
		t.Fatalf("counts should all be zero, got meta=%+v", out.Meta)
	}
	if out.Meta.Skipped {
		t.Fatalf("empty page is not 'skipped', got meta.skipped=%v", out.Meta.Skipped)
	}
}

func TestFetchMuteStatus_OverLimit(t *testing.T) {
	rt := runtimeForOrchestrator(t)
	ids := make([]string, MaxMuteStatusBatchSize+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("oc_%d", i)
	}
	_, _, err := FetchMuteStatus(rt, ids)
	if err == nil {
		t.Fatalf("expected error on over-limit batch")
	}
}

func TestFetchMuteStatus_Empty(t *testing.T) {
	rt := runtimeForOrchestrator(t)
	muted, unknown, err := FetchMuteStatus(rt, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(muted) != 0 || len(unknown) != 0 {
		t.Fatalf("expected empty results, got muted=%v unknown=%v", muted, unknown)
	}
}
