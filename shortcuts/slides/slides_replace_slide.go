// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/larksuite/cli/errs"
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
//  5. It asks the backend to lint the page these parts produce, and renders the
//     refusal when the lint blocks the write. --no-lint opts out.
//
// `str_replace` is intentionally NOT exposed: product direction is that
// slide edits go through structural (block-level) operations only. The backend
// still accepts str_replace, but the CLI rejects it up front.
var SlidesReplaceSlide = common.Shortcut{
	Service:     "slides",
	Command:     "+replace-slide",
	Description: "Replace elements on a slide via block_replace / block_insert parts (auto-injects id + <content/> on shape elements)",
	Risk:        "write",
	Scopes:      []string{"slides:presentation:update", "slides:presentation:write_only"},
	// wiki:node:read is required only when --presentation is a wiki URL.
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: []common.Flag{
		requiredPresentationRefFlag(),
		{Name: "slide-id", Desc: "slide page identifier (slide_id)", Required: true},
		{Name: "parts", Desc: "JSON array of replace parts; accepts replace/insert action aliases, target_id for block_id, and block/content/shape/element for the action's XML payload; max 200", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "revision-id", Type: "int", Default: "-1", Desc: "presentation revision (-1 = latest; pass a specific number for optimistic locking)"},
		{Name: "tid", Desc: "transaction id for concurrent-edit locking (usually empty)"},
		noLintFlag(),
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ref, err := parsePresentationRef(runtime.Str("presentation"))
		if err != nil {
			return err
		}
		if ref.Kind == "wiki" {
			if err := runtime.EnsureScopes([]string{"wiki:node:read"}); err != nil {
				return err
			}
		}
		if strings.TrimSpace(runtime.Str("slide-id")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--slide-id cannot be empty").WithParam("--slide-id")
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
		parts, normalizations, err := parseReplacePartsWithNormalization(runtime.Str("parts"))
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
		body := replaceSlideBody(injected, runtime)

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
		dry.POST(slideReplaceAPIPath(presentationID)).
			Params(query).
			Body(body)
		dry.Set("parts_count", len(parts))
		if len(normalizations) > 0 {
			dry.Set("normalizations", normalizations)
		}
		return dry
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

		parts, normalizations, err := parseReplacePartsWithNormalization(runtime.Str("parts"))
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
		body := replaceSlideBody(injected, runtime)

		data, err := runtime.CallAPITyped("POST", slideReplaceAPIPath(presentationID), query, body)
		if err != nil {
			// Lint first: enrichSlidesReplaceError only fills an empty hint, so
			// running it second leaves the specific lint finding in place and
			// the generic 3350001 checklist for everything else.
			return enrichSlidesReplaceError(enrichSlidesLintError(err))
		}

		result := map[string]interface{}{
			"xml_presentation_id": presentationID,
			"slide_id":            slideID,
			"parts_count":         len(injected),
		}
		if len(normalizations) > 0 {
			result["normalizations"] = normalizations
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
		// issues points the other way from failed_reason: the parts were applied
		// and committed, and the backend still had something to say about the page
		// they produced. A finding serious enough to refuse the write leaves as an
		// error carrying the same report, so whatever arrives here describes a page
		// that is already stored.
		if raw, ok := data["issues"]; ok {
			result["issues"] = raw
		}

		runtime.Out(result, nil)
		return nil
	},
}

// replaceSlideBody builds the request body shared by dry-run and execute, so the
// two cannot disagree about the lint switch — the failure mode being a --dry-run
// that shows a linted request and an execute that sends an unlinted one.
func replaceSlideBody(parts []map[string]interface{}, runtime *common.RuntimeContext) map[string]interface{} {
	return withLintXML(map[string]interface{}{"parts": parts}, runtime)
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

// replacePartNormalization records each compatibility conversion so callers can
// see the exact canonical request shape the CLI chose.
type replacePartNormalization struct {
	PartIndex int    `json:"part_index"`
	Kind      string `json:"kind"`
	From      string `json:"from"`
	To        string `json:"to"`
}

// parseReplaceParts decodes the --parts JSON into typed structs.
func parseReplaceParts(raw string) ([]replacePart, error) {
	parts, _, err := parseReplacePartsWithNormalization(raw)
	return parts, err
}

// parseReplacePartsWithNormalization applies only aliases whose semantics are
// determined by the action, then runs the existing strict field and type checks.
// Ambiguous actions remain errors; they are never guessed into block operations.
func parseReplacePartsWithNormalization(raw string) ([]replacePart, []replacePartNormalization, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts cannot be empty").WithParam("--parts")
	}
	var decoded []map[string]interface{}
	if err := json.Unmarshal([]byte(s), &decoded); err != nil {
		return nil, nil, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--parts invalid JSON, must be an array of objects: %v",
			err,
		).WithParam("--parts").
			WithHint("avoid shell-escaping errors by passing `--parts @parts.json` or piping JSON to `--parts -`").
			WithCause(err)
	}
	out := make([]replacePart, 0, len(decoded))
	normalizations := make([]replacePartNormalization, 0)
	for i, m := range decoded {
		itemNormalizations, err := normalizeReplacePartAliases(i, m)
		if err != nil {
			return nil, nil, err
		}
		normalizations = append(normalizations, itemNormalizations...)

		p := replacePart{}
		if v, ok := m["action"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d].action must be a string", i).WithParam("--parts")
			}
			p.Action = s
		} else if err := checkMisspelledAction(i, m); err != nil {
			// "Action" selects no schema, so the per-action check below would skip
			// the part entirely and validateReplaceParts would only say action is
			// required. Name the misspelling instead.
			return nil, nil, err
		}
		if err := checkReplacePartFields(i, m, p.Action); err != nil {
			return nil, nil, err
		}
		if v, ok := m["replacement"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d].replacement must be a string", i).WithParam("--parts")
			}
			p.Replacement = &s
		}
		if v, ok := m["block_id"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d].block_id must be a string", i).WithParam("--parts")
			}
			p.BlockID = &s
		}
		if v, ok := m["insertion"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d].insertion must be a string", i).WithParam("--parts")
			}
			p.Insertion = &s
		}
		if v, ok := m["insert_before_block_id"]; ok {
			s, ok := v.(string)
			if !ok {
				return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d].insert_before_block_id must be a string", i).WithParam("--parts")
			}
			p.InsertBeforeBlockID = &s
		}
		out = append(out, p)
	}
	return out, normalizations, nil
}

