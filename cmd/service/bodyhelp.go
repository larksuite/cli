// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// bodyhelp renders the request-body contract into a method command's help so
// that help is self-sufficient: whoever reached `<method> --help` can construct
// the call without a follow-up schema query. Scope is deliberately input-only —
// the response structure stays in `lark-cli schema <path>`, which is what the
// pointer at the end of method help is for.

package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/schema"
)

// bodyHelp renders the --data contract for the method's declared body fields,
// or "" when the method declares none (the bare --data escape hatch then keeps
// its current one-line usage). Skeleton first (a copyable JSON shape), then one
// facts line per field. Nesting renders one level deep; anything deeper is left
// to the schema pointer rather than inflating every method's help.
func bodyHelp(fields []meta.Field) string {
	if len(fields) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Request body (--data '<json>'):\n")
	b.WriteString("  " + bodySkeleton(fields) + "\n")
	b.WriteString("  Fields:\n")
	for _, f := range fields {
		writeFieldLine(&b, f, "    ", false)
		for _, ch := range f.Children() {
			// Second level is where rendering stops, so a child that has
			// children of its own is the point where the contract goes silent.
			writeFieldLine(&b, ch, "      ", true)
		}
	}
	return b.String()
}

// atDepthLimit says this line is the deepest one rendered, so any children of
// its own will not appear below it.
func writeFieldLine(b *strings.Builder, f meta.Field, indent string, atDepthLimit bool) {
	req := "optional"
	if f.Required {
		req = "required"
	}
	// Name and type come from the same upstream document as the description, so
	// they get the same sanitizing — a facts line that cleaned only one of the
	// three would still be forgeable through the others.
	line := fmt.Sprintf("%s%s  (%s, %s)", indent,
		schema.SanitizeIndexDesc(f.Name), req, schema.SanitizeIndexDesc(f.CanonicalType()))
	if d := schema.SanitizeIndexDesc(schema.FirstSentence(f.Description)); d != "" {
		line += "  " + d
	}
	// Allowed values and bounds are what make the line enough to construct the
	// call with: a description like "参与人权限" names the field but not its four
	// legal values, and "智能体任务状态" hides the 1-4 range entirely. Only the two
	// inline formatters are shared with the param-flag side — NOT fieldFacts,
	// which also carries a cobra-specific bool clause and cuts descriptions at
	// `；` (that would truncate body descriptions such as mode's "任务完成模式, 1 -
	// 会签任务; 2 - 或签任务" to just the first alternative).
	//
	// The enum clause is sanitized because both the values and their meanings are
	// upstream text, same as the name/type/description above; bounds come from
	// strconv, so they carry nothing to sanitize.
	if opts := f.EnumOptions(); len(opts) > 0 {
		line += "  enum: " + schema.SanitizeIndexDesc(formatEnumInline(opts))
	}
	if bounds := formatBoundsInline(f); bounds != "" {
		line += "  " + bounds
	}
	// A field whose children are out of budget gets {} in the skeleton, which is
	// the same shape a genuinely empty object gets. Left at that the two are
	// indistinguishable, and "consult the schema only for deep nesting" has
	// nothing to act on — the caller would need to already know the nesting is
	// there in order to know to look for it.
	if atDepthLimit {
		if n := len(f.Children()); n > 0 {
			noun := "fields"
			if n == 1 {
				noun = "field"
			}
			line += fmt.Sprintf("  nested: %d %s not shown", n, noun)
		}
	}
	b.WriteString(line + "\n")
}

