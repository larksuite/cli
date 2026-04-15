// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

import (
	"strings"
	"testing"
)

func TestInterpolateTemplate_UserSignatureUnchanged(t *testing.T) {
	sig := &Signature{
		Content:       "<b>My signature</b>",
		SignatureType: SignatureTypeUser,
	}
	got := InterpolateTemplate(sig, "zh_cn", "Alice", "alice@example.com")
	if got != sig.Content {
		t.Errorf("USER signature should be unchanged, got %q", got)
	}
}

func TestInterpolateTemplate_TenantTextVariables(t *testing.T) {
	sig := &Signature{
		Content:          `姓名：<span data-variable-meta-props='{"id":"B-NAME","type":"text"}'>{text}</span>, 部门：<span data-variable-meta-props='{"id":"B-DEPARTMENT","type":"text"}'>{text}</span>`,
		SignatureType:    SignatureTypeTenant,
		TemplateJSONKeys: []string{"B-NAME", "B-DEPARTMENT"},
		UserFields: map[string]UserFieldValue{
			"B-NAME":       {DefaultVal: "张三", I18nVals: map[string]string{"zh_cn": "", "en_us": "Zhang San"}},
			"B-DEPARTMENT": {DefaultVal: "默认部门", I18nVals: map[string]string{"zh_cn": "研发部", "en_us": "R&D"}},
		},
	}

	// zh_cn: B-DEPARTMENT should resolve to "研发部" (from i18n), B-NAME overridden by senderName
	got := InterpolateTemplate(sig, "zh_cn", "李四", "lisi@example.com")
	if !strings.Contains(got, "李四") {
		t.Errorf("expected senderName override for B-NAME, got %q", got)
	}
	if !strings.Contains(got, "研发部") {
		t.Errorf("expected zh_cn i18n value for B-DEPARTMENT, got %q", got)
	}
	if strings.Contains(got, "{text}") {
		t.Errorf("should not contain raw placeholder {text}, got %q", got)
	}
	if strings.Contains(got, "data-variable-meta-props") {
		t.Errorf("should not contain data-variable-meta-props attribute, got %q", got)
	}
}

func TestInterpolateTemplate_I18nFallback(t *testing.T) {
	sig := &Signature{
		Content:          `<span data-variable-meta-props='{"id":"B-DEPARTMENT","type":"text"}'>{text}</span>`,
		SignatureType:    SignatureTypeTenant,
		TemplateJSONKeys: []string{"B-DEPARTMENT"},
		UserFields: map[string]UserFieldValue{
			"B-DEPARTMENT": {DefaultVal: "默认部门", I18nVals: map[string]string{"zh_cn": "", "en_us": ""}},
		},
	}

	got := InterpolateTemplate(sig, "zh_cn", "", "")
	if !strings.Contains(got, "默认部门") {
		t.Errorf("expected fallback to DefaultVal, got %q", got)
	}
}

func TestInterpolateTemplate_HTMLEntityEscaping(t *testing.T) {
	// Simulate the HTML-entity-escaped attribute format from real API responses.
	sig := &Signature{
		Content:          `<span data-variable-meta-props="{&quot;id&quot;:&quot;B-NAME&quot;,&quot;type&quot;:&quot;text&quot;}">{text}</span>`,
		SignatureType:    SignatureTypeTenant,
		TemplateJSONKeys: []string{"B-NAME"},
		UserFields: map[string]UserFieldValue{
			"B-NAME": {DefaultVal: "default"},
		},
	}

	got := InterpolateTemplate(sig, "zh_cn", "陈煌", "")
	if !strings.Contains(got, "陈煌") {
		t.Errorf("expected interpolated name, got %q", got)
	}
}

func TestInterpolateTemplate_URLAsText(t *testing.T) {
	sig := &Signature{
		Content:          `<span data-variable-meta-props='{"id":"B-URL","type":"text"}'>{text}</span>`,
		SignatureType:    SignatureTypeTenant,
		TemplateJSONKeys: []string{"B-URL"},
		UserFields: map[string]UserFieldValue{
			"B-URL": {DefaultVal: "https://example.com"},
		},
	}

	got := InterpolateTemplate(sig, "zh_cn", "", "")
	if !strings.Contains(got, "<a href=") {
		t.Errorf("expected URL to be wrapped in <a> tag, got %q", got)
	}
	if !strings.Contains(got, "https://example.com") {
		t.Errorf("expected URL in output, got %q", got)
	}
}

func TestInterpolateTemplate_ImageVariable(t *testing.T) {
	sig := &Signature{
		Content:          `<span data-variable-meta-props='{"id":"B-LOGO","type":"image","width":"40"}'><img src="cid:old"/></span>`,
		SignatureType:    SignatureTypeTenant,
		TemplateJSONKeys: []string{"B-LOGO"},
		UserFields: map[string]UserFieldValue{
			"B-LOGO": {DefaultVal: "cid:new-logo-cid"},
		},
	}

	got := InterpolateTemplate(sig, "zh_cn", "", "")
	if !strings.Contains(got, `src="cid:new-logo-cid"`) {
		t.Errorf("expected new image src, got %q", got)
	}
	if !strings.Contains(got, `width="40"`) {
		t.Errorf("expected width attribute, got %q", got)
	}
}

func TestUserFieldValue_Resolve(t *testing.T) {
	v := UserFieldValue{
		DefaultVal: "default",
		I18nVals:   map[string]string{"zh_cn": "中文", "en_us": "", "ja_jp": "日本語"},
	}
	if got := v.Resolve("zh_cn"); got != "中文" {
		t.Errorf("zh_cn = %q, want 中文", got)
	}
	if got := v.Resolve("en_us"); got != "default" {
		t.Errorf("en_us (empty) should fallback to default, got %q", got)
	}
	if got := v.Resolve("ja_jp"); got != "日本語" {
		t.Errorf("ja_jp = %q, want 日本語", got)
	}
	if got := v.Resolve("fr_fr"); got != "default" {
		t.Errorf("unknown lang should fallback, got %q", got)
	}
}
