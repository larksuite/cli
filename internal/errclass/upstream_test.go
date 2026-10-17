// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
)

func TestBuildAPIError_PreservesUpstreamPermissionDiagnostic(t *testing.T) {
	err := errclass.BuildAPIError(map[string]any{
		"code": 1061004,
		"msg":  "permission denied",
		"error": map[string]any{
			"message": "The requested document belongs to another tenant.",
			"details": []any{map[string]any{"value": "Ask the resource owner to grant access."}},
		},
	}, errclass.ClassifyContext{Identity: "user"})
	typed, ok := errs.UnwrapTypedError(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	b, marshalErr := json.Marshal(typed)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	var wire struct {
		Message  string `json:"message"`
		Upstream *struct {
			Message string   `json:"message"`
			Details []string `json:"details"`
		} `json:"upstream"`
	}
	if decodeErr := json.Unmarshal(b, &wire); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if wire.Upstream == nil {
		t.Fatalf("canonical permission error dropped upstream diagnostic: %s", b)
	}
	if wire.Upstream.Message != "The requested document belongs to another tenant." ||
		len(wire.Upstream.Details) != 1 || wire.Upstream.Details[0] != "Ask the resource owner to grant access." {
		t.Fatalf("upstream diagnostic = %+v", wire.Upstream)
	}
	if wire.Message != "user lacks permission for the requested resource" {
		t.Fatalf("canonical message changed: %q", wire.Message)
	}
}

func TestBuildAPIError_PermissionDiagnosticRedaction(t *testing.T) {
	const secret = "private-value-example"
	cases := map[string]string{
		"bearer":             "The request used Bearer " + secret,
		"unicode_whitespace": "The request used Bearer\u00a0" + secret,
		"multiline_bearer":   "The request used Bearer\n" + secret,
		"ansi_bearer":        "The request used Bear\x1b[31mer " + secret + "\x1b[0m",
		"basic":              "The request used Basic " + secret,
		"header":             "Request denied.\r\nAuthorization: Bearer " + secret,
		"folded_header":      "Request denied.\r\nCookie: first=value\r\n second=" + secret,
		"json_token":         `Request denied: {"access_token":"` + secret + `","extra":"payload"}`,
		"nested_payload":     `Request denied: {"payload":{"name":"` + secret + `"}}`,
		"escaped_quote":      `Request denied for "a\"` + secret + `"`,
		"unclosed_quote":     `Request denied for "` + secret,
		"unclosed_escape":    `Request denied for "` + secret + `\`,
		"single_quote":       "Request denied for '" + secret + "'",
		"unclosed_single":    "Request denied for '" + secret,
		"escaped_newline":    "Request denied for \"a\\\n" + secret + "\"",
		"app_secret":         "Request denied: app_secret = " + secret,
		"password":           "Request denied: PASSWORD: " + secret + " with spaces",
		"api_key":            "Request denied: X-API-Key: " + secret,
		"refresh_token":      "Request denied: refresh-token=" + secret,
		"url":                "Request denied at https://name:" + secret + "@example.test/path?token=value#fragment",
		"email":              "Request denied for " + secret + "@example.test",
		"unicode_email":      "Request denied for 用户@example.test " + `"` + secret + `"`,
		"phone_assignment":   "Request denied: mobile=" + secret,
		"open_id_assignment": "Request denied: open_id=" + secret,
		"format_obfuscation": "Request denied: access_to\u200bken=" + secret,
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			err := errclass.BuildAPIError(map[string]any{
				"code": 1061004, "msg": text,
				"error": map[string]any{"details": []any{map[string]any{"value": text}}},
			}, errclass.ClassifyContext{})
			diagnostic := requirePermissionError(t, err).Upstream
			if diagnostic == nil {
				t.Fatal("missing diagnostic")
			}
			b, marshalErr := json.Marshal(diagnostic)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if strings.Contains(string(b), secret) || !strings.Contains(string(b), "[REDACTED]") {
				t.Fatalf("sensitive diagnostic was not redacted: %s", b)
			}
			if diagnostic.Truncated {
				t.Fatal("redaction alone must not set truncated")
			}
		})
	}

	for _, value := range []string{
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJleGFtcGxlIn0.dummySignature",
		"eyJhbGciOiJub25lIn0.e30.signature",
		"u-exampleAccessToken", "t-exampleTenantToken", "a-exampleAppToken",
		"ou_exampleUserID", "oc_exampleChatID",
		"+1 (202) 555-0100", "+86 138 0000 0000",
		strings.Repeat("s", 40),
	} {
		t.Run(value, func(t *testing.T) {
			err := errclass.BuildAPIError(map[string]any{"code": 1061004, "msg": "Request denied for " + value}, errclass.ClassifyContext{})
			got := requirePermissionError(t, err).Upstream.Message
			if got != "Request denied for [REDACTED]" {
				t.Fatalf("redacted message = %q", got)
			}
		})
	}
}

func TestBuildAPIError_PermissionDiagnosticRedactsCompleteSensitiveValues(t *testing.T) {
	// A narrower redaction must not split a larger sensitive value and leave
	// its identifying or credential-bearing fragments visible to later passes.
	cases := map[string]string{
		"email_with_subdomain": "first.last@team.example.test",
		"email_with_identity":  "ou_exampleUserID@team.example.test",
		"opaque_with_digits":   "prefixKeyExample12345678901234567890suffixKeyExample",
		"opaque_with_identity": "prefixPrivateKey-ou_exampleUserID0123456789suffix",
		"jwt_with_digits":      "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature",
		"url_with_email":       "https://first.last@example.test/path/12345678901234567890",
		"bearer_with_digits":   "Bearer prefixKeyExample12345678901234567890suffixKeyExample",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			text := "Request denied for " + value
			err := errclass.BuildAPIError(map[string]any{
				"code": 1061004, "msg": text,
				"error": map[string]any{"details": []any{map[string]any{"value": text}}},
			}, errclass.ClassifyContext{})
			diagnostic := requirePermissionError(t, err).Upstream
			const want = "Request denied for [REDACTED]"
			if diagnostic == nil || diagnostic.Message != want || len(diagnostic.Details) != 1 || diagnostic.Details[0] != want {
				t.Fatalf("sensitive value was only partially redacted: %+v", diagnostic)
			}
		})
	}
}

func TestBuildAPIError_PermissionDiagnosticMultilineQuotedAssignments(t *testing.T) {
	const secret = "private-value-example"
	cases := map[string]string{
		"double_assignment":         "password=\"first-line\n" + secret + "\"",
		"single_assignment":         "password='first-line\n" + secret + "'",
		"double_payload":            "\"token=first-line\n" + secret + "\"",
		"single_payload":            "'token=first-line\n" + secret + "'",
		"double_assignment_escaped": "password=\"first-line\\\n" + secret + "\"",
		"single_assignment_escaped": "password='first-line\\\n" + secret + "'",
		"double_payload_escaped":    "\"token=first-line\\\n" + secret + "\"",
		"single_payload_escaped":    "'token=first-line\\\n" + secret + "'",
		"adjacent_single_payload":   "'previous''token=first-line\n" + secret + "'",
	}
	for name, value := range cases {
		for closure, payload := range map[string]string{"closed": value, "unclosed": value[:len(value)-1]} {
			t.Run(name+"/"+closure, func(t *testing.T) {
				text := "Request denied for " + payload
				err := errclass.BuildAPIError(map[string]any{
					"code": 1061004, "msg": text,
					"error": map[string]any{"details": []any{map[string]any{"value": text}}},
				}, errclass.ClassifyContext{})
				diagnostic := requirePermissionError(t, err).Upstream
				const want = "Request denied for [REDACTED]"
				if diagnostic == nil || diagnostic.Message != want || len(diagnostic.Details) != 1 || diagnostic.Details[0] != want {
					t.Fatalf("multiline quoted content leaked: %+v", diagnostic)
				}
			})
		}
	}
}

func TestBuildAPIError_PermissionDiagnosticApostrophes(t *testing.T) {
	cases := map[string]string{
		"url_path":          "https://example.test/owner's/private-value-example",
		"url_query":         "https://example.test/path?name=owner's&value=private-value-example",
		"email":             "first.o'connor@example.test",
		"email_punctuation": "first.!#$%&'*+-/=?^_`{|}~last@example.test",
		"quoted_possessive": "'Alice's private payroll'",
		"quoted_name":       "'O'Connor private payroll'",
		"unclosed_quote":    "'Alice's private payroll",
		"adjacent_quotes":   "'Alice's private payroll''O'Connor private payroll'",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			text := "Request denied for " + value
			err := errclass.BuildAPIError(map[string]any{
				"code": 1061004, "msg": text,
				"error": map[string]any{"details": []any{map[string]any{"value": text}}},
			}, errclass.ClassifyContext{})
			diagnostic := requirePermissionError(t, err).Upstream
			const want = "Request denied for [REDACTED]"
			if diagnostic == nil || diagnostic.Message != want || len(diagnostic.Details) != 1 || diagnostic.Details[0] != want {
				t.Fatalf("apostrophe split a sensitive value: %+v", diagnostic)
			}
		})
	}
}

