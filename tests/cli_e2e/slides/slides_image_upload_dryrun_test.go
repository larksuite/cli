// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestSlides_ImageUploadDryRunParentType pins the parent_type the slides
// image-upload surfaces emit for native vs. imported "office" presentations.
// Native decks upload as "slide_file"; a deck whose token carries the
// interleaved "OFL0X" marker (or a legacy synthetic office prefix) must upload
// as "office_slide_file", the presentation counterpart of the office_sheet_file
// rule the sheets domain already applies.
//
// This runs through the built binary because the parent_type has to survive the
// whole path — flag parsing, ref classification, the @path placeholder scan.
//
// Covered surfaces: +media-upload directly, and +add-slide / +update-slide via
// <img src="@...">, each with a direct token and with an unresolved wiki
// reference. +create is deliberately absent: it has no --presentation flag, so it
// always uploads into a deck it just created through the API, which is never an
// imported office one. Its preview is pinned in the unit lane instead, where the
// native-only expectation can be stated as such.
//
// The wiki cases pin slide_file for a token the preview never sees. That is sound
// rather than a guess: resolvePresentationID rejects any wiki node whose obj_type
// is not "slides", and an imported office deck is a drive "file" node, so it
// cannot reach an upload through a wiki ref at all. They are here because the
// production code now asserts that value instead of arriving at it by running the
// placeholder through the office check, and only an end-to-end case can tell the
// two apart if the placeholder is ever respelled.
func TestSlides_ImageUploadDryRunParentType(t *testing.T) {
	setSlidesDryRunEnv(t)

	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "img.png"), []byte("png-bytes"), 0o600))

	const (
		nativeToken = "presDryRunNative"
		officeToken = "aaaaOaaaaFaaaaLaaaa0aaaaXaaa"
		wikiURL     = "https://example.feishu.cn/wiki/wikcnDryRunProbe123"
		// What a preview shows in place of a token it must not resolve.
		unresolvedNode = "<resolved_slides_token>"
		slideXML       = `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
			`<img src="@img.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
			`</data></slide>`
	)

	tests := []struct {
		name           string
		args           []string
		wantParentNode string
		wantParentType string
		// uploadStep is the index of the upload_all call in data.api. A wiki ref
		// plans get_node first, so its upload is not the leading step.
		uploadStep string
	}{
		{
			name: "media-upload native",
			args: []string{
				"slides", "+media-upload",
				"--presentation", nativeToken,
				"--file", "img.png",
				"--dry-run",
			},
			wantParentNode: nativeToken,
			wantParentType: "slide_file",
			uploadStep:     "0",
		},
		{
			name: "media-upload office",
			args: []string{
				"slides", "+media-upload",
				"--presentation", officeToken,
				"--file", "img.png",
				"--dry-run",
			},
			wantParentNode: officeToken,
			wantParentType: "office_slide_file",
			uploadStep:     "0",
		},
		{
			name: "media-upload wiki, wiki deck is always native",
			args: []string{
				"slides", "+media-upload",
				"--presentation", wikiURL,
				"--file", "img.png",
				"--dry-run",
			},
			wantParentNode: unresolvedNode,
			wantParentType: "slide_file",
			uploadStep:     "1",
		},
		{
			name: "add-slide placeholder native",
			args: []string{
				"slides", "+add-slide",
				"--presentation", nativeToken,
				"--slide", slideXML,
				"--dry-run",
			},
			wantParentNode: nativeToken,
			wantParentType: "slide_file",
			uploadStep:     "0",
		},
		{
			name: "add-slide placeholder office",
			args: []string{
				"slides", "+add-slide",
				"--presentation", officeToken,
				"--slide", slideXML,
				"--dry-run",
			},
			wantParentNode: officeToken,
			wantParentType: "office_slide_file",
			uploadStep:     "0",
		},
		{
			name: "add-slide placeholder wiki, wiki deck is always native",
			args: []string{
				"slides", "+add-slide",
				"--presentation", wikiURL,
				"--slide", slideXML,
				"--dry-run",
			},
			wantParentNode: unresolvedNode,
			wantParentType: "slide_file",
			uploadStep:     "1",
		},
		{
			name: "update-slide placeholder office",
			args: []string{
				"slides", "+update-slide",
				"--presentation", officeToken,
				"--slide-id", "slide_1",
				"--content", slideXML,
				"--dry-run",
			},
			wantParentNode: officeToken,
			wantParentType: "office_slide_file",
			uploadStep:     "0",
		},
		{
			name: "update-slide placeholder wiki, wiki deck is always native",
			args: []string{
				"slides", "+update-slide",
				"--presentation", wikiURL,
				"--slide-id", "slide_1",
				"--content", slideXML,
				"--dry-run",
			},
			wantParentNode: unresolvedNode,
			wantParentType: "slide_file",
			uploadStep:     "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      tt.args,
				DefaultAs: "user",
				WorkDir:   workDir,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			// The upload precedes the page XML that references its file_token,
			// and follows the get_node that names the deck to upload into.
			require.Equal(t, "POST", clie2e.DryRunGet(out, "api."+tt.uploadStep+".method").String(),
				"data.api.%s must be the drive upload; stdout:\n%s", tt.uploadStep, out)
			require.Equal(t, "/open-apis/drive/v1/medias/upload_all",
				clie2e.DryRunGet(out, "api."+tt.uploadStep+".url").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantParentType,
				clie2e.DryRunGet(out, "api."+tt.uploadStep+".body.parent_type").String(),
				"parent_type for node %q must be %q; stdout:\n%s", tt.wantParentNode, tt.wantParentType, out)
			require.Equal(t, tt.wantParentNode,
				clie2e.DryRunGet(out, "api."+tt.uploadStep+".body.parent_node").String(),
				"parent_node must equal the presentation token, or the placeholder when unresolved; stdout:\n%s", out)
		})
	}
}
