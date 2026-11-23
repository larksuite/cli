// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import (
	"strings"
	"testing"
)

func TestScanCommentAuditsPublishedCommentBodies(t *testing.T) {
	got := ScanComment("issue_comment", `The published comment included /tmp/harness`+`-agent/run and CCM`+`-Harness: stage-4`)
	rules := findingRules(got)
	if !rules["public_content_harness_metadata"] || !rules["public_content_ccm_harness_trailer"] {
		t.Fatalf("comment audit findings = %#v", got)
	}
	for _, item := range got {
		if item.File != "issue_comment" {
			t.Fatalf("comment finding file = %q, want issue_comment", item.File)
		}
	}
}

func TestScanCommentAllowsMermaidCredentialTerminology(t *testing.T) {
	body := strings.Join([]string{
		"```mermaid",
		"sequenceDiagram",
		"  participant Client",
		"  participant AccessTokenHashTransport",
		"  participant SecurityPolicyTransport",
		"  Client->>AccessTokenHashTransport: Send request with bearer token",
		"  AccessTokenHashTransport->>AccessTokenHashTransport: Clone request and inject token hash",
		"  Client -> ClientSecret: Resolve configured credential",
		"  AccessTokenHashTransport->>SecurityPolicyTransport: Forward enriched request",
		"```",
	}, "\n")

	got := ScanComment("issue_comment", body)
	for _, item := range got {
		if item.Rule == "public_content_generic_credential" {
			t.Fatalf("mermaid credential terminology should not be a credential finding: %#v", got)
		}
	}
}

func TestScanCommentDetectsCredentialAssignmentInsideMermaidMessage(t *testing.T) {
	providerValue := strings.Join([]string{"gh", "p_", "1234567890abcdef", "1234567890abcdef", "1234"}, "")
	credentialAssignment := "password=" + providerValue
	body := strings.Join([]string{
		"```mermaid",
		"sequenceDiagram",
		"  Client->>Server: Send " + credentialAssignment,
		"```",
	}, "\n")

	got := ScanComment("issue_comment", body)
	if !findingRules(got)["public_content_generic_credential"] {
		t.Fatalf("credential assignment inside mermaid message should be reported: %#v", got)
	}
}

func TestScanCommentAtPathAllowsTestFixtureCredentialPlaceholder(t *testing.T) {
	body := `cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret"}`
	got := ScanCommentAtPath("pull_request_review_comment", "cmd/agent/list_test.go", body)
	for _, item := range got {
		if item.Rule == "public_content_generic_credential" {
			t.Fatalf("review comment test fixture should not be a credential finding: %#v", got)
		}
	}
}

func TestScanCommentAtPathDetectsProviderCredentialInTestFile(t *testing.T) {
	providerValue := strings.Join([]string{"gh", "p_", "1234567890abcdef", "1234567890abcdef", "1234"}, "")
	body := `cfg := &Config{AccessToken: "` + providerValue + `"}`
	got := ScanCommentAtPath("pull_request_review_comment", "cmd/agent/list_test.go", body)
	if !findingRules(got)["public_content_generic_credential"] {
		t.Fatalf("provider credential in review comment should be reported: %#v", got)
	}
}
