// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseFormSubmitDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-submit",
			"--share-token", "shrXXXX",
			"--json", `{"fields":{"Rating":5}}`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	output := strings.TrimSpace(result.Stdout)
	assert.Contains(t, output, "/open-apis/base/v3/bases/tables/forms/submit")
	assert.Contains(t, output, `"share_token"`)
	assert.Contains(t, output, "shrXXXX")
	assert.Contains(t, output, `"method": "POST"`)
}

func TestBaseFormSubmitSignatureAttachmentDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-submit",
			"--share-token", "shrXXXX",
			"--base-token", "app_x",
			"--json", `{"fields":{"作品链接":"https://example.com"},"attachments":{"本人签字":["./signature.png"]}}`,
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	// The signature image is uploaded to the form's temporary attachment point first ...
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "/open-apis/drive/v1/medias/upload_all", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "bitable_tmp_point", clie2e.DryRunGet(out, "api.0.body.parent_type").String(), out)
	require.Equal(t, "app_x", clie2e.DryRunGet(out, "api.0.body.parent_node").String(), out)
	require.Equal(t, "signature.png", clie2e.DryRunGet(out, "api.0.body.file_name").String(), out)
	require.Equal(t, "@./signature.png", clie2e.DryRunGet(out, "api.0.body.file").String(), out)
	require.JSONEq(t, `{"share_token":"shrXXXX"}`, clie2e.DryRunGet(out, "api.0.body.extra").String(), out)

	// ... and only then is the form submitted, with the hyperlink value kept as a URL string.
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.1.method").String(), out)
	require.Equal(t, "/open-apis/base/v3/bases/tables/forms/submit", clie2e.DryRunGet(out, "api.1.url").String(), out)
	require.Equal(t, "https://example.com", clie2e.DryRunGet(out, "api.1.body.content.作品链接").String(), out)
	require.False(t, clie2e.DryRunGet(out, "api.2").Exists(), out)
}
