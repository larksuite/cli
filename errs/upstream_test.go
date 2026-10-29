// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs_test

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestPermissionErrorWithUpstreamOwnsDiagnostic(t *testing.T) {
	diagnostic := &errs.UpstreamDiagnostic{
		Message: "API reason", Details: []string{"API detail"}, Truncated: true,
	}
	err := errs.NewPermissionError(errs.SubtypePermissionDenied, "canonical message").WithUpstream(diagnostic)
	diagnostic.Message = "changed"
	diagnostic.Details[0] = "changed"
	diagnostic.Truncated = false
	if err.Upstream.Message != "API reason" || err.Upstream.Details[0] != "API detail" || !err.Upstream.Truncated {
		t.Fatalf("builder aliases input: %+v", err.Upstream)
	}
	if got := err.WithUpstream(nil); got != err || got.Upstream != nil {
		t.Fatalf("nil must clear the optional diagnostic: %+v", got)
	}
}

func TestPermissionErrorUpstreamJSONCompatibility(t *testing.T) {
	err := errs.NewPermissionError(errs.SubtypePermissionDenied, "canonical message").WithCode(1061004).WithIdentity("user")
	const legacy = `{"type":"authorization","subtype":"permission_denied","code":1061004,"message":"canonical message","identity":"user"}`
	b, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if string(b) != legacy {
		t.Fatalf("absent diagnostic changed legacy JSON: %s", b)
	}
	err.WithUpstream(&errs.UpstreamDiagnostic{Message: "API reason", Details: []string{"API detail"}, Truncated: true})
	b, marshalErr = json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	const want = `{"type":"authorization","subtype":"permission_denied","code":1061004,"message":"canonical message","identity":"user","upstream":{"message":"API reason","details":["API detail"],"truncated":true}}`
	if string(b) != want {
		t.Fatalf("diagnostic is not an additive typed-error extension: %s", b)
	}
	// Existing consumers that ignore unknown fields still decode the same
	// machine fields and canonical message from the extended error.
	var oldConsumer struct {
		Type     string `json:"type"`
		Subtype  string `json:"subtype"`
		Code     int    `json:"code"`
		Message  string `json:"message"`
		Identity string `json:"identity"`
	}
	if decodeErr := json.Unmarshal(b, &oldConsumer); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if oldConsumer.Type != "authorization" || oldConsumer.Subtype != "permission_denied" || oldConsumer.Code != 1061004 ||
		oldConsumer.Message != "canonical message" || oldConsumer.Identity != "user" {
		t.Fatalf("legacy consumer contract changed: %+v", oldConsumer)
	}
	err.WithUpstream(&errs.UpstreamDiagnostic{Message: "API reason"})
	b, marshalErr = json.Marshal(err.Upstream)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if string(b) != `{"message":"API reason"}` {
		t.Fatalf("empty details and false truncated must be omitted: %s", b)
	}
}
