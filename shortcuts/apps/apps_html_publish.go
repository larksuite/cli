// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsHTMLPublish packs --path as tar.gz and uploads + publishes via one multipart POST.
var AppsHTMLPublish = common.Shortcut{
	Service:     appsService,
	Command:     "+html-publish",
	Description: "Publish HTML to a Miaoda app (single multipart POST returns the access URL)",
	Risk:        "write",
	Scopes:      []string{"spark:app:publish"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID", Required: true},
		{Name: "path", Desc: "path to HTML file or directory", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return output.ErrValidation("--app-id is required")
		}
		path := strings.TrimSpace(rctx.Str("path"))
		if path == "" {
			return output.ErrValidation("--path is required")
		}
		// Reject --path equal to the current working directory. Publishing
		// cwd recursively packs .git/ / .env / node_modules / .aws/credentials
		// alongside the intended HTML, and combined with --scope public puts
		// those on an internet-reachable URL.
		if filepath.Clean(path) == "." {
			return output.ErrWithHint(output.ExitValidation, "validation",
				"--path 不能指向当前工作目录（避免误把整个工程一并发布出去）",
				"改成具体的子目录或文件，如 './dist' / './public' / './index.html'")
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
		// Advisory scan: surface paths matching well-known secret / credential
		// patterns so the caller can review before going public. Dry-run still
		// exits 0; this is non-blocking by design (legit doc sites may ship
		// example .env files).
		var warnings []string
		for _, c := range candidates {
			if isSensitiveRelPath(c.RelPath) {
				warnings = append(warnings, c.RelPath)
			}
		}
		if len(warnings) > 0 {
			dry.Set("warnings", warnings)
			dry.Set("warning_summary", fmt.Sprintf("manifest contains %d sensitive path(s); review before publishing", len(warnings)))
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
	return output.ErrWithHint(output.ExitAPI, "validation",
		"--path 中缺少 index.html",
		"妙搭以 index.html 作为应用入口；目录形态把首页放在根目录命名 index.html，单文件形态把文件命名为 index.html")
}

func runHTMLPublish(ctx context.Context, fio fileio.FileIO, client appsHTMLPublishClient, spec appsHTMLPublishSpec) (map[string]interface{}, error) {
	// Defense in depth: callers reaching runHTMLPublish bypass the shortcut's
	// Validate closure. Re-check that --path is not cwd before walking.
	if filepath.Clean(spec.Path) == "." {
		return nil, output.ErrWithHint(output.ExitValidation, "validation",
			"--path 不能指向当前工作目录（避免误把整个工程一并发布出去）",
			"改成具体的子目录或文件，如 './dist' / './public' / './index.html'")
	}
	candidates, err := walkHTMLPublishCandidates(fio, spec.Path)
	if err != nil {
		return nil, output.Errorf(output.ExitAPI, "io", "scan --path %s: %v", spec.Path, err)
	}
	if err := ensureIndexHTML(candidates); err != nil {
		return nil, err
	}
	var rawTotal int64
	for _, c := range candidates {
		rawTotal += c.Size
	}
	if rawTotal > maxHTMLPublishRawBytes {
		return nil, output.ErrWithHint(output.ExitAPI, "validation",
			fmt.Sprintf("--path total raw bytes %d exceeds %d bytes limit (uncompressed pre-pack cap)", rawTotal, maxHTMLPublishRawBytes),
			"在 tar+gzip 进入内存前拦截，避免 OOM；精简 --path 内容或选择更小的子目录")
	}
	tarball, err := buildHTMLPublishTarball(fio, candidates)
	if err != nil {
		return nil, output.Errorf(output.ExitAPI, "io", "pack: %v", err)
	}

	if tarball.Size > maxHTMLPublishTarballBytes {
		return nil, output.ErrWithHint(output.ExitAPI, "validation",
			fmt.Sprintf("packed tar.gz size %d bytes exceeds %d bytes limit", tarball.Size, maxHTMLPublishTarballBytes),
			"请精简 --path 目录（去掉无关大文件 / 压缩资源）后重试；本期接口上限 20MB")
	}

	resp, err := client.HTMLPublish(ctx, spec.AppID, tarball)
	if err != nil {
		return nil, err
	}

	out := map[string]interface{}{}
	if resp.URL != "" {
		out["url"] = resp.URL
	}
	return out, nil
}
