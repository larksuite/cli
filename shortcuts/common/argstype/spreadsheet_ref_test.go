// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"context"
	"testing"
)

func TestSpreadsheetRef_NormalizeFromURL(t *testing.T) {
	url := "https://feishu.cn/sheets/shtcnAbCdEf01234567"
	v, hints, err := SpreadsheetRef(url).Normalize(context.Background(), url)
	if err != nil {
		t.Fatalf("Normalize error: %v", err)
	}
	if string(v) != "shtcnAbCdEf01234567" {
		t.Errorf("expected extracted token, got %q", v)
	}
	if len(hints) == 0 {
		t.Errorf("expected at least one hint about URL extraction")
	}
}

func TestSpreadsheetRef_NormalizeStripsQueryAndFragment(t *testing.T) {
	for _, url := range []string{
		"https://feishu.cn/sheets/shtcnAbCdEf01234567?sheet=0",
		"https://feishu.cn/sheets/shtcnAbCdEf01234567#row=5",
		"https://feishu.cn/sheets/shtcnAbCdEf01234567?sheet=0#row=5",
	} {
		v, _, err := SpreadsheetRef(url).Normalize(context.Background(), url)
		if err != nil {
			t.Fatalf("Normalize(%q) error: %v", url, err)
		}
		if string(v) != "shtcnAbCdEf01234567" {
			t.Errorf("Normalize(%q): expected clean token, got %q", url, v)
		}
	}
}

func TestSpreadsheetRef_NormalizePassThroughToken(t *testing.T) {
	v, hints, err := SpreadsheetRef("shtcnXyz").Normalize(context.Background(), "shtcnXyz")
	if err != nil {
		t.Fatalf("Normalize error: %v", err)
	}
	if string(v) != "shtcnXyz" {
		t.Errorf("raw token should pass through, got %q", v)
	}
	if len(hints) != 0 {
		t.Errorf("no hint expected for raw token, got %v", hints)
	}
}

func TestSpreadsheetRef_ValidateValueRejectsEmpty(t *testing.T) {
	if err := SpreadsheetRef("").ValidateValue(nil, "spreadsheet"); err == nil {
		t.Fatal("empty value should fail validation")
	}
}

func TestSpreadsheetRef_ValidateValueAcceptsToken(t *testing.T) {
	if err := SpreadsheetRef("shtcnAbCd").ValidateValue(nil, "spreadsheet"); err != nil {
		t.Errorf("token should pass: %v", err)
	}
}
