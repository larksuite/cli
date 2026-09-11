// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deptest

import (
	"bytes"
	"encoding/hex"
	"os/exec"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/imageconfig"
)

// A 3x2 truecolour PNG. Held as hex so this test file imports no image codec
// of its own -- registration must come from imageconfig, not from the test.
const tinyPNGHex = "89504e470d0a1a0a0000000d49484452000000030000000208020000001216f14d0000001049444154789c63606462862006380b0001700025cd70fae00000000049454e44ae426082"

// imageconfig.Decode dispatches PNG, JPEG and GIF to the standard library's
// format registry, which only answers for codecs some package in the binary
// has imported. Owning those imports is what makes Decode work for any
// caller; if imageconfig ever stops registering them, a caller that imports
// nothing else silently loses dimension detection for the three most common
// formats -- a hard error on the sheets and docs paths, a silent downgrade
// elsewhere. This package imports no codec, so it can prove the ownership.
func TestImageConfigRegistersStandardCodecs(t *testing.T) {
	raw, err := hex.DecodeString(tinyPNGHex)
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := imageconfig.Decode(bytes.NewReader(raw))
	if err != nil || format != "png" || cfg.Width != 3 || cfg.Height != 2 {
		t.Fatalf("config=%+v format=%q err=%v; imageconfig must register the standard codecs itself", cfg, format, err)
	}
}

// The behavioural check above can only cover one format without embedding a
// fixture per codec. Pin the other two structurally.
func TestImageConfigImportsEveryStandardCodec(t *testing.T) {
	cmd := exec.Command("go", "list", "-f", `{{join .Imports " "}}`, "./internal/imageconfig")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list failed: %v\n%s", err, out)
	}
	imports := strings.Fields(string(out))
	for _, codec := range []string{"image/gif", "image/jpeg", "image/png"} {
		if !containsDep(imports, codec) {
			t.Errorf("internal/imageconfig no longer imports %s; Decode's standard-library fallback depends on it", codec)
		}
	}
}
