// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func assertMeetingScreenshotProblem(t *testing.T, err error, wantCategory errs.Category, wantSubtype errs.Subtype) {
	t.Helper()
	if err == nil {
		t.Fatal("expected typed error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if problem.Category != wantCategory || problem.Subtype != wantSubtype {
		t.Fatalf("problem = (%s, %s), want (%s, %s)", problem.Category, problem.Subtype, wantCategory, wantSubtype)
	}
}

func encodedMeetingScreenshotJPEG(t *testing.T) []byte {
	t.Helper()
	var imageBytes bytes.Buffer
	if err := jpeg.Encode(&imageBytes, image.NewGray(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatalf("encode JPEG fixture: %v", err)
	}
	return imageBytes.Bytes()
}

func TestVCMeetingScreenshot_FlagDescriptionsExplainInputs(t *testing.T) {
	flags := make(map[string]string, len(VCMeetingScreenshot.Flags))
	for _, flag := range VCMeetingScreenshot.Flags {
		flags[flag.Name] = flag.Desc
	}

	if got := flags["meeting-id"]; !strings.Contains(got, "long numeric meeting ID") ||
		!strings.Contains(got, "not the 9-digit meeting number") {
		t.Fatalf("--meeting-id help = %q", got)
	}
	if got := flags["output"]; !strings.Contains(got, "current working directory") ||
		!strings.Contains(got, "parent directories are created") {
		t.Fatalf("--output help = %q", got)
	}
	if got := flags["overwrite"]; !strings.Contains(got, "already exists") {
		t.Fatalf("--overwrite help = %q", got)
	}
}

func TestVCMeetingScreenshot_Validation(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingScreenshot, []string{
		"+meeting-screenshot", "--as", "user", "--meeting-id", "123456789",
	}, f, nil)
	assertMeetingScreenshotProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	if !strings.Contains(err.Error(), "long meeting_id") {
		t.Fatalf("error = %v, want long meeting_id validation", err)
	}

}

func TestVCMeetingScreenshot_SavesJPEGWithoutLogID(t *testing.T) {
	chdirForTest(t)
	image := encodedMeetingScreenshotJPEG(t)
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	stub := &httpmock.Stub{
		Method:  http.MethodPost,
		URL:     vcMeetingScreenshotAPIPath,
		RawBody: image,
		Headers: http.Header{
			"Content-Type": {"image/jpeg"},
			"X-Tt-Logid":   {"log-screenshot"},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, VCMeetingScreenshot, []string{
		"+meeting-screenshot", "--as", "user", "--meeting-id", "9876543210123", "--output", "shot.jpg",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run screenshot command: %v", err)
	}
	reg.Verify(t)
	if got, err := os.ReadFile("shot.jpg"); err != nil || string(got) != string(image) {
		t.Fatalf("saved screenshot = %v, %v; want %v", got, err, image)
	}
	var request map[string]string
	if err := json.Unmarshal(stub.CapturedBody, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if request["meeting_id"] != "9876543210123" {
		t.Fatalf("meeting_id = %q", request["meeting_id"])
	}
	if strings.Contains(stdout.String(), "log-screenshot") || strings.Contains(stdout.String(), `"log_id"`) {
		t.Fatalf("success output contains log_id: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "shot.jpg") {
		t.Fatalf("output = %s", stdout.String())
	}
	digest := sha256.Sum256(image)
	if !strings.Contains(stdout.String(), fmt.Sprintf("%x", digest)) {
		t.Fatalf("output is missing SHA-256: %s", stdout.String())
	}
}

func TestVCMeetingScreenshot_AcceptsJPEGContentTypeParameters(t *testing.T) {
	chdirForTest(t)
	image := encodedMeetingScreenshotJPEG(t)
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method:  http.MethodPost,
		URL:     vcMeetingScreenshotAPIPath,
		RawBody: image,
		Headers: http.Header{"Content-Type": {"image/jpeg; charset=binary"}},
	})

	err := mountAndRun(t, VCMeetingScreenshot, []string{
		"+meeting-screenshot", "--as", "user", "--meeting-id", "9876543210123", "--output", "shot.jpg",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run screenshot command: %v", err)
	}
	if got, readErr := os.ReadFile("shot.jpg"); readErr != nil || !bytes.Equal(got, image) {
		t.Fatalf("saved screenshot = %v, %v; want valid JPEG", got, readErr)
	}
}

func TestVCMeetingScreenshot_ClassifiesSuccessfulJSONBusinessError(t *testing.T) {
	chdirForTest(t)
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method:      http.MethodPost,
		URL:         vcMeetingScreenshotAPIPath,
		Body:        map[string]interface{}{"code": 121005, "msg": "not in meeting"},
		ContentType: "application/json; charset=utf-8",
		Headers: http.Header{
			"Content-Type":              {"application/json; charset=utf-8"},
			larkcore.HttpHeaderKeyLogId: {"log-json-error"},
		},
	})

	err := mountAndRun(t, VCMeetingScreenshot, []string{
		"+meeting-screenshot", "--as", "bot", "--meeting-id", "9876543210123", "--output", "shot.jpg",
	}, f, stdout)
	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) {
		t.Fatalf("run screenshot command error = %T %v, want *errs.PermissionError", err, err)
	}
	if permissionErr.Subtype != errs.SubtypePermissionDenied || permissionErr.Code != 121005 || permissionErr.LogID != "log-json-error" {
		t.Fatalf("problem = %+v, want authorization/permission_denied code 121005 with log id", permissionErr.Problem)
	}
	if _, statErr := os.Stat("shot.jpg"); !os.IsNotExist(statErr) {
		t.Fatalf("shot.jpg stat error = %v, want file not created", statErr)
	}
}

