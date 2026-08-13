// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestDiagnosticMetadataPreservesTypedErrorContract(t *testing.T) {
	permission := NewPermissionError(SubtypeMissingScope, "missing scope").
		WithMissingScopes("im:message")
	wrapped := WithDiagnosticMetadata(permission, DiagnosticMetadata{
		Origin:         "proxy",
		ProxyRequestID: "proxy_req_1",
	})

	var gotPermission *PermissionError
	if !errors.As(wrapped, &gotPermission) || gotPermission != permission {
		t.Fatalf("errors.As() = %p, want original permission error %p", gotPermission, permission)
	}
	problem, ok := ProblemOf(wrapped)
	if !ok || problem != &permission.Problem {
		t.Fatalf("ProblemOf() = (%p, %v), want original Problem %p", problem, ok, &permission.Problem)
	}
	metadata, ok := DiagnosticMetadataOf(wrapped)
	if !ok || metadata.Origin != "proxy" || metadata.ProxyRequestID != "proxy_req_1" {
		t.Fatalf("DiagnosticMetadataOf() = (%#v, %v)", metadata, ok)
	}

	raw, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	if object["origin"] != "proxy" || object["proxy_request_id"] != "proxy_req_1" {
		t.Fatalf("metadata missing from JSON: %s", raw)
	}
	missingScopes, ok := object["missing_scopes"].([]any)
	if !ok || len(missingScopes) != 1 || missingScopes[0] != "im:message" {
		t.Fatalf("typed extension fields missing from JSON: %s", raw)
	}
}

func TestDiagnosticMetadataMergesWithoutMutatingExistingWrapper(t *testing.T) {
	typed := NewNetworkError(SubtypeUpstreamUnavailable, "unavailable")
	withOrigin := WithDiagnosticMetadata(typed, DiagnosticMetadata{Origin: "proxy"})
	withRequestID := WithDiagnosticMetadata(withOrigin, DiagnosticMetadata{ProxyRequestID: "proxy_req_2"})

	original, _ := DiagnosticMetadataOf(withOrigin)
	if original.ProxyRequestID != "" {
		t.Fatalf("existing wrapper was mutated: %#v", original)
	}
	merged, ok := DiagnosticMetadataOf(withRequestID)
	if !ok || merged.Origin != "proxy" || merged.ProxyRequestID != "proxy_req_2" {
		t.Fatalf("merged metadata = (%#v, %v)", merged, ok)
	}
}

func TestDiagnosticMetadataPreservesOuterErrorContext(t *testing.T) {
	cause := errors.New("transport failed")
	typed := NewNetworkError(SubtypeNetworkTransport, "request failed").WithCause(cause)
	outer := fmt.Errorf("fetch document: %w", typed)

	wrapped := WithDiagnosticMetadata(outer, DiagnosticMetadata{Origin: "proxy"})
	if got, want := wrapped.Error(), outer.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(wrapped, cause) {
		t.Fatal("metadata wrapper lost the original cause chain")
	}
	var gotTyped *NetworkError
	if !errors.As(wrapped, &gotTyped) || gotTyped != typed {
		t.Fatalf("errors.As() = %p, want original typed error %p", gotTyped, typed)
	}
}

func TestDiagnosticMetadataDoesNotCrossTypedProducerBoundary(t *testing.T) {
	inner := NewNetworkError(SubtypeUpstreamUnavailable, "proxy unavailable")
	annotatedInner := WithDiagnosticMetadata(inner, DiagnosticMetadata{
		Origin:         "proxy",
		ProxyRequestID: "proxy_req_inner",
	})
	outer := NewInternalError(SubtypeUnknown, "business reclassified failure").
		WithCause(annotatedInner)

	if metadata, ok := DiagnosticMetadataOf(outer); ok {
		t.Fatalf("outer typed producer inherited inner metadata: %#v", metadata)
	}

	wrapped := WithDiagnosticMetadata(outer, DiagnosticMetadata{Origin: "cli"})
	problem, ok := ProblemOf(wrapped)
	if !ok || problem != &outer.Problem {
		t.Fatalf("ProblemOf() = (%p, %v), want outer Problem %p", problem, ok, &outer.Problem)
	}
	if problem.Category != CategoryInternal ||
		problem.Subtype != SubtypeUnknown ||
		problem.Message != "business reclassified failure" {
		t.Fatalf("outer typed identity changed: %#v", problem)
	}
	metadata, ok := DiagnosticMetadataOf(wrapped)
	if !ok || metadata.Origin != "cli" || metadata.ProxyRequestID != "" {
		t.Fatalf("outer metadata = (%#v, %v), want cli without inner request id", metadata, ok)
	}

	innerMetadata, ok := DiagnosticMetadataOf(annotatedInner)
	if !ok ||
		innerMetadata.Origin != "proxy" ||
		innerMetadata.ProxyRequestID != "proxy_req_inner" {
		t.Fatalf("inner metadata was mutated: (%#v, %v)", innerMetadata, ok)
	}
}
