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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	// disableClientTimeout removes the global 30s client timeout for large media downloads.
	// The download is bounded by the caller's context (e.g. Ctrl+C). A fixed timeout
	// would cut off legitimate large file transfers.
	disableClientTimeout = 0

	maxBatchSize           = 50
	maxConcurrentDownloads = 3
	maxDownloadRedirects   = 5
	tokenSuffixLen         = 6
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
		{Name: "output", Desc: "local save path (single token only)"},
		{Name: "output-dir", Desc: "output directory for batch download (default: current dir)"},
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
		if len(tokens) > 1 && runtime.Str("output") != "" {
			return output.ErrValidation("--output cannot be used with multiple tokens; use --output-dir instead")
		}
		if outDir := runtime.Str("output-dir"); outDir != "" {
			if err := common.ValidateSafeOutputDir(outDir); err != nil {
				return err
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		tokens := common.SplitCSV(runtime.Str("minute-tokens"))
		api := common.NewDryRunAPI().
			GET("/open-apis/minutes/v1/minutes/:minute_token/media")
		api.Set("minute_tokens", tokens)
		if len(tokens) > 1 {
			api.Set("concurrent_downloads", maxConcurrentDownloads)
		}
		return api
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		tokens := common.SplitCSV(runtime.Str("minute-tokens"))
		if len(tokens) == 1 {
			return executeSingle(ctx, runtime, tokens[0])
		}
		return executeBatch(ctx, runtime, tokens)
	},
}

