// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

// highRiskDeleteMethod mirrors a simple DELETE API with a required path
// parameter and risk metadata. The returned map is what service registration
// reads; the test exercises --yes registration and the gate behavior.
func highRiskDeleteMethod() map[string]interface{} {
	return map[string]interface{}{
		"path":       "files/{file_token}",
		"httpMethod": "DELETE",
		"risk":       "high-risk-write",
		"parameters": map[string]interface{}{
			"file_token": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
		},
	}
}

func writeMethodNoRisk() map[string]interface{} {
	return map[string]interface{}{
		"path":       "files/{file_token}",
		"httpMethod": "DELETE",
		"parameters": map[string]interface{}{
			"file_token": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
		},
	}
}

func TestServiceMethod_YesFlagRegisteredForHighRisk(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, driveSpec(), highRiskDeleteMethod(), "delete", "files", nil)

	if cmd.Flags().Lookup("yes") == nil {
		t.Error("expected --yes flag registered for risk=high-risk-write")
	}
}

func TestServiceMethod_YesFlagNotRegisteredForWrite(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, driveSpec(), writeMethodNoRisk(), "delete", "files", nil)

	if cmd.Flags().Lookup("yes") != nil {
		t.Error("expected --yes flag NOT registered when risk is unset")
	}
}

func TestServiceMethod_RiskAnnotationSet(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, driveSpec(), highRiskDeleteMethod(), "delete", "files", nil)

	level, ok := cmdutil.GetRisk(cmd)
	if !ok {
		t.Fatal("expected Risk annotation to be set")
	}
	if level != "high-risk-write" {
		t.Errorf("level = %q, want high-risk-write", level)
	}
}

func TestServiceMethod_RiskAnnotationAbsentForUnsetRisk(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, driveSpec(), writeMethodNoRisk(), "delete", "files", nil)

	if _, ok := cmdutil.GetRisk(cmd); ok {
		t.Error("expected no Risk annotation when meta risk is unset")
	}
}

func TestServiceMethod_GateBlocksWithoutYes(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, driveSpec(), highRiskDeleteMethod(), "delete", "files", nil)
	// --as bot skips the scope check so we reach the gate without external creds.
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"file_token":"tok_abc"}`})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected confirmation error, got nil")
	}
	if !strings.Contains(err.Error(), "requires confirmation") {
		t.Errorf("expected 'requires confirmation' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "drive.files.delete") {
		t.Errorf("expected schema path in error action, got: %v", err)
	}
}

func TestServiceMethod_DryRunBypassesGate(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, driveSpec(), highRiskDeleteMethod(), "delete", "files", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"file_token":"tok_abc"}`,
		"--dry-run",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run should not hit confirmation gate; got: %v", err)
	}
	if !strings.Contains(stdout.String(), "files/tok_abc") {
		t.Errorf("expected dry-run output to contain URL, got:\n%s", stdout.String())
	}
}
