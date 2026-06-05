// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package-level helper: client-side filter that drops muted chats from
// search/list results by calling /open-apis/im/v1/chat_user_setting/batch_get_mute_status.
//
// The native chat search/list APIs do not return mute status; we fetch it as
// a separate batch lookup, then drop is_muted=true items. Non-member /
// invalid-format chat_ids come back via invalid_id_list and are silently
// retained (we don't know their mute state). Bot identity is unsupported by
// the upstream API (UAT-only), so we skip the filter and emit a machine-readable
// skipped indicator instead of erroring.

package im

import (
	"fmt"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// MuteFilterMeta describes the outcome of a single page's mute filter run.
// UnknownCount is internal — used to compose the hint, not exposed in JSON.
type MuteFilterMeta struct {
	Applied       string
	Skipped       bool
	SkipReason    string
	FetchedCount  int
	ReturnedCount int
	FilteredCount int
	UnknownCount  int
	Hint          string
}

// MaxMuteStatusBatchSize is the upstream cap for chat_ids per
// batch_get_mute_status call (after dedupe).
const MaxMuteStatusBatchSize = 100

// BatchGetMuteStatusPath is the upstream HTTP path.
const BatchGetMuteStatusPath = "/open-apis/im/v1/chat_user_setting/batch_get_mute_status"

// SkipReason constants — written to filter.skip_reason when Skipped=true.
const (
	SkipReasonBotIdentity  = "bot_identity_no_mute_data"
	SkipReasonAllNonMember = "all_non_member_search_types"
)

// BuildMuteFilterHint composes the user/AI-facing English hint for a finished
// filter run. hasMore is the underlying API's has_more (so we can suggest paging).
// Returns "" when the filter ran but had no effect (FilteredCount==0 and not skipped).
func BuildMuteFilterHint(meta MuteFilterMeta, hasMore bool) string {
	if meta.Skipped {
		switch meta.SkipReason {
		case SkipReasonBotIdentity:
			return "--exclude-muted has no effect under bot identity (mute is a per-user setting, bots have no mute data); returned all results unfiltered. Use --as user to filter."
		case SkipReasonAllNonMember:
			if hasMore {
				return "All results on this page are non-member public groups; mute filter does not apply. Use --page-token to fetch more."
			}
			return "All results on this page are non-member public groups; mute filter does not apply. No more pages."
		}
		return ""
	}
	if meta.FilteredCount == 0 {
		return ""
	}

	tail := "no more pages."
	if hasMore {
		tail = "use --page-token to fetch more."
	}

	if meta.UnknownCount > 0 {
		return fmt.Sprintf("Filtered out %d muted chat(s) on this page (%d remaining, including %d non-member public group(s)); %s",
			meta.FilteredCount, meta.ReturnedCount, meta.UnknownCount, tail)
	}
	return fmt.Sprintf("Filtered out %d muted chat(s) on this page (%d remaining); %s",
		meta.FilteredCount, meta.ReturnedCount, tail)
}

// BuildBatchGetMuteStatusBody constructs the request body for
// POST /open-apis/im/v1/chat_user_setting/batch_get_mute_status.
func BuildBatchGetMuteStatusBody(chatIDs []string) map[string]interface{} {
	return map[string]interface{}{"chat_ids": chatIDs}
}

// ParseBatchGetMuteStatusResponse maps the API response to:
//   - muted:   chat_id -> is_muted, only for ids returned in items
//   - unknown: chat_ids that came back in invalid_id_list (any msg) OR
//     were in input but missing from both lists.
//
// unknown preserves input order for stable hint output.
func ParseBatchGetMuteStatusResponse(input []string, resp map[string]interface{}) (map[string]bool, []string) {
	muted := make(map[string]bool, len(input))
	if rawItems, ok := resp["items"].([]interface{}); ok {
		for _, raw := range rawItems {
			item, _ := raw.(map[string]interface{})
			if item == nil {
				continue
			}
			cid, _ := item["chat_id"].(string)
			if cid == "" {
				continue
			}
			isMuted, _ := item["is_muted"].(bool)
			muted[cid] = isMuted
		}
	}

	unknownSet := make(map[string]struct{})
	if rawInvalid, ok := resp["invalid_id_list"].([]interface{}); ok {
		for _, raw := range rawInvalid {
			item, _ := raw.(map[string]interface{})
			if item == nil {
				continue
			}
			id, _ := item["id"].(string)
			if id != "" {
				unknownSet[id] = struct{}{}
			}
		}
	}
	for _, id := range input {
		if _, hasMute := muted[id]; hasMute {
			continue
		}
		unknownSet[id] = struct{}{}
	}

	unknown := make([]string, 0, len(unknownSet))
	for _, id := range input {
		if _, ok := unknownSet[id]; ok {
			unknown = append(unknown, id)
			delete(unknownSet, id) // dedupe while preserving input order
		}
	}
	return muted, unknown
}

// ApplyMuteFilter drops chats whose mute map entry is true. Chats whose id
// is in the unknown set, or which have no chatIDKey value, are retained
// (we have no basis to filter them) and counted as UnknownCount.
//
// Pure function; no API calls. The caller is responsible for fetching the
// mute map via FetchMuteStatus.
//
// Invariant: meta.FetchedCount == meta.ReturnedCount + meta.FilteredCount.
func ApplyMuteFilter(
	chats []map[string]interface{},
	chatIDKey string,
	muted map[string]bool,
	unknown []string,
) ([]map[string]interface{}, MuteFilterMeta) {
	unknownSet := make(map[string]struct{}, len(unknown))
	for _, id := range unknown {
		unknownSet[id] = struct{}{}
	}

	out := make([]map[string]interface{}, 0, len(chats))
	meta := MuteFilterMeta{Applied: "exclude_muted", FetchedCount: len(chats)}

	for _, row := range chats {
		cid, _ := row[chatIDKey].(string)
		if cid == "" {
			out = append(out, row)
			meta.UnknownCount++
			continue
		}
		if _, isUnknown := unknownSet[cid]; isUnknown {
			out = append(out, row)
			meta.UnknownCount++
			continue
		}
		if isMuted, ok := muted[cid]; ok {
			if isMuted {
				meta.FilteredCount++
				continue
			}
			out = append(out, row)
			continue
		}
		// Defensive: id not in muted, not in unknown — treat as unknown, retain.
		out = append(out, row)
		meta.UnknownCount++
	}
	meta.ReturnedCount = len(out)
	return out, meta
}

// ExtractChatIDs collects unique chat_ids (in input order) from a page of rows.
// Rows missing the key or with an empty value are skipped.
func ExtractChatIDs(chats []map[string]interface{}, chatIDKey string) []string {
	if len(chats) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(chats))
	out := make([]string, 0, len(chats))
	for _, row := range chats {
		cid, _ := row[chatIDKey].(string)
		if cid == "" {
			continue
		}
		if _, dup := seen[cid]; dup {
			continue
		}
		seen[cid] = struct{}{}
		out = append(out, cid)
	}
	return out
}

