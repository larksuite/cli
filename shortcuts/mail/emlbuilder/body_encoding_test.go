// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package emlbuilder

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

func TestSelectCTE_LineLength(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"empty", "", "7bit"},
		{"below limit", strings.Repeat("a", 997), "7bit"},
		{"at limit", strings.Repeat("a", 998), "7bit"},
		{"over limit", strings.Repeat("a", 999), "base64"},
		{"short lines", strings.Repeat(strings.Repeat("a", 998)+"\n", 3), "7bit"},
		{"long later line", "short\n" + strings.Repeat("a", 999), "base64"},
		{"long CRLF line", strings.Repeat("a", 998) + "\r\n", "base64"},
		{"non ASCII", "你好", "base64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectCTE([]byte(tc.body)); got != tc.want {
				t.Fatalf("CTE = %q, want %q", got, tc.want)
			}
		})
	}
}

// Decode real MIME output to catch content corruption, not just line lengths.
func TestBuild_LongASCIIBodyRoundTrip(t *testing.T) {
	body := []byte("<div>" + strings.Repeat("word ", 450) + "</div>\n" +
		`<div style="font-family:Arial;margin-top:0">signature</div>`)
	for _, tc := range []struct {
		name  string
		build func(Builder) Builder
		parts int
	}{
		{"text", func(b Builder) Builder { return b.TextBody(body) }, 1},
		{"html", func(b Builder) Builder { return b.HTMLBody(body) }, 1},
		{"calendar", func(b Builder) Builder { return b.CalendarBody(body) }, 1},
		{"alternative", func(b Builder) Builder { return b.TextBody(body).HTMLBody(body) }, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := tc.build(New().From("", "alice@example.com").To("", "bob@example.com")).Build()
			if err != nil {
				t.Fatal(err)
			}
			message, err := mail.ReadMessage(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			assertBody := func(cte string, reader io.Reader) {
				t.Helper()
				encoded, err := io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				decoded := encoded
				if cte == "base64" {
					decoded, err = io.ReadAll(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(encoded)))
					if err != nil {
						t.Fatal(err)
					}
				}
				if !bytes.Equal(decoded, body) {
					t.Fatal("decoded MIME body differs from the supplied body")
				}
				if cte != "base64" {
					t.Fatalf("CTE = %q, want base64", cte)
				}
				for _, line := range bytes.Split(encoded, []byte("\n")) {
					if len(line) > 76 {
						t.Fatalf("encoded line has %d bytes, want at most 76", len(line))
					}
				}
			}
			mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
			if err != nil {
				t.Fatal(err)
			}
			if mediaType != "multipart/alternative" {
				assertBody(message.Header.Get("Content-Transfer-Encoding"), message.Body)
				return
			}
			reader := multipart.NewReader(message.Body, params["boundary"])
			parts := 0
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				assertBody(part.Header.Get("Content-Transfer-Encoding"), part)
				parts++
			}
			if parts != tc.parts {
				t.Fatalf("MIME parts = %d, want %d", parts, tc.parts)
			}
		})
	}
}
