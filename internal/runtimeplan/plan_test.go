// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package runtimeplan

import (
	"errors"
	"net/http"
	"testing"

	"github.com/larksuite/cli/errs"
)

type testFilePolicy struct {
	err     error
	managed bool
}

func (p testFilePolicy) ValidateRemoteFile(string) error { return p.err }
func (p testFilePolicy) UsesManagedFilePlane() bool      { return p.managed }

func TestDefaultPreservesOrdinaryRuntime(t *testing.T) {
	plan := Default()
	base := http.DefaultTransport
	got, err := plan.Wrap(base)
	if err != nil {
		t.Fatal(err)
	}
	if got != base {
		t.Fatalf("Wrap() = %T, want original transport", got)
	}
	if !plan.AllowsRemoteMetadata() {
		t.Fatal("default plan must allow established metadata loading")
	}
	if plan.UsesManagedFilePlane() {
		t.Fatal("default plan must keep direct file transfer")
	}
	if err := plan.Require(CapabilityRealtimeEvents); err != nil {
		t.Fatalf("default capability check = %v", err)
	}
}

func TestCapabilityPolicyIsApplied(t *testing.T) {
	deniedErr := errors.New("events disabled")
	plan := New(Options{
		Capabilities: func(capability Capability) error {
			if capability == CapabilityRealtimeEvents {
				return deniedErr
			}
			return nil
		},
	})

	if err := plan.Require(CapabilityRealtimeEvents); !errors.Is(err, deniedErr) {
		t.Fatalf("Require() = %v, want policy denial", err)
	}
}

func TestFailedPlanBlocksEveryRuntimeBoundary(t *testing.T) {
	startupErr := errors.New("invalid runtime policy")
	plan := Failed(startupErr, MetadataEmbeddedOnly)

	if _, err := plan.Wrap(http.DefaultTransport); !errors.Is(err, startupErr) {
		t.Fatalf("Wrap() = %v, want startup error", err)
	}
	if err := plan.ValidateRemoteFile("https://example.com/file"); !errors.Is(err, startupErr) {
		t.Fatalf("ValidateRemoteFile() = %v, want startup error", err)
	}
	if err := plan.Require(CapabilityRealtimeEvents); !errors.Is(err, startupErr) {
		t.Fatalf("Require() = %v, want startup error", err)
	}
	if plan.AllowsRemoteMetadata() {
		t.Fatal("failed plan unexpectedly allows remote metadata")
	}
}

func TestRemoteFilePolicyIsEncapsulated(t *testing.T) {
	policyErr := errors.New("invalid handle")
	plan := New(Options{
		RemoteFiles: testFilePolicy{err: policyErr, managed: true},
	})
	if err := plan.ValidateRemoteFile("opaque"); !errors.Is(err, policyErr) {
		t.Fatalf("ValidateRemoteFile() = %v, want policy error", err)
	}
	if !plan.UsesManagedFilePlane() {
		t.Fatal("managed file-plane capability was lost")
	}
}

func TestNewZeroValuePreservesRemoteMetadata(t *testing.T) {
	if !New(Options{}).AllowsRemoteMetadata() {
		t.Fatal("Options zero value must preserve ordinary metadata loading")
	}
	if New(Options{Metadata: MetadataEmbeddedOnly}).AllowsRemoteMetadata() {
		t.Fatal("embedded-only metadata policy was not applied")
	}
	if New(Options{Metadata: MetadataPolicy(99)}).AllowsRemoteMetadata() {
		t.Fatal("unknown metadata policy must fail closed")
	}
}

func TestUnknownCapabilityIsRejectedInDefaultPlan(t *testing.T) {
	err := Default().Require(Capability("misspelled_capability"))
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("unknown capability error is not typed: %v", err)
	}
	if problem.Category != errs.CategoryInternal {
		t.Fatalf("unknown capability category = %q, want %q", problem.Category, errs.CategoryInternal)
	}
}
