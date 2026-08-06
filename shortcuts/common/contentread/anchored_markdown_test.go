// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"strings"
	"testing"
)

func mustRenderAnchoredMarkdown(t *testing.T, xmlContent string, metas map[string]*ImageMeta, maxRows int) string {
	t.Helper()
	md, err := renderAnchoredMarkdown(xmlContent, metas, maxRows)
	if err != nil {
		t.Fatalf("renderAnchoredMarkdown error: %v", err)
	}
	return md
}

func TestRenderAnchoredMarkdown_HeadingsAnchorParagraphsDont(t *testing.T) {
	t.Parallel()
	xml := `<h1 id="blk_root">文档树结构</h1>` +
		`<p id="blk_para">涉及的节点类别包括：</p>` +
		`<h2 id="blk_h2">二级标题</h2>`
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)

	for _, want := range []string{
		"# 文档树结构 {#blk_root}",
		"## 二级标题 {#blk_h2}",
		"涉及的节点类别包括：",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{#blk_para}") {
		t.Errorf("paragraph must not get an anchor, got:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_StripsIllegalXMLControlChars(t *testing.T) {
	t.Parallel()
	xml := "<h1 id=\"blk\">A\x0cB</h1><p>x\x08y</p>"
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)

	if !strings.Contains(got, "# AB {#blk}") {
		t.Errorf("heading should render with control char stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "xy") {
		t.Errorf("paragraph should render with control char stripped, got:\n%s", got)
	}
	if strings.ContainsAny(got, "\x0c\x08") {
		t.Errorf("output still carries a control char:\n%q", got)
	}
}

func TestStripInvalidXMLChars_FastPathReturnsCleanInputUnchanged(t *testing.T) {
	t.Parallel()
	clean := "<h1 id=\"x\">tab\there\nand newline</h1>"
	if got := stripInvalidXMLChars(clean); got != clean {
		t.Errorf("clean input must be returned unchanged, got:\n%q", got)
	}
	if got := stripInvalidXMLChars("a\x0c\x08b"); got != "ab" {
		t.Errorf("control chars must be stripped, got: %q", got)
	}
}

func TestRenderAnchoredMarkdown_HeadingWithoutIDStaysPlain(t *testing.T) {
	t.Parallel()
	got := mustRenderAnchoredMarkdown(t, `<h3>无 id 标题</h3>`, nil, 0)
	if !strings.Contains(got, "### 无 id 标题") {
		t.Errorf("want plain heading, got:\n%s", got)
	}
	if strings.Contains(got, "{#") {
		t.Errorf("no anchor expected, got:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_List(t *testing.T) {
	t.Parallel()
	ul := `<ul><li id="a">根节点</li><li id="b">表格</li></ul>`
	got := mustRenderAnchoredMarkdown(t, ul, nil, 0)
	if !strings.Contains(got, "- 根节点") || !strings.Contains(got, "- 表格") {
		t.Errorf("unordered list wrong:\n%s", got)
	}
	if strings.Contains(got, "{#a}") {
		t.Errorf("list item must not get anchor, got:\n%s", got)
	}

	ol := `<ol><li>一</li><li>二</li></ol>`
	gotO := mustRenderAnchoredMarkdown(t, ol, nil, 0)
	if !strings.Contains(gotO, "1. 一") || !strings.Contains(gotO, "2. 二") {
		t.Errorf("ordered list wrong:\n%s", gotO)
	}
}

func TestRenderAnchoredMarkdown_Code(t *testing.T) {
	t.Parallel()
	xml := `<pre id="c" lang="go"><code>fmt.Println("hi")</code></pre>`
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)
	if !strings.Contains(got, "```go") || !strings.Contains(got, `fmt.Println("hi")`) {
		t.Errorf("code fence wrong:\n%s", got)
	}
	if strings.Contains(got, "{#c}") {
		t.Errorf("code must not get anchor, got:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_ImageAnchoredAndJoined(t *testing.T) {
	t.Parallel()
	metas := map[string]*ImageMeta{
		"imgtok": {Caption: "架构图"},
	}
	xml := `<img id="blk_img" token="imgtok"/>`

	got := mustRenderAnchoredMarkdown(t, xml, metas, 0)
	if !strings.Contains(got, "![架构图](imgtok) {#blk_img}") {
		t.Errorf("image wrong:\n%s", got)
	}

	missing := mustRenderAnchoredMarkdown(t, `<img id="x" token="nope"/>`, metas, 0)
	if !strings.Contains(missing, "![image](nope) {#x}") {
		t.Errorf("image missing-meta wrong:\n%s", missing)
	}
}

func TestRenderAnchoredMarkdown_NativeSheetToGFM(t *testing.T) {
	t.Parallel()
	xml := `<sheet id="blk_sheet"><table>` +
		`<tr><th>姓名</th><th>分数</th></tr>` +
		`<tr><td>张三</td><td>90</td></tr>` +
		`<tr><td>李四</td><td>85</td></tr>` +
		`</table></sheet>`
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)

	if !strings.Contains(got, "**表** {#blk_sheet}") {
		t.Errorf("sheet heading/anchor wrong:\n%s", got)
	}
	for _, want := range []string{
		"| 姓名 | 分数 |",
		"| --- | --- |",
		"| 张三 | 90 |",
		"| 李四 | 85 |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing GFM row %q in:\n%s", want, got)
		}
	}
}

func TestRenderAnchoredMarkdown_SheetNestedInListNotDropped(t *testing.T) {
	t.Parallel()
	xml := `<ol>` +
		`<li id="li1">第一步</li>` +
		`<sheet id="blk_sheet"><table>` +
		`<thead><tr><th>边</th><th>方式</th></tr></thead>` +
		`<tbody><tr><td>文档-文档</td><td>引用</td></tr></tbody>` +
		`</table></sheet>` +
		`<li id="li2">第二步</li>` +
		`</ol>`
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)

	if !strings.Contains(got, "1. 第一步") || !strings.Contains(got, "第二步") {
		t.Errorf("list items wrong:\n%s", got)
	}
	if !strings.Contains(got, "**表** {#blk_sheet}") {
		t.Errorf("nested sheet anchor missing (table dropped?):\n%s", got)
	}
	for _, want := range []string{
		"| 边 | 方式 |",
		"| --- | --- |",
		"| 文档-文档 | 引用 |",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing GFM row %q in:\n%s", want, got)
		}
	}
}

