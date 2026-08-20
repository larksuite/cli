// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestSlidesMediaParentType pins the token→parent_type mapping every slides
// image-upload entry point funnels through. Native presentation tokens use
// "slide_file"; imported "office" presentations carry either a legacy synthetic
// prefix or the interleaved "OFL0X" marker and must upload with
// "office_slide_file".
//
// The negative cases matter as much as the positive ones. A false positive —
// a native token read as office — still uploads successfully, because the drive
// backend does not validate that parent_node actually names an office file; the
// damage only shows up later as an image that will not render, far from its
// cause. So the marker check is pinned at its exact length and position rather
// than left to a looser "contains OFL0X" reading.
func TestSlidesMediaParentType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"native presentation token", "pptcnABC123", slideFileParentType},
		{"empty token", "", slideFileParentType},
		{"dry-run wiki placeholder stays native", "<resolved_slides_token>", slideFileParentType},
		{"fake_office imported token", "fake_office_abc123", officeSlideFileParentType},
		{"fake_office token, only the prefix", fakeOfficePrefix, officeSlideFileParentType},
		{"local_office imported token", "local_office_abc123", officeSlideFileParentType},
		{"local_office token, only the prefix", localOfficePrefix, officeSlideFileParentType},
		{"interleaved OFL0X office token", "aaaaOaaaaFaaaaLaaaa0aaaaXaaa", officeSlideFileParentType},
		{"interleaved pptcn native token", "abcdpefghpijkltmnopcqrstnuv", slideFileParentType},
		{"interleaved shtcn token", "abcdsefghhijkltmnopcqrstnuv", slideFileParentType},
		{"interleaved OFL0X marker with short length", "aaaaOaaaaFaaaaLaaaa0aaaaXaa", slideFileParentType},
		{"interleaved OFL0X marker with long length", "aaaaOaaaaFaaaaLaaaa0aaaaXaaaa", slideFileParentType},
		{"fake_office prefix mid-string is not matched", "pptfake_office_abc", slideFileParentType},
		{"local_office prefix mid-string is not matched", "pptlocal_office_abc", slideFileParentType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := slidesMediaParentType(tc.token); got != tc.want {
				t.Fatalf("slidesMediaParentType(%q) = %q, want %q", tc.token, got, tc.want)
			}
		})
	}
}

// TestSlidesMediaUploadExecuteParentType exercises the Execute path the dry-run
// tests cannot reach, asserting the parent_type that actually goes out on the
// multipart wire is derived from the presentation token.
func TestSlidesMediaUploadExecuteParentType(t *testing.T) {
	cases := []struct {
		name           string
		token          string
		wantParentType string
	}{
		{"native presentation", "pptcnTOK123", slideFileParentType},
		{"fake_office imported deck", "fake_office_abc123", officeSlideFileParentType},
		{"local_office imported deck", "local_office_abc123", officeSlideFileParentType},
		{"interleaved OFL0X imported deck", "aaaaOaaaaFaaaaLaaaa0aaaaXaaa", officeSlideFileParentType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withSlidesTestWorkingDir(t, t.TempDir())
			if err := os.WriteFile("img.png", []byte("png-bytes"), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			f, stdout, _, reg := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/medias/upload_all",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"file_token": "file_tok_xyz"},
				},
			}
			reg.Register(stub)

			err := runSlidesShortcut(t, f, stdout, SlidesMediaUpload, []string{
				"+media-upload",
				"--file", "img.png",
				"--presentation", tc.token,
				"--as", "user",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			body := decodeMultipartBody(t, stub)
			if got := body.Fields["parent_type"]; got != tc.wantParentType {
				t.Fatalf("parent_type = %q, want %q", got, tc.wantParentType)
			}
			// parent_node must stay the token verbatim: the parent_type switch
			// selects a namespace, it does not rewrite what is being pointed at.
			if got := body.Fields["parent_node"]; got != tc.token {
				t.Fatalf("parent_node = %q, want %q", got, tc.token)
			}
		})
	}
}

// TestSlidesImagePlaceholderDryRunOfficeParentType covers the @path placeholder
// pipeline, which is the other way a file reaches upload_all: +add-slide and
// +update-slide upload the images referenced by the page XML before posting it.
// Those previews are built by appendSlidesUploadDryRun, so they must show the
// office parent_type too — a preview that says slide_file for a deck the
// Execute path will upload as office_slide_file is a preview of a different
// request.
func TestSlidesImagePlaceholderDryRunOfficeParentType(t *testing.T) {
	const officeToken = "aaaaOaaaaFaaaaLaaaa0aaaaXaaa"
	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	cases := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{"add-slide", SlidesAddSlide, []string{
			"+add-slide", "--presentation", officeToken, "--slide", slideXML, "--dry-run", "--as", "user",
		}},
		{"update-slide", SlidesUpdateSlide, []string{
			"+update-slide", "--presentation", officeToken, "--slide-id", "s1",
			"--content", slideXML, "--dry-run", "--as", "user",
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "chart.png"), []byte("png-bytes"), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			withSlidesTestWorkingDir(t, dir)

			f, stdout, _, _ := cmdutil.TestFactory(t, slidesTestConfig(t, ""))
			if err := runSlidesShortcut(t, f, stdout, tc.shortcut, tc.args); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			steps := decodeShortcutDryRunAPI(t, stdout)
			upload := assertDryRunStep(t, steps, 0, "POST", "/open-apis/drive/v1/medias/upload_all")
			uploadBody, _ := upload["body"].(map[string]interface{})
			if uploadBody["parent_type"] != officeSlideFileParentType {
				t.Fatalf("upload parent_type = %v, want %q", uploadBody["parent_type"], officeSlideFileParentType)
			}
			if uploadBody["parent_node"] != officeToken {
				t.Fatalf("upload parent_node = %v, want %q", uploadBody["parent_node"], officeToken)
			}
		})
	}
}
