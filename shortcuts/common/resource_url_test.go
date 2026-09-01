// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
)

func TestParseResourceURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		rawURL    string
		wantType  string
		wantToken string
		wantOK    bool
	}{
		// All 9 supported types
		{"docx", "https://xxx.feishu.cn/docx/doxcnABC", "docx", "doxcnABC", true},
		{"doc", "https://xxx.feishu.cn/doc/doccnABC", "doc", "doccnABC", true},
		{"sheet", "https://xxx.feishu.cn/sheets/shtcnABC", "sheet", "shtcnABC", true},
		{"bitable via /base/", "https://xxx.feishu.cn/base/bascnABC", "bitable", "bascnABC", true},
		{"bitable via /bitable/", "https://xxx.feishu.cn/bitable/bascnABC", "bitable", "bascnABC", true},
		{"wiki", "https://xxx.feishu.cn/wiki/wikcnABC", "wiki", "wikcnABC", true},
		{"file", "https://xxx.feishu.cn/file/boxcnABC", "file", "boxcnABC", true},
		{"folder", "https://xxx.feishu.cn/drive/folder/fldcnABC", "folder", "fldcnABC", true},
		{"file via /drive/file/", "https://feishu.doubao.com/drive/file/boxcnABC", "file", "boxcnABC", true},
		{"folder via /chat/drive/", "https://feishu.doubao.com/chat/drive/fldcnABC", "folder", "fldcnABC", true},
		{"folder via /drive/shr/", "https://feishu.doubao.com/drive/shr/fldcnABC", "folder", "fldcnABC", true},
		{"mindnote", "https://xxx.feishu.cn/mindnote/mncnABC", "mindnote", "mncnABC", true},
		{"slides", "https://xxx.feishu.cn/slides/slkcnABC", "slides", "slkcnABC", true},

		// Lark domain
		{"lark docx", "https://xxx.larksuite.com/docx/doxcnABC", "docx", "doxcnABC", true},
		{"lark wiki", "https://xxx.larksuite.com/wiki/wikcnABC", "wiki", "wikcnABC", true},

		// With query parameters
		{"with query", "https://xxx.feishu.cn/docx/doxcnABC?from=wiki", "docx", "doxcnABC", true},
		{"with fragment", "https://xxx.feishu.cn/docx/doxcnABC#section", "docx", "doxcnABC", true},

		// With trailing slash
		{"trailing slash", "https://xxx.feishu.cn/docx/doxcnABC/", "docx", "doxcnABC", true},

		// With extra path segments after token
		{"extra path", "https://xxx.feishu.cn/docx/doxcnABC/edit", "docx", "doxcnABC", true},

		// Non-Lark host with Lark-like path (host validation is the caller's responsibility)
		{"non-lark host with lark path", "https://google.com/docx/doxcnABC", "docx", "doxcnABC", true},

		// Negative cases
		{"unrecognized path", "https://xxx.feishu.cn/calendar/calABC", "", "", false},
		{"non-lark host unrecognized path", "https://example.com/page", "", "", false},
		{"empty input", "", "", "", false},
		{"bare token", "doxcnABC", "", "", false},
		{"invalid url parse", "://not-a-valid-url", "", "", false},
		{"matching prefix but empty token", "https://xxx.feishu.cn/docx/", "", "", false},
		{"matching prefix but whitespace-only token", "https://xxx.feishu.cn/docx/   ", "", "", false},
		{"whitespace-only input", "   ", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := ParseResourceURL(tt.rawURL)
			if ok != tt.wantOK {
				t.Errorf("ParseResourceURL(%q) ok = %v, want %v", tt.rawURL, ok, tt.wantOK)
			}
			if ok {
				if ref.Type != tt.wantType {
					t.Errorf("ParseResourceURL(%q) Type = %q, want %q", tt.rawURL, ref.Type, tt.wantType)
				}
				if ref.Token != tt.wantToken {
					t.Errorf("ParseResourceURL(%q) Token = %q, want %q", tt.rawURL, ref.Token, tt.wantToken)
				}
			}
		})
	}
}