func TestBuildAPIError_PermissionDiagnosticCredentialWhitespace(t *testing.T) {
	for _, scheme := range []string{"Bearer", "Basic"} {
		for i, separator := range []string{"\r", "\v", "\f", "\r\n", "\t", "\n", "\u0085", "\u2028", "\u2029"} {
			t.Run(fmt.Sprintf("%s/%d", scheme, i), func(t *testing.T) {
				text := "Request denied for " + scheme + separator + "private-value-example"
				err := errclass.BuildAPIError(map[string]any{
					"code": 1061004, "msg": text,
					"error": map[string]any{"details": []any{map[string]any{"value": text}}},
				}, errclass.ClassifyContext{})
				diagnostic := requirePermissionError(t, err).Upstream
				const want = "Request denied for [REDACTED]"
				if diagnostic == nil || diagnostic.Message != want || len(diagnostic.Details) != 1 || diagnostic.Details[0] != want {
					t.Fatalf("normalization joined a credential to its scheme: %+v", diagnostic)
				}
			})
		}
	}
}

func TestBuildAPIError_PermissionDiagnosticAllowlistAndMalformedShapes(t *testing.T) {
	const secret = "ignored-sensitive-payload"
	response := map[string]any{
		"code": 1061004, "msg": "Static reason",
		"headers": map[string]any{"Authorization": secret},
		"data":    map[string]any{"body": secret},
		"error": map[string]any{
			"message": "Rendered reason",
			"unknown": secret,
			"details": []any{
				map[string]any{"key": secret, "value": "First reason"},
				map[string]any{"value": map[string]any{"body": secret}},
				"ignored string: " + secret,
				map[string]any{"value": "Second reason", "other": secret},
			},
		},
	}
	err := errclass.BuildAPIError(response, errclass.ClassifyContext{})
	got := requirePermissionError(t, err).Upstream
	want := &errs.UpstreamDiagnostic{Message: "Rendered reason", Details: []string{"First reason", "Second reason"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic = %+v, want %+v", got, want)
	}
	for _, block := range []any{nil, "bad shape", []any{secret}, map[string]any{"message": 123, "details": secret}} {
		err := errclass.BuildAPIError(map[string]any{"code": 1061004, "msg": "Fallback reason", "error": block}, errclass.ClassifyContext{})
		if got := requirePermissionError(t, err).Upstream; got == nil || got.Message != "Fallback reason" || len(got.Details) != 0 {
			t.Fatalf("malformed block %T: diagnostic = %+v", block, got)
		}
	}
	for _, response := range []map[string]any{
		{"code": 1061004},
		{"code": 1061004, "msg": 1, "error": map[string]any{"message": false, "details": []any{nil, 1, map[string]any{"value": 2}}}},
		{"code": 1061004, "msg": " \t\n", "error": map[string]any{"details": []any{map[string]any{"value": ""}}}},
	} {
		if got := requirePermissionError(t, errclass.BuildAPIError(response, errclass.ClassifyContext{})).Upstream; got != nil {
			t.Fatalf("absent diagnostic must be omitted, got %+v", got)
		}
	}
}

func TestBuildAPIError_PermissionDiagnosticLimits(t *testing.T) {
	// An exact output cap is retained without an ellipsis or truncation flag.
	err := errclass.BuildAPIError(map[string]any{
		"code": 1061004, "msg": strings.Repeat("x ", 511) + "ok",
		"error": map[string]any{"details": []any{map[string]any{"value": strings.Repeat("x ", 127) + "ok"}}},
	}, errclass.ClassifyContext{})
	if got := requirePermissionError(t, err).Upstream; got == nil || got.Truncated || len(got.Message) != 1024 || len(got.Details[0]) != 256 {
		t.Fatalf("exact output limits must fit: %+v", got)
	}
	for _, size := range []int{8192, 8193} {
		message := (strings.Repeat("x ", 4095) + "okay")[:size]
		err := errclass.BuildAPIError(map[string]any{"code": 1061004, "msg": message}, errclass.ClassifyContext{})
		got := requirePermissionError(t, err).Upstream
		if got == nil || !got.Truncated || (got.Message == "") != (size > 8192) {
			t.Fatalf("input boundary %d: %+v", size, got)
		}
	}
	for _, unit := range []string{"word ", "中文🙂 "} {
		t.Run(unit, func(t *testing.T) {
			message := strings.Repeat(unit, 300)
			detail := strings.Repeat(unit, 80)
			err := errclass.BuildAPIError(map[string]any{
				"code": 1061004, "msg": message,
				"error": map[string]any{"details": []any{
					map[string]any{"value": detail}, map[string]any{"value": detail},
					map[string]any{"value": detail}, map[string]any{"value": detail},
					map[string]any{"value": "Fifth reason must not appear"},
				}},
			}, errclass.ClassifyContext{})
			got := requirePermissionError(t, err).Upstream
			if got == nil || !got.Truncated || len(got.Details) != 4 {
				t.Fatalf("expected a bounded diagnostic: %+v", got)
			}
			if len(got.Message) > 1024 || !utf8.ValidString(got.Message) || !strings.HasSuffix(got.Message, "…") {
				t.Fatalf("message has invalid truncation (%d bytes): %q", len(got.Message), got.Message)
			}
			for _, value := range got.Details {
				if len(value) > 256 || !utf8.ValidString(value) || !strings.HasSuffix(value, "…") {
					t.Fatalf("detail has invalid truncation (%d bytes): %q", len(value), value)
				}
			}
		})
	}

	// Redaction must precede truncation even when a credential straddles the
	// output boundary. An input too long to inspect is omitted in full.
	const secret = "partial-credential-must-not-leak"
	for _, prefix := range []string{strings.Repeat("safe ", 200), strings.Repeat("safe ", 1800)} {
		err := errclass.BuildAPIError(map[string]any{
			"code": 1061004, "msg": prefix + "Bearer " + secret,
			"error": map[string]any{"details": []any{map[string]any{"value": strings.Repeat("safe ", 1800) + secret}}},
		}, errclass.ClassifyContext{})
		got := requirePermissionError(t, err).Upstream
		if got == nil || !got.Truncated || len(got.Details) != 0 || strings.Contains(got.Message, "partial-credential") {
			t.Fatalf("unsafe or unmarked truncation: %+v", got)
		}
		if len(prefix) > 8192 && got.Message != "" {
			t.Fatalf("oversized input must be omitted in full: %+v", got)
		}
	}

	// The fifth detail is not inspected, even when the first four are empty.
	err = errclass.BuildAPIError(map[string]any{
		"code":  1061004,
		"error": map[string]any{"details": []any{nil, nil, nil, nil, map[string]any{"value": secret}}},
	}, errclass.ClassifyContext{})
	if got := requirePermissionError(t, err).Upstream; got == nil || !got.Truncated || got.Message != "" || len(got.Details) != 0 {
		t.Fatalf("uninspected details must only mark truncation: %+v", got)
	}
}

func TestBuildAPIError_PermissionDiagnosticNormalizesUntrustedText(t *testing.T) {
	err := errclass.BuildAPIError(map[string]any{
		"code": 1061004, "msg": "拒绝\u202e access\x00\n\t \xffreason\u200b",
	}, errclass.ClassifyContext{})
	got := requirePermissionError(t, err).Upstream.Message
	if got != "拒绝 access reason" || !utf8.ValidString(got) {
		t.Fatalf("normalized diagnostic = %q", got)
	}
	for _, r := range got {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			t.Fatalf("diagnostic retains control or format character: %U", r)
		}
	}
	for _, prose := range []string{"User's account isn't allowed to access this resource.", "O'Connor's account isn't allowed.", "用户's account isn't allowed."} {
		err = errclass.BuildAPIError(map[string]any{"code": 1061004, "msg": prose}, errclass.ClassifyContext{})
		if got := requirePermissionError(t, err).Upstream.Message; got != prose {
			t.Fatalf("apostrophes in prose must not be treated as quoted values: %q", got)
		}
	}
}

func TestPermissionDiagnosticDoesNotChangeLocalOrNonPermissionErrors(t *testing.T) {
	local := errclass.NewMissingScopeError("lark", "cli_demo", "user", []string{"docx:document"})
	if got := requirePermissionError(t, local).Upstream; got != nil {
		t.Fatalf("local preflight must not invent an upstream diagnostic: %+v", got)
	}
	err := errclass.BuildAPIError(map[string]any{
		"code": 1470400, "msg": "Invalid request parameter",
		"error": map[string]any{"details": []any{map[string]any{"value": "Field is required"}}},
	}, errclass.ClassifyContext{})
	typed, _ := errs.UnwrapTypedError(err)
	problem, _ := errs.ProblemOf(err)
	b, marshalErr := json.Marshal(typed)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if problem.Message != "Invalid request parameter" || problem.Hint != "Field is required" || strings.Contains(string(b), "upstream") {
		t.Fatalf("non-permission wire contract changed: %s", b)
	}
}
