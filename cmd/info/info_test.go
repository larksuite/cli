// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package info

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

func TestInfoRun_Success(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	opts := &InfoOptions{Factory: f}
	err := infoRun(opts)
	if err != nil {
		t.Fatalf("infoRun returned error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if ok, _ := result["ok"].(bool); !ok {
		t.Error("expected ok=true")
	}
	if _, ok := result["version"]; !ok {
		t.Error("expected version field")
	}
	if _, ok := result["os"]; !ok {
		t.Error("expected os field")
	}
	if _, ok := result["arch"]; !ok {
		t.Error("expected arch field")
	}
	if _, ok := result["go"]; !ok {
		t.Error("expected go field")
	}
}
