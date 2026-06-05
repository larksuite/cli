// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lint

import (
	"strings"
	"testing"
)

// =====================================================================
// Tier 1 — pass-through tags / attrs / styles (tag classification row "通过").
// =====================================================================

// TestRun_AllowedTagsPassThrough verifies that the canonical Feishu-native
// tag set passes through without findings (tag classification row "通过").
func TestRun_AllowedTagsPassThrough(t *testing.T) {
	cases := []struct {
		name string
		html string
	}{
		{"plain paragraph", `<p>hello world</p>`},
		{"div with span", `<div><span>nested</span></div>`},
		{"unordered list", `<ul><li>a</li><li>b</li></ul>`},
		{"ordered list", `<ol><li>x</li></ol>`},
		{"table", `<table><thead><tr><th>h</th></tr></thead><tbody><tr><td>v</td></tr></tbody></table>`},
		{"headings", `<h1>t</h1><h2>t</h2><h3>t</h3><h4>t</h4><h5>t</h5><h6>t</h6>`},
		{"emphasis", `<b>b</b><i>i</i><em>e</em><strong>s</strong><u>u</u><s>k</s>`},
		{"sub sup", `<sub>s</sub><sup>p</sup>`},
		{"hr br", `<p>x<br>y</p><hr>`},
		{"blockquote", `<blockquote>q</blockquote>`},
		{"code pre", `<pre><code>x = 1</code></pre>`},
		{"safe href", `<a href="https://example.com">link</a>`},
		{"mailto href", `<a href="mailto:a@b.c">m</a>`},
		{"cid img", `<img src="cid:abc123">`},
		{"data:image png", `<img src="data:image/png;base64,iVBOR" alt="x">`},
		{"feishu native quote class",
			`<div class="adit-html-block adit-html-block--collapsed"><div>x</div></div>`},
	}

	// Feishu-native autofix rules apply to <p>/<ul>/<ol>/<li>/<blockquote>/<a>
	// — those are not "violations" so must not be flagged as errors. We
	// allow STYLE_*_NATIVE_INLINE_APPLIED + STYLE_PARA_WRAPPER_REWRITTEN
	// findings here but reject any other rule.
	feishuNativeRules := map[string]bool{
		RuleStyleListNative:       true,
		RuleStyleListItemNative:   true,
		RuleStyleBlockquoteNative: true,
		RuleStyleLinkNative:       true,
		RuleStyleParaWrapper:      true,
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := Run(tc.html, Options{})
			if len(rep.Blocked) != 0 {
				t.Errorf("expected no errors, got %d: %+v", len(rep.Blocked), rep.Blocked)
			}
			for _, f := range rep.Applied {
				if !feishuNativeRules[f.RuleID] {
					t.Errorf("unexpected non-Feishu-native warning: %+v", f)
				}
			}
		})
	}
}

// TestRun_AllowedStylePropertiesPassThrough verifies all allowed style
// properties survive a round-trip without dropping.
func TestRun_AllowedStylePropertiesPassThrough(t *testing.T) {
	allowed := []string{
		"color:rgb(31,35,41)",
		"background-color:rgb(245,246,247)",
		"font-size:14px",
		"font-weight:bold",
		"font-style:italic",
		"text-align:center",
		"text-decoration:underline",
		"line-height:1.6",
		"padding:8px",
		"margin:12px",
		"border:1px solid #ccc",
		"border-top:1px solid red",
		"border-bottom:2px solid blue",
		"border-left:1px",
		"border-right:1px",
		"width:100%",
		"height:auto",
		"display:block",
		"text-indent:2em",
	}
	for _, prop := range allowed {
		t.Run(prop, func(t *testing.T) {
			html := `<p style="` + prop + `">x</p>`
			rep := Run(html, Options{})
			for _, f := range rep.Applied {
				if f.RuleID == RuleStylePropertyDropped {
					t.Errorf("property %q unexpectedly dropped: %+v", prop, f)
				}
			}
		})
	}
}

// =====================================================================
// Tier 2 — warning + autofix tags (tag classification row "警告 + 自动修复").
// =====================================================================

