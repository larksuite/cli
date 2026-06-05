// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/tidwall/gjson"
)

// CreateDriveFolder creates a Drive folder, optionally under a parent folder, and
// deletes it during parent cleanup.
func CreateDriveFolder(t *testing.T, parentT *testing.T, ctx context.Context, name string, defaultAs string, parentFolderToken string) string {
	t.Helper()

	if defaultAs == "" {
		defaultAs = "bot"
	}

	args := []string{"drive", "+create-folder", "--name", name}
	if parentFolderToken != "" {
		args = append(args, "--folder-token", parentFolderToken)
	}

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      args,
		DefaultAs: defaultAs,
	})
	if err != nil {
		t.Fatalf("create drive folder %q: %v", name, err)
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	folderToken := gjson.Get(result.Stdout, "data.folder_token").String()
	if folderToken == "" {
		t.Fatalf("drive folder token should not be empty, stdout:\n%s", result.Stdout)
	}

	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := DeleteDriveResourceAndVerify(cleanupCtx, folderToken, "folder", defaultAs)
		clie2e.ReportCleanupFailure(parentT, "delete drive folder "+folderToken, deleteResult, deleteErr)
	})

	return folderToken
}

// DeleteDriveResourceAndVerify deletes a Drive-backed resource, then polls
// drive meta until the token is either gone or no longer has an accessible URL.
// This prevents cleanup from looking successful when the delete command
// returned a suppressed not_found or partial API error but the resource still
// exists.
func DeleteDriveResourceAndVerify(ctx context.Context, token, docType, defaultAs string) (*clie2e.Result, error) {
	if defaultAs == "" {
		defaultAs = "bot"
	}

	deleteResult, deleteErr := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"drive", "+delete", "--file-token", token, "--type", docType, "--yes"},
		DefaultAs: defaultAs,
	}, clie2e.RetryOptions{})
	if deleteErr != nil || deleteResult == nil {
		return deleteResult, deleteErr
	}
	if deleteResult.ExitCode != 0 {
		deleted, verifyErr := IsDriveResourceDeleted(ctx, token, docType, defaultAs)
		if verifyErr != nil {
			return deleteResult, verifyErr
		}
		if deleted {
			deleteResult.ExitCode = 0
			return deleteResult, nil
		}
		return deleteResult, fmt.Errorf("drive resource %s/%s still exists after delete failed: exit=%d stdout=%s stderr=%s", docType, token, deleteResult.ExitCode, deleteResult.Stdout, deleteResult.Stderr)
	}
	if err := WaitDriveResourceDeleted(ctx, token, docType, defaultAs); err != nil {
		return deleteResult, err
	}
	return deleteResult, nil
}

func WaitDriveResourceDeleted(ctx context.Context, token, docType, defaultAs string) error {
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		deleted, err := IsDriveResourceDeleted(ctx, token, docType, defaultAs)
		if err != nil {
			return err
		}
		if deleted {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("drive resource %s/%s still exists after delete", docType, token)
		case <-ticker.C:
		}
	}
}

func IsDriveResourceDeleted(ctx context.Context, token, docType, defaultAs string) (bool, error) {
	if defaultAs == "" {
		defaultAs = "bot"
	}
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "post", "/open-apis/drive/v1/metas/batch_query"},
		DefaultAs: defaultAs,
		Data: map[string]any{
			"request_docs": []map[string]any{{
				"doc_token": token,
				"doc_type":  docType,
			}},
		},
	})
	if err != nil {
		return false, err
	}
	if result.ExitCode != 0 {
		combined := strings.ToLower(result.Stdout + "\n" + result.Stderr)
		if strings.Contains(combined, "not found") || strings.Contains(combined, "404") {
			return true, nil
		}
		return false, fmt.Errorf("verify drive resource %s/%s after delete: exit=%d stdout=%s stderr=%s", docType, token, result.ExitCode, result.Stdout, result.Stderr)
	}
	if !isDriveMetaQuerySuccessful(result.Stdout) {
		return false, fmt.Errorf("verify drive resource %s/%s after delete returned unsuccessful envelope: stdout=%s stderr=%s", docType, token, result.Stdout, result.Stderr)
	}
	metas := gjson.Get(result.Stdout, "data.metas").Array()
	if len(metas) == 0 {
		return true, nil
	}
	if docType != "folder" {
		allURLsCleared := true
		for _, meta := range metas {
			if meta.Get("url").String() != "" {
				allURLsCleared = false
				break
			}
		}
		if allURLsCleared {
			return true, nil
		}
	}
	return false, nil
}

func isDriveMetaQuerySuccessful(stdout string) bool {
	if ok := gjson.Get(stdout, "ok"); ok.Exists() {
		return ok.Bool()
	}
	if code := gjson.Get(stdout, "code"); code.Exists() {
		return code.Int() == 0
	}
	return false
}
