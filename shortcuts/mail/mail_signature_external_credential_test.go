// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package mail

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/runtimeplan"
)

type mailManagedFilePolicy struct{}

func (mailManagedFilePolicy) ValidateRemoteFile(rawURL string) error {
	if strings.HasPrefix(rawURL, "https://proxy.example/lark-cli/v1/files/") {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"managed runtime requires an opaque file handle")
}

func (mailManagedFilePolicy) UsesManagedFilePlane() bool { return true }

func TestMailSignatureDetailProxyRejectsRawImageDownloadURL(t *testing.T) {
	factory, stdout, _, registry := mailShortcutTestFactory(t)
	cmdutil.TestSetRuntimePlan(t, factory, runtimeplan.New(runtimeplan.Options{
		RemoteFiles: mailManagedFilePolicy{},
	}))

	const leakedURL = "https://storage.example/raw-presigned?signature=secret"
	registry.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/proxy-signature-test@example.com/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": []interface{}{map[string]interface{}{
					"id": "sig-proxy-test", "name": "Proxy", "signature_type": "USER",
					"images": []interface{}{map[string]interface{}{
						"image_name": "logo.png", "download_url": leakedURL,
					}},
				}},
				"usages": []interface{}{},
			},
		},
	})

	err := runMountedMailShortcut(t, MailSignature, []string{
		"+signature", "--from", "proxy-signature-test@example.com", "--detail", "sig-proxy-test", "--as", "user",
	}, factory, stdout)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("err = %T %v, want validation/failed_precondition", err, err)
	}
	if strings.Contains(stdout.String(), leakedURL) {
		t.Fatalf("raw image download URL leaked to stdout: %s", stdout.String())
	}
}
