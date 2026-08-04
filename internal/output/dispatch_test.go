// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestDispatchErrorNil(t *testing.T) {
	env, code, has := DispatchError(nil, "user")
	if env != nil || code != 0 || has {
		t.Fatalf("DispatchError(nil) = (%v, %d, %v), want (nil, 0, false)", env, code, has)
	}
}

func TestDispatchErrorTyped(t *testing.T) {
	err := errs.NewValidationError(errs.SubtypeInvalidArgument, "missing --id")
	env, code, has := DispatchError(err, "user")
	if !has || code != ExitCodeOf(err) {
		t.Fatalf("has=%v code=%d, want true / %d", has, code, ExitCodeOf(err))
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal(env, &parsed); jsonErr != nil {
		t.Fatalf("envelope not valid JSON: %v", jsonErr)
	}
	if parsed["ok"] != false || parsed["identity"] != "user" {
		t.Errorf("envelope ok/identity = %v/%v", parsed["ok"], parsed["identity"])
	}
}

func TestDispatchErrorPartialFailure(t *testing.T) {
	env, code, has := DispatchError(PartialFailure(1), "user")
	if env != nil || code != 1 || has {
		t.Fatalf("got (%v, %d, %v), want (nil, 1, false)", env, code, has)
	}
}

func TestDispatchErrorBare(t *testing.T) {
	env, code, has := DispatchError(ErrBare(3), "user")
	if env != nil || code != 3 || has {
		t.Fatalf("got (%v, %d, %v), want (nil, 3, false)", env, code, has)
	}
}

func TestDispatchErrorCobraUsage(t *testing.T) {
	env, code, has := DispatchError(fmt.Errorf(`required flag(s) "values" not set`), "user")
	if !has || code != 2 {
		t.Fatalf("has=%v code=%d, want true / 2", has, code)
	}
	var parsed map[string]any
	_ = json.Unmarshal(env, &parsed)
	errObj := parsed["error"].(map[string]any)
	if errObj["subtype"] != "invalid_argument" {
		t.Errorf("subtype = %v, want invalid_argument", errObj["subtype"])
	}
}

func TestDispatchErrorLeakedUntyped(t *testing.T) {
	env, code, has := DispatchError(errors.New("boom"), "bot")
	if !has || code != 5 {
		t.Fatalf("has=%v code=%d, want true / 5", has, code)
	}
	var parsed map[string]any
	_ = json.Unmarshal(env, &parsed)
	if parsed["identity"] != "bot" {
		t.Errorf("identity = %v, want bot", parsed["identity"])
	}
}

func TestDispatchErrorEmptyIdentityOmitted(t *testing.T) {
	env, _, _ := DispatchError(errs.NewValidationError(errs.SubtypeInvalidArgument, "x"), "")
	var parsed map[string]any
	_ = json.Unmarshal(env, &parsed)
	if _, present := parsed["identity"]; present {
		t.Error("identity field present, want omitted for empty identity")
	}
}
