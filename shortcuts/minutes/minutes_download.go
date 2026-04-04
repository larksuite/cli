// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/vfs"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	// disableClientTimeout removes the global 30s client timeout for large media downloads.
	// The download is bounded by the caller's context (e.g. Ctrl+C). A fixed timeout
	// would cut off legitimate large file transfers.
	disableClientTimeout = 0

	maxBatchSize         = 50
	maxDownloadRedirects = 5
)

// validMinuteToken matches minute tokens: lowercase alphanumeric characters only.
var validMinuteToken = regexp.MustCompile(`^[a-z0-9]+$`)

var MinutesDownload = common.Shortcut{
	Service:     "minutes",
	Command:     "+download",
	Description: "Download audio/video media file of a minute",
	Risk:        "read",
	Scopes:      []string{"minutes:minutes.media:export"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-tokens", Desc: "minute tokens, comma-separated for batch download (max 50)", Required: true},
		{Name: "output", Desc: "output path: file path for single token, directory for batch (default: current dir)"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
		{Name: "url-only", Type: "bool", Desc: "only print the download URL(s) without downloading"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		tokens := common.SplitCSV(runtime.Str("minute-tokens"))
		if len(tokens) == 0 {
			return output.ErrValidation("--minute-tokens is required")
		}
		if len(tokens) > maxBatchSize {
			return output.ErrValidation("--minute-tokens: too many tokens (%d), maximum is %d", len(tokens), maxBatchSize)
		}
		for _, token := range tokens {
			if !validMinuteToken.MatchString(token) {
				return output.ErrValidation("invalid minute token %q: must contain only lowercase alphanumeric characters (e.g. obcnq3b9jl72l83w4f149w9c)", token)
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		tokens := common.SplitCSV(runtime.Str("minute-tokens"))
		return common.NewDryRunAPI().
			GET("/open-apis/minutes/v1/minutes/:minute_token/media").
			Set("minute_tokens", tokens)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		tokens := common.SplitCSV(runtime.Str("minute-tokens"))
		outputPath := runtime.Str("output")
		overwrite := runtime.Bool("overwrite")
		urlOnly := runtime.Bool("url-only")
		errOut := runtime.IO().ErrOut
		single := len(tokens) == 1

		// Batch mode: --output must be a directory, not an existing file.
		if !single && outputPath != "" {
			if fi, err := vfs.Stat(outputPath); err == nil && !fi.IsDir() {
				return output.ErrValidation("--output %q is a file; batch mode expects a directory path", outputPath)
			}
		}

		if !single {
			fmt.Fprintf(errOut, "[minutes +download] batch: %d token(s)\n", len(tokens))
		}

		type result struct {
			MinuteToken string `json:"minute_token"`
			SavedPath   string `json:"saved_path,omitempty"`
			SizeBytes   int64  `json:"size_bytes,omitempty"`
			DownloadURL string `json:"download_url,omitempty"`
			Error       string `json:"error,omitempty"`
		}

		results := make([]result, len(tokens))
		seen := make(map[string]int)
		usedNames := make(map[string]bool)

		// Clone the factory client for download use. We clone the struct (not the
		// pointer) to avoid mutating the shared singleton's Timeout. The original
		// transport chain is preserved so security headers and test mocks still work.
		// SSRF protection: ValidateDownloadSourceURL (URL-level) + CheckRedirect
		// (redirect-level). Transport-level IP check is intentionally omitted because
		// download URLs originate from the trusted Lark API, not user input.
		baseClient, err := runtime.Factory.HttpClient()
		if err != nil {
			return output.ErrNetwork("failed to get HTTP client: %s", err)
		}
		clonedClient := *baseClient
		clonedClient.Timeout = disableClientTimeout
		clonedClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxDownloadRedirects {
				return fmt.Errorf("too many redirects")
			}
			if len(via) > 0 {
				prev := via[len(via)-1]
				if strings.EqualFold(prev.URL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http") {
					return fmt.Errorf("redirect from https to http is not allowed")
				}
			}
			return validate.ValidateDownloadSourceURL(req.Context(), req.URL.String())
		}
		dlClient := &clonedClient

		ticker := time.NewTicker(time.Second / 5) // rate-limit to 5 req/s
		defer ticker.Stop()

		for i, token := range tokens {
			if i > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
				}
			}

			if err := validate.ResourceName(token, "--minute-tokens"); err != nil {
				results[i] = result{MinuteToken: token, Error: err.Error()}
				continue
			}
			if firstIdx, dup := seen[token]; dup {
				results[i] = result{MinuteToken: token, Error: fmt.Sprintf("duplicate token, same as index %d", firstIdx)}
				continue
			}
			seen[token] = i

			downloadURL, err := fetchDownloadURL(ctx, runtime, token)
			if err != nil {
				results[i] = result{MinuteToken: token, Error: err.Error()}
				continue
			}

			if urlOnly {
				results[i] = result{MinuteToken: token, DownloadURL: downloadURL}
				continue
			}

			fmt.Fprintf(errOut, "Downloading media: %s\n", common.MaskToken(token))

			// single token: --output is a file path; batch: --output is a directory
			opts := downloadOpts{overwrite: overwrite, usedNames: usedNames}
			if single {
				opts.outputPath = outputPath
			} else {
				opts.outputDir = outputPath
			}

			dl, err := downloadMediaFile(ctx, dlClient, downloadURL, token, opts)
			if err != nil {
				results[i] = result{MinuteToken: token, Error: err.Error()}
				continue
			}
			results[i] = result{MinuteToken: token, SavedPath: dl.savedPath, SizeBytes: dl.sizeBytes}
		}

		// output
		if single {
			r := results[0]
			if r.Error != "" {
				return output.ErrAPI(0, r.Error, nil)
			}
			if urlOnly {
				runtime.Out(map[string]interface{}{"download_url": r.DownloadURL}, nil)
			} else {
				runtime.Out(map[string]interface{}{"saved_path": r.SavedPath, "size_bytes": r.SizeBytes}, nil)
			}
			return nil
		}

		// batch output
		successCount := 0
		for _, r := range results {
			if r.Error == "" {
				successCount++
			}
		}
		fmt.Fprintf(errOut, "[minutes +download] done: %d total, %d succeeded, %d failed\n", len(results), successCount, len(results)-successCount)

		runtime.OutFormat(map[string]interface{}{"downloads": results}, &output.Meta{Count: len(results)}, nil)
		if successCount == 0 && len(results) > 0 {
			return output.ErrAPI(0, fmt.Sprintf("all %d downloads failed", len(results)), nil)
		}
		return nil
	},
}

// fetchDownloadURL retrieves the pre-signed download URL for a minute token.
func fetchDownloadURL(ctx context.Context, runtime *common.RuntimeContext, minuteToken string) (string, error) {
	data, err := runtime.DoAPIJSON(http.MethodGet,
		fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/media", validate.EncodePathSegment(minuteToken)),
		nil, nil)
	if err != nil {
		return "", err
	}
	downloadURL := common.GetString(data, "download_url")
	if downloadURL == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "API returned empty download_url for %s", minuteToken)
	}
	return downloadURL, nil
}

