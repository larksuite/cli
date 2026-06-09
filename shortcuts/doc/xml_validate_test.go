// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"testing"
)

func TestValidatePreTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		wantWarn int
	}{
		{
			name:     "empty content",
			content:  "",
			wantWarn: 0,
		},
		{
			name:     "no pre tags",
			content:  "<title>test</title><p>hello</p>",
			wantWarn: 0,
		},
		{
			name:     "pre with code child",
			content:  `<pre lang="text"><code>fmt.Println("hello")</code></pre>`,
			wantWarn: 0,
		},
		{
			name:     "pre with code child and attributes",
			content:  `<pre lang="go" caption="示例"><code>func main() {}</code></pre>`,
			wantWarn: 0,
		},
		{
			name:     "pre without code child",
			content:  "<pre lang=\"text\">line1\nline2\nline3</pre>",
			wantWarn: 1,
		},
		{
			name:     "pre without code child, self-closing pre",
			content:  "<pre lang=\"text\"/>",
			wantWarn: 0,
		},
		{
			name:     "multiple pre blocks, one missing code",
			content:  "<pre lang=\"go\"><code>ok</code></pre>\n<pre lang=\"text\">missing code</pre>",
			wantWarn: 1,
		},
		{
			name:     "multiple pre blocks, all missing code",
			content:  "<pre lang=\"text\">first</pre>\n<pre lang=\"python\">second</pre>",
			wantWarn: 2,
		},
		{
			name:     "multiple pre blocks, all have code",
			content:  "<pre lang=\"go\"><code>a</code></pre>\n<pre lang=\"py\"><code>b</code></pre>",
			wantWarn: 0,
		},
		{
			name:     "pre with multiline content missing code",
			content:  "<pre lang=\"xml\">\n<pre lang=\"text\"><code>...content...</code></pre>\n</pre>",
			wantWarn: 0,
		},
		{
			name:     "pre with code tag in attributes only",
			content:  "<pre lang=\"text\" caption=\"<code>example</code>\">plain text</pre>",
			wantWarn: 1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			warnings := validatePreTags(tt.content)
			if len(warnings) != tt.wantWarn {
				t.Fatalf("validatePreTags() returned %d warnings, want %d\nwarnings: %v\ncontent: %s",
					len(warnings), tt.wantWarn, warnings, tt.content)
			}
		})
	}
}