// bodySkeleton renders a single-line JSON object with type placeholders, e.g.
// {"receive_id": "<string>", "members": [{"id": "<string>"}]}. It is meant to be
// copied straight into --data, so field names are JSON-quoted rather than
// interpolated: a name carrying a quote or newline must not be able to break the
// shape.
func bodySkeleton(fields []meta.Field) string {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		parts = append(parts, quoteJSONKey(f.Name)+": "+skeletonValue(f, 1))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// quoteJSONKey renders name as a JSON string literal. The name is sanitized
// first: json.Marshal escapes quotes, backslashes and C0, but passes bidi
// controls, C1 and zero-width characters through — which would let the skeleton
// line and the facts line below it disagree about what a field is called, and
// let two different fields render identically.
func quoteJSONKey(name string) string {
	name = schema.SanitizeIndexDesc(name)
	if q, err := json.Marshal(name); err == nil {
		return string(q)
	}
	return fmt.Sprintf("%q", name)
}

// skeletonValue renders one field's placeholder. Two rules govern the whole
// skeleton, and the array branch below is an instance of the first, not a
// special case.
//
// One: a placeholder should not positively assert something the API rejects. The
// skeleton is meant to be copied verbatim and --dry-run does not validate the
// body, so a wrong assertion has nothing local to catch it. This is a default,
// not an invariant — two cases knowingly fall short of it. A shape past the
// nesting budget renders {} or [{}], which the API rejects when it has required
// children, and an integer that upstream documents as a formatted value renders
// 0 (attendance user_tasks query's required check_date_from is a yyyyMMdd date).
// Both predate the enum and bounds work. The date one is not for want of data —
// upstream declares example "20190817" — but for the reason `example` was left
// out generally: it is not uniformly better than what renders today, and
// reaching for it only where the current value is illegal would make a field's
// placeholder depend on metadata the reader cannot see.
//
// Two: an OPTIONAL field keeps a bare type marker, even when a legal value is
// known. Filling one in makes the skeleton read as ready to send while quietly
// deciding something the caller never asked about: 56 of the enum-carrying body
// fields sit on write or high-risk-write methods, four of them on drive
// permission.public.patch, where share_entity's first allowed value is "anyone",
// the most permissive of its three. That first value tracks upstream field
// order, not caution — attendee_ability starts at the strictest value and
// share_entity at the loosest. Nothing is lost by leaving them alone: the facts
// line below the skeleton lists the allowed values, so the skeleton is no longer
// the only place to learn them.
//
// A REQUIRED field is the opposite case. The caller cannot omit it, so a legal
// value is a head start rather than a decision taken on their behalf, and it is
// mostly type discriminators that benefit (obj_type, image_type, event_type).
//
// The rules conflict only where an optional field's marker is itself illegal — a
// numeric floor above 0. Rule two wins there, and what carries it is uniformity
// rather than the rejection: an optional number now always reads 0, where before
// it read either 0 or a floor depending on metadata the reader cannot see. Do not
// lean on "the API will reject it" — a page_size of 0 is as likely to be coerced
// to unset, and on a read method there is no protective upside at all. The cost
// is visible on approval tasks add_sign, which renders add_sign_type 1 beside
// approval_method 0 with the same enum and the same bounds, purely because one is
// required; that reads as a bug before it reads as a rule.
//
// Rule two is also imperfectly served by the markers available. Only a string has
// one that reads as a placeholder; an optional number renders 0 and an optional
// boolean false, both concrete legal values, and there are 88 such fields. They
// are at least not chosen by position the way an enum's first value is, and no
// optional boolean in the catalog documents a true default — but false is only
// the inert side of an IMPERATIVE flag. For a STATE flag it is a change: im chats
// update's restricted_mode_setting.status is 保密模式是否开启, so a copied false
// turns that control off. is_muted, is_mute_at_all and archive_tasklist are the
// same shape. Those predate this work and are not addressed here.
func skeletonValue(f meta.Field, depth int) string {
	ct := f.CanonicalType()
	switch ct {
	case "object":
		// Both reasons to stop collapse to {} here, unlike the array case below:
		// JSON offers no second empty object to tell them apart, so the shape
		// cannot carry the distinction. The facts line carries it instead — a
		// field truncated for budget is marked with the count it is hiding.
		if depth <= 0 || len(f.Properties) == 0 {
			return "{}"
		}
		return "{" + joinProperties(f, depth) + "}"
	case "array":
		// The metadata wraps an array's *element* schema in "properties", so
		// these children describe one element rather than sibling fields. The
		// two reasons to stop recursing are not interchangeable here: with no
		// element schema the array really is scalar, but running out of nesting
		// budget says nothing about the element type. Collapsing both to
		// ["<item>"] would assert "array of strings" for an array of objects —
		// the wrong assertion the rule above forbids. So the budget case
		// degrades to [{}], mirroring how an object degrades to {}.
		if len(f.Properties) == 0 {
			return `["<item>"]`
		}
		if depth <= 0 {
			return "[{}]"
		}
		return "[{" + joinProperties(f, depth) + "}]"
	}

	// Scalars, per rule two: only a required field is filled in.
	if f.Required {
		if opts := f.EnumOptions(); len(opts) > 0 {
			if v, ok := skeletonLiteral(opts[0].Value); ok {
				return v
			}
		}
		// The floor displaces 0 only when 0 is below it. Reaching for it
		// unconditionally trades a legal neutral value for a legal absurd one: okr
		// indicators patch declares min -99999999999 on three value fields whose
		// upstream example is plain 0. No field declares a max below 0, so "below
		// the floor" is the whole of "out of range" here.
		//
		// Only numbers consult it: min/max are type-agnostic upstream and may bound
		// a value or a string's length (see meta.Field.MinBound), and a length
		// reading is meaningless only for numbers.
		if ct == "integer" || ct == "number" {
			// The bound is carried as a json.Number so a large integer floor
			// survives verbatim; only the sign test needs a float, and that
			// comparison is exact enough for it.
			if min := f.MinBound(); min != nil {
				if floor, err := min.Float64(); err == nil && floor > 0 {
					if v, ok := skeletonLiteral(*min); ok {
						return v
					}
				}
			}
		}
	}
	switch ct {
	case "boolean":
		return "false"
	case "integer", "number":
		return "0"
	default:
		return `"<string>"`
	}
}

// skeletonLiteral renders a concrete placeholder — an allowed value or a numeric
// floor — as a JSON literal, reporting false when it cannot be rendered both
// safely and faithfully. Callers fall back to the type marker.
//
// Two guards, both live. The marshal error covers non-finite floats: a min or an
// allowed value arrives as a string and meta.coerceLiteral reaches "number"
// through strconv.ParseFloat, which accepts "Inf" and "NaN"; rendering those with
// strconv would print `+Inf` and cost the skeleton its one hard promise, that it
// parses. json.Marshal refuses them.
//
// The sanitize comparison covers upstream text, which reaches here through a
// required string enum's allowed value. json.Marshal escapes quotes, backslashes
// and C0 but passes bidi controls, C1 and zero-width characters through — the
// same gap quoteJSONKey covers for field names. Sanitizing a *value* is not the
// fix it is for a name, though: the sanitized text is no longer the value the API
// accepts, so it would trade a rendering risk for the wrong-assertion risk
// skeletonValue exists to prevent. Hence reject rather than clean.
//
// Reading the rendered literal rather than the Go value also means neither guard
// depends on the value's type, at any nesting depth.
//
// Marshal FIRST, then compare — the order is load-bearing, not stylistic.
// Comparing the marshalled form puts quotes around the payload, so
// SanitizeIndexDesc's TrimSpace cannot reach a value's own leading or trailing
// space, and a tab arrives already escaped to `\t` so it cannot trip the
// whitespace-run collapse. Sanitizing before marshalling loses both properties
// and starts rejecting legitimate values.
func skeletonLiteral(v any) (string, bool) {
	q, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	s := string(q)
	// A composite literal would replace the field's shape rather than fill it in,
	// which is rule one's own failure mode. Only the scalar branches call this, but
	// meta.coerceLiteral passes a string-typed field's allowed values through
	// untouched, so an object or a list can arrive here from upstream.
	if s == "" || s[0] == '{' || s[0] == '[' {
		return "", false
	}
	if schema.SanitizeIndexDesc(s) != s {
		return "", false
	}
	return s, true
}

// joinProperties renders f's children as JSON members, one nesting level down.
func joinProperties(f meta.Field, depth int) string {
	children := f.Children()
	inner := make([]string, 0, len(children))
	for _, ch := range children {
		inner = append(inner, quoteJSONKey(ch.Name)+": "+skeletonValue(ch, depth-1))
	}
	return strings.Join(inner, ", ")
}
