// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDriveDownloadContinuePreservesArtifactsOnMetadataFailure(t *testing.T) {
	cause := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "metadata timeout").WithRetryable()
	for _, phase := range []string{"before", "after append", "complete partial"} {
		for _, failure := range []string{"timeout", "permission", "missing revision"} {
			t.Run(phase+"/"+failure, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				withDriveDownloadPartSize(t, 4)
				f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
				f.HttpClient = func() (*http.Client, error) {
					return &http.Client{Transport: driveRoundTripFunc(func(req *http.Request) (*http.Response, error) {
						resp, err := reg.RoundTrip(req)
						if err == nil && req.Method == http.MethodGet && resp.StatusCode == http.StatusPartialContent {
							resp.ContentLength = -1
						}
						return resp, err
					})}, nil
				}
				withDriveWorkingDir(t, t.TempDir())
				registerDriveDownloadViewAuth(reg, "file_resume", true)
				content := []byte("abcdefghij")
				prefix := content[:4]
				if phase == "complete partial" {
					prefix = content
				}
				if err := os.WriteFile("out.bin.partial", prefix, 0600); err != nil {
					t.Fatal(err)
				}
				driveDownloadWriteTestCheckpoint(t, "out.bin.partial.meta", "file_resume", int64(len(content)), "1792558577")
				checkpoint, err := os.ReadFile("out.bin.partial.meta")
				if err != nil {
					t.Fatal(err)
				}
				if phase != "before" {
					registerDriveDownloadRevision(reg, "file_resume", "1792558577")
					registerDriveDownloadPart(reg, "file_resume", content, 0, 0)
					if phase == "after append" {
						registerDriveDownloadPart(reg, "file_resume", content, 4, 7)
						registerDriveDownloadPart(reg, "file_resume", content, 8, 9)
					}
				}
				switch failure {
				case "timeout":
					reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/drive/v1/metas/batch_query", Error: cause})
				case "permission":
					reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/drive/v1/metas/batch_query", Body: map[string]interface{}{"code": 99991679, "msg": "missing metadata scope"}})
				case "missing revision":
					registerDriveDownloadRevision(reg, "file_resume", nil)
				}
				err = mountAndRunDrive(t, DriveDownload, []string{"+download", "--file-token", "file_resume", "--output", "out.bin", "--continue", "--as", "bot"}, f, stdout)
				problem, ok := errs.ProblemOf(err)
				if !ok {
					t.Fatalf("expected typed failure, got %v", err)
				}
				if failure == "timeout" && (!problem.Retryable || !errors.Is(err, cause)) {
					t.Fatalf("lost retryability or cause: %v", err)
				}
				if failure == "permission" && problem.Category != errs.CategoryAuthorization {
					t.Fatalf("lost permission failure: %+v", problem)
				}
				// An absent field does not prove a new revision. Retain the bytes even
				// when the metadata request itself succeeded.
				partial, readErr := os.ReadFile("out.bin.partial")
				want := prefix
				if phase == "after append" {
					want = content
				}
				if readErr != nil || !bytes.Equal(partial, want) {
					t.Fatalf("partial=%q err=%v want=%q", partial, readErr, want)
				}
				savedCheckpoint, readErr := os.ReadFile("out.bin.partial.meta")
				if readErr != nil || !bytes.Equal(savedCheckpoint, checkpoint) {
					t.Fatalf("checkpoint changed: %s err=%v", savedCheckpoint, readErr)
				}
				if _, statErr := os.Stat("out.bin"); !os.IsNotExist(statErr) {
					t.Fatalf("published unverified output: %v", statErr)
				}
				if stdout.Len() != 0 {
					t.Fatalf("success output on failure: %s", stdout)
				}
				reg.Verify(t)
			})
		}
	}
}

type driveDownloadReadFailure struct{ err error }

func (r driveDownloadReadFailure) Read([]byte) (int, error) { return 0, r.err }

