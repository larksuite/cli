// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package citation

import (
	"strings"
	"testing"
)

func TestEncodeXMLFullEntry(t *testing.T) {
	got := EncodeXML([]Citation{{
		SourceType:  SourceMessage,
		URL:         "https://applink.feishu.cn/client/chat/open?openChatId=oc_1&position=7",
		Title:       "hello",
		Snippet:     "excerpt",
		PublishTime: "2026-07-26T20:26:00+08:00",
	}})
	if len(got) != 1 {
		t.Fatalf("EncodeXML() = %#v, want 1 entry", got)
	}
	want := `<document reference_id="https://applink.feishu.cn/client/chat/open?openChatId=oc_1&amp;position=7">` +
		`<title>hello</title><source_type>3</source_type><snippet>excerpt</snippet>` +
		`<url>https://applink.feishu.cn/client/chat/open?openChatId=oc_1&amp;position=7</url>` +
		`<publish_time>2026-07-26T20:26:00+08:00</publish_time></document>`
	if got[0] != want {
		t.Errorf("EncodeXML() =\n%s\nwant\n%s", got[0], want)
	}
}

func TestEncodeXMLEscapesText(t *testing.T) {
	got := EncodeXML([]Citation{{
		SourceType: SourceWiki,
		URL:        `https://x.example/?a="1"&b=<2>`,
		Title:      `a <b> & "c"`,
	}})
	if len(got) != 1 {
		t.Fatalf("EncodeXML() = %#v", got)
	}
	s := got[0]
	for _, frag := range []string{
		`<title>a &lt;b&gt; &amp; &#34;c&#34;</title>`,
		`reference_id="https://x.example/?a=&#34;1&#34;&amp;b=&lt;2&gt;"`,
	} {
		if !strings.Contains(s, frag) {
			t.Errorf("EncodeXML() = %q, missing %q", s, frag)
		}
	}
}

func TestEncodeXMLOmitsEmptyOptionalsAndKeepsRequired(t *testing.T) {
	got := EncodeXML([]Citation{{SourceType: SourceWiki, URL: "https://x.example/1", Title: ""}})
	if len(got) != 1 {
		t.Fatalf("EncodeXML() = %#v", got)
	}
	s := got[0]
	if strings.Contains(s, "<snippet>") || strings.Contains(s, "<publish_time>") {
		t.Errorf("empty optionals must be omitted: %q", s)
	}
	if !strings.Contains(s, "<title></title>") {
		t.Errorf("empty title must still be present: %q", s)
	}
}

func TestEncodeXMLEmptyInputReturnsNil(t *testing.T) {
	if got := EncodeXML(nil); got != nil {
		t.Errorf("EncodeXML(nil) = %#v, want nil", got)
	}
	if got := EncodeXML([]Citation{}); got != nil {
		t.Errorf("EncodeXML(empty) = %#v, want nil", got)
	}
}