var replaceActionAliases = map[string]string{
	"replace": "block_replace",
	"insert":  "block_insert",
}

var compatibleXMLPayloadAliases = []string{"block", "content", "element", "shape"}

func normalizeReplacePartAliases(i int, part map[string]interface{}) ([]replacePartNormalization, error) {
	normalizations := make([]replacePartNormalization, 0)
	rawAction, hasAction := part["action"]
	if !hasAction {
		return normalizations, nil
	}
	action, ok := rawAction.(string)
	if !ok {
		return normalizations, nil
	}
	if canonical, ok := replaceActionAliases[action]; ok {
		part["action"] = canonical
		normalizations = append(normalizations, replacePartNormalization{
			PartIndex: i, Kind: "action", From: action, To: canonical,
		})
		action = canonical
	}

	schema, ok := replacePartSchemas[action]
	if !ok {
		return normalizations, nil
	}
	if action == "block_replace" {
		changed, err := normalizeReplacePartField(i, part, "target_id", "block_id")
		if err != nil {
			return nil, err
		}
		if changed {
			normalizations = append(normalizations, replacePartNormalization{
				PartIndex: i, Kind: "field", From: "target_id", To: "block_id",
			})
		}
	}
	for _, alias := range compatibleXMLPayloadAliases {
		changed, err := normalizeReplacePartField(i, part, alias, schema.payload)
		if err != nil {
			return nil, err
		}
		if changed {
			normalizations = append(normalizations, replacePartNormalization{
				PartIndex: i, Kind: "field", From: alias, To: schema.payload,
			})
		}
	}
	return normalizations, nil
}