// MuteFilterMetaToMap renders the meta as the "filter" sub-object the
// command writes into outData. The schema is fixed-shape: exactly 5 fields,
// regardless of skip state.
//
// Skip context (bot identity / all-non-member search-types) is encoded
// entirely in the Hint string — consumers read the natural-language hint
// to understand why the filter did or did not apply. UnknownCount and the
// Skipped / SkipReason struct fields are internal-only (used to compose
// Hint) and are not exposed in JSON.
func MuteFilterMetaToMap(meta MuteFilterMeta) map[string]interface{} {
	return map[string]interface{}{
		"applied":        meta.Applied,
		"fetched_count":  meta.FetchedCount,
		"returned_count": meta.ReturnedCount,
		"filtered_count": meta.FilteredCount,
		"hint":           meta.Hint,
	}
}

// FetchMuteStatus calls batch_get_mute_status for the given chat_ids and
// parses the result. Caller MUST ensure len(chatIDs) <= MaxMuteStatusBatchSize
// (the shortcuts already cap --page-size at 100, so a single page is safe).
//
// Empty input is a no-op (avoids triggering the upstream "chat_ids is empty"
// InvalidParam).
func FetchMuteStatus(runtime *common.RuntimeContext, chatIDs []string) (map[string]bool, []string, error) {
	if len(chatIDs) == 0 {
		return map[string]bool{}, nil, nil
	}
	if len(chatIDs) > MaxMuteStatusBatchSize {
		return nil, nil, output.ErrValidation(
			"batch_get_mute_status accepts at most %d chat_ids per call (got %d)",
			MaxMuteStatusBatchSize, len(chatIDs))
	}
	body := BuildBatchGetMuteStatusBody(chatIDs)
	resp, err := runtime.CallAPI("POST", BatchGetMuteStatusPath, nil, body)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch mute status: %w", err)
	}
	muted, unknown := ParseBatchGetMuteStatusResponse(chatIDs, resp)
	return muted, unknown, nil
}

