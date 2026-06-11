// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"net/http"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── mention token resolution ─────────────────────────────────────────
//
// A sheet snapshot stores a user @-mention as the tenant user_id (rich_text
// segment .token, e.g. "5d7g1c33"). Agents never hold a user_id — contact
// +search-user only ever yields an open_id — so the raw mention_token an agent
// can supply is an open_id (or union_id). We bridge that gap at the CLI
// boundary, mirroring what the legacy open-platform values API used to do at
// its edge:
//
//   - write (+cells-set / set_cell_range): open_id|union_id → user_id before
//     the cells matrix is sent, so the snapshot stores a valid user_id.
//   - read  (+cells-get / get_cell_ranges): user_id → open_id in the returned
//     mention_token, so the agent gets back the id type it actually speaks.
//
// User mentions are told apart from document mentions (@文档, whose token is a
// doc token like shtXXX): on write only ou_/on_ tokens are resolved (a doc
// token never matches); on read only segments with mention_type 0 (user) whose
// token is not already an open_id are converted. Already-correct or non-user
// tokens pass through untouched, keeping the change backward compatible.

const mentionResolveBatchSize = 50

func mentionTokenIsOpenID(s string) bool  { return strings.HasPrefix(s, "ou_") }
func mentionTokenIsUnionID(s string) bool { return strings.HasPrefix(s, "on_") }

// isUserMentionSeg reports whether a rich_text mention segment refers to a user
// (mention_type 0) rather than a document (mention_type 1, @文档). A user
// mention's stored token is the tenant user_id; a document mention's token is a
// doc token and must never be treated as a contact id. An absent mention_type
// decodes as the user default.
func isUserMentionSeg(seg map[string]interface{}) bool {
	mt, present := seg["mention_type"]
	if !present || mt == nil {
		return true
	}
	n, _ := mt.(float64) // JSON numbers decode to float64
	return n == 0
}

// walkMentionTokens visits every user-mention segment in a set_cell_range /
// get_cell_ranges cells matrix. `cells` is the 2D rows-of-cells array; each
// cell may carry a rich_text array whose mention segments hold mention_token.
func walkMentionTokens(cells []interface{}, visit func(seg map[string]interface{}, token string)) {
	for _, row := range cells {
		cols, _ := row.([]interface{})
		for _, c := range cols {
			cell, _ := c.(map[string]interface{})
			if cell == nil {
				continue
			}
			rt, _ := cell["rich_text"].([]interface{})
			for _, s := range rt {
				seg, _ := s.(map[string]interface{})
				if seg == nil || seg["type"] != "mention" {
					continue
				}
				if tok, _ := seg["mention_token"].(string); tok != "" {
					visit(seg, tok)
				}
			}
		}
	}
}

// resolveMentionTokensForWrite rewrites open_id / union_id mention tokens in a
// set_cell_range cells matrix into the tenant user_id the snapshot stores. It
// is strict: if any user mention can't be resolved to a user_id the write is
// rejected, so a broken (unresolvable) id is never stored silently. Tokens that
// already look like a user_id, or are document mentions, are left untouched and
// trigger no contact lookup. (Email tokens are not resolved here — agents get
// an open_id from contact +search-user, not an email; email support can be
// added later via /contact/v3/users/batch_get_id.)
func resolveMentionTokensForWrite(runtime *common.RuntimeContext, cells []interface{}) error {
	openIDs := map[string]bool{}
	unionIDs := map[string]bool{}
	walkMentionTokens(cells, func(_ map[string]interface{}, tok string) {
		switch {
		case mentionTokenIsOpenID(tok):
			openIDs[tok] = true
		case mentionTokenIsUnionID(tok):
			unionIDs[tok] = true
		}
	})
	if len(openIDs) == 0 && len(unionIDs) == 0 {
		return nil
	}

	resolved := map[string]string{} // original token → user_id
	if err := batchUsersConvert(runtime, mapKeys(openIDs), "open_id", "user_id", resolved); err != nil {
		return err
	}
	if err := batchUsersConvert(runtime, mapKeys(unionIDs), "union_id", "user_id", resolved); err != nil {
		return err
	}

	var unresolved []string
	walkMentionTokens(cells, func(seg map[string]interface{}, tok string) {
		if !mentionTokenIsOpenID(tok) && !mentionTokenIsUnionID(tok) {
			return
		}
		if uid := resolved[tok]; uid != "" {
			seg["mention_token"] = uid
		} else {
			unresolved = append(unresolved, tok)
		}
	})
	if len(unresolved) > 0 {
		return common.FlagErrorf(
			"--cells: could not resolve @-mention(s) to a tenant user: %s — the id must belong to a same-tenant user (open_id / union_id)",
			strings.Join(dedupe(unresolved), ", "))
	}
	return nil
}

