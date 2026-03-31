// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	// disableClientTimeout removes the global 30s client timeout for large media downloads.
	// The request is still bounded by the caller's context.
	disableClientTimeout = 0

	maxBatchSize           = 50
	maxConcurrentDownloads = 3
	maxDownloadRedirects   = 5
	tokenSuffixLen         = 6
)

var MinutesDownload = common.Shortcut{
	Service:     "minutes",
	Command:     "+download",
	Description: "Download audio/video media file of a minute",
	Risk:        "read",
	Scopes:      []string{"minutes:minutes.media:export"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "minute-token", Desc: "single minute token (mutually exclusive with --minute-tokens)"},
		{Name: "minute-tokens", Desc: "comma-separated minute tokens for batch download (max 50)"},
		{Name: "output", Desc: "local save path (single mode only)"},
		{Name: "output-dir", Desc: "output directory for batch download (default: current dir)"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
		{Name: "url-only", Type: "bool", Desc: "only print the download URL(s) without downloading"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		single := runtime.Str("minute-token")
		batch := runtime.Str("minute-tokens")

		if single == "" && batch == "" {
			return output.ErrValidation("one of --minute-token or --minute-tokens is required")
		}
		if single != "" && batch != "" {
			return output.ErrValidation("--minute-token and --minute-tokens are mutually exclusive")
		}

		if batch != "" {
			tokens := common.SplitCSV(batch)
			if len(tokens) > maxBatchSize {
				return output.ErrValidation("--minute-tokens: too many tokens (%d), maximum is %d", len(tokens), maxBatchSize)
			}
			if runtime.Str("output") != "" {
				return output.ErrValidation("--output cannot be used with --minute-tokens; use --output-dir instead")
			}
		}

		if outDir := runtime.Str("output-dir"); outDir != "" {
			if err := common.ValidateSafeOutputDir(outDir); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		api := common.NewDryRunAPI().
			GET("/open-apis/minutes/v1/minutes/:minute_token/media")

		if token := runtime.Str("minute-token"); token != "" {
			api.Set("minute_token", token)
		}
		if tokens := runtime.Str("minute-tokens"); tokens != "" {
			api.Set("minute_tokens", common.SplitCSV(tokens))
			api.Set("concurrent_downloads", maxConcurrentDownloads)
		}
		return api
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if tokens := runtime.Str("minute-tokens"); tokens != "" {
			return executeBatch(ctx, runtime)
		}
		return executeSingle(ctx, runtime)
	},
}

// executeSingle handles the single --minute-token mode.
func executeSingle(ctx context.Context, runtime *common.RuntimeContext) error {
	minuteToken := runtime.Str("minute-token")
	outputPath := runtime.Str("output")
	overwrite := runtime.Bool("overwrite")
	urlOnly := runtime.Bool("url-only")

	if err := validate.ResourceName(minuteToken, "--minute-token"); err != nil {
		return output.ErrValidation("%s", err)
	}

	downloadURL, err := fetchDownloadURL(ctx, runtime, minuteToken)
	if err != nil {
		return err
	}

	if urlOnly {
		runtime.Out(map[string]interface{}{
			"download_url": downloadURL,
		}, nil)
		return nil
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Downloading media: %s\n", common.MaskToken(minuteToken))

	result, err := downloadMediaFile(ctx, runtime, downloadURL, minuteToken, downloadOpts{
		outputPath: outputPath,
		overwrite:  overwrite,
	})
	if err != nil {
		return err
	}

	runtime.Out(map[string]interface{}{
		"saved_path": result.savedPath,
		"size_bytes": result.sizeBytes,
	}, nil)
	return nil
}

// executeBatch handles the batch --minute-tokens mode.
// Phase 1: sequentially fetch download URLs (RuntimeContext is not concurrency-safe).
// Phase 2: concurrently download files (HTTP client is concurrency-safe).
func executeBatch(ctx context.Context, runtime *common.RuntimeContext) error {
	tokens := common.SplitCSV(runtime.Str("minute-tokens"))
	overwrite := runtime.Bool("overwrite")
	urlOnly := runtime.Bool("url-only")
	outputDir := runtime.Str("output-dir")
	errOut := runtime.IO().ErrOut

	fmt.Fprintf(errOut, "[minutes +download] batch: %d token(s), concurrency=%d\n", len(tokens), maxConcurrentDownloads)

	type batchResult struct {
		MinuteToken string `json:"minute_token"`
		SavedPath   string `json:"saved_path,omitempty"`
		SizeBytes   int64  `json:"size_bytes,omitempty"`
		DownloadURL string `json:"download_url,omitempty"`
		Error       string `json:"error,omitempty"`
	}

	results := make([]batchResult, len(tokens))

	// Phase 1: fetch all download URLs sequentially (RuntimeContext is not concurrency-safe)
	type downloadTask struct {
		index       int
		token       string
		downloadURL string
	}
	var tasks []downloadTask
	seen := make(map[string]int) // dedup: token -> index of first occurrence

	for i, token := range tokens {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validate.ResourceName(token, "--minute-tokens"); err != nil {
			results[i] = batchResult{MinuteToken: token, Error: err.Error()}
			continue
		}

		// skip duplicate tokens, point to first occurrence
		if firstIdx, dup := seen[token]; dup {
			results[i] = batchResult{MinuteToken: token, Error: fmt.Sprintf("duplicate token, same as index %d", firstIdx)}
			continue
		}
		seen[token] = i

		downloadURL, err := fetchDownloadURL(ctx, runtime, token)
		if err != nil {
			results[i] = batchResult{MinuteToken: token, Error: err.Error()}
			continue
		}

		if urlOnly {
			results[i] = batchResult{MinuteToken: token, DownloadURL: downloadURL}
			continue
		}

		tasks = append(tasks, downloadTask{index: i, token: token, downloadURL: downloadURL})
	}

	// Phase 2: download files concurrently (WaitGroup + semaphore)
	if len(tasks) > 0 {
		var usedNames sync.Map
		var logMu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, maxConcurrentDownloads)

		for _, task := range tasks {
			wg.Add(1)
			go func(task downloadTask) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				logMu.Lock()
				fmt.Fprintf(errOut, "[minutes +download] downloading: %s\n", common.MaskToken(task.token))
				logMu.Unlock()

				result, err := downloadMediaFile(ctx, runtime, task.downloadURL, task.token, downloadOpts{
					outputDir: outputDir,
					overwrite: overwrite,
					usedNames: &usedNames,
				})
				if err != nil {
					results[task.index] = batchResult{MinuteToken: task.token, Error: err.Error()}
					return
				}

				results[task.index] = batchResult{
					MinuteToken: task.token,
					SavedPath:   result.savedPath,
					SizeBytes:   result.sizeBytes,
				}
			}(task)
		}
		wg.Wait()
	}

	// summary
	successCount := 0
	for _, r := range results {
		if r.Error == "" {
			successCount++
		}
	}
	fmt.Fprintf(errOut, "[minutes +download] done: %d total, %d succeeded, %d failed\n", len(results), successCount, len(results)-successCount)

	outData := map[string]interface{}{"downloads": results}
	runtime.OutFormat(outData, &output.Meta{Count: len(results)}, nil)

	if successCount == 0 && len(results) > 0 {
		return output.ErrAPI(0, fmt.Sprintf("all %d downloads failed", len(results)), nil)
	}
	return nil
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

// deduplicateFilename ensures uniqueness by appending a token suffix on collision.
// Returns the deduplicated filename (without directory prefix).
func deduplicateFilename(name, minuteToken string, usedNames *sync.Map) string {
	if _, loaded := usedNames.LoadOrStore(name, true); !loaded {
		return name
	}

	// collision: insert _<token_prefix> before extension
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	suffix := minuteToken
	if len(suffix) > tokenSuffixLen {
		suffix = suffix[:tokenSuffixLen]
	}
	deduped := base + "_" + suffix + ext
	usedNames.Store(deduped, true)
	return deduped
}

type downloadResult struct {
	savedPath string
	sizeBytes int64
}

// downloadOpts controls how downloadMediaFile resolves the output path.
type downloadOpts struct {
	outputPath string    // explicit output path (single mode); empty = resolve from response headers
	outputDir  string    // output directory prefix (batch mode)
	overwrite  bool      // overwrite existing files
	usedNames  *sync.Map // filename dedup table for batch mode; nil for single mode
}

// downloadMediaFile streams a media file from a pre-signed URL to disk.
// Output path resolution: opts.outputPath > Content-Disposition > Content-Type extension > <token>.media.
// When opts.usedNames is non-nil (batch mode), deduplicates filenames by appending token suffix.
func downloadMediaFile(ctx context.Context, runtime *common.RuntimeContext, downloadURL, minuteToken string, opts downloadOpts) (*downloadResult, error) {
	// SSRF: validate download URL before making any request
	if err := validate.ValidateDownloadSourceURL(ctx, downloadURL); err != nil {
		return nil, output.ErrValidation("blocked download URL: %s", err)
	}

	httpClient, err := runtime.Factory.HttpClient()
	if err != nil {
		return nil, output.ErrNetwork("failed to get HTTP client: %s", err)
	}

	// clone client: disable timeout for large files, add redirect safety policy
	downloadClient := *httpClient
	downloadClient.Timeout = disableClientTimeout
	downloadClient.CheckRedirect = safeRedirectPolicy

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, output.ErrNetwork("invalid download URL: %s", err)
	}
	// no Authorization header — download_url is a pre-signed URL

	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, output.ErrNetwork("download failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(body) > 0 {
			return nil, output.ErrNetwork("download failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil, output.ErrNetwork("download failed: HTTP %d", resp.StatusCode)
	}

	// resolve output path
	outputPath := opts.outputPath
	if outputPath == "" {
		filename := resolveOutputFromResponse(resp, minuteToken)
		if opts.usedNames != nil {
			filename = deduplicateFilename(filename, minuteToken, opts.usedNames)
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
	if err := os.MkdirAll(filepath.Dir(safePath), 0755); err != nil {
		return nil, output.Errorf(output.ExitInternal, "api_error", "cannot create parent directory: %s", err)
	}

	sizeBytes, err := validate.AtomicWriteFromReader(safePath, resp.Body, 0644)
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "api_error", "cannot create file: %s", err)
	}
	return &downloadResult{savedPath: safePath, sizeBytes: sizeBytes}, nil
}

// resolveOutputFromResponse derives the output filename from HTTP response headers.
// Priority: Content-Disposition filename > Content-Type extension > fallback to <token>.media.
func resolveOutputFromResponse(resp *http.Response, minuteToken string) string {
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if filename := params["filename"]; filename != "" {
				return filename
			}
		}
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		if mediaType, _, err := mime.ParseMediaType(ct); err == nil {
			if exts, err := mime.ExtensionsByType(mediaType); err == nil && len(exts) > 0 {
				return minuteToken + exts[0]
			}
		}
	}

	return minuteToken + ".media"
}

// safeRedirectPolicy prevents HTTPS→HTTP downgrade, limits redirect count,
// and validates each redirect target against SSRF rules.
func safeRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= maxDownloadRedirects {
		return fmt.Errorf("too many redirects")
	}
	if len(via) > 0 {
		prev := via[len(via)-1]
		if strings.EqualFold(prev.URL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http") {
			return fmt.Errorf("redirect from https to http is not allowed")
		}
	}
	if err := validate.ValidateDownloadSourceURL(req.Context(), req.URL.String()); err != nil {
		return fmt.Errorf("blocked redirect target: %w", err)
	}
	return nil
}