type downloadResult struct {
	savedPath string
	sizeBytes int64
}

type downloadOpts struct {
	outputPath string // explicit output file path (single mode only)
	outputDir  string // output directory (batch mode)
	overwrite  bool
	usedNames  map[string]bool // tracks used filenames to deduplicate in batch mode
}

// downloadMediaFile streams a media file from a pre-signed URL to disk.
// Filename resolution: opts.outputPath > Content-Disposition filename > Content-Type ext > <token>.media.
func downloadMediaFile(ctx context.Context, client *http.Client, downloadURL, minuteToken string, opts downloadOpts) (*downloadResult, error) {
	if err := validate.ValidateDownloadSourceURL(ctx, downloadURL); err != nil {
		return nil, output.ErrValidation("blocked download URL: %s", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, output.ErrNetwork("invalid download URL: %s", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, output.ErrNetwork("download failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(body) > 0 {
			return nil, output.ErrNetwork("download failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil, output.ErrNetwork("download failed: HTTP %d", resp.StatusCode)
	}

	// resolve output path
	outputPath := opts.outputPath
	if outputPath == "" {
		filename := resolveFilenameFromResponse(resp, minuteToken)
		// Deduplicate filenames in batch mode: prefix with token on collision.
		if opts.usedNames != nil {
			if opts.usedNames[filename] {
				filename = minuteToken + "-" + filename
			}
			opts.usedNames[filename] = true
		}
		outputPath = filepath.Join(opts.outputDir, filename)
	}

	safePath, err := validate.SafeOutputPath(outputPath)
	if err != nil {
		return nil, output.ErrValidation("unsafe output path: %s", err)
	}
	if err := common.EnsureWritableFile(safePath, opts.overwrite); err != nil {
		return nil, err
	}
	if err := vfs.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
		return nil, output.Errorf(output.ExitInternal, "api_error", "cannot create parent directory: %s", err)
	}

	sizeBytes, err := validate.AtomicWriteFromReader(safePath, resp.Body, 0600)
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "api_error", "cannot create file: %s", err)
	}
	return &downloadResult{savedPath: safePath, sizeBytes: sizeBytes}, nil
}

// resolveFilenameFromResponse derives the filename from HTTP response headers.
// Priority: Content-Disposition filename > Content-Type extension > <token>.media.
func resolveFilenameFromResponse(resp *http.Response, minuteToken string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if filename := params["filename"]; filename != "" {
				return filename
			}
		}
	}
	if ext := extFromContentType(resp.Header.Get("Content-Type")); ext != "" {
		return minuteToken + ext
	}
	return minuteToken + ".media"
}

// preferredExt overrides Go's mime.ExtensionsByType which returns alphabetically sorted
// results (e.g. .m4v before .mp4 for video/mp4).
var preferredExt = map[string]string{
	"video/mp4":  ".mp4",
	"audio/mp4":  ".m4a",
	"audio/mpeg": ".mp3",
}

// newDownloadClient wraps the base HTTP client with SSRF protection
// (redirect safety + transport-level IP validation). When the base transport
// is not *http.Transport (e.g. test mocks), it falls back to cloning
// http.DefaultTransport via NewDownloadHTTPClient.
// extFromContentType returns a file extension for the given Content-Type, or "" if unknown.
func extFromContentType(contentType string) string {
	if contentType == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	if ext, ok := preferredExt[mediaType]; ok {
		return ext
	}
	if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ""
}