func normalizeReplacePartField(i int, part map[string]interface{}, alias, canonical string) (bool, error) {
	aliasValue, hasAlias := part[alias]
	if !hasAlias {
		return false, nil
	}
	if canonicalValue, hasCanonical := part[canonical]; hasCanonical && !valuesEqual(canonicalValue, aliasValue) {
		return false, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--parts[%d] fields %q and %q conflict; pass only %q, or give both the same value",
			i, canonical, alias, canonical,
		).WithParam("--parts")
	}
	if _, hasCanonical := part[canonical]; !hasCanonical {
		part[canonical] = aliasValue
	}
	delete(part, alias)
	return true, nil
}

func valuesEqual(a, b interface{}) bool {
	aJSON, aErr := json.Marshal(a)
	bJSON, bErr := json.Marshal(b)
	return aErr == nil && bErr == nil && string(aJSON) == string(bJSON)
}

// replacePartSchema describes one exposed action: the fields it accepts, the
// field carrying its XML fragment, and a correct one-liner to echo back.
type replacePartSchema struct {
	fields  []string
	payload string
	hint    string
}

// replacePartSchemas is the field contract of the two exposed actions. Holding
// it in one place lets the error name the exact set the caller may use, instead
// of a generic "unknown field".
var replacePartSchemas = map[string]replacePartSchema{
	"block_replace": {
		fields:  []string{"action", "block_id", "replacement"},
		payload: "replacement",
		hint:    `correct shape: {"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"text\"><content><p>text</p></content></shape>"}`,
	},
	"block_insert": {
		fields:  []string{"action", "insertion", "insert_before_block_id"},
		payload: "insertion",
		hint:    `correct shape: {"action":"block_insert","insertion":"<shape type=\"rect\" width=\"100\" height=\"100\"/>"}`,
	},
}

// xmlPayloadAliases are field names callers reach for when they mean the XML
// payload. Exact deterministic aliases are normalized earlier; this broader
// list remains for unsupported spellings and casing variants so their errors
// can still suggest the action's canonical payload field. internal/suggest
// cannot route these reliably: edit distance misses most of them outright
// (content → replacement), and prefix ranking sends block_xml to block_id.
//
// Entries are limited to names actually observed carrying a fragment. A
// shape attribute like "fill" is deliberately absent: whoever writes it means
// "recolor this block", not "here is my XML", so pointing them at replacement
// would be guessing. The unknown-field error already names the valid set.
var xmlPayloadAliases = []string{"block", "block_xml", "content", "data", "element", "new_xml", "xml"}

// normalizeFieldKey folds case and the _/- separators so "Content", "newXml",
// "new-xml" and "new_xml" collapse onto one key. Callers pick the casing of
// whatever language they happen to be thinking in (the CLI's own flags are
// kebab-case), and the suggestion should survive that; the whitelist itself
// stays exact, because the API only accepts snake_case.
func normalizeFieldKey(k string) string {
	k = strings.ReplaceAll(k, "_", "")
	k = strings.ReplaceAll(k, "-", "")
	return strings.ToLower(k)
}

// matchNormalized returns the candidate that matches k once both are
// normalized, so "blockId" resolves to "block_id".
func matchNormalized(k string, candidates []string) (string, bool) {
	nk := normalizeFieldKey(k)
	for _, c := range candidates {
		if normalizeFieldKey(c) == nk {
			return c, true
		}
	}
	return "", false
}

// checkMisspelledAction runs when a part has no exact "action" key: if some key
// normalizes to it, that spelling is the real problem, and saying so beats the
// bare "action is required" the caller would otherwise get.
func checkMisspelledAction(i int, m map[string]interface{}) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if normalizeFieldKey(k) != "action" {
			continue
		}
		err := errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--parts[%d] unknown field %q; did you mean %q?", i, k, "action",
		).WithParam("--parts")
		// The misspelled key still holds the action name, so the example can be
		// the one the caller was actually reaching for.
		if name, ok := m[k].(string); ok {
			if schema, known := replacePartSchemas[name]; known {
				return err.WithHint("%s", schema.hint)
			}
		}
		return err
	}
	return nil
}

// checkReplacePartFields rejects fields that don't belong to the part's action,
// naming the field the caller most likely meant. Actions this shortcut doesn't
// expose ("", str_replace, unknown) are skipped so their own errors from
// validateReplaceParts still win.
func checkReplacePartFields(i int, m map[string]interface{}, action string) error {
	schema, ok := replacePartSchemas[action]
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Sorted so a part carrying several bad fields always reports the same one.
	slices.Sort(keys)
	for _, k := range keys {
		if slices.Contains(schema.fields, k) {
			continue
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"%s", unknownPartFieldMessage(i, k, action, schema),
		).WithParam("--parts").WithHint("%s", schema.hint)
	}
	return nil
}

