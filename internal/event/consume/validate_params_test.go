// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

func requireParamValidationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument || ve.Param != "--param" {
		t.Errorf("subtype/param = %s/%q, want %s/%q", ve.Subtype, ve.Param, errs.SubtypeInvalidArgument, "--param")
	}
	if ve.Hint == "" {
		t.Error("param validation error should hint at `lark-cli event schema`")
	}
}

func TestValidateParams_RequiredMissing(t *testing.T) {
	def := &event.KeyDefinition{
		Key:    "x.test",
		Params: []event.ParamDef{{Name: "chat_id", Required: true}},
	}
	requireParamValidationError(t, validateParams(def, map[string]string{}))
}

func TestValidateParams_UnknownParam(t *testing.T) {
	def := &event.KeyDefinition{
		Key:    "x.test",
		Params: []event.ParamDef{{Name: "chat_id"}},
	}
	requireParamValidationError(t, validateParams(def, map[string]string{"nope": "1"}))
}

func TestValidateParams_UnknownParamNoParamsAccepted(t *testing.T) {
	def := &event.KeyDefinition{Key: "x.test"}
	requireParamValidationError(t, validateParams(def, map[string]string{"nope": "1"}))
}

func TestValidateParams_DefaultAppliedAndValidPasses(t *testing.T) {
	def := &event.KeyDefinition{
		Key:    "x.test",
		Params: []event.ParamDef{{Name: "mode", Required: true, Default: "all"}},
	}
	params := map[string]string{}
	if err := validateParams(def, params); err != nil {
		t.Fatalf("default should satisfy required param, got: %v", err)
	}
	if params["mode"] != "all" {
		t.Errorf("default not applied, params=%v", params)
	}
}
