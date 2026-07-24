// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestIsLocalImageSrc(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"./images/pic.png", true},
		{"images/pic.png", true},
		{"../assets/a.png", true},
		{"/Users/me/Desktop/a.png", true},
		{`C:\Users\me\a.png`, true},
		{"file:///Users/me/a.png", true},
		{"图片和附件/测试图片.png", true},
		{"https://example.com/a.png", false},
		{"http://example.com/a.png", false},
		{"HTTPS://EXAMPLE.com/a.png", false},
		{"data:image/png;base64,iVBOR", false},
		{"ftp://host/a.png", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isLocalImageSrc(c.src); got != c.want {
			t.Errorf("isLocalImageSrc(%q) = %v, want %v", c.src, got, c.want)
		}
	}
}

func TestLocalImagePath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"images/pic.png", "images/pic.png"},
		{"images/my%20pic.png", "images/my pic.png"},
		{"file:///Users/me/a.png", "/Users/me/a.png"},
	}
	for _, c := range cases {
		if got := localImagePath(c.in); got != c.want {
			t.Errorf("localImagePath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBuildCalendarImagePreviewURL guards the contract the OpenAPI service
// relies on: a Lark host (so token extraction triggers) whose final path
// segment is exactly the uploaded file token.
func TestBuildCalendarImagePreviewURL(t *testing.T) {
	for _, tc := range []struct {
		brand    core.LarkBrand
		hostFrag string
	}{
		{core.BrandFeishu, "larkoffice"},
		{core.BrandLark, "larksuite"},
	} {
		raw := buildCalendarImagePreviewURL(tc.brand, "boxcnTOKEN123", 416, 306, 142568)
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("built URL not parseable: %v", err)
		}
		if !strings.Contains(u.Host, tc.hostFrag) {
			t.Errorf("brand %s host = %q, want fragment %q", tc.brand, u.Host, tc.hostFrag)
		}
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if last := segs[len(segs)-1]; last != "boxcnTOKEN123" {
			t.Errorf("last path segment = %q, want token", last)
		}
		q := u.Query()
		if q.Get("im_w") != "416" || q.Get("im_h") != "306" || q.Get("im_size") != "142568" {
			t.Errorf("dimension params missing: im_w=%q im_h=%q im_size=%q", q.Get("im_w"), q.Get("im_h"), q.Get("im_size"))
		}
	}

	// With unknown dimensions the helper params are omitted entirely.
	raw := buildCalendarImagePreviewURL(core.BrandFeishu, "boxcnTOKEN123", 0, 0, 0)
	if strings.Contains(raw, "im_w") || strings.Contains(raw, "im_size") {
		t.Errorf("expected no dimension params for unknown size, got %q", raw)
	}
}

// TestUploadLocalDescriptionImages_RemoteUntouched verifies remote/data images
// pass through unchanged and never trigger an upload (runtime unused → nil).
func TestUploadLocalDescriptionImages_RemoteUntouched(t *testing.T) {
	md := "text ![a](https://example.com/a.png) more ![b](data:image/png;base64,xx)"
	got, changed, err := uploadLocalDescriptionImages(nil, "cal", md)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if changed {
		t.Errorf("changed = true, want false")
	}
	if got != md {
		t.Errorf("markdown mutated: %q", got)
	}
}

// TestCreate_UploadsLocalDescriptionImage runs +create with a local image path,
// mocks the drive upload, and asserts the create body's description_rich carries
// the uploaded token (not the local path).
func TestCreate_UploadsLocalDescriptionImage(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)
	if err := os.WriteFile(filepath.Join(dir, "pic.png"), []byte("PNGDATA"), 0600); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"file_token": "boxcnTOKEN123"}},
	}
	reg.Register(uploadStub)

	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{
			"event": map[string]interface{}{
				"event_id":   "evt_001",
				"summary":    "Pic",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			},
		}},
	}
	reg.Register(createStub)

	runErr := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Pic",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--description-rich", "![pic](./pic.png)",
		"--as", "bot",
	}, f, stdout)
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}

	if uploadStub.CapturedBody == nil {
		t.Fatalf("expected drive upload to be called")
	}
	if createStub.CapturedBody == nil {
		t.Fatalf("expected create event to be called")
	}
	var body map[string]interface{}
	if err := json.Unmarshal(createStub.CapturedBody, &body); err != nil {
		t.Fatalf("create body unmarshal: %v", err)
	}
	dr, _ := body["description_rich"].(string)
	if !strings.Contains(dr, "boxcnTOKEN123") {
		t.Fatalf("description_rich should contain uploaded token, got %q", dr)
	}
	if strings.Contains(dr, "./pic.png") {
		t.Fatalf("local path should be rewritten away, got %q", dr)
	}
}

// TestCreate_LocalImageCarriesDimensions verifies a real decodable image's
// intrinsic width/height and byte size are appended to the rewritten drive URL
// (so the facade can populate originalWidth/originalHeight and the client can
// render the image inline).
func TestCreate_LocalImageCarriesDimensions(t *testing.T) {
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 5, 7))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pic.png"), buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"file_token": "boxcnTOKEN123"}},
	})
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test123/events",
		Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{
			"event": map[string]interface{}{
				"event_id":   "evt_001",
				"summary":    "Pic",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			},
		}},
	}
	reg.Register(createStub)

	runErr := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Pic",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--description-rich", "![pic](./pic.png)",
		"--as", "bot",
	}, f, stdout)
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(createStub.CapturedBody, &body); err != nil {
		t.Fatalf("create body unmarshal: %v", err)
	}
	dr, _ := body["description_rich"].(string)
	if !strings.Contains(dr, "im_w=5") || !strings.Contains(dr, "im_h=7") {
		t.Fatalf("description_rich should carry image dimensions, got %q", dr)
	}
	if !strings.Contains(dr, "im_size=") {
		t.Fatalf("description_rich should carry image byte size, got %q", dr)
	}
}

// TestCreate_LocalImageAbsolutePathRejected verifies an out-of-cwd absolute path
// yields a typed --description-rich validation error before any API call.
func TestCreate_LocalImageAbsolutePathRejected(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())

	runErr := mountAndRun(t, CalendarCreate, []string{
		"+create",
		"--summary", "Pic",
		"--start", "2025-03-21T00:00:00+08:00",
		"--end", "2025-03-21T01:00:00+08:00",
		"--calendar-id", "cal_test123",
		"--description-rich", "![p](/etc/hosts)",
		"--as", "bot",
	}, f, stdout)
	if runErr == nil {
		t.Fatalf("expected error for absolute image path")
	}
	var ve *errs.ValidationError
	if !errors.As(runErr, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", runErr, runErr)
	}
	if ve.Param != "--description-rich" {
		t.Errorf("param = %q, want --description-rich", ve.Param)
	}
}
