// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"net/url"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDraftPreviewURLForBrand(t *testing.T) {
	tests := []struct {
		name     string
		brand    core.LarkBrand
		draftID  string
		wantBase string
	}{
		{
			name:     "lark brand",
			brand:    core.BrandLark,
			draftID:  "d_abc123",
			wantBase: "https://www.larkoffice.com",
		},
		{
			name:     "feishu brand",
			brand:    core.BrandFeishu,
			draftID:  "d_xyz789",
			wantBase: "https://www.feishu.cn",
		},
		{
			name:     "empty brand defaults to feishu",
			brand:    "",
			draftID:  "d_test",
			wantBase: "https://www.feishu.cn",
		},
		{
			name:     "unknown brand defaults to feishu",
			brand:    "unknown",
			draftID:  "d_test",
			wantBase: "https://www.feishu.cn",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := draftPreviewURLForBrand(tt.brand, tt.draftID)
			if result == "" {
				t.Fatalf("draftPreviewURLForBrand(%q, %q) returned empty string", tt.brand, tt.draftID)
			}

			u, err := url.Parse(result)
			if err != nil {
				t.Fatalf("draftPreviewURLForBrand(%q, %q) returned invalid URL: %v", tt.brand, tt.draftID, err)
			}

			if u.Scheme != "https" {
				t.Errorf("scheme = %q, want %q", u.Scheme, "https")
			}
			if u.Host != tt.wantBase[len("https://"):] {
				t.Errorf("host = %q, want %q", u.Host, tt.wantBase[len("https://"):])
			}
			if u.Path != "/mail" {
				t.Errorf("path = %q, want %q", u.Path, "/mail")
			}
			if u.Query().Get("draftId") != tt.draftID {
				t.Errorf("draftId = %q, want %q", u.Query().Get("draftId"), tt.draftID)
			}
			if u.Query().Get("scene") != "send-preview" {
				t.Errorf("scene = %q, want %q", u.Query().Get("scene"), "send-preview")
			}
		})
	}
}

func TestDraftPreviewURLForBrand_SpecialCharsInDraftID(t *testing.T) {
	result := draftPreviewURLForBrand(core.BrandFeishu, "d_测试_id")
	u, err := url.Parse(result)
	if err != nil {
		t.Fatalf("url.Parse failed: %v", err)
	}
	if u.Query().Get("draftId") != "d_测试_id" {
		t.Errorf("draftId = %q, want %q", u.Query().Get("draftId"), "d_测试_id")
	}
}

func TestDraftPreviewURL_NilRuntime(t *testing.T) {
	result := draftPreviewURL(nil, "d_test")
	if result == "" {
		t.Error("draftPreviewURL(nil, ...) should not return empty string")
	}
	if result != draftPreviewURLForBrand(core.BrandFeishu, "d_test") {
		t.Errorf("draftPreviewURL(nil, ...) = %q, want %q", result, draftPreviewURLForBrand(core.BrandFeishu, "d_test"))
	}
}

func TestDraftPreviewURL_EmptyDraftID(t *testing.T) {
	runtime := &common.RuntimeContext{Config: &core.CliConfig{Brand: core.BrandLark}}
	result := draftPreviewURL(runtime, "")
	if result != "" {
		t.Errorf("draftPreviewURL(runtime, %q) = %q, want empty string", "", result)
	}

	result = draftPreviewURL(runtime, "   ")
	if result != "" {
		t.Errorf("draftPreviewURL(runtime, %q) = %q, want empty string", "   ", result)
	}
}

func TestDraftPreviewURL_UsesRuntimeBrand(t *testing.T) {
	runtime := &common.RuntimeContext{Config: &core.CliConfig{Brand: core.BrandLark}}
	result := draftPreviewURL(runtime, "d_test")
	if result != draftPreviewURLForBrand(core.BrandLark, "d_test") {
		t.Errorf("draftPreviewURL with BrandLark = %q, want %q", result, draftPreviewURLForBrand(core.BrandLark, "d_test"))
	}

	runtime = &common.RuntimeContext{Config: &core.CliConfig{Brand: core.BrandFeishu}}
	result = draftPreviewURL(runtime, "d_test")
	if result != draftPreviewURLForBrand(core.BrandFeishu, "d_test") {
		t.Errorf("draftPreviewURL with BrandFeishu = %q, want %q", result, draftPreviewURLForBrand(core.BrandFeishu, "d_test"))
	}
}

func TestDraftPreviewOriginForBrand(t *testing.T) {
	if got := draftPreviewOriginForBrand(core.BrandLark); got != "https://www.larkoffice.com" {
		t.Errorf("BrandLark = %q, want %q", got, "https://www.larkoffice.com")
	}
	if got := draftPreviewOriginForBrand(core.BrandFeishu); got != "https://www.feishu.cn" {
		t.Errorf("BrandFeishu = %q, want %q", got, "https://www.feishu.cn")
	}
	if got := draftPreviewOriginForBrand("unknown"); got != "https://www.feishu.cn" {
		t.Errorf("unknown brand = %q, want %q", got, "https://www.feishu.cn")
	}
}

func TestAddDraftPreviewURL(t *testing.T) {
	t.Run("nil out", func(t *testing.T) {
		addDraftPreviewURL(nil, nil, "d_test") // should not panic
	})

	t.Run("adds preview_url when draftID is valid", func(t *testing.T) {
		out := map[string]interface{}{"draft_id": "d_test"}
		addDraftPreviewURL(nil, out, "d_test")
		if _, ok := out["preview_url"]; !ok {
			t.Error("preview_url not added to out map")
		}
	})

	t.Run("does not add preview_url when draftID is empty", func(t *testing.T) {
		out := map[string]interface{}{"draft_id": ""}
		addDraftPreviewURL(nil, out, "")
		if _, ok := out["preview_url"]; ok {
			t.Error("preview_url should not be added when draftID is empty")
		}
	})

	t.Run("preserves existing fields", func(t *testing.T) {
		out := map[string]interface{}{"draft_id": "d_test", "foo": "bar"}
		addDraftPreviewURL(nil, out, "d_test")
		if out["foo"] != "bar" {
			t.Errorf("foo = %v, want %v", out["foo"], "bar")
		}
	})
}
