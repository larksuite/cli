// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestSetRisk_EmptyLevelShortCircuits(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	SetRisk(cmd, "")
	if cmd.Annotations != nil {
		t.Errorf("expected annotations untouched for empty level, got %v", cmd.Annotations)
	}
}

func TestSetRisk_PopulatesLevel(t *testing.T) {
	cases := []string{"read", "write", "high-risk-write"}
	for _, level := range cases {
		t.Run(level, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			SetRisk(cmd, level)
			got, ok := GetRisk(cmd)
			if !ok {
				t.Fatal("expected ok=true after SetRisk")
			}
			if got != level {
				t.Errorf("level = %q, want %q", got, level)
			}
		})
	}
}

func TestSetRisk_PreservesExistingAnnotations(t *testing.T) {
	cmd := &cobra.Command{
		Use:         "test",
		Annotations: map[string]string{"other": "val"},
	}
	SetRisk(cmd, "high-risk-write")
	if cmd.Annotations["other"] != "val" {
		t.Error("existing annotation should be preserved")
	}
	if level, ok := GetRisk(cmd); !ok || level != "high-risk-write" {
		t.Errorf("risk not written: level=%q ok=%v", level, ok)
	}
}

func TestSetRisk_InitializesNilAnnotations(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	if cmd.Annotations != nil {
		t.Fatal("precondition: Annotations should be nil on a fresh command")
	}
	SetRisk(cmd, "write")
	if cmd.Annotations == nil {
		t.Fatal("SetRisk should lazily initialize Annotations")
	}
}

func TestGetRisk_NilAnnotations(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	level, ok := GetRisk(cmd)
	if ok {
		t.Error("expected ok=false for nil Annotations")
	}
	if level != "" {
		t.Errorf("expected empty level, got %q", level)
	}
}

func TestGetRisk_NoRiskKey(t *testing.T) {
	cmd := &cobra.Command{
		Use:         "test",
		Annotations: map[string]string{"unrelated": "x"},
	}
	if _, ok := GetRisk(cmd); ok {
		t.Error("expected ok=false when risk key is absent")
	}
}

func TestGetRisk_EmptyValueReturnsNotOK(t *testing.T) {
	cmd := &cobra.Command{
		Use:         "test",
		Annotations: map[string]string{riskLevelAnnotationKey: ""},
	}
	level, ok := GetRisk(cmd)
	if ok {
		t.Error("expected ok=false for empty level value")
	}
	if level != "" {
		t.Errorf("expected empty level, got %q", level)
	}
}
