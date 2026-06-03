// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"encoding/json"
	"strings"
	"testing"
)

// Per-type marshal tests pin each typed error's wire shape against its
// canonical fields. They guard against future refactors that change struct
// layout from accidentally altering the externally visible JSON contract.
//
// Each test asserts (a) Problem fields surface at the top level via embed
// promotion, (b) extension fields sit alongside as siblings (NOT under a
// `detail` sub-object), and (c) omitempty is honored on optional fields.

func TestPermissionError_MarshalJSON_HasAllWireFields(t *testing.T) {
	pe := &PermissionError{
		Problem: Problem{
			Category: CategoryAuthorization, Subtype: SubtypeMissingScope, Code: 99991679,
			Message: "x", Hint: "y", LogID: "lg", Retryable: false,
		},
		MissingScopes: []string{"docx:document"},
		Identity:      "user",
		ConsoleURL:    "https://example",
	}
	b, err := json.Marshal(pe)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"type":"authorization"`,
		`"subtype":"missing_scope"`,
		`"code":99991679`,
		`"message":"x"`,
		`"hint":"y"`,
		`"log_id":"lg"`,
		`"missing_scopes":["docx:document"]`,
		`"identity":"user"`,
		`"console_url":"https://example"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
	if strings.Contains(s, `"retryable"`) {
		t.Errorf("retryable should be omitted when false; got %s", s)
	}
	if strings.Contains(s, `"detail"`) {
		t.Errorf("extension fields must not be wrapped under detail; got %s", s)
	}
}

func TestPermissionError_RequestedGrantedMarshal(t *testing.T) {
	err := NewPermissionError(SubtypeMissingScope, "partial grant").
		WithRequestedScopes("docx:document", "im:message:send").
		WithGrantedScopes("docx:document").
		WithMissingScopes("im:message:send")

	b, e := json.Marshal(err)
	if e != nil {
		t.Fatal(e)
	}
	got := string(b)
	for _, want := range []string{
		`"requested_scopes":["docx:document","im:message:send"]`,
		`"granted_scopes":["docx:document"]`,
		`"missing_scopes":["im:message:send"]`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("envelope missing %s\nactual: %s", want, got)
		}
	}
}

