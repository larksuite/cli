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
// whole path — flag parsing, ref classification, the @path placeholder scan —
// and the covered entries are every surface a local file can enter through:
// +media-upload directly, and +add-slide / +update-slide via <img src="@...">.
func TestSlides_ImageUploadDryRunParentType(t *testing.T) {
	setSlidesDryRunEnv(t)

	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "img.png"), []byte("png-bytes"), 0o600))

	const (
		nativeToken = "presDryRunNative"
		officeToken = "aaaaOaaaaFaaaaLaaaa0aaaaXaaa"
		slideXML    = `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
			`<img src="@img.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
			`</data></slide>`
	)

	tests := []struct {
		name           string
		args           []string
		token          string
		wantParentType string
	}{
		{
			name: "media-upload native",
			args: []string{
				"slides", "+media-upload",
				"--presentation", nativeToken,
				"--file", "img.png",
				"--dry-run",
			},
			token:          nativeToken,
			wantParentType: "slide_file",
		},
		{
			name: "media-upload office",
			args: []string{
				"slides", "+media-upload",
				"--presentation", officeToken,
				"--file", "img.png",
				"--dry-run",
			},
			token:          officeToken,
			wantParentType: "office_slide_file",
		},
		{
			name: "add-slide placeholder native",
			args: []string{
				"slides", "+add-slide",
				"--presentation", nativeToken,
				"--slide", slideXML,
				"--dry-run",
			},
			token:          nativeToken,
			wantParentType: "slide_file",
		},
		{
			name: "add-slide placeholder office",
			args: []string{
				"slides", "+add-slide",
				"--presentation", officeToken,
				"--slide", slideXML,
				"--dry-run",
			},
			token:          officeToken,
			wantParentType: "office_slide_file",
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
			token:          officeToken,
			wantParentType: "office_slide_file",
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
			// The upload is planned first in every case: the file_token has to
			// exist before the page XML that references it can be posted.
			require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(),
				"data.api.0 must be the drive upload; stdout:\n%s", out)
			require.Equal(t, "/open-apis/drive/v1/medias/upload_all",
				clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, tt.wantParentType, clie2e.DryRunGet(out, "api.0.body.parent_type").String(),
				"parent_type for token %q must be %q; stdout:\n%s", tt.token, tt.wantParentType, out)
			require.Equal(t, tt.token, clie2e.DryRunGet(out, "api.0.body.parent_node").String(),
				"parent_node must equal the presentation token; stdout:\n%s", out)
		})
	}
}
