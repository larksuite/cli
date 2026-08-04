// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apimeta_test

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/extension/apimeta"
	"github.com/larksuite/cli/internal/registry"
)

// A1: 非法数据经门脸返回错误，且不是 ErrAlreadyLoaded
func TestSetEmbedded_InvalidDataRejected(t *testing.T) {
	err := apimeta.SetEmbedded([]byte(`{"broken`))
	if err == nil {
		t.Fatalf("SetEmbedded(invalid) = nil, want error")
	}
	if errors.Is(err, apimeta.ErrAlreadyLoaded) {
		t.Fatalf("SetEmbedded(invalid) = ErrAlreadyLoaded, want parse error")
	}
}

// A2: sentinel 与 internal/registry 同值，errors.Is 语义成立
func TestErrAlreadyLoaded_AliasesRegistrySentinel(t *testing.T) {
	if !errors.Is(apimeta.ErrAlreadyLoaded, registry.ErrMetaAlreadyLoaded) {
		t.Fatalf("apimeta.ErrAlreadyLoaded is not registry.ErrMetaAlreadyLoaded")
	}
}