func TestValidationError_MarshalJSON(t *testing.T) {
	ve := &ValidationError{
		Problem: Problem{Category: CategoryValidation, Subtype: SubtypeInvalidArgument, Message: "bad"},
		Param:   "--scope",
	}
	b, _ := json.Marshal(ve)
	s := string(b)
	for _, want := range []string{
		`"type":"validation"`,
		`"subtype":"invalid_argument"`,
		`"message":"bad"`,
		`"param":"--scope"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}

	// Param omitempty when ""
	ve2 := &ValidationError{Problem: Problem{Category: CategoryValidation, Message: "x"}}
	b2, _ := json.Marshal(ve2)
	if strings.Contains(string(b2), `"param"`) {
		t.Errorf("param should be omitted when empty; got %s", b2)
	}
}

func TestAuthError_MarshalJSON(t *testing.T) {
	ae := &AuthenticationError{
		Problem:    Problem{Category: CategoryAuthentication, Subtype: SubtypeTokenExpired, Message: "expired"},
		UserOpenID: "ou_x",
	}
	b, _ := json.Marshal(ae)
	s := string(b)
	for _, want := range []string{
		`"type":"authentication"`,
		`"subtype":"token_expired"`,
		`"message":"expired"`,
		`"user_open_id":"ou_x"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestConfigError_MarshalJSON(t *testing.T) {
	ce := &ConfigError{
		Problem: Problem{Category: CategoryConfig, Subtype: SubtypeInvalidClient, Message: "bad"},
		Field:   "app_id",
	}
	b, _ := json.Marshal(ce)
	s := string(b)
	for _, want := range []string{`"type":"config"`, `"subtype":"invalid_client"`, `"field":"app_id"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestNetworkError_MarshalJSON(t *testing.T) {
	ne := &NetworkError{
		Problem: Problem{Category: CategoryNetwork, Subtype: SubtypeNetworkTimeout, Message: "dial timeout"},
	}
	b, _ := json.Marshal(ne)
	s := string(b)
	for _, want := range []string{
		`"type":"network"`,
		`"subtype":"timeout"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
	if strings.Contains(s, `"cause"`) {
		t.Errorf("cause field should no longer be on the wire; got %s", s)
	}
}

func TestAPIError_MarshalJSON(t *testing.T) {
	ae := &APIError{
		Problem: Problem{Category: CategoryAPI, Subtype: SubtypeRateLimit, Code: 99991400, Message: "slow", Retryable: true},
	}
	b, _ := json.Marshal(ae)
	s := string(b)
	for _, want := range []string{
		`"type":"api"`,
		`"subtype":"rate_limit"`,
		`"code":99991400`,
		`"retryable":true`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

// TestProblem_MarshalJSON_Troubleshooter pins the upstream Lark API
// troubleshooter URL (resp.error.troubleshooter) surfacing on the wire under
// "troubleshooter". Carried via Problem so any typed error that embeds it
// inherits the field — populated by errclass.BuildAPIError before the
// category switch.
func TestProblem_MarshalJSON_Troubleshooter(t *testing.T) {
	ae := &APIError{
		Problem: Problem{
			Category:       CategoryAPI,
			Subtype:        SubtypeUnknown,
			Code:           99991400,
			Message:        "x",
			Troubleshooter: "https://open.feishu.cn/document/troubleshoot/abc",
		},
	}
	b, _ := json.Marshal(ae)
	s := string(b)
	if !strings.Contains(s, `"troubleshooter":"https://open.feishu.cn/document/troubleshoot/abc"`) {
		t.Errorf("missing troubleshooter in %s", s)
	}

	// Absent Troubleshooter must omit the wire key.
	bare := &APIError{Problem: Problem{Category: CategoryAPI, Message: "x"}}
	b2, _ := json.Marshal(bare)
	if strings.Contains(string(b2), `"troubleshooter"`) {
		t.Errorf("absent Troubleshooter must omit wire key; got %s", string(b2))
	}
}

func TestSecurityPolicyError_MarshalJSON(t *testing.T) {
	spe := &SecurityPolicyError{
		Problem:      Problem{Category: CategoryPolicy, Subtype: SubtypeChallengeRequired, Message: "blocked"},
		ChallengeURL: "https://chal.example",
	}
	b, _ := json.Marshal(spe)
	s := string(b)
	for _, want := range []string{
		`"type":"policy"`,
		`"subtype":"challenge_required"`,
		`"challenge_url":"https://chal.example"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

// Pin per-Subtype symmetry: SubtypeAccessDenied must serialize the same
// envelope shape as SubtypeChallengeRequired so callers can switch on
// subtype without conditional field probing. The constructor + builder
// path (mirroring how callsites actually construct these) is exercised
// here rather than the struct literal, since SubtypeAccessDenied is the
// path threaded through cmd/* sites that surface policy-deny outcomes.
func TestSecurityPolicyError_MarshalJSON_AccessDenied(t *testing.T) {
	err := NewSecurityPolicyError(SubtypeAccessDenied, "user denied").
		WithChallengeURL("https://chal.example/2")

	b, e := json.Marshal(err)
	if e != nil {
		t.Fatal(e)
	}
	got := string(b)
	for _, want := range []string{
		`"type":"policy"`,
		`"subtype":"access_denied"`,
		`"challenge_url":"https://chal.example/2"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("envelope missing %s\nactual: %s", want, got)
		}
	}
}

func TestContentSafetyError_MarshalJSON(t *testing.T) {
	cse := &ContentSafetyError{
		Problem: Problem{Category: CategoryPolicy, Subtype: Subtype("content_blocked"), Message: "blocked"},
		Rules:   []string{"pii", "violence"},
	}
	b, _ := json.Marshal(cse)
	s := string(b)
	for _, want := range []string{
		`"type":"policy"`,
		`"rules":["pii","violence"]`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestInternalError_MarshalJSON(t *testing.T) {
	ie := &InternalError{
		Problem: Problem{Category: CategoryInternal, Subtype: SubtypeSDKError, Message: "boom"},
	}
	b, _ := json.Marshal(ie)
	s := string(b)
	for _, want := range []string{`"type":"internal"`, `"subtype":"sdk_error"`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}

func TestConfirmationRequiredError_MarshalJSON(t *testing.T) {
	cre := &ConfirmationRequiredError{
		Problem: Problem{Category: CategoryConfirmation, Subtype: Subtype("confirmation_required"), Message: "confirm"},
		Risk:    "write",
		Action:  "mail +send",
	}
	b, _ := json.Marshal(cre)
	s := string(b)
	for _, want := range []string{
		`"type":"confirmation"`,
		`"risk":"write"`,
		`"action":"mail +send"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
}
