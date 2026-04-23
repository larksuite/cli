// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// maxReplaceParts matches the server-side cap declared in meta_data.json
// ("最少1条，最多200条"). Enforced client-side so a too-large batch fails fast
// with a clear message instead of a 400 from the backend.
const maxReplaceParts = 200

// SlidesReplaceSlide wraps slides.xml_presentation.slide.replace with specific
// value-adds over the raw auto-generated command:
//
//  1. It accepts --presentation as token / slides URL / wiki URL (and resolves
//     wiki tokens), same as other slides shortcuts.
//  2. For every `block_replace` part it auto-injects `id="<block_id>"` into the
//     root element of `replacement`. The backend requires the replacement
//     fragment's root carry that id and returns 3350001 otherwise; the
//     requirement is undocumented and catches callers repeatedly, so we fix it
//     at the CLI layer.
//  3. For `<shape>` elements it auto-injects `<content/>` when missing. The
//     SML 2.0 schema requires every shape to carry a content child; omitting
//     it triggers 3350001.
//  4. On 3350001 errors it enriches the hint with context-specific guidance
//     so AI agents can self-correct.
//
// `str_replace` is intentionally NOT exposed: product direction is that
// slide edits go through structural (block-level) operations only. The backend
// still accepts str_replace, but the CLI rejects it up front.
var SlidesReplaceSlide = common.Shortcut{
	Service:     "slides",
	Command:     "+replace-slide",
	Description: "Replace elements on a slide via block_replace / block_insert parts (auto-injects id + <content/> on shape elements)",
	Risk:        "write",
	Scopes:      []string{"slides:presentation:update", "slides:presentation:write_only", "wiki:node:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "presentation", Desc: "xml_presentation_id, slides URL, or wiki URL that resolves to slides", Required: true},
		{Name: "slide-id", Desc: "slide page identifier (slide_id)", Required: true},
		{Name: "parts", Desc: "JSON array of replace parts (each: {action: block_replace|block_insert, ...}); max 200", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "revision-id", Type: "int", Default: "-1", Desc: "presentation revision (-1 = latest; pass a specific number for optimistic locking)"},
		{Name: "tid", Desc: "transaction id for concurrent-edit locking (usually empty)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := parsePresentationRef(runtime.Str("presentation")); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("slide-id")) == "" {
			return common.FlagErrorf("--slide-id cannot be empty")
		}
		parts, err := parseReplaceParts(runtime.Str("parts"))
		if err != nil {
			return err
		}
		if err := validateReplaceParts(parts); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		parts, err := parseReplaceParts(runtime.Str("parts"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		if err := validateReplaceParts(parts); err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		// Apply the same id-injection the real Execute does, so dry-run body
		// shows what will actually be sent.
		injected, err := injectBlockReplaceIDs(parts)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		slideID := runtime.Str("slide-id")
		query := map[string]interface{}{
			"slide_id":    slideID,
			"revision_id": runtime.Int("revision-id"),
		}
		if tid := runtime.Str("tid"); tid != "" {
			query["tid"] = tid
		}
		body := map[string]interface{}{"parts": injected}

		dry := common.NewDryRunAPI()
		presentationID := ref.Token
		if ref.Kind == "wiki" {
			presentationID = "<resolved_slides_token>"
			dry.Desc("2-step orchestration: resolve wiki → replace slide parts").
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to slides presentation").
				Params(map[string]interface{}{"token": ref.Token})
		} else {
			dry.Desc(fmt.Sprintf("Replace %d part(s) on slide %s", len(parts), slideID))
		}
		dry.POST(fmt.Sprintf(
			"/open-apis/slides_ai/v1/xml_presentations/%s/slide/replace",
			validate.EncodePathSegment(presentationID),
		)).
			Params(query).
			Body(body)
		return dry.Set("parts_count", len(parts))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		presentationID, err := resolvePresentationID(runtime, ref)
		if err != nil {
			return err
		}
		slideID := strings.TrimSpace(runtime.Str("slide-id"))

		parts, err := parseReplaceParts(runtime.Str("parts"))
		if err != nil {
			return err
		}
		if err := validateReplaceParts(parts); err != nil {
			return err
		}
		injected, err := injectBlockReplaceIDs(parts)
		if err != nil {
			return err
		}

		query := map[string]interface{}{
			"slide_id":    slideID,
			"revision_id": runtime.Int("revision-id"),
		}
		if tid := strings.TrimSpace(runtime.Str("tid")); tid != "" {
			query["tid"] = tid
		}
		body := map[string]interface{}{"parts": injected}

		url := fmt.Sprintf(
			"/open-apis/slides_ai/v1/xml_presentations/%s/slide/replace",
			validate.EncodePathSegment(presentationID),
		)
		data, err := runtime.CallAPI("POST", url, query, body)
		if err != nil {
			return enrichSlidesReplaceError(err)
		}

		result := map[string]interface{}{
			"xml_presentation_id": presentationID,
			"slide_id":            slideID,
			"parts_count":         len(injected),
		}
		// Presence check (not `v > 0`) mirrors the failed_part_index / failed_reason
		// branches below, so behavior stays consistent across the three fields.
		if _, ok := data["revision_id"]; ok {
			result["revision_id"] = int(common.GetFloat(data, "revision_id"))
		}
		// Backend reports partial failures via failed_part_index / failed_reason.
		// Surface them untouched so the caller can react.
		if raw, ok := data["failed_part_index"]; ok {
			result["failed_part_index"] = raw
		}
		if raw, ok := data["failed_reason"]; ok {
			result["failed_reason"] = raw
		}

		runtime.Out(result, nil)
		return nil
	},
}

// replacePart is the normalized (post-JSON) representation of one entry in the
// parts array. Fields are nullable so we can tell "not provided" from "empty".
type replacePart struct {
	Action              string
	Replacement         *string
	BlockID             *string
	Insertion           *string
	InsertBeforeBlockID *string
}

// parseReplaceParts decodes the --parts JSON into typed structs.
//
// Accepts JSON with extra keys (pattern / is_multiple) so that a user who
// copy-pasted a doc example doesn't get a decoder error; those keys are
// ignored because str_replace isn't exposed. validateReplaceParts enforces
// that nothing from the str_replace family actually gets used.
func parseReplaceParts(raw string) ([]replacePart, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, common.FlagErrorf("--parts cannot be empty")
	}
	var decoded []map[string]interface{}
	if err := json.Unmarshal([]byte(s), &decoded); err != nil {
		return nil, common.FlagErrorf("--parts invalid JSON, must be an array of objects: %v", err)
	}
	out := make([]replacePart, 0, len(decoded))
	for i, m := range decoded {
		p := replacePart{}
		if v, ok := m["action"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, common.FlagErrorf("--parts[%d].action must be a string", i)
			}
			p.Action = s
		}
		if v, ok := m["replacement"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, common.FlagErrorf("--parts[%d].replacement must be a string", i)
			}
			p.Replacement = &s
		}
		if v, ok := m["block_id"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, common.FlagErrorf("--parts[%d].block_id must be a string", i)
			}
			p.BlockID = &s
		}
		if v, ok := m["insertion"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, common.FlagErrorf("--parts[%d].insertion must be a string", i)
			}
			p.Insertion = &s
		}
		if v, ok := m["insert_before_block_id"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, common.FlagErrorf("--parts[%d].insert_before_block_id must be a string", i)
			}
			p.InsertBeforeBlockID = &s
		}
		out = append(out, p)
	}
	return out, nil
}

