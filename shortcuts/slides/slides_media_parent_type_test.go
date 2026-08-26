// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"encoding/json"
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

// mustJSONArray renders XML pages as the JSON array --slides expects.
func mustJSONArray(t *testing.T, pages ...string) string {
	t.Helper()

	encoded, err := json.Marshal(pages)
	if err != nil {
		t.Fatalf("marshal --slides: %v", err)
	}
	return string(encoded)
}

// uploadStepBody finds the planned upload_all call and returns its body. Wiki
// refs put a get_node step first, so the upload is not at a fixed index.
func uploadStepBody(t *testing.T, steps []map[string]interface{}) map[string]interface{} {
	t.Helper()

	for i, step := range steps {
		if step["url"] == "/open-apis/drive/v1/medias/upload_all" {
			body, ok := step["body"].(map[string]interface{})
			if !ok {
				t.Fatalf("api[%d].body = %#v, want an object", i, step["body"])
			}
			return body
		}
	}
	t.Fatalf("no upload_all step planned: %#v", steps)
	return nil
}

// TestSlidesDryRunParentType pins the preview-time mapping, which differs from
// the Execute-time one in exactly one way: a wiki ref is native by construction.
//
// resolvePresentationID rejects any wiki node whose obj_type is not "slides"
// (helpers.go), and an imported office deck sits in drive as a "file" node, so it
// cannot survive that gate to reach an upload. A wiki preview may therefore say
// slide_file about a token it has never seen.
//
// The load-bearing case is the last one. A wiki node token can itself be
// office-shaped — it is a different token in a different namespace than the deck
// it points at — so classifying ref.Token would be wrong for a wiki ref even
// though it is right for every other kind. That is what stops a future
// "simplification" to slidesMediaParentType(ref.Token).
func TestSlidesDryRunParentType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ref  presentationRef
		want string
	}{
		{"native token", presentationRef{Kind: "slides", Token: "pptcnABC123"}, slideFileParentType},
		{"imported office token", presentationRef{Kind: "slides", Token: "aaaaOaaaaFaaaaLaaaa0aaaaXaaa"}, officeSlideFileParentType},
		{"legacy office prefix", presentationRef{Kind: "slides", Token: "fake_office_abc123"}, officeSlideFileParentType},
		{"wiki node token", presentationRef{Kind: "wiki", Token: "wikcnABC123"}, slideFileParentType},
		{"office-shaped wiki node token", presentationRef{Kind: "wiki", Token: "aaaaOaaaaFaaaaLaaaa0aaaaXaaa"}, slideFileParentType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := slidesDryRunParentType(tc.ref); got != tc.want {
				t.Fatalf("slidesDryRunParentType(%+v) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

// TestSlidesDryRunPlaceholderNodeParentType pins the end-to-end preview for every
// surface whose parent_node is a placeholder, which is where the parent_type has
// no token to be read off of.
//
// The placeholder must never be the thing that decides the type. Feeding it to
// slidesMediaParentType yields slide_file, which is the right answer here — but
// by accident, because a placeholder matches no office token shape. That leaves
// the preview hostage to the placeholder's spelling and to every rule later added
// to isOfficePresentation, so these cases pin the value while the production code
// asserts it directly instead of deriving it.
func TestSlidesDryRunPlaceholderNodeParentType(t *testing.T) {
	const wikiURL = "https://example.feishu.cn/wiki/wikcnDryRunProbe123"
	slideXML := `<slide xmlns="https://www.larkoffice.com/sml/2.0"><data>` +
		`<img src="@./chart.png" topLeftX="10" topLeftY="10" width="100" height="100"/>` +
		`</data></slide>`

	cases := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		wantNode string
	}{
		{
			name:     "media-upload via wiki",
			shortcut: SlidesMediaUpload,
			args: []string{"+media-upload", "--presentation", wikiURL,
				"--file", "chart.png", "--dry-run", "--as", "user"},
			wantNode: unresolvedSlidesTokenPlaceholder,
		},
		{
			name:     "add-slide via wiki",
			shortcut: SlidesAddSlide,
			args: []string{"+add-slide", "--presentation", wikiURL,
				"--slide", slideXML, "--dry-run", "--as", "user"},
			wantNode: unresolvedSlidesTokenPlaceholder,
		},
		{
			name:     "update-slide via wiki",
			shortcut: SlidesUpdateSlide,
			args: []string{"+update-slide", "--presentation", wikiURL, "--slide-id", "s1",
				"--content", slideXML, "--dry-run", "--as", "user"},
			wantNode: unresolvedSlidesTokenPlaceholder,
		},
		{
			// +create has no --presentation flag at all: its node is minted by
			// the create call earlier in the same orchestration, so the deck is
			// always one the API just made and never an imported office file.
			name:     "create mints its own deck",
			shortcut: SlidesCreate,
			args: []string{"+create", "--title", "probe",
				"--slides", mustJSONArray(t, slideXML), "--dry-run", "--as", "user"},
			wantNode: "<xml_presentation_id>",
		},
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

			body := uploadStepBody(t, decodeShortcutDryRunAPI(t, stdout))
			if body["parent_node"] != tc.wantNode {
				t.Fatalf("parent_node = %v, want %q", body["parent_node"], tc.wantNode)
			}
			if body["parent_type"] != slideFileParentType {
				t.Fatalf("parent_type = %v, want %q", body["parent_type"], slideFileParentType)
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