// TestParseResourceURL_RoundTrip verifies that ParseResourceURL is the inverse
// of BuildResourceURL for all supported types.
func TestParseResourceURL_RoundTrip(t *testing.T) {
	t.Parallel()

	types := []string{"docx", "doc", "sheet", "bitable", "wiki", "file", "folder", "mindnote", "slides"}
	token := "testTOKEN123"

	for _, kind := range types {
		t.Run(kind, func(t *testing.T) {
			built := BuildResourceURL(core.BrandFeishu, kind, token)
			if built == "" {
				t.Fatalf("BuildResourceURL returned empty for kind %q", kind)
			}
			ref, ok := ParseResourceURL(built)
			if !ok {
				t.Fatalf("ParseResourceURL(%q) returned ok=false", built)
			}
			if ref.Type != kind {
				t.Errorf("round-trip type mismatch: got %q, want %q", ref.Type, kind)
			}
			if ref.Token != token {
				t.Errorf("round-trip token mismatch: got %q, want %q", ref.Token, token)
			}
		})
	}
}

func TestBuildResourceURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		brand core.LarkBrand
		kind  string
		token string
		want  string
	}{
		{"feishu docx", core.BrandFeishu, "docx", "doxcnABC", "https://www.feishu.cn/docx/doxcnABC"},
		{"feishu doc legacy", core.BrandFeishu, "doc", "doccnABC", "https://www.feishu.cn/doc/doccnABC"},
		{"feishu sheet", core.BrandFeishu, "sheet", "shtcnABC", "https://www.feishu.cn/sheets/shtcnABC"},
		{"feishu bitable", core.BrandFeishu, "bitable", "bascnABC", "https://www.feishu.cn/base/bascnABC"},
		{"feishu wiki", core.BrandFeishu, "wiki", "wikcnABC", "https://www.feishu.cn/wiki/wikcnABC"},
		{"feishu file", core.BrandFeishu, "file", "boxcnABC", "https://www.feishu.cn/file/boxcnABC"},
		{"feishu folder", core.BrandFeishu, "folder", "fldcnABC", "https://www.feishu.cn/drive/folder/fldcnABC"},
		{"feishu mindnote", core.BrandFeishu, "mindnote", "mncnABC", "https://www.feishu.cn/mindnote/mncnABC"},
		{"feishu slides", core.BrandFeishu, "slides", "slkcnABC", "https://www.feishu.cn/slides/slkcnABC"},
		{"lark docx", core.BrandLark, "docx", "doxcnABC", "https://www.larksuite.com/docx/doxcnABC"},
		{"lark wiki", core.BrandLark, "wiki", "wikcnABC", "https://www.larksuite.com/wiki/wikcnABC"},
		{"empty brand defaults to feishu", core.LarkBrand(""), "docx", "doxcnABC", "https://www.feishu.cn/docx/doxcnABC"},
		{"kind case-insensitive", core.BrandFeishu, "DOCX", "doxcnABC", "https://www.feishu.cn/docx/doxcnABC"},
		{"token whitespace trimmed", core.BrandFeishu, "docx", "  doxcnABC  ", "https://www.feishu.cn/docx/doxcnABC"},
		{"empty token", core.BrandFeishu, "docx", "", ""},
		{"whitespace-only token", core.BrandFeishu, "docx", "   ", ""},
		{"unknown kind", core.BrandFeishu, "calendar", "calABC", ""},
		{"empty kind", core.BrandFeishu, "", "doxcnABC", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildResourceURL(tt.brand, tt.kind, tt.token)
			if got != tt.want {
				t.Errorf("BuildResourceURL(%q, %q, %q) = %q, want %q", tt.brand, tt.kind, tt.token, got, tt.want)
			}
		})
	}
}
