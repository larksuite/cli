// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package argstype

import (
	"testing"
)

func TestMediaInput_AcceptURL(t *testing.T) {
	for _, v := range []string{"https://example.com/x.png", "http://a.b/y"} {
		if err := MediaInput(v).ValidateValue(nil, "image"); err != nil {
			t.Errorf("URL %q should pass: %v", v, err)
		}
	}
}

func TestMediaInput_AcceptKey(t *testing.T) {
	for _, v := range []string{"img_abc123", "file_xyz"} {
		if err := MediaInput(v).ValidateValue(nil, "image"); err != nil {
			t.Errorf("key %q should pass: %v", v, err)
		}
	}
}

func TestMediaInput_RejectAbsolutePath(t *testing.T) {
	if err := MediaInput("/etc/passwd").ValidateValue(nil, "image"); err == nil {
		t.Fatal("absolute path must be rejected")
	}
}

func TestMediaInput_AcceptRelativePath(t *testing.T) {
	if err := MediaInput("./pic.png").ValidateValue(nil, "image"); err != nil {
		t.Errorf("relative path should pass: %v", err)
	}
}
