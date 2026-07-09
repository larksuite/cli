// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

// isLarkCode must match the API code on typed errs.* errors — the consume
// runtime classifies OAPI failures via errclass.BuildAPIError, so the
// not-found retry in fillVCNoteGeneratedDetails depends on this reading
// Problem.Code rather than the legacy envelope shape.
func TestIsLarkCode_MatchesTypedAPIErrorCode(t *testing.T) {
	typedNotFound := errs.NewAPIError(errs.SubtypeNotFound, "note not ready").
		WithCode(vcNoteDetailNotFoundCode)
	if !isLarkCode(typedNotFound, vcNoteDetailNotFoundCode) {
		t.Fatal("typed API error carrying the not-found code must match (retry path)")
	}
	if isLarkCode(typedNotFound, 99999) {
		t.Error("a different expected code must not match")
	}

	otherTyped := errs.NewAPIError(errs.SubtypeServerError, "boom").WithCode(500)
	if isLarkCode(otherTyped, vcNoteDetailNotFoundCode) {
		t.Error("typed error with another code must not match")
	}

	if isLarkCode(errors.New("plain failure"), vcNoteDetailNotFoundCode) {
		t.Error("untyped error must not match")
	}
}
