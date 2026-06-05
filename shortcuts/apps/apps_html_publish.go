// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsHTMLPublish packs --path as tar.gz and uploads + publishes via one multipart POST.
var AppsHTMLPublish = common.Shortcut{
	Service:     appsService,
	Command:     "+html-publish",
	Description: "Publish HTML to a Miaoda app (single multipart POST returns the access URL)",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +html-publish --app-id <app_id> --path ./dist",
		"Example: lark-cli apps +html-publish --app-id <app_id> --path ./site --dry-run",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID", Required: true},
		{Name: "path", Desc: "path to HTML file or directory", Required: true},
		{Name: "allow-sensitive", Type: "bool", Desc: "skip the credential-file scan (allow .env / .npmrc / .aws/credentials / etc. in the publish payload)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		path := strings.TrimSpace(rctx.Str("path"))
		if path == "" {
			return appsValidationParamError("--path", "--path is required")
		}
		// Block well-known credential files in the publish payload unless the
		// caller explicitly opts in. Lives in Validate (not DryRun) so that
		// `--dry-run` returns non-zero on hit — the framework runs Validate
		// before branching to DryRun/Execute, so both paths share this gate.
		if rctx.Bool("allow-sensitive") {
			return nil
		}
		candidates, err := walkHTMLPublishCandidates(rctx.FileIO(), path)
		if err != nil {
			// Don't fail Validate on walk errors (bad --path, etc.) — let
			// DryRun/Execute surface them in their own (richer) envelopes.
			return nil
		}
		var hits []string
		for _, c := range candidates {
			if isSensitiveCandidate(path, c) {
				hits = append(hits, c.RelPath)
			}
		}
		if len(hits) > 0 {
			return sensitiveCandidatesError(hits)
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		path := strings.TrimSpace(rctx.Str("path"))
		dry := common.NewDryRunAPI()
		dry.Desc("Upload tar.gz + publish HTML (multipart, returns url)")
		dry.POST(fmt.Sprintf("%s/apps/%s/upload_and_release_html_code", apiBasePath, validate.EncodePathSegment(appID))).
			Set("content_type", "multipart/form-data")

		candidates, err := walkHTMLPublishCandidates(rctx.FileIO(), path)
		if err != nil {
			dry.Set("path_error", err.Error())
			return dry
		}
		if err := ensureIndexHTML(candidates); err != nil {
			// Surface the same failure Execute would hit, but as a structured
			// envelope field so dry-run still exits 0 (matches repo convention
			// for dry-run "advisory preview" semantics).
			dry.Set("validation_error", err.Error())
		}
		dry.Set("file_count", len(candidates))
		var totalSize int64
		names := make([]string, 0, len(candidates))
		for _, c := range candidates {
			totalSize += c.Size
			names = append(names, c.RelPath)
		}
		dry.Set("total_size_bytes", totalSize)
		dry.Set("files", names)
		// Sensitive-file rejection lives in Validate (so dry-run exits non-zero
		// on hit). When --allow-sensitive is set, still surface the list here
		// as an info field so the caller sees what was waived.
		if rctx.Bool("allow-sensitive") {
			var waived []string
			for _, c := range candidates {
				if isSensitiveCandidate(path, c) {
					waived = append(waived, c.RelPath)
				}
			}
			if len(waived) > 0 {
				dry.Set("sensitive_waived", waived)
				dry.Set("sensitive_waived_summary", fmt.Sprintf("%d credential file(s) included because --allow-sensitive is set", len(waived)))
			}
		}
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		spec := appsHTMLPublishSpec{
			AppID: strings.TrimSpace(rctx.Str("app-id")),
			Path:  strings.TrimSpace(rctx.Str("path")),
		}
		client := appsHTMLPublishAPI{runtime: rctx}
		out, err := runHTMLPublish(ctx, rctx.FileIO(), client, spec)
		if err != nil {
			return err
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			if url, ok := out["url"].(string); ok && url != "" {
				fmt.Fprintf(w, "url: %s\n", url)
			}
		})
		return nil
	},
}