// executeSingle handles a single token download.
func executeSingle(ctx context.Context, runtime *common.RuntimeContext, minuteToken string) error {
	outputPath := runtime.Str("output")
	overwrite := runtime.Bool("overwrite")
	urlOnly := runtime.Bool("url-only")

	if err := validate.ResourceName(minuteToken, "--minute-tokens"); err != nil {
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

	// pre-fetch title before download (RuntimeContext access stays sequential)
	var title string
	if outputPath == "" {
		title = fetchMinuteTitle(ctx, runtime, minuteToken)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Downloading media: %s\n", common.MaskToken(minuteToken))

	result, err := downloadMediaFile(ctx, runtime, downloadURL, minuteToken, downloadOpts{
		outputPath: outputPath,
		overwrite:  overwrite,
		title:      title,
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

// executeBatch handles multiple tokens with concurrent downloads.
// Phase 1: sequentially fetch download URLs (RuntimeContext is not concurrency-safe).
// Phase 2: concurrently download files (HTTP client is concurrency-safe).
func executeBatch(ctx context.Context, runtime *common.RuntimeContext, tokens []string) error {
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

	// Phase 1: sequentially fetch download URLs and titles (RuntimeContext is not concurrency-safe)
	type downloadTask struct {
		index       int
		token       string
		downloadURL string
		title       string // pre-fetched title for filename resolution
	}
	var tasks []downloadTask
	seen := make(map[string]int)

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

		if err := ctx.Err(); err != nil {
			return err
		}
		if err := validate.ResourceName(token, "--minute-tokens"); err != nil {
			results[i] = batchResult{MinuteToken: token, Error: err.Error()}
			continue
		}

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

		title := fetchMinuteTitle(ctx, runtime, token)
		tasks = append(tasks, downloadTask{index: i, token: token, downloadURL: downloadURL, title: title})
	}

	// Phase 2: download files concurrently
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
					title:     task.title,
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

// deduplicateFilename ensures uniqueness by probing with token suffix and counter.
func deduplicateFilename(name, minuteToken string, usedNames *sync.Map) string {
	if _, loaded := usedNames.LoadOrStore(name, true); !loaded {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	suffix := minuteToken
	if len(suffix) > tokenSuffixLen {
		suffix = suffix[:tokenSuffixLen]
	}
	for i := 0; ; i++ {
		candidate := base + "_" + suffix + ext
		if i > 0 {
			candidate = fmt.Sprintf("%s_%s_%d%s", base, suffix, i+1, ext)
		}
		if _, loaded := usedNames.LoadOrStore(candidate, true); !loaded {
			return candidate
		}
	}
}

type downloadResult struct {
	savedPath string
	sizeBytes int64
}

type downloadOpts struct {
	outputPath string // explicit output path (single mode)
	outputDir  string // output directory prefix (batch mode)
	overwrite  bool
	usedNames  *sync.Map // filename dedup table for batch mode
	title      string    // pre-fetched minute title (avoids RuntimeContext in concurrent path)
}

// downloadMediaFile streams a media file from a pre-signed URL to disk.
// Output path resolution: opts.outputPath > Content-Disposition > Content-Type extension > <token>.media.
func downloadMediaFile(ctx context.Context, runtime *common.RuntimeContext, downloadURL, minuteToken string, opts downloadOpts) (*downloadResult, error) {
	if err := validate.ValidateDownloadSourceURL(ctx, downloadURL); err != nil {
		return nil, output.ErrValidation("blocked download URL: %s", err)
	}

	httpClient, err := runtime.Factory.HttpClient()
	if err != nil {
		return nil, output.ErrNetwork("failed to get HTTP client: %s", err)
	}

	downloadClient := *httpClient
	downloadClient.Timeout = disableClientTimeout
	downloadClient.CheckRedirect = safeRedirectPolicy

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, output.ErrNetwork("invalid download URL: %s", err)
	}

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

	outputPath := opts.outputPath
	if outputPath == "" {
		filename := resolveOutputFilename(resp, minuteToken, opts.title)
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

// resolveOutputFilename derives the output filename from the pre-fetched title and
// Content-Type extension. Falls back to <token>.media when both are unavailable.
func resolveOutputFilename(resp *http.Response, minuteToken, title string) string {
	ext := extFromContentType(resp.Header.Get("Content-Type"))

	if title != "" {
		name := sanitizeFileName(title)
		if name == "" {
			name = minuteToken
		}
		if ext != "" {
			return name + ext
		}
		return name + ".media"
	}
	if ext != "" {
		return minuteToken + ext
	}
	return minuteToken + ".media"
}

// fetchMinuteTitle retrieves the minute title via minutes get API. Returns "" on failure.
func fetchMinuteTitle(ctx context.Context, runtime *common.RuntimeContext, minuteToken string) string {
	if runtime == nil {
		return ""
	}
	data, err := runtime.DoAPIJSON(http.MethodGet,
		fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", validate.EncodePathSegment(minuteToken)),
		nil, nil)
	if err != nil {
		return ""
	}
	if minute, ok := data["minute"].(map[string]interface{}); ok {
		return common.GetString(minute, "title")
	}
	return ""
}

// preferredExt overrides Go's mime.ExtensionsByType which returns alphabetically sorted
// results (e.g. .m4v before .mp4 for video/mp4). Map the most common extensions here.
var preferredExt = map[string]string{
	"video/mp4":  ".mp4",
	"audio/mp4":  ".m4a",
	"audio/mpeg": ".mp3",
}

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

// sanitizeFileName replaces filesystem-unsafe characters with visually similar fullwidth
// Unicode equivalents so the filename stays readable and cross-platform safe.
func sanitizeFileName(name string) string {
	const maxLen = 200
	replacer := strings.NewReplacer(
		"/", "／", "\\", "＼", ":", "：", "*", "＊", "?", "？",
		"\"", "＂", "<", "＜", ">", "＞", "|", "｜",
		"\n", " ", "\r", "", "\t", " ", "\x00", "",
	)
	safe := replacer.Replace(strings.TrimSpace(name))
	safe = strings.Trim(safe, ".")
	if len([]rune(safe)) > maxLen {
		safe = string([]rune(safe)[:maxLen])
	}
	return safe
}

// safeRedirectPolicy prevents HTTPS→HTTP downgrade and validates redirect targets.
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