// resolveMentionTokensInBatchOps applies write-side mention resolution to every
// set_cell_range operation inside a translated batch_update operations array
// (each op is {tool_name, input}). This covers +cells-set when it is dispatched
// through +batch-update, which bypasses CellsSet.Execute.
func resolveMentionTokensInBatchOps(runtime *common.RuntimeContext, ops interface{}) error {
	list, _ := ops.([]interface{})
	for _, o := range list {
		op, _ := o.(map[string]interface{})
		if op == nil || op["tool_name"] != "set_cell_range" {
			continue
		}
		in, _ := op["input"].(map[string]interface{})
		if in == nil {
			continue
		}
		if cells, ok := in["cells"].([]interface{}); ok {
			if err := resolveMentionTokensForWrite(runtime, cells); err != nil {
				return err
			}
		}
	}
	return nil
}

// resolveMentionTokensForRead rewrites stored user_id mention tokens in a
// get_cell_ranges output into open_ids, so the agent receives the id type it
// can use. Best-effort: on any contact lookup failure the user_ids are left
// as-is rather than failing the read.
func resolveMentionTokensForRead(runtime *common.RuntimeContext, out interface{}) {
	matrices := readCellMatrices(out)
	if len(matrices) == 0 {
		return
	}
	userIDs := map[string]bool{}
	for _, cells := range matrices {
		walkMentionTokens(cells, func(seg map[string]interface{}, tok string) {
			// A user mention whose token is already an open_id needs no
			// translation; everything else that is a user mention is a stored
			// user_id to convert back.
			if isUserMentionSeg(seg) && !mentionTokenIsOpenID(tok) {
				userIDs[tok] = true
			}
		})
	}
	if len(userIDs) == 0 {
		return
	}
	resolved := map[string]string{} // user_id → open_id
	if err := batchUsersConvert(runtime, mapKeys(userIDs), "user_id", "open_id", resolved); err != nil {
		return
	}
	for _, cells := range matrices {
		walkMentionTokens(cells, func(seg map[string]interface{}, tok string) {
			if oid := resolved[tok]; oid != "" {
				seg["mention_token"] = oid
			}
		})
	}
}

// readCellMatrices extracts the per-range cells matrices from a get_cell_ranges
// output (shape: { ranges: [ { cells: [[cell,…],…] }, … ] }).
func readCellMatrices(out interface{}) [][]interface{} {
	m, _ := out.(map[string]interface{})
	if m == nil {
		return nil
	}
	ranges, _ := m["ranges"].([]interface{})
	var result [][]interface{}
	for _, r := range ranges {
		rm, _ := r.(map[string]interface{})
		if rm == nil {
			continue
		}
		if cells, ok := rm["cells"].([]interface{}); ok {
			result = append(result, cells)
		}
	}
	return result
}

// batchUsersConvert resolves contact ids of type fromType (open_id | union_id |
// user_id) to the field wantField (user_id | open_id) via
// GET /contact/v3/users/batch, writing token→value into out. Ids the API does
// not return are simply absent from out. Batched at 50 per call.
func batchUsersConvert(runtime *common.RuntimeContext, ids []string, fromType, wantField string, out map[string]string) error {
	for i := 0; i < len(ids); i += mentionResolveBatchSize {
		end := i + mentionResolveBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		data, err := runtime.CallAPITyped(http.MethodGet, "/open-apis/contact/v3/users/batch",
			map[string]interface{}{"user_id_type": fromType, "user_ids": ids[i:end]}, nil)
		if err != nil {
			return err
		}
		items, _ := data["items"].([]interface{})
		for _, it := range items {
			u, _ := it.(map[string]interface{})
			if u == nil {
				continue
			}
			from, _ := u[fromType].(string)
			val, _ := u[wantField].(string)
			if from != "" && val != "" {
				out[from] = val
			}
		}
	}
	return nil
}

func mapKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