// TestRun_FontTagAutofixedToSpan verifies <font color="..."> rewrites to
// <span style="color:..."> with AutoFix=true.
func TestRun_FontTagAutofixedToSpan(t *testing.T) {
	// Use <div> wrapper to avoid the Feishu-native paragraph autofix
	// firing alongside the <font> rewrite.
	rep := Run(`<div><font color="red">x</font></div>`, Options{})
	if len(rep.Applied) != 1 {
		t.Fatalf("expected 1 warning, got %d: %+v", len(rep.Applied), rep.Applied)
	}
	got := rep.Applied[0]
	if got.RuleID != RuleTagFontToSpan {
		t.Errorf("rule = %s, want %s", got.RuleID, RuleTagFontToSpan)
	}
	if got.Severity != SeverityWarning {
		t.Errorf("severity = %s, want warning", got.Severity)
	}
	if !strings.Contains(rep.CleanedHTML, "<span") || strings.Contains(rep.CleanedHTML, "<font") {
		t.Errorf("expected <font>→<span> rewrite, cleaned=%q", rep.CleanedHTML)
	}
	if !strings.Contains(rep.CleanedHTML, "color:red") {
		t.Errorf("expected color preserved as inline style, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_FontTagSizeMappedToPx checks legacy <font size="N"> → font-size:Npx.
func TestRun_FontTagSizeMappedToPx(t *testing.T) {
	rep := Run(`<font size="3">x</font>`, Options{})
	if !strings.Contains(rep.CleanedHTML, "font-size:16px") {
		t.Errorf("expected size=3 → 16px, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_CenterTagAutofixedToDiv verifies <center> → <div text-align:center>.
func TestRun_CenterTagAutofixedToDiv(t *testing.T) {
	rep := Run(`<center>x</center>`, Options{})
	if len(rep.Applied) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(rep.Applied))
	}
	if rep.Applied[0].RuleID != RuleTagCenterToDiv {
		t.Errorf("rule = %s, want %s", rep.Applied[0].RuleID, RuleTagCenterToDiv)
	}
	if !strings.Contains(rep.CleanedHTML, "<div") || !strings.Contains(rep.CleanedHTML, "text-align:center") {
		t.Errorf("expected <center>→<div text-align:center>, cleaned=%q", rep.CleanedHTML)
	}
	if strings.Contains(rep.CleanedHTML, "<center") {
		t.Errorf("<center> should have been replaced, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_MarqueeBlinkCollapseToSpan verifies <marquee>/<blink> → <span>.
func TestRun_MarqueeBlinkCollapseToSpan(t *testing.T) {
	for _, tag := range []string{"marquee", "blink"} {
		rep := Run("<"+tag+">x</"+tag+">", Options{})
		if len(rep.Applied) != 1 {
			t.Errorf("[%s] expected 1 warning, got %d", tag, len(rep.Applied))
			continue
		}
		if !strings.Contains(rep.CleanedHTML, "<span") {
			t.Errorf("[%s] expected <span> wrapper, cleaned=%q", tag, rep.CleanedHTML)
		}
	}
}

// =====================================================================
// Tier 3 — error / delete tags (tag classification row "错误（删除）").
// =====================================================================

// TestRun_ScriptTagBlocked checks that <script> is removed unconditionally.
func TestRun_ScriptTagBlocked(t *testing.T) {
	rep := Run(`<p>safe</p><script>alert(1)</script><p>after</p>`, Options{})
	if len(rep.Blocked) != 1 {
		t.Fatalf("expected 1 blocked finding, got %d", len(rep.Blocked))
	}
	if rep.Blocked[0].RuleID != RuleTagScriptBlocked {
		t.Errorf("rule = %s, want %s", rep.Blocked[0].RuleID, RuleTagScriptBlocked)
	}
	if strings.Contains(rep.CleanedHTML, "<script") || strings.Contains(rep.CleanedHTML, "alert(1)") {
		t.Errorf("<script> content should be deleted, cleaned=%q", rep.CleanedHTML)
	}
	if !strings.Contains(rep.CleanedHTML, "safe") || !strings.Contains(rep.CleanedHTML, "after") {
		t.Errorf("surrounding content lost, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_BlockedTagsRemoved iterates all error-tier tags.
func TestRun_BlockedTagsRemoved(t *testing.T) {
	cases := map[string]string{
		`<iframe src="x"></iframe>`:               RuleTagIframeBlocked,
		`<object data="x"></object>`:              RuleTagObjectBlocked,
		`<embed src="x">`:                         RuleTagEmbedBlocked,
		`<form action="x"><input></form>`:         RuleTagFormBlocked,
		`<link rel="stylesheet" href="x.css">`:    RuleTagLinkBlocked,
		`<meta http-equiv="refresh" content="0">`: RuleTagMetaBlocked,
		`<base href="https://evil.com">`:          RuleTagBaseBlocked,
	}
	for input, wantRule := range cases {
		t.Run(input[:min(len(input), 30)], func(t *testing.T) {
			rep := Run(input, Options{})
			found := false
			for _, f := range rep.Blocked {
				if f.RuleID == wantRule {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected rule %s, got %+v", wantRule, rep.Blocked)
			}
		})
	}
}

// TestRun_EventHandlerAttrBlocked verifies on*-handlers (onclick etc.) are
// stripped — they are an event-handler injection vector.
func TestRun_EventHandlerAttrBlocked(t *testing.T) {
	rep := Run(`<p onclick="alert(1)" id="ok">x</p>`, Options{})
	if len(rep.Blocked) != 1 {
		t.Fatalf("expected 1 blocked finding, got %d", len(rep.Blocked))
	}
	if rep.Blocked[0].RuleID != RuleAttrEventHandlerBlocked {
		t.Errorf("rule = %s, want %s", rep.Blocked[0].RuleID, RuleAttrEventHandlerBlocked)
	}
	if strings.Contains(rep.CleanedHTML, "onclick") {
		t.Errorf("onclick should be stripped, cleaned=%q", rep.CleanedHTML)
	}
	if !strings.Contains(rep.CleanedHTML, `id="ok"`) {
		t.Errorf("non-handler attrs should survive, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_OnErrorAttrBlocked tests one of the more common XSS vectors.
func TestRun_OnErrorAttrBlocked(t *testing.T) {
	rep := Run(`<img src="cid:x" onerror="alert(1)">`, Options{})
	hasErr := false
	for _, f := range rep.Blocked {
		if f.RuleID == RuleAttrEventHandlerBlocked && f.TagOrAttr == "onerror" {
			hasErr = true
		}
	}
	if !hasErr {
		t.Errorf("onerror should fire, got %+v", rep.Blocked)
	}
}

// =====================================================================
// URL scheme allow-list.
// =====================================================================

// TestRun_JavaScriptURLBlocked verifies javascript: hrefs are stripped.
func TestRun_JavaScriptURLBlocked(t *testing.T) {
	rep := Run(`<a href="javascript:alert(1)">click</a>`, Options{})
	hasErr := false
	for _, f := range rep.Blocked {
		if f.RuleID == RuleAttrJSURLBlocked {
			hasErr = true
		}
	}
	if !hasErr {
		t.Errorf("javascript: URL should fire ATTR_JS_URL_BLOCKED, got %+v", rep.Blocked)
	}
	if strings.Contains(rep.CleanedHTML, "javascript:") {
		t.Errorf("javascript: should be stripped, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_VBScriptURLBlocked verifies vbscript: is rejected.
func TestRun_VBScriptURLBlocked(t *testing.T) {
	rep := Run(`<a href="vbscript:msgbox 1">x</a>`, Options{})
	if len(rep.Blocked) == 0 {
		t.Errorf("expected vbscript: to be blocked, got 0 findings")
	}
}

// TestRun_DataNonImageURLBlocked verifies data:text/html is rejected
// (only data:image/* is allowed).
func TestRun_DataNonImageURLBlocked(t *testing.T) {
	rep := Run(`<img src="data:text/html,<script>1</script>">`, Options{})
	if len(rep.Blocked) == 0 {
		t.Errorf("expected data:text/html to be blocked")
	}
}

// TestRun_DataImageAllowed verifies data:image/png passes.
func TestRun_DataImageAllowed(t *testing.T) {
	rep := Run(`<img src="data:image/png;base64,iVBORw0KGg=">`, Options{})
	for _, f := range rep.Blocked {
		if f.RuleID == RuleAttrJSURLBlocked {
			t.Errorf("data:image/* should pass, got %+v", f)
		}
	}
}

// TestRun_RelativeURLAllowed verifies relative URLs (no scheme) pass.
func TestRun_RelativeURLAllowed(t *testing.T) {
	rep := Run(`<img src="./local.png"><a href="/path">x</a>`, Options{})
	for _, f := range rep.Blocked {
		if f.RuleID == RuleAttrJSURLBlocked || f.RuleID == RuleAttrUnsafeSchemeBlocked {
			t.Errorf("relative URL should pass, got %+v", f)
		}
	}
}

// =====================================================================
// Style property allow-list.
// =====================================================================

// TestRun_StylePropertyDropped verifies non-allow-list properties drop.
func TestRun_StylePropertyDropped(t *testing.T) {
	rep := Run(`<p style="color:red; position:absolute; z-index:99">x</p>`, Options{})
	dropped := []string{}
	for _, f := range rep.Applied {
		if f.RuleID == RuleStylePropertyDropped {
			dropped = append(dropped, f.TagOrAttr)
		}
	}
	if !sliceContains(dropped, "style.position") {
		t.Errorf("expected position to be dropped, got %v", dropped)
	}
	if !sliceContains(dropped, "style.z-index") {
		t.Errorf("expected z-index to be dropped, got %v", dropped)
	}
	if strings.Contains(rep.CleanedHTML, "position:") || strings.Contains(rep.CleanedHTML, "z-index:") {
		t.Errorf("dropped properties should be removed from cleaned style, cleaned=%q", rep.CleanedHTML)
	}
	if !strings.Contains(rep.CleanedHTML, "color:red") {
		t.Errorf("allowed property should survive, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_StyleBorderPrefixAllowed verifies the border-* prefix rule.
func TestRun_StyleBorderPrefixAllowed(t *testing.T) {
	rep := Run(`<p style="border-top:1px; border-bottom-color:red; border-radius:4px">x</p>`, Options{})
	for _, f := range rep.Applied {
		if f.RuleID == RuleStylePropertyDropped {
			t.Errorf("border-* should pass, got %+v", f)
		}
	}
}

// TestRun_FeishuListShorthandMarginPreserved guards the nested-list indent
// regression: when a user writes shorthand `margin:0 0 0 24px` on an inner
// <ul> (mail-editor's own native nested-list shape), the Feishu-list autofix
// must NOT clobber it by appending `margin-left:0`. ensureInlineStyleProps
// is supposed to skip props the user already declared, but earlier
// hasInlineStyleProp was only matching longhand `margin-left:` literally
// and missed the shorthand form, causing 24px indents to be reset to 0.
func TestRun_FeishuListShorthandMarginPreserved(t *testing.T) {
	in := `<ul style="margin:0px 0px 0px 24px;padding-left:0px;list-style-position:inside" data-list-bullet="true"><li class="temp-li bullet2" data-li-line="true" data-list="bullet2" style="line-height:1.6;list-style-type:circle;font-size:14px" dir="auto"><span style="font-family:inherit"><span style="color:rgb(0,0,0)">indented</span></span></li></ul>`
	rep := Run(in, Options{})
	cleaned := rep.CleanedHTML
	// Extract just the <ul ...> opening tag's style attr (li has its own
	// independent margin-left:0 longhand which is correct — list indent
	// belongs on the container, not the item).
	ulOpen := cleaned
	if i := strings.Index(ulOpen, ">"); i >= 0 {
		ulOpen = ulOpen[:i]
	}
	if !strings.Contains(ulOpen, "margin:0px 0px 0px 24px") {
		t.Errorf("shorthand margin with 24px left should survive on <ul>, ulOpen=%q", ulOpen)
	}
	// The bug signature: extra `margin-left:` appended after the shorthand
	// on the <ul> element itself (CSS rule says the later one wins, so any
	// margin-left:0 after the shorthand resets the indent to 0).
	if strings.Contains(ulOpen, "margin-left") {
		t.Errorf("autofix must not append margin-left longhand onto <ul> when shorthand already declares it, ulOpen=%q", ulOpen)
	}
}

// TestRun_BlockquoteShorthandBorderPreserved verifies the blockquote native
// autofix does not override a user-authored border shorthand by appending
// border-left. CSS applies the later longhand over the earlier shorthand, so
// adding border-left here would replace the user's left border.
func TestRun_BlockquoteShorthandBorderPreserved(t *testing.T) {
	rep := Run(`<blockquote style="border:1px solid red">quoted</blockquote>`, Options{})
	cleaned := rep.CleanedHTML
	if !strings.Contains(cleaned, `border:1px solid red`) {
		t.Fatalf("user-authored border shorthand should survive, cleaned=%q", cleaned)
	}
	if strings.Contains(cleaned, `border-left:`) {
		t.Fatalf("autofix must not append border-left when border shorthand already declares it, cleaned=%q", cleaned)
	}
	if !strings.Contains(cleaned, `color:rgb(100,106,115)`) {
		t.Fatalf("blockquote native autofix should still add missing non-border style props, cleaned=%q", cleaned)
	}
}

func TestRun_BlockquoteNativeContentWrapper(t *testing.T) {
	rep := Run(`<blockquote>quoted</blockquote>`, Options{})
	cleaned := rep.CleanedHTML
	for _, want := range []string{
		`class="lark-mail-doc-quote"`,
		`border-left:2px solid rgb(187,191,196)`,
		`<div dir="auto" style="font-size:14px;padding-left:12px">quoted</div>`,
	} {
		if !strings.Contains(cleaned, want) {
			t.Fatalf("cleaned blockquote missing %q, cleaned=%q", want, cleaned)
		}
	}
}

func TestRun_BlockquoteNativeContentWrapperIdempotent(t *testing.T) {
	in := `<blockquote class="lark-mail-doc-quote" style="padding-left:0px;color:rgb(100,106,115);border-left:2px solid rgb(187,191,196);margin:0px"><div dir="auto" style="font-size:14px;padding-left:12px">quoted</div></blockquote>`
	rep := Run(in, Options{})
	if strings.Count(rep.CleanedHTML, `padding-left:12px`) != 1 {
		t.Fatalf("native-shaped blockquote should not get nested content wrappers, cleaned=%q", rep.CleanedHTML)
	}
}

func TestRun_ParagraphRewritePreservesDirAndFontSize(t *testing.T) {
	rep := Run(`<p style="font-size:20px" dir="rtl">hello</p>`, Options{})
	cleaned := rep.CleanedHTML
	if !strings.Contains(cleaned, `style="font-size:20px;margin-top:4px;margin-bottom:4px;line-height:1.6" dir="rtl"`) {
		t.Fatalf("outer paragraph wrapper should preserve author font-size and dir, cleaned=%q", cleaned)
	}
	if !strings.Contains(cleaned, `<div dir="rtl">hello</div>`) {
		t.Fatalf("inner paragraph wrapper should inherit author dir and omit default font-size, cleaned=%q", cleaned)
	}
	if strings.Contains(cleaned, `font-size:14px`) {
		t.Fatalf("inner paragraph wrapper must not force default font-size over author value, cleaned=%q", cleaned)
	}
	if strings.Contains(cleaned, `dir="auto"`) {
		t.Fatalf("inner paragraph wrapper must not force dir=auto over author value, cleaned=%q", cleaned)
	}
}

// =====================================================================
// CleanedHTML output / contract guarantees.
// =====================================================================

// TestRun_EmptyArraysAlwaysPresent verifies the report has non-nil empty
// slices when nothing is found (the JSON envelope contract requires `[]`,
// not `null`).
func TestRun_EmptyArraysAlwaysPresent(t *testing.T) {
	// Use <div> instead of <p> to avoid the Feishu-native paragraph
	// rewrite autofix, which would surface a finding even on otherwise
	// clean input.
	rep := Run(`<div>nothing here</div>`, Options{})
	if rep.Applied == nil || rep.Blocked == nil {
		t.Errorf("Applied/Blocked must be non-nil; got applied=%v blocked=%v", rep.Applied, rep.Blocked)
	}
	if len(rep.Applied) != 0 || len(rep.Blocked) != 0 {
		t.Errorf("expected empty findings, got applied=%d blocked=%d", len(rep.Applied), len(rep.Blocked))
	}
}

// TestEmptyReport_HasContractFields covers the helper used by compose 5's
// plain-text branch.
func TestEmptyReport_HasContractFields(t *testing.T) {
	rep := EmptyReport(`plain text`)
	if rep.Applied == nil {
		t.Error("Applied must be non-nil")
	}
	if rep.Blocked == nil {
		t.Error("Blocked must be non-nil")
	}
	if rep.CleanedHTML != "plain text" {
		t.Errorf("CleanedHTML = %q, want %q", rep.CleanedHTML, "plain text")
	}
}

// TestRun_CleanedHTMLPreservesStructure verifies that the round-trip through
// the parser doesn't accidentally lose user content.
func TestRun_CleanedHTMLPreservesStructure(t *testing.T) {
	html := `<div style="line-height:1.6"><h3>title</h3><p>body <b>bold</b> end</p><ul><li>a</li><li>b</li></ul></div>`
	rep := Run(html, Options{})
	if len(rep.Blocked) != 0 {
		t.Fatalf("unexpected blocked: %+v", rep.Blocked)
	}
	// Feishu-native autofix expected to fire on <p>, <ul>, <li> — content
	// must still survive untouched even though structure is augmented.
	for _, want := range []string{"line-height:1.6", "<h3>", "title", "<b>", "bold", "<ul", "<li", "</ul>"} {
		if !strings.Contains(rep.CleanedHTML, want) {
			t.Errorf("expected %q in cleaned, got %q", want, rep.CleanedHTML)
		}
	}
}

// TestRun_EmptyInput verifies the lib short-circuits cleanly on empty input.
func TestRun_EmptyInput(t *testing.T) {
	rep := Run("", Options{})
	if rep.CleanedHTML != "" {
		t.Errorf("CleanedHTML = %q, want empty", rep.CleanedHTML)
	}
	if len(rep.Applied) != 0 || len(rep.Blocked) != 0 {
		t.Errorf("empty input must produce empty findings")
	}
}

// TestRun_HasErrorFindingsFlag verifies the flag tracks blocked findings.
func TestRun_HasErrorFindingsFlag(t *testing.T) {
	rep := Run(`<script>x</script>`, Options{})
	if !rep.HasErrorFindings {
		t.Error("expected HasErrorFindings=true")
	}
	clean := Run(`<p>safe</p>`, Options{})
	if clean.HasErrorFindings {
		t.Error("expected HasErrorFindings=false on clean HTML")
	}
}

// TestRun_HasWarningFindingsFlag verifies the flag tracks warnings.
func TestRun_HasWarningFindingsFlag(t *testing.T) {
	rep := Run(`<font color="red">x</font>`, Options{})
	if !rep.HasWarningFindings {
		t.Error("expected HasWarningFindings=true")
	}
}

// =====================================================================
// Excerpt cap.
// =====================================================================

// TestTruncateExcerpt_RespectsCap verifies the per-finding excerpt cap.
func TestTruncateExcerpt_RespectsCap(t *testing.T) {
	long := strings.Repeat("x", MaxExcerptBytes+50)
	got := truncateExcerpt(long)
	if len(got) > MaxExcerptBytes {
		t.Errorf("excerpt len %d exceeds cap %d", len(got), MaxExcerptBytes)
	}
	if !strings.HasSuffix(got, " ...") {
		t.Errorf("expected truncation suffix, got %q", got[len(got)-10:])
	}
}

// TestRun_ExcerptCappedForLargeOffender verifies large blocked content
// produces a short excerpt (envelope size protection).
func TestRun_ExcerptCappedForLargeOffender(t *testing.T) {
	bigAttr := strings.Repeat("a", MaxExcerptBytes*2)
	rep := Run(`<a href="javascript:`+bigAttr+`">x</a>`, Options{})
	if len(rep.Blocked) == 0 {
		t.Fatal("expected blocked finding")
	}
	for _, f := range rep.Blocked {
		if len(f.Excerpt) > MaxExcerptBytes {
			t.Errorf("excerpt len %d exceeds cap %d", len(f.Excerpt), MaxExcerptBytes)
		}
	}
}

// =====================================================================
// Helpers.
// =====================================================================

func sliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// =====================================================================
// Additional coverage for edge cases and exhaustive value mapping.
// =====================================================================

// TestMapFontSize_ExhaustiveSpan covers every <font size="N"> mapping
// + invalid values fall through to "" so the property is dropped.
func TestMapFontSize_ExhaustiveSpan(t *testing.T) {
	cases := map[string]string{
		"1":   "10px",
		"2":   "13px",
		"3":   "16px",
		"4":   "18px",
		"5":   "24px",
		"6":   "32px",
		"7":   "48px",
		"":    "",
		"8":   "",
		"abc": "",
		"3.5": "",
		" 3 ": "16px",
	}
	for raw, want := range cases {
		got := mapFontSize(raw)
		if got != want {
			t.Errorf("mapFontSize(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestRun_FontTagWithFaceMappedToFontFamily ensures <font face="..."> →
// font-family inline style.
func TestRun_FontTagWithFaceMappedToFontFamily(t *testing.T) {
	rep := Run(`<font face="Arial">x</font>`, Options{})
	if !strings.Contains(rep.CleanedHTML, "font-family:Arial") {
		t.Errorf("expected font-family preserved, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_FontTagWithExistingStyleMerged ensures distillation merges with an
// existing style attribute on the same element.
func TestRun_FontTagWithExistingStyleMerged(t *testing.T) {
	rep := Run(`<font color="red" style="line-height:1.6">x</font>`, Options{})
	if !strings.Contains(rep.CleanedHTML, "line-height:1.6") {
		t.Errorf("expected line-height retained, cleaned=%q", rep.CleanedHTML)
	}
	if !strings.Contains(rep.CleanedHTML, "color:red") {
		t.Errorf("expected color merged, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_CenterTagWithExistingStyleMerged ensures <center>'s style merge.
func TestRun_CenterTagWithExistingStyleMerged(t *testing.T) {
	rep := Run(`<center style="line-height:1.6">x</center>`, Options{})
	if !strings.Contains(rep.CleanedHTML, "text-align:center") {
		t.Errorf("expected text-align:center, cleaned=%q", rep.CleanedHTML)
	}
	if !strings.Contains(rep.CleanedHTML, "line-height:1.6") {
		t.Errorf("expected line-height preserved, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_MarqueeRetainsClassAndID verifies marquee → span keeps class/id.
func TestRun_MarqueeRetainsClassAndID(t *testing.T) {
	rep := Run(`<marquee class="cls" id="x" direction="left">y</marquee>`, Options{})
	if !strings.Contains(rep.CleanedHTML, `class="cls"`) {
		t.Errorf("expected class preserved, cleaned=%q", rep.CleanedHTML)
	}
	if strings.Contains(rep.CleanedHTML, `direction`) {
		t.Errorf("expected marquee-specific attrs stripped, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_UnknownSchemeBlocked verifies an unknown URL scheme produces a
// blocked (error) finding and the attribute is dropped.
func TestRun_UnknownSchemeBlocked(t *testing.T) {
	rep := Run(`<a href="webcal://x">x</a>`, Options{})
	gotBlocked := false
	for _, f := range rep.Blocked {
		if f.RuleID == RuleAttrUnsafeSchemeBlocked {
			gotBlocked = true
		}
	}
	if !gotBlocked {
		t.Errorf("expected ATTR_UNSAFE_SCHEME_BLOCKED in Blocked, got blocked=%+v applied=%+v", rep.Blocked, rep.Applied)
	}
	if strings.Contains(rep.CleanedHTML, "webcal:") {
		t.Errorf("expected unknown scheme stripped, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_WhitespaceObfuscatedJavaScriptScheme verifies "java\tscript:..."
// is still caught after control-byte stripping in classifyURLValue.
func TestRun_WhitespaceObfuscatedJavaScriptScheme(t *testing.T) {
	rep := Run("<a href=\"java\tscript:alert(1)\">x</a>", Options{})
	gotErr := false
	for _, f := range rep.Blocked {
		if f.RuleID == RuleAttrJSURLBlocked {
			gotErr = true
		}
	}
	if !gotErr {
		t.Errorf("expected obfuscated javascript: to be caught, got %+v", rep.Blocked)
	}
}

// TestRun_FileSchemeBlocked verifies file: URLs are rejected.
func TestRun_FileSchemeBlocked(t *testing.T) {
	rep := Run(`<a href="file:///etc/passwd">x</a>`, Options{})
	if len(rep.Blocked) == 0 {
		t.Error("expected file: to be blocked")
	}
}

// TestRun_StyleMalformedDeclarationDropped verifies a property without a
// colon delimiter is treated as malformed and dropped.
func TestRun_StyleMalformedDeclarationDropped(t *testing.T) {
	rep := Run(`<p style="color:red; malformed; line-height:1.6">x</p>`, Options{})
	gotMalformed := false
	for _, f := range rep.Applied {
		if f.RuleID == RuleStylePropertyDropped && f.TagOrAttr == "style.malformed" {
			gotMalformed = true
		}
	}
	if !gotMalformed {
		t.Errorf("expected malformed declaration to be dropped, got %+v", rep.Applied)
	}
	if !strings.Contains(rep.CleanedHTML, "color:red") || !strings.Contains(rep.CleanedHTML, "line-height:1.6") {
		t.Errorf("valid declarations should survive, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_StyleAllPropertiesDroppedRemovesAttribute verifies the style
// attribute is removed entirely when every property is invalid.
func TestRun_StyleAllPropertiesDroppedRemovesAttribute(t *testing.T) {
	// Use <div> to avoid the Feishu-native paragraph autofix, which adds
	// a fresh style attribute on the rewritten outer wrapper.
	rep := Run(`<div style="position:absolute; z-index:99">x</div>`, Options{})
	if strings.Contains(rep.CleanedHTML, "style=") {
		t.Errorf("style attribute should be removed when all props invalid, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_StyleEmptyValuePassThrough verifies an empty style attr passes.
func TestRun_StyleEmptyValuePassThrough(t *testing.T) {
	// Use <div> to avoid the Feishu-native paragraph autofix.
	rep := Run(`<div style="">x</div>`, Options{})
	if len(rep.Applied) != 0 {
		t.Errorf("empty style attr should not produce findings, got %+v", rep.Applied)
	}
}

// TestRun_HintsForAllBlockedTags verifies every blocked-tag rule has a
// non-empty hint (consumer contract).
func TestRun_HintsForAllBlockedTags(t *testing.T) {
	cases := []string{
		`<script>x</script>`, `<iframe src="x"></iframe>`,
		`<object data="x"></object>`, `<embed src="x">`, `<form><input></form>`,
		`<select></select>`, `<button>x</button>`, `<link href="x">`,
		`<meta name="x">`, `<base href="x">`,
	}
	for _, html := range cases {
		rep := Run(html, Options{})
		for _, f := range rep.Blocked {
			if f.Hint == "" {
				t.Errorf("blocked rule %s missing hint for %q", f.RuleID, html)
			}
		}
	}
}

// TestRun_HintsForAllWarnTags verifies every warn-tag rule has a non-empty hint.
func TestRun_HintsForAllWarnTags(t *testing.T) {
	cases := []string{
		`<font>x</font>`, `<center>x</center>`,
		`<marquee>x</marquee>`, `<blink>x</blink>`,
	}
	for _, html := range cases {
		rep := Run(html, Options{})
		for _, f := range rep.Applied {
			if f.Hint == "" {
				t.Errorf("warn rule %s missing hint for %q", f.RuleID, html)
			}
		}
	}
}

// TestClassifyTag_Coverage exercises classifyTag with every category.
func TestClassifyTag_Coverage(t *testing.T) {
	if k, _ := classifyTag("p"); k != "allow" {
		t.Errorf("p classified as %q", k)
	}
	if k, id := classifyTag("script"); k != "error" || id != RuleTagScriptBlocked {
		t.Errorf("script classified as %q/%q", k, id)
	}
	if k, id := classifyTag("font"); k != "warn" || id != RuleTagFontToSpan {
		t.Errorf("font classified as %q/%q", k, id)
	}
	// Niche tag passes silently (e.g. <details>).
	if k, _ := classifyTag("details"); k != "allow" {
		t.Errorf("niche tag <details> should pass through, got %q", k)
	}
	// Case-insensitive.
	if k, _ := classifyTag("SCRIPT"); k != "error" {
		t.Errorf("SCRIPT (uppercase) should still classify as error")
	}
}

// TestClassifyURLValue_CoverageEdges covers empty, whitespace-only,
// no-scheme variants.
func TestClassifyURLValue_CoverageEdges(t *testing.T) {
	cases := map[string]string{
		"":                          "ok",
		"   ":                       "ok",
		"https://x":                 "ok",
		"https://x/path?q=1":        "ok",
		"#fragment":                 "ok",
		"/relative":                 "ok",
		"javascript:alert(1)":       "error",
		"vbscript:msgbox 1":         "error",
		"data:image/png;base64,XYZ": "ok",
		"data:text/html,<script>":   "error",
		"webcal://x":                "warn",
	}
	for raw, want := range cases {
		got, _ := classifyURLValue(raw)
		if got != want {
			t.Errorf("classifyURLValue(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestClassifyStyleProperty_Coverage covers prefixes & explicit set.
func TestClassifyStyleProperty_Coverage(t *testing.T) {
	cases := map[string]bool{
		"color":            true,
		"BACKGROUND-COLOR": true, // case-insensitive
		"border-top":       true,
		"padding-left":     true,
		"margin-bottom":    true,
		"position":         false,
		"z-index":          false,
		"":                 false,
		"  ":               false,
	}
	for prop, want := range cases {
		got := classifyStyleProperty(prop)
		if got != want {
			t.Errorf("classifyStyleProperty(%q) = %v, want %v", prop, got, want)
		}
	}
}

// TestIsEventHandlerAttr_Coverage covers the on*-detection rule.
func TestIsEventHandlerAttr_Coverage(t *testing.T) {
	cases := map[string]bool{
		"onclick":     true,
		"onmouseover": true,
		"OnLoad":      true, // case-insensitive
		"on0":         true,
		"on":          false, // need at least one char after "on"
		"onerror":     true,
		"onsubmit":    true,
		"once":        true, // would match unfortunately because "once" starts with "on" + 'c'
		"id":          false,
		"href":        false,
		"data-on":     false,
	}
	for k, want := range cases {
		got := isEventHandlerAttr(k)
		if got != want {
			t.Errorf("isEventHandlerAttr(%q) = %v, want %v", k, got, want)
		}
	}
}

// TestRun_ParseFailureFallsBackGracefully verifies extreme malformed input
// short-circuits to EmptyReport.
func TestRun_PlainTextInputProducesNoFindings(t *testing.T) {
	rep := Run("just a plain string with no markup", Options{})
	if len(rep.Blocked) != 0 || len(rep.Applied) != 0 {
		t.Errorf("plain text should produce no findings, got %+v %+v", rep.Blocked, rep.Applied)
	}
}

// TestRun_MultipleErrorsAccumulate ensures multiple offenders all surface.
func TestRun_MultipleErrorsAccumulate(t *testing.T) {
	html := `<script>1</script><iframe></iframe><a href="javascript:0">x</a>` +
		`<form></form><p onclick="">y</p>`
	rep := Run(html, Options{})
	if len(rep.Blocked) < 4 {
		t.Errorf("expected ≥4 errors, got %d: %+v", len(rep.Blocked), rep.Blocked)
	}
}

// TestRun_NestedStructurePreserved verifies deep nesting passes through.
func TestRun_NestedStructurePreserved(t *testing.T) {
	html := `<div><div><div><p><span><b>deep</b></span></p></div></div></div>`
	rep := Run(html, Options{})
	if len(rep.Blocked) != 0 {
		t.Errorf("nested allowed tags should pass, got %+v", rep.Blocked)
	}
	if !strings.Contains(rep.CleanedHTML, "deep") {
		t.Errorf("inner text lost, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_BlockedInsideAllowedRemovedNotParent verifies that removing a
// blocked tag inside an allowed parent leaves the parent intact.
func TestRun_BlockedInsideAllowedRemovedNotParent(t *testing.T) {
	html := `<div>before<script>1</script>after</div>`
	rep := Run(html, Options{})
	if !strings.Contains(rep.CleanedHTML, "before") || !strings.Contains(rep.CleanedHTML, "after") {
		t.Errorf("parent text should survive, cleaned=%q", rep.CleanedHTML)
	}
	if strings.Contains(rep.CleanedHTML, "<script") {
		t.Errorf("script should be removed, cleaned=%q", rep.CleanedHTML)
	}
}

// TestRun_ListDirectChildNonLIWrapped verifies that a <ul><ul> nested
// directly without an <li> wrapper triggers LIST_DIRECT_CHILD_NON_LI and
// the inner <ul> ends up wrapped in a synthetic <li>. Same for <ol><ol>.
func TestRun_ListDirectChildNonLIWrapped(t *testing.T) {
	cases := []struct {
		name string
		html string
	}{
		{"ul wraps ul", `<ul><ul><li>x</li></ul></ul>`},
		{"ol wraps ol", `<ol><ol><li>x</li></ol></ol>`},
		{"ul wraps div", `<ul><div>orphan</div><li>real</li></ul>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := Run(tc.html, Options{})
			gotRule := false
			for _, f := range rep.Applied {
				if f.RuleID == RuleListDirectChildNonLI {
					gotRule = true
					break
				}
			}
			if !gotRule {
				t.Errorf("expected LIST_DIRECT_CHILD_NON_LI, got %+v", rep.Applied)
			}
			// The cleaned HTML should not have a direct ul>ul or ol>ol or
			// ul>div sequence anymore.
			if strings.Contains(rep.CleanedHTML, "<ul><ul") ||
				strings.Contains(rep.CleanedHTML, "<ol><ol") ||
				strings.Contains(rep.CleanedHTML, "<ul><div") {
				t.Errorf("expected synthetic <li> wrapper, cleaned=%q", rep.CleanedHTML)
			}
		})
	}
}
