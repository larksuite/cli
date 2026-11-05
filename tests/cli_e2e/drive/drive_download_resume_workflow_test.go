// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_DownloadResumeWorkflow uploads its own multipart fixture and cleans
// up both remote resources. It verifies a complete download, then seeds a
// partial from those verified bytes and resumes in a separate CLI process.
// Interrupted invocations and exact Range headers have deterministic unit tests.
func TestDrive_DownloadResumeWorkflow(t *testing.T) {
	if os.Getenv("LARK_DRIVE_DOWNLOAD_RESUME_E2E") != "1" {
		t.Skip("set LARK_DRIVE_DOWNLOAD_RESUME_E2E=1 for the 129 MiB Drive download/resume workflow")
	}
	brand := string(core.ParseBrand(os.Getenv("LARKSUITE_CLI_BRAND")))
	t.Setenv("LARKSUITE_CLI_BRAND", brand)
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	t.Cleanup(cancel)
	folder := createDriveFolder(t, t, ctx, "lark-cli-e2e-drive-resume-"+clie2e.GenerateSuffix(), "")
	work := t.TempDir()
	const size int64 = 129 << 20
	const offset int64 = 1 << 20
	source, err := os.Create(filepath.Join(work, "source.bin"))
	require.NoError(t, err)
	hash := sha256.New()
	_, copyErr := io.CopyN(io.MultiWriter(source, hash), rand.Reader, size)
	closeErr := source.Close()
	require.NoError(t, copyErr)
	require.NoError(t, closeErr)
	wantHash := hash.Sum(nil)
	upload, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"drive", "+upload", "--file", "source.bin", "--folder-token", folder}, WorkDir: work, DefaultAs: "bot"})
	require.NoError(t, err)
	upload.AssertExitCode(t, 0)
	token := gjson.Get(upload.Stdout, "data.file_token").String()
	require.NotEmpty(t, token)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, err := DeleteDriveResourceAndVerify(cleanupCtx, token, "file", "bot")
		clie2e.ReportCleanupFailure(t, "delete resume fixture", result, err)
	})
	metadata, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"drive", "metas", "batch_query"}, Data: map[string]interface{}{"request_docs": []interface{}{map[string]interface{}{"doc_token": token, "doc_type": "file"}}}, DefaultAs: "bot"})
	require.NoError(t, err)
	metadata.AssertExitCode(t, 0)
	revision := gjson.Get(metadata.Stdout, "data.metas.0.latest_modify_time").String()
	require.NotEmpty(t, revision, "fixture requires readable latest_modify_time")
	args := []string{"drive", "+download", "--file-token", token, "--output", "out.bin"}
	download, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, WorkDir: work, DefaultAs: "bot"})
	require.NoError(t, err)
	download.AssertExitCode(t, 0)
	verifyHash := func() {
		t.Helper()
		file, err := os.Open(filepath.Join(work, "out.bin"))
		require.NoError(t, err)
		h := sha256.New()
		n, readErr := io.Copy(h, file)
		closeErr := file.Close()
		require.NoError(t, readErr)
		require.NoError(t, closeErr)
		require.Equal(t, size, n)
		require.Equal(t, wantHash, h.Sum(nil))
	}
	verifyHash()
	require.NoError(t, os.Rename(filepath.Join(work, "out.bin"), filepath.Join(work, "out.bin.partial")))
	require.NoError(t, os.Truncate(filepath.Join(work, "out.bin.partial"), offset))
	cp, err := json.Marshal(map[string]interface{}{"base_url": core.ResolveOpenBaseURL(core.LarkBrand(brand)), "file_token": token, "size": size, "latest_modify_time": revision})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(work, "out.bin.partial.meta"), cp, 0600))
	resume, err := clie2e.RunCmd(ctx, clie2e.Request{Args: append(args, "--continue"), WorkDir: work, DefaultAs: "bot", Env: map[string]string{"LARKSUITE_CLI_BRAND": brand}})
	require.NoError(t, err)
	resume.AssertExitCode(t, 0)
	require.True(t, gjson.Get(resume.Stdout, "data.resumed").Bool(), "resume must reuse the seeded prefix")
	verifyHash()
	for _, path := range []string{"out.bin.partial", "out.bin.partial.meta"} {
		_, err := os.Stat(filepath.Join(work, path))
		require.True(t, os.IsNotExist(err), "artifact remains: %s", path)
	}
}
