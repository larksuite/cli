// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

// TestMultipartWriter_CreateFormFile_EscapesFilename verifies that filenames
// containing backslash or double-quote — the two characters every supported
// Go version's stdlib escapes via quoted-pair — are properly encoded on the
// wire and round-trip through mime.ParseMediaType.
//
// Regression test: an earlier custom CreateFormFile concatenated raw strings
// without escaping, so a filename like `report "draft".pdf` produced a
// malformed header that servers parsed as `filename="report "` (truncated at
// the first internal quote).
//
// CR / LF in filenames are not covered here: Go 1.23's stdlib does not
// percent-encode them, so they would break the header — but a CR or LF in a
// real filename is essentially never legal on any supported OS, so leaving it
// out of scope keeps the test stable across stdlib versions.
//
// Filename parameters are read via mime.ParseMediaType on the raw
// Content-Disposition header — Part.FileName runs the result through
// filepath.Base which is platform-dependent for backslash on Windows.
func TestMultipartWriter_CreateFormFile_EscapesFilename(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		filename    string
		wantEncoded string // expected escaped form embedded in the header
	}{
		// happy path: no characters need escaping
		{"plain ASCII", "report.pdf", "report.pdf"},
		{"unicode", "报告 v2.pdf", "报告 v2.pdf"},

		// backslash escaping: round-trips exactly through mime.ParseMediaType
		{"double quote", `report "draft" v2.pdf`, `report \"draft\" v2.pdf`},
		{"backslash", `report\draft.pdf`, `report\\draft.pdf`},
		{"backslash and quote", `path\to "weird" file.bin`, `path\\to \"weird\" file.bin`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			mw := NewMultipartWriter(&buf)
			w, err := mw.CreateFormFile("file", tc.filename)
			if err != nil {
				t.Fatalf("CreateFormFile error: %v", err)
			}
			if _, err := io.WriteString(w, "body-bytes"); err != nil {
				t.Fatalf("write body: %v", err)
			}
			if err := mw.Close(); err != nil {
				t.Fatalf("close writer: %v", err)
			}

			body := buf.String()
			wantHeader := `filename="` + tc.wantEncoded + `"`
			if !strings.Contains(body, wantHeader) {
				t.Errorf("Content-Disposition does not contain %q\nbody:\n%s", wantHeader, body)
			}

			r := multipart.NewReader(strings.NewReader(body), mw.Boundary())
			part, err := r.NextPart()
			if err != nil {
				t.Fatalf("read part: %v", err)
			}
			_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			if err != nil {
				t.Fatalf("ParseMediaType on Content-Disposition: %v", err)
			}
			if got := params["filename"]; got != tc.filename {
				t.Errorf("filename round-trip: got %q, want %q", got, tc.filename)
			}
			if got := params["name"]; got != "file" {
				t.Errorf("name: got %q, want %q", got, "file")
			}
		})
	}
}

// TestMultipartWriter_CreateFormFile_ContentType verifies that the file part
// carries the expected Content-Type for binary uploads.
func TestMultipartWriter_CreateFormFile_ContentType(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	mw := NewMultipartWriter(&buf)
	if _, err := mw.CreateFormFile("file", "x.bin"); err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	r := multipart.NewReader(&buf, mw.Boundary())
	part, err := r.NextPart()
	if err != nil {
		t.Fatalf("read part: %v", err)
	}
	if got := part.Header.Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type: got %q, want application/octet-stream", got)
	}
}
