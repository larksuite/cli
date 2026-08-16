// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/core"
)

func TestWikiNodeCitation(t *testing.T) {
	got := wikiNodeCitation(core.BrandFeishu, "spc1", "wikcnTok", "标题", "1721996760")
	if len(got) != 1 {
		t.Fatalf("wikiNodeCitation() = %#v, want 1 entry", got)
	}
	c := got[0]
	if c.SourceType != citation.SourceWiki {
		t.Errorf("source_type = %d", c.SourceType)
	}
	if c.URL != "https://applink.feishu.cn/client/wiki/open?wikiToken=wikcnTok" {
		t.Errorf("url = %q", c.URL)
	}
	if c.Title != "标题" || c.ResourceID != "spc1/wikcnTok" {
		t.Errorf("title/resource_id = %q %q", c.Title, c.ResourceID)
	}
	if c.PublishTime != citation.Time("1721996760") {
		t.Errorf("publish_time = %q", c.PublishTime)
	}
}

func TestWikiNodeCitationLarkBrand(t *testing.T) {
	got := wikiNodeCitation(core.BrandLark, "spc1", "wikcnTok", "t", "")
	if got[0].URL != "https://applink.larksuite.com/client/wiki/open?wikiToken=wikcnTok" {
		t.Errorf("lark brand url = %q", got[0].URL)
	}
	if got[0].PublishTime != "" {
		t.Errorf("empty edit time must omit publish_time, got %q", got[0].PublishTime)
	}
}

func TestWikiNodeCitationEmptyToken(t *testing.T) {
	got := wikiNodeCitation(core.BrandFeishu, "spc1", "", "t", "")
	if len(got) != 1 || got[0].URL != "" {
		t.Fatalf("empty token must yield empty url (Normalize drops it): %#v", got)
	}
}
