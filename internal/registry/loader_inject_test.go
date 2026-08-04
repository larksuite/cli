// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
)

const injectValidMetaJSON = `{"version":"9.9.9","services":[{"name":"testsvc","title":"Test Service","resources":{}}]}`

// resetInjectState resets package state for injection tests and restores the
// original embedded bytes afterwards (same save/restore pattern as
// catalog_test.go). resetInit() itself clears embeddedParsed.
func resetInjectState(t *testing.T) {
	t.Helper()
	orig := embeddedMetaJSON
	resetInit()
	embeddedServices = nil
	embeddedServicesByName = nil
	t.Cleanup(func() {
		resetInit()
		embeddedServices = nil
		embeddedServicesByName = nil
		embeddedMetaJSON = orig
	})
}

// R1: parse 前注入合法 meta → 生效，version 基线更新
func TestSetEmbeddedMeta_InjectBeforeParse(t *testing.T) {
	resetInjectState(t)
	if err := SetEmbeddedMeta([]byte(injectValidMetaJSON)); err != nil {
		t.Fatalf("SetEmbeddedMeta() = %v, want nil", err)
	}
	svcs := EmbeddedServicesTyped()
	if len(svcs) != 1 || svcs[0].Name != "testsvc" {
		t.Fatalf("EmbeddedServicesTyped() = %+v, want single service testsvc", svcs)
	}
	if embeddedVersion != "9.9.9" { // R7: overlay 门禁的比较基线来源正确
		t.Fatalf("embeddedVersion = %q, want %q", embeddedVersion, "9.9.9")
	}
}

// R1b: parse 前多次注入 → 后写覆盖（注入者赢，spec §3.2 末行）
func TestSetEmbeddedMeta_LastWriteWinsBeforeParse(t *testing.T) {
	resetInjectState(t)
	first := `{"version":"1.0.0","services":[{"name":"firstsvc","resources":{}}]}`
	if err := SetEmbeddedMeta([]byte(first)); err != nil {
		t.Fatalf("SetEmbeddedMeta(first) = %v, want nil", err)
	}
	if err := SetEmbeddedMeta([]byte(injectValidMetaJSON)); err != nil {
		t.Fatalf("SetEmbeddedMeta(second) = %v, want nil", err)
	}
	svcs := EmbeddedServicesTyped()
	if len(svcs) != 1 || svcs[0].Name != "testsvc" {
		t.Fatalf("EmbeddedServicesTyped() = %+v, want last-injected testsvc", svcs)
	}
}

// R2: 注入后 SchemaCatalog 走 embedded 快路径
func TestSetEmbeddedMeta_SchemaCatalogUsesEmbedded(t *testing.T) {
	resetInjectState(t)
	if err := SetEmbeddedMeta([]byte(injectValidMetaJSON)); err != nil {
		t.Fatalf("SetEmbeddedMeta() = %v, want nil", err)
	}
	cat := SchemaCatalog()
	if cat.Source() != apicatalog.SourceEmbedded {
		t.Fatalf("SchemaCatalog().Source() = %q, want %q", cat.Source(), apicatalog.SourceEmbedded)
	}
	if _, ok := cat.Service("testsvc"); !ok {
		t.Fatalf("SchemaCatalog() missing injected service testsvc")
	}
}

// R3: 非法 JSON 拒绝且状态不变
func TestSetEmbeddedMeta_InvalidJSONRejected(t *testing.T) {
	resetInjectState(t)
	before := embeddedMetaJSON
	err := SetEmbeddedMeta([]byte(`{"broken`))
	if err == nil || !strings.Contains(err.Error(), "invalid api metadata") {
		t.Fatalf("SetEmbeddedMeta(invalid) = %v, want invalid api metadata error", err)
	}
	if string(embeddedMetaJSON) != string(before) {
		t.Fatalf("embeddedMetaJSON mutated on rejected input")
	}
}

// R4: 合法 JSON 但 services 为空 → 拒绝且状态不变
func TestSetEmbeddedMeta_EmptyServicesRejected(t *testing.T) {
	resetInjectState(t)
	before := embeddedMetaJSON
	for _, in := range []string{`{}`, `{"version":"1.0.0","services":[]}`} {
		err := SetEmbeddedMeta([]byte(in))
		if err == nil || !strings.Contains(err.Error(), "api metadata contains no services") {
			t.Fatalf("SetEmbeddedMeta(%q) = %v, want no-services error", in, err)
		}
	}
	if string(embeddedMetaJSON) != string(before) {
		t.Fatalf("embeddedMetaJSON mutated on rejected input")
	}
}

// R5: 首次 parse 之后注入 → ErrMetaAlreadyLoaded
func TestSetEmbeddedMeta_AfterParseRejected(t *testing.T) {
	resetInjectState(t)
	_ = EmbeddedServicesTyped() // 触发首次 parse
	err := SetEmbeddedMeta([]byte(injectValidMetaJSON))
	if !errors.Is(err, ErrMetaAlreadyLoaded) {
		t.Fatalf("SetEmbeddedMeta(after parse) = %v, want ErrMetaAlreadyLoaded", err)
	}
}

// R6: nil / 空字节 → 恒走 services 为空路径（meta.Parse 空输入返回零值无错误）
func TestSetEmbeddedMeta_NilAndEmptyRejected(t *testing.T) {
	resetInjectState(t)
	for _, in := range [][]byte{nil, {}} {
		err := SetEmbeddedMeta(in)
		if err == nil || !strings.Contains(err.Error(), "api metadata contains no services") {
			t.Fatalf("SetEmbeddedMeta(len=%d) = %v, want no-services error", len(in), err)
		}
	}
}