// unknownPartFieldMessage builds the error text for one rejected field,
// naming the field the caller most likely meant.
func unknownPartFieldMessage(i int, key, action string, schema replacePartSchema) string {
	valid := fmt.Sprintf("Valid fields for %s: %s", action, strings.Join(schema.fields, ", "))
	// Right field, wrong casing or separator (e.g. "blockId", "block-id").
	if field, ok := matchNormalized(key, schema.fields); ok {
		return fmt.Sprintf("--parts[%d] unknown field %q; did you mean %q? %s", i, key, field, valid)
	}
	if _, ok := matchNormalized(key, xmlPayloadAliases); ok {
		return fmt.Sprintf("--parts[%d] unknown field %q; did you mean %q? %s", i, key, schema.payload, valid)
	}
	// Other actions are checked in sorted-name order so the one named in the
	// error stays deterministic even if a future field matches several.
	others := make([]string, 0, len(replacePartSchemas))
	for name := range replacePartSchemas {
		if name != action {
			others = append(others, name)
		}
	}
	slices.Sort(others)
	for _, other := range others {
		if _, ok := matchNormalized(key, replacePartSchemas[other].fields); ok {
			return fmt.Sprintf("--parts[%d] unknown field %q; it belongs to %s. %s", i, key, other, valid)
		}
	}
	return fmt.Sprintf("--parts[%d] unknown field %q. %s", i, key, valid)
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
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != larkCodeSlidesInvalidParam {
		return err
	}
	// Only fall back to the generic checklist when no upstream hint is
	// already attached — don't clobber a more specific hint set by the
	// backend or an earlier wrapper. p points at the embedded Problem, so
	// the mutation is reflected in the returned err.
	if p.Hint == "" {
		p.Hint = slides3350001Hint
	}
	return err
}

// validateReplaceParts enforces CLI-level invariants:
//   - size is within [1, 200]
//   - action is one of the exposed actions (block_replace / block_insert)
//   - per-action required fields are present
func validateReplaceParts(parts []replacePart) error {
	if len(parts) == 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts must contain at least 1 item").WithParam("--parts")
	}
	if len(parts) > maxReplaceParts {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts contains %d items, exceeds maximum of %d", len(parts), maxReplaceParts).WithParam("--parts")
	}
	for i, p := range parts {
		switch p.Action {
		case "block_replace":
			if p.BlockID == nil || strings.TrimSpace(*p.BlockID) == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d] (block_replace) requires non-empty block_id", i).WithParam("--parts")
			}
			if p.Replacement == nil || strings.TrimSpace(*p.Replacement) == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d] (block_replace) requires non-empty replacement", i).WithParam("--parts")
			}
		case "block_insert":
			if p.Insertion == nil || strings.TrimSpace(*p.Insertion) == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d] (block_insert) requires non-empty insertion", i).WithParam("--parts")
			}
		case "str_replace":
			// Backend still accepts str_replace, but product decision is to
			// force structural edits through the CLI. Block it up-front so
			// users don't build tooling around an option we won't keep.
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d] action %q is not supported by this shortcut; use block_replace or block_insert", i, p.Action).WithParam("--parts")
		case "replace_all":
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--parts[%d] action %q is not equivalent to a block operation and cannot be normalized safely; use block_replace for each known block or block_insert for new elements",
				i, p.Action,
			).WithParam("--parts")
		case "page_replace", "slide_replace":
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--parts[%d] action %q means whole-page replacement and cannot be normalized to block_replace; use `slides +update-slide` to rewrite the whole page in place",
				i, p.Action,
			).WithParam("--parts")
		case "":
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d].action is required", i).WithParam("--parts")
		default:
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d] unknown action %q, supported: block_replace, block_insert", i, p.Action).WithParam("--parts")
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
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--parts[%d].replacement: %v", i, err).WithParam("--parts").WithCause(err)
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