// Two independent invocations exercise a checkpoint written by the actual
// first download, not a synthetic successful-resume fixture.
func TestDriveDownloadContinueInterruptedInvocationResumesWithoutETag(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	withDriveDownloadPartSize(t, 4)
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	withDriveWorkingDir(t, t.TempDir())
	content := []byte("abcdefghij")
	registerDriveDownloadViewAuth(reg, "file_resume", true)
	revision := registerDriveDownloadRevision(reg, "file_resume", "1792558577")
	revision.Reusable = true
	failure := errs.NewNetworkError(errs.SubtypeNetworkProtocol, "fixture interrupted")
	first := true
	var ranges []string
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: driveRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/open-apis/drive/v1/files/file_resume/download" {
				return reg.RoundTrip(req)
			}
			if req.Header.Get("If-Range") != "" {
				t.Fatalf("timestamp sent as HTTP validator: %s", req.Header.Get("If-Range"))
			}
			requested := req.Header.Get("Range")
			ranges = append(ranges, requested)
			var start, end int
			if _, err := fmt.Sscanf(requested, "bytes=%d-%d", &start, &end); err != nil {
				t.Fatal(err)
			}
			if end >= len(content) {
				end = len(content) - 1
			}
			var body io.Reader = bytes.NewReader(content[start : end+1])
			if first {
				first = false
				body = io.MultiReader(body, driveDownloadReadFailure{failure})
			}
			return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, len(content))}}, Body: io.NopCloser(body), ContentLength: int64(end - start + 1)}, nil
		})}, nil
	}
	args := []string{"+download", "--file-token", "file_resume", "--output", "out.bin", "--continue", "--as", "bot"}
	err := mountAndRunDrive(t, DriveDownload, args, f, stdout)
	if !errors.Is(err, failure) {
		t.Fatalf("first invocation: %v", err)
	}
	partial, err := os.ReadFile("out.bin.partial")
	if err != nil || !bytes.Equal(partial, content[:4]) {
		t.Fatalf("partial=%q err=%v", partial, err)
	}
	raw, err := os.ReadFile("out.bin.partial.meta")
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint map[string]interface{}
	if err := json.Unmarshal(raw, &checkpoint); err != nil {
		t.Fatal(err)
	}
	if checkpoint["file_token"] != "file_resume" || checkpoint["latest_modify_time"] != "1792558577" || checkpoint["size"] != float64(10) || checkpoint["base_url"] != core.ResolveOpenBaseURL(core.BrandFeishu) {
		t.Fatalf("checkpoint=%s", raw)
	}
	registerDriveDownloadViewAuth(reg, "file_resume", true)
	err = mountAndRunDrive(t, DriveDownload, args, f, stdout)
	if err != nil {
		t.Fatalf("second invocation: %v", err)
	}
	data, err := os.ReadFile("out.bin")
	if err != nil || !bytes.Equal(data, content) {
		t.Fatalf("output=%q err=%v", data, err)
	}
	if fmt.Sprint(ranges) != "[bytes=0-3 bytes=0-0 bytes=4-7 bytes=8-9]" {
		t.Fatalf("requests=%v", ranges)
	}
	for _, path := range []string{"out.bin.partial", "out.bin.partial.meta"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("artifact remains %s: %v", path, err)
		}
	}
	reg.Verify(t)
}

func TestDriveDownloadContinueDiscardsConfirmedRevisionChange(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(fmt.Sprint(complete), func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			withDriveDownloadPartSize(t, 4)
			f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
			f.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: driveRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					resp, err := reg.RoundTrip(req)
					if err == nil && req.Method == http.MethodGet && resp.StatusCode == http.StatusPartialContent {
						resp.ContentLength = -1
					}
					return resp, err
				})}, nil
			}
			withDriveWorkingDir(t, t.TempDir())
			content := []byte("abcdefghij")
			prefix := content[:4]
			if complete {
				prefix = content
			}
			if err := os.WriteFile("out.bin.partial", prefix, 0600); err != nil {
				t.Fatal(err)
			}
			driveDownloadWriteTestCheckpoint(t, "out.bin.partial.meta", "file_resume", 10, "1792558577")
			registerDriveDownloadViewAuth(reg, "file_resume", true)
			registerDriveDownloadRevision(reg, "file_resume", "1792558577")
			registerDriveDownloadPart(reg, "file_resume", content, 0, 0)
			if !complete {
				registerDriveDownloadPart(reg, "file_resume", content, 4, 7)
				registerDriveDownloadPart(reg, "file_resume", content, 8, 9)
			}
			registerDriveDownloadRevision(reg, "file_resume", "1792558578")
			err := mountAndRunDrive(t, DriveDownload, []string{"+download", "--file-token", "file_resume", "--output", "out.bin", "--continue", "--as", "bot"}, f, stdout)
			p, ok := errs.ProblemOf(err)
			if !ok || p.Subtype != errs.SubtypeNetworkRepresentationChanged || !p.Retryable {
				t.Fatalf("err=%v", err)
			}
			for _, path := range []string{"out.bin", "out.bin.partial", "out.bin.partial.meta"} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("invalid artifact %s: %v", path, err)
				}
			}
			reg.Verify(t)
		})
	}
}

func TestDriveDownloadContinueRejectsForeignOrLegacyCheckpoint(t *testing.T) {
	for _, field := range []string{"file_token", "base_url", "latest_modify_time", "size", "legacy"} {
		t.Run(field, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			content := driveDownloadRangePayload()
			var ranges []string
			f, _ := driveDownloadRangeFactory(t, "file_resume", content, &ranges)
			withDriveWorkingDir(t, t.TempDir())
			if err := os.WriteFile("out.bin.partial", []byte("WRONG PREFIX"), 0600); err != nil {
				t.Fatal(err)
			}
			cp := map[string]interface{}{"file_token": "file_resume", "base_url": core.ResolveOpenBaseURL(core.BrandFeishu), "size": len(content), "latest_modify_time": "1792558577"}
			switch field {
			case "size":
				cp[field] = len(content) + 1
			case "legacy":
				cp = map[string]interface{}{"size": len(content), "etag": `"v1"`}
			default:
				cp[field] = "different"
			}
			raw, err := json.Marshal(cp)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("out.bin.partial.meta", raw, 0600); err != nil {
				t.Fatal(err)
			}
			if err := mountAndRunDrive(t, DriveDownload, []string{"+download", "--file-token", "file_resume", "--output", "out.bin", "--continue", "--as", "bot"}, f, nil); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile("out.bin")
			if err != nil || !bytes.Equal(data, content) {
				t.Fatalf("output mismatch err=%v", err)
			}
			want := fmt.Sprintf("bytes=0-%d", driveDownloadPartSize-1)
			if field == "size" {
				if len(ranges) != 2 || ranges[0] != "bytes=0-0" || ranges[1] != want {
					t.Fatalf("requests=%v", ranges)
				}
			} else if len(ranges) != 1 || ranges[0] != want {
				t.Fatalf("requests=%v", ranges)
			}
		})
	}
}
