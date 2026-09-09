// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package deploy holds the pure logic behind publishing a bare HTML file or
// directory as a Miaoda app: payload collection, entry resolution, guards,
// zip manifest and the content fingerprint. It is deliberately independent
// of the +html-publish implementation so the two can evolve separately.
package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/larksuite/cli/errs"
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

// signatureEntry keeps path before signature, the order the web client's
// object literal produces and therefore the order JSON.stringify emits.
type signatureEntry struct {
	Path      string
	Signature string
}

// hexDigits is lowercase because that is what JSON.stringify emits for the
// escapes it does produce.
const hexDigits = "0123456789abcdef"

// appendJSONString writes s the way JavaScript's JSON.stringify writes a
// string. encoding/json cannot be used even with SetEscapeHTML(false): Go also
// escapes U+2028 and U+2029, which JSON.stringify leaves literal. A file name
// carrying either character would produce a different digest and leave the GUI
// permanently reporting the content as out of sync -- with no error anywhere to
// explain it. The rest of the rules match, but they are written out here rather
// than relied upon, since the whole value of this function is being byte-exact.
func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	for _, r := range s {
		switch r {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if r < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[r>>4], hexDigits[r&0xF])
				continue
			}
			dst = append(dst, string(r)...)
		}
	}
	return append(dst, '"')
}

// marshalSignatures renders the array exactly as JSON.stringify would: no
// whitespace, keys in literal order.
func marshalSignatures(entries []signatureEntry) []byte {
	out := make([]byte, 0, 64*len(entries))
	out = append(out, '[')
	for i, e := range entries {
		if i > 0 {
			out = append(out, ',')
		}
		out = append(out, `{"path":`...)
		out = appendJSONString(out, e.Path)
		out = append(out, `,"signature":`...)
		out = appendJSONString(out, e.Signature)
		out = append(out, '}')
	}
	return append(out, ']')
}

// ContentHash returns the fingerprint for the given file set: a single file is
// hashed from its raw bytes; two or more go through the [{path,signature}]
// digest. Callers must exclude CLI-generated files such as routes.json.
func ContentHash(files []HashFile) (string, error) {
	switch len(files) {
	case 0:
		return "", errs.NewInternalError(errs.SubtypeUnknown, "content hash needs at least one file")
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

	return sha256Hex(marshalSignatures(entries)), nil
}
