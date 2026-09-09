// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package deploy holds the pure logic behind publishing a bare HTML file or
// directory as a Miaoda app: payload collection, entry resolution, guards,
// zip manifest and the content fingerprint. It is deliberately independent
// of the +html-publish implementation so the two can evolve separately.
package deploy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// HashFile is one file taking part in the content fingerprint. Path uses the
// published-path convention (the entry is recorded as index.html, everything
// else relative to the entry's directory); Raw is the file's original bytes.
type HashFile struct {
	Path string
	Raw  []byte
}

// hashedExts are fingerprinted by content. Anything outside this set (images,
// fonts, svg, other binaries) contributes its byte count instead, matching the
// algorithm the web client uses.
var hashedExts = map[string]bool{
	"css": true, "htm": true, "html": true, "js": true, "json": true, "mjs": true,
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// utf16Less compares by UTF-16 code unit, matching JavaScript's default
// Array.sort(). Go's native string comparison is UTF-8 byte order, which
// disagrees on supplementary-plane characters.
func utf16Less(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}

func sortPathsUTF16(paths []string) []string {
	sort.Slice(paths, func(i, j int) bool { return utf16Less(paths[i], paths[j]) })
	return paths
}

// signatureEntry keeps path before signature; a struct rather than a map so
// the key order is fixed.
type signatureEntry struct {
	Path      string `json:"path"`
	Signature string `json:"signature"`
}

// ContentHash returns the fingerprint for the given file set: a single file is
// hashed from its raw bytes; two or more go through the [{path,signature}]
// digest. Callers must exclude CLI-generated files such as routes.json.
func ContentHash(files []HashFile) (string, error) {
	switch len(files) {
	case 0:
		return "", fmt.Errorf("content hash needs at least one file")
	case 1:
		return sha256Hex(files[0].Raw), nil
	}

	entries := make([]signatureEntry, 0, len(files))
	for _, f := range files {
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Path), "."))
		sig := strconv.Itoa(len(f.Raw))
		if hashedExts[ext] {
			sig = sha256Hex(f.Raw)
		}
		entries = append(entries, signatureEntry{Path: f.Path, Signature: sig})
	}
	sort.Slice(entries, func(i, j int) bool { return utf16Less(entries[i].Path, entries[j].Path) })

	// SetEscapeHTML(false) is load-bearing: Go escapes < > & by default while
	// JSON.stringify does not, and any difference changes the digest.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(entries); err != nil {
		return "", fmt.Errorf("marshal signature array: %w", err)
	}
	return sha256Hex(bytes.TrimRight(buf.Bytes(), "\n")), nil
}