func TestVCMeetingScreenshot_ClassifiesHTTPErrorJSONBusinessError(t *testing.T) {
	chdirForTest(t)
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: http.MethodPost,
		URL:    vcMeetingScreenshotAPIPath,
		Status: http.StatusForbidden,
		Body:   map[string]interface{}{"code": 121005, "msg": "not in meeting"},
		Headers: http.Header{
			"Content-Type":              {"application/json; charset=utf-8"},
			larkcore.HttpHeaderKeyLogId: {"log-http-json-error"},
		},
	})

	err := mountAndRun(t, VCMeetingScreenshot, []string{
		"+meeting-screenshot", "--as", "bot", "--meeting-id", "9876543210123", "--output", "shot.jpg",
	}, f, stdout)
	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) {
		t.Fatalf("run screenshot command error = %T %v, want *errs.PermissionError", err, err)
	}
	if permissionErr.Subtype != errs.SubtypePermissionDenied || permissionErr.Code != 121005 || permissionErr.LogID != "log-http-json-error" {
		t.Fatalf("problem = %+v, want authorization/permission_denied code 121005 with log id", permissionErr.Problem)
	}
	if _, statErr := os.Stat("shot.jpg"); !os.IsNotExist(statErr) {
		t.Fatalf("shot.jpg stat error = %v, want file not created", statErr)
	}
}

func TestVCMeetingScreenshot_DefaultOutputIncludesMilliseconds(t *testing.T) {
	chdirForTest(t)
	originalNow := meetingScreenshotNow
	meetingScreenshotNow = func() time.Time { return time.Date(2026, 8, 18, 10, 11, 12, 123000000, time.UTC) }
	t.Cleanup(func() { meetingScreenshotNow = originalNow })

	image := encodedMeetingScreenshotJPEG(t)
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method:      http.MethodPost,
		URL:         vcMeetingScreenshotAPIPath,
		RawBody:     image,
		ContentType: "image/jpeg",
	})

	err := mountAndRun(t, VCMeetingScreenshot, []string{
		"+meeting-screenshot", "--as", "user", "--meeting-id", "9876543210123",
	}, f, stdout)
	if err != nil {
		t.Fatalf("run screenshot command: %v", err)
	}
	want := filepath.Join("meeting-screenshots", "9876543210123-20260818T101112.123Z.jpg")
	if got, readErr := os.ReadFile(want); readErr != nil || string(got) != string(image) {
		t.Fatalf("default screenshot = %v, %v; want %v", got, readErr, image)
	}
}

func TestVCMeetingScreenshot_RejectsInvalidJPEGWithoutReplacingExistingFile(t *testing.T) {
	chdirForTest(t)
	if err := os.WriteFile("shot.jpg", []byte("existing"), 0600); err != nil {
		t.Fatalf("create existing output: %v", err)
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method:      http.MethodPost,
		URL:         vcMeetingScreenshotAPIPath,
		RawBody:     []byte{0xff, 0xd8, 0x00, 0xff, 0xd9},
		ContentType: "image/jpeg",
	})

	err := mountAndRun(t, VCMeetingScreenshot, []string{
		"+meeting-screenshot", "--as", "user", "--meeting-id", "9876543210123", "--output", "shot.jpg", "--overwrite",
	}, f, stdout)
	assertMeetingScreenshotProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse)
	if !strings.Contains(err.Error(), "invalid screenshot JPEG") {
		t.Fatalf("error = %v, want JPEG validation", err)
	}
	if got, readErr := os.ReadFile("shot.jpg"); readErr != nil || string(got) != "existing" {
		t.Fatalf("existing output changed to %q, err=%v", got, readErr)
	}
}

func TestVCMeetingScreenshot_RequiresOverwriteForExistingFile(t *testing.T) {
	chdirForTest(t)
	if err := os.WriteFile("shot.jpg", []byte("existing"), 0600); err != nil {
		t.Fatalf("create existing output: %v", err)
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCMeetingScreenshot, []string{
		"+meeting-screenshot", "--as", "user", "--meeting-id", "9876543210123", "--output", "shot.jpg",
	}, f, stdout)
	assertMeetingScreenshotProblem(t, err, errs.CategoryValidation, errs.SubtypeFailedPrecondition)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if validationErr.Param != "--output" {
		t.Fatalf("param = %q, want --output", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "use --overwrite") {
		t.Fatalf("error = %v, want overwrite requirement", err)
	}
	if got, readErr := os.ReadFile("shot.jpg"); readErr != nil || string(got) != "existing" {
		t.Fatalf("existing output changed to %q, err=%v", got, readErr)
	}
}