const larkCodeSlidesInvalidParam = 3350001

// slides3350001Hint is the generic checklist attached to 3350001 errors.
// 3350001 is a catch-all on the backend; listing the common root causes gives
// AI agents and humans a concrete starting point. Mixed block_replace+block_insert
// batches are supported, so splitting them is deliberately NOT suggested.
const slides3350001Hint = "common causes: (1) block_id not found in current slide — re-run slide.get for latest XML; (2) invalid XML structure or unsupported element; (3) element coordinates exceed slide bounds (960×540)"

// enrichSlidesReplaceError attaches slides3350001Hint when the API returns
// 3350001 (invalid param). Other error codes pass through untouched.
func enrichSlidesReplaceError(err error) error {
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil || exitErr.Detail.Code != larkCodeSlidesInvalidParam {
		return err
	}
	// Only fall back to the generic checklist when no upstream hint is
	// already attached — don't clobber a more specific hint set by the
	// backend or an earlier wrapper.
	if exitErr.Detail.Hint == "" {
		exitErr.Detail.Hint = slides3350001Hint
	}
	return exitErr
}

// validateReplaceParts enforces CLI-level invariants:
//   - size is within [1, 200]
//   - action is one of the exposed actions (block_replace / block_insert)
//   - per-action required fields are present
func validateReplaceParts(parts []replacePart) error {
	if len(parts) == 0 {
		return common.FlagErrorf("--parts must contain at least 1 item")
	}
	if len(parts) > maxReplaceParts {
		return common.FlagErrorf("--parts contains %d items, exceeds maximum of %d", len(parts), maxReplaceParts)
	}
	for i, p := range parts {
		switch p.Action {
		case "block_replace":
			if p.BlockID == nil || strings.TrimSpace(*p.BlockID) == "" {
				return common.FlagErrorf("--parts[%d] (block_replace) requires non-empty block_id", i)
			}
			if p.Replacement == nil || strings.TrimSpace(*p.Replacement) == "" {
				return common.FlagErrorf("--parts[%d] (block_replace) requires non-empty replacement", i)
			}
		case "block_insert":
			if p.Insertion == nil || strings.TrimSpace(*p.Insertion) == "" {
				return common.FlagErrorf("--parts[%d] (block_insert) requires non-empty insertion", i)
			}
		case "str_replace":
			// Backend still accepts str_replace, but product decision is to
			// force structural edits through the CLI. Block it up-front so
			// users don't build tooling around an option we won't keep.
			return common.FlagErrorf("--parts[%d] action %q is not supported by this shortcut; use block_replace or block_insert", i, p.Action)
		case "":
			return common.FlagErrorf("--parts[%d].action is required", i)
		default:
			return common.FlagErrorf("--parts[%d] unknown action %q, supported: block_replace, block_insert", i, p.Action)
		}
	}
	return nil
}

// injectBlockReplaceIDs rewrites each block_replace part's `replacement` so
// that the root element carries id="<block_id>". Backend (3350001) requires
// this; doing it in the CLI means users write natural-looking XML (e.g.
// `<shape type="rect">…</shape>`) and get the id stitched in automatically.
//
// Returns a slice of `map[string]interface{}` ready to be encoded as the
// request body, preserving field order handed to the JSON encoder.
func injectBlockReplaceIDs(parts []replacePart) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(parts))
	for i, p := range parts {
		m := map[string]interface{}{"action": p.Action}
		switch p.Action {
		case "block_replace":
			fixed, err := ensureXMLRootID(*p.Replacement, *p.BlockID)
			if err != nil {
				return nil, output.ErrValidation("--parts[%d].replacement: %v", i, err)
			}
			fixed = ensureShapeHasContent(fixed)
			m["block_id"] = *p.BlockID
			m["replacement"] = fixed
		case "block_insert":
			m["insertion"] = ensureShapeHasContent(*p.Insertion)
			if p.InsertBeforeBlockID != nil {
				m["insert_before_block_id"] = *p.InsertBeforeBlockID
			}
		}
		out = append(out, m)
	}
	return out, nil
}