type appsHTMLPublishSpec struct {
	AppID string
	Path  string
}

// maxSensitiveListInError caps how many credential-file matches we list inline
// in the validation error, so the message stays readable when a misconfigured
// payload has many hits (e.g. a directory tree accidentally containing
// per-environment .env.* files for every stage).
const maxSensitiveListInError = 5

// sensitiveCandidatesError builds the Validate-time rejection when --path
// contains credential files and --allow-sensitive was not set.
func sensitiveCandidatesError(hits []string) error {
	var sample string
	if len(hits) <= maxSensitiveListInError {
		sample = strings.Join(hits, ", ")
	} else {
		sample = strings.Join(hits[:maxSensitiveListInError], ", ") +
			fmt.Sprintf(" (and %d more)", len(hits)-maxSensitiveListInError)
	}
	return appsValidationParamError("--path",
		"--path contains %d credential file(s) that should not be published: %s", len(hits), sample).
		WithHint("remove these files from the publish payload, OR pass --allow-sensitive if shipping them is intentional (e.g. a docs site demoing credential-file formats)")
}

// maxHTMLPublishTarballBytes 是 client 端 tar.gz 包体上限，对齐 OAPI 设计 20MB 约束。
// 用 var 而非 const，便于单测调小覆盖拦截路径。
var maxHTMLPublishTarballBytes int64 = 20 * 1024 * 1024

// maxHTMLPublishRawBytes caps the total UNCOMPRESSED candidate size before
// tar+gzip writes them into the in-memory buffer. Defends against
// highly-compressible "decompression bomb" inputs (e.g. 50GB of zeros)
// that would balloon process memory before the gzip-after check fires.
// 200MB is much higher than any plausible legitimate HTML/static-site
// payload but low enough to stay well under typical container memory.
// Mutable for tests.
var maxHTMLPublishRawBytes int64 = 200 * 1024 * 1024

// ensureIndexHTML 要求 walker 抓到的 candidates 里必须含 index.html。
// 目录形态：根目录下必须有 index.html。
// 单文件形态：文件名必须就是 index.html。
// 妙搭服务端用 index.html 作为应用入口。
func ensureIndexHTML(candidates []htmlPublishCandidate) error {
	for _, c := range candidates {
		if c.RelPath == "index.html" {
			return nil
		}
	}
	return appsFailedPreconditionParamError("--path", "--path is missing index.html").
		WithHint("Miaoda uses index.html as the app entrypoint; for a directory put index.html at the root, or pass a single file named index.html")
}

func runHTMLPublish(ctx context.Context, fio fileio.FileIO, publisher appsHTMLPublishClient, spec appsHTMLPublishSpec) (map[string]interface{}, error) {
	candidates, err := walkHTMLPublishCandidates(fio, spec.Path)
	if err != nil {
		return nil, err
	}
	if err := ensureIndexHTML(candidates); err != nil {
		return nil, err
	}
	var rawTotal int64
	for _, c := range candidates {
		rawTotal += c.Size
	}
	if rawTotal > maxHTMLPublishRawBytes {
		return nil, appsValidationParamError("--path",
			"--path total raw bytes %d exceeds %d bytes limit (uncompressed pre-pack cap)", rawTotal, maxHTMLPublishRawBytes).
			WithHint("reduce --path contents or choose a smaller subdirectory before packaging")
	}
	tarball, err := buildHTMLPublishTarball(fio, candidates)
	if err != nil {
		return nil, err
	}

	if tarball.Size > maxHTMLPublishTarballBytes {
		return nil, appsValidationParamError("--path",
			"packed tar.gz size %d bytes exceeds %d bytes limit", tarball.Size, maxHTMLPublishTarballBytes).
			WithHint("reduce --path contents, remove unrelated large files, then retry")
	}

	resp, err := publisher.HTMLPublish(ctx, spec.AppID, tarball)
	if err != nil {
		return nil, client.WrapDoAPIError(err)
	}

	out := map[string]interface{}{}
	if resp.URL != "" {
		out["url"] = resp.URL
	}
	return out, nil
}