func TestRenderAnchoredMarkdown_SheetTruncation(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString(`<sheet id="s"><table><tr><th>n</th></tr>`)
	for i := 0; i < 5; i++ {
		b.WriteString(`<tr><td>r</td></tr>`)
	}
	b.WriteString(`</table></sheet>`)
	got := mustRenderAnchoredMarkdown(t, b.String(), nil, 2)

	if strings.Count(got, "| r |") != 2 {
		t.Errorf("want 2 kept data rows, got:\n%s", got)
	}
	if !strings.Contains(got, "还有 3 行") {
		t.Errorf("want truncation hint for 3 dropped rows, got:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_EmbeddedBitablePlaceholder(t *testing.T) {
	t.Parallel()
	xml := `<bitable id="blk_bt" token="bbl_secret" table-id="tblX" source-doc-id="docY"></bitable>`
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)

	if !strings.Contains(got, "**[多维表格](token=bbl_secret)** {#blk_bt}") {
		t.Errorf("bitable placeholder wrong:\n%s", got)
	}
	if !strings.Contains(got, "内容可能未展开，用 base +record-list 取") {
		t.Errorf("want 'may not be expanded, fetch via base' hint, got:\n%s", got)
	}
	if strings.Contains(got, "docY") {
		t.Errorf("source-doc-id must not leak, got:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_EmbeddedBitableExpanded(t *testing.T) {
	t.Parallel()
	xml := `<bitable id="blk_bt" token="bbl_secret" table-id="tblX"><table><tr><th>业务</th><th>poc</th></tr><tr><td>知识问答</td><td>@崔</td></tr></table></bitable>`
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)

	if !strings.Contains(got, "**[表](token=bbl_secret)** {#blk_bt}") {
		t.Errorf("bitable header line wrong:\n%s", got)
	}
	if !strings.Contains(got, "| 业务 | poc |") {
		t.Errorf("want GFM header with client-side spacing, got:\n%s", got)
	}
	if !strings.Contains(got, "| 知识问答 | @崔 |") {
		t.Errorf("want GFM data row, got:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_EmbeddedBitableTruncation(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString(`<bitable id="blk_bt" token="bbl" table-id="tblX"><table>`)
	b.WriteString(`<tr><th>n</th></tr>`)
	for i := 0; i < 5; i++ {
		b.WriteString(`<tr><td>r</td></tr>`)
	}
	b.WriteString(`</table></bitable>`)
	got := mustRenderAnchoredMarkdown(t, b.String(), nil, 2)

	if strings.Count(got, "| r |") != 2 {
		t.Errorf("want 2 kept data rows, got:\n%s", got)
	}
	if !strings.Contains(got, "**[表](token=bbl)** {#blk_bt}") {
		t.Errorf("bitable anchor/token link must survive truncation, got:\n%s", got)
	}
	if !strings.Contains(got, "还有 3 行") {
		t.Errorf("want truncation hint for 3 dropped rows, got:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_EmbeddedSyncedMarkdown(t *testing.T) {
	t.Parallel()
	xml := "<synced id=\"blk_syn\" source-doc-id=\"docY\">同步块第一行\n第二行&lt;a&gt;</synced>"
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)

	if !strings.Contains(got, "**同步块** {#blk_syn}") {
		t.Errorf("synced header line wrong:\n%s", got)
	}
	if !strings.Contains(got, "同步块第一行") || !strings.Contains(got, "第二行<a>") {
		t.Errorf("want inlined + un-escaped markdown text, got:\n%s", got)
	}
	if strings.Contains(got, "base 技能") {
		t.Errorf("expanded synced must not be a placeholder, got:\n%s", got)
	}
	if strings.Contains(got, "docY") {
		t.Errorf("source-doc-id must not leak, got:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_EmbeddedComponentMarkdown(t *testing.T) {
	t.Parallel()
	xml := "<component id=\"blk_task\">任务：完成方案设计\n状态：未完成</component>"
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)

	if !strings.Contains(got, "**引用内容** {#blk_task}") {
		t.Errorf("component header line wrong:\n%s", got)
	}
	if !strings.Contains(got, "任务：完成方案设计") || !strings.Contains(got, "状态：未完成") {
		t.Errorf("want inlined markdown text, got:\n%s", got)
	}
	if strings.Contains(got, "base 技能") {
		t.Errorf("expanded component must not be a placeholder, got:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_WhiteboardPlaceholder(t *testing.T) {
	t.Parallel()
	got := mustRenderAnchoredMarkdown(t, `<whiteboard id="wb" token="board_tok"></whiteboard>`, nil, 0)
	if !strings.Contains(got, "> [画板](token=board_tok) {#wb}") {
		t.Errorf("whiteboard placeholder wrong:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_UnescapesEntities(t *testing.T) {
	t.Parallel()
	got := mustRenderAnchoredMarkdown(t, `<p>a &amp; b &lt; c &gt; d &quot; e &apos; f</p>`, nil, 0)
	if !strings.Contains(got, `a & b < c > d " e ' f`) {
		t.Errorf("entities not un-escaped:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_ToleratesBareLessThan(t *testing.T) {
	t.Parallel()
	xml := `<p>cmd &lt; ok</p>` +
		`<p>heredoc: <<'EOF'</p>` +
		`<p>range a < b</p>` +
		`<h2 id="blk_h2">after</h2>`
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)

	for _, want := range []string{
		"heredoc: <<'EOF'",
		"range a < b",
		"cmd < ok",
		"## after {#blk_h2}",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderAnchoredMarkdown_CellPipeEscaped(t *testing.T) {
	t.Parallel()
	xml := `<sheet id="s"><table><tr><td>a|b</td><td>c</td></tr></table></sheet>`
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)
	if !strings.Contains(got, `a\|b`) {
		t.Errorf("pipe in cell not escaped:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_CellImageMarker(t *testing.T) {
	t.Parallel()
	metas := map[string]*ImageMeta{
		"K1": {Caption: "图1"},
	}
	xml := `<sheet id="s"><table>` +
		`<tr><th>col</th></tr>` +
		`<tr><td>&lt;qa:image&gt;anchor="b" image_token="K1" w="0" h="0"&lt;/qa&gt;</td></tr>` +
		`<tr><td>&lt;qa:image&gt;image_token="K2"&lt;/qa&gt;</td></tr>` +
		`</table></sheet>`
	got := mustRenderAnchoredMarkdown(t, xml, metas, 0)

	if !strings.Contains(got, "![图1](K1)") {
		t.Errorf("cell image with meta should join url:\n%s", got)
	}
	if !strings.Contains(got, "![image](K2)") {
		t.Errorf("cell image without meta should fall back to token:\n%s", got)
	}
	if strings.Contains(got, "<qa:image>") || strings.Contains(got, "</qa>") || strings.Contains(got, "image_token=") {
		t.Errorf("raw qa:image marker must not leak:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_EmptyInput(t *testing.T) {
	t.Parallel()
	got := mustRenderAnchoredMarkdown(t, ``, nil, 0)
	if strings.TrimSpace(got) != "" {
		t.Errorf("empty input should render empty, got: %q", got)
	}
}

func TestRenderAnchoredMarkdown_HTMLishTableTolerated(t *testing.T) {
	t.Parallel()
	xml := `<sheet id="s"><table><tr><td>line1<br>line2</td><td>a&nbsp;b</td></tr></table></sheet>`
	got := mustRenderAnchoredMarkdown(t, xml, nil, 0)
	if !strings.Contains(got, "**表** {#s}") {
		t.Errorf("html-ish table should still render, got:\n%s", got)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Errorf("cell text lost:\n%s", got)
	}
}

func TestRenderAnchoredMarkdown_NilOrEmptyResp(t *testing.T) {
	t.Parallel()
	if got, err := RenderAnchoredMarkdown(nil, 0); err != nil || got != "" {
		t.Errorf("nil resp: got (%q, %v), want (\"\", nil)", got, err)
	}
	if got, err := RenderAnchoredMarkdown(&Response{}, 0); err != nil || got != "" {
		t.Errorf("empty resp: got (%q, %v), want (\"\", nil)", got, err)
	}
}
