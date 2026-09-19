// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import "testing"

func TestContentHashSingleFile(t *testing.T) {
	// sha256("hello")
	got, err := ContentHash([]HashFile{{Path: "index.html", Raw: []byte("hello")}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestContentHashMultiFileSignatureKinds(t *testing.T) {
	files := []HashFile{
		{Path: "index.html", Raw: []byte("<p>hi</p>")},
		{Path: "logo.png", Raw: []byte{0x89, 0x50, 0x4e, 0x47}},
	}
	got, err := ContentHash(files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := sha256Hex([]byte(
		`[{"path":"index.html","signature":"` + sha256Hex([]byte("<p>hi</p>")) + `"},` +
			`{"path":"logo.png","signature":"4"}]`))
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestContentHashDoesNotEscapeHTML(t *testing.T) {
	files := []HashFile{
		{Path: "a&b.png", Raw: []byte("xy")},
		{Path: "index.html", Raw: []byte("z")},
	}
	got, err := ContentHash(files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := sha256Hex([]byte(
		`[{"path":"a&b.png","signature":"2"},` +
			`{"path":"index.html","signature":"` + sha256Hex([]byte("z")) + `"}]`))
	if got != want {
		t.Errorf("HTML escaping leaked into the digest input: got %q, want %q", got, want)
	}
}

func TestContentHashSortsByUTF16CodeUnit(t *testing.T) {
	// U+FF3A（BMP，UTF-16 = 0xFF3A）与 U+1D400（补充平面，代理对首码元 0xD835）。
	// UTF-8 字节序：U+FF3A < U+1D400；UTF-16 码元序相反。
	got := sortPathsUTF16([]string{"Ｚ.png", "\U0001D400.png"})
	if got[0] != "\U0001D400.png" || got[1] != "Ｚ.png" {
		t.Errorf("got %q, want UTF-16 code-unit order", got)
	}
}