// MuteFilterInput captures everything the orchestrator needs from the calling shortcut.
type MuteFilterInput struct {
	ExcludeMuted  bool                     // value of --exclude-muted
	IsBot         bool                     // current identity
	PreSkipReason string                   // optional caller-supplied skip reason (e.g. SkipReasonAllNonMember); leave empty under bot — IsBot is handled separately
	Chats         []map[string]interface{} // page of result rows
	ChatIDKey     string                   // key in row holding the chat_id ("chat_id" for both v1 list and v2 search meta_data)
	HasMore       bool                     // for hint composition
}

// MuteFilterOutput is what the shortcut writes back into outData.
type MuteFilterOutput struct {
	Chats []map[string]interface{} // filtered (or unchanged when not applied)
	Meta  MuteFilterMeta           // zero-valued when ExcludeMuted=false; callers detect via Meta.Applied != ""
}

// MaybeApplyMuteFilter is the single entry point shortcuts call.
//
// Behavior:
//   - ExcludeMuted=false: returns chats unchanged, Meta is zero-valued (Applied=="")
//   - ExcludeMuted=true && IsBot:  skip the API call, mark Skipped with SkipReasonBotIdentity
//   - ExcludeMuted=true && PreSkipReason!="" (not bot): skip the API call, mark Skipped with that reason
//   - ExcludeMuted=true && len(chats)==0: skip the API call (avoids upstream
//     InvalidParam on empty chat_ids); meta has zero counts, Skipped=false
//   - ExcludeMuted=true && otherwise: fetch + apply; populate counts and Hint
//
// Callers detect whether the filter ran via out.Meta.Applied != "".
// Callers compose the JSON map via MuteFilterMetaToMap(out.Meta) at the use site.
func MaybeApplyMuteFilter(runtime *common.RuntimeContext, in MuteFilterInput) (MuteFilterOutput, error) {
	if !in.ExcludeMuted {
		return MuteFilterOutput{Chats: in.Chats}, nil
	}

	meta := MuteFilterMeta{
		Applied:       "exclude_muted",
		FetchedCount:  len(in.Chats),
		ReturnedCount: len(in.Chats),
	}

	switch {
	case in.IsBot:
		meta.Skipped = true
		meta.SkipReason = SkipReasonBotIdentity
	case in.PreSkipReason != "":
		meta.Skipped = true
		meta.SkipReason = in.PreSkipReason
	case len(in.Chats) == 0:
		// counts already zero; Skipped stays false
	default:
		ids := ExtractChatIDs(in.Chats, in.ChatIDKey)
		muted, unknown, err := FetchMuteStatus(runtime, ids)
		if err != nil {
			return MuteFilterOutput{}, err
		}
		var filtered []map[string]interface{}
		filtered, meta = ApplyMuteFilter(in.Chats, in.ChatIDKey, muted, unknown)
		in.Chats = filtered
	}

	meta.Hint = BuildMuteFilterHint(meta, in.HasMore)
	return MuteFilterOutput{
		Chats: in.Chats,
		Meta:  meta,
	}, nil
}
