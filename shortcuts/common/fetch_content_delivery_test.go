// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/spf13/cobra"

	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

type fetchDeliveryTestOptions struct {
	full bool
	jq   string
}

const testFetchContentJQPath = ".data.content"

func newFetchDeliveryTestRuntime(t *testing.T, opts fetchDeliveryTestOptions) (*RuntimeContext, *cmdutil.Factory) {
	t.Helper()

	root := &cobra.Command{Use: "lark-cli"}
	service := &cobra.Command{Use: "drive"}
	cmd := &cobra.Command{Use: "+fetch"}
	root.AddCommand(service)
	service.AddCommand(cmd)
	cmd.Flags().Bool("full", false, "")
	if err := cmd.Flags().Set("full", boolString(opts.full)); err != nil {
		t.Fatalf("set --full: %v", err)
	}

	cfg := &core.CliConfig{Brand: core.BrandFeishu}
	factory, _, _, _ := cmdutil.TestFactory(t, cfg)
	rctx := TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	rctx.JqExpr = opts.jq
	return rctx, factory
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func setFetchDeliveryTempDir(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	if runtime.GOOS == "windows" {
		t.Setenv("TEMP", tempDir)
		t.Setenv("TMP", tempDir)
	}
	return tempDir
}

func assertFetchDeliveryFile(t *testing.T, delivery FetchContentDelivery, content string) {
	t.Helper()
	if delivery.Inline() {
		t.Fatal("delivery.Inline() = true, want materialized content")
	}
	if delivery.Content != "" {
		t.Fatalf("delivery.Content has %d bytes, want omitted content", len(delivery.Content))
	}
	if delivery.File == nil {
		t.Fatal("delivery.File = nil")
	}
	if !filepath.IsAbs(delivery.File.Path) {
		t.Errorf("content file path = %q, want absolute path", delivery.File.Path)
	}
	if delivery.File.SizeBytes != int64(len([]byte(content))) {
		t.Errorf("size_bytes = %d, want %d", delivery.File.SizeBytes, len([]byte(content)))
	}
	wantHash := sha256.Sum256([]byte(content))
	if delivery.File.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Errorf("sha256 = %q, want %q", delivery.File.SHA256, hex.EncodeToString(wantHash[:]))
	}
	if delivery.File.Encoding != "utf-8" {
		t.Errorf("encoding = %q, want utf-8", delivery.File.Encoding)
	}
	if !delivery.File.Temporary {
		t.Error("temporary = false, want true")
	}
	if !strings.Contains(delivery.File.Hint, "Oversized content was saved to temporary file:") ||
		!strings.Contains(delivery.File.Hint, "Consider reading or searching this file locally") {
		t.Errorf("hint = %q, want save reason and local-read recommendation", delivery.File.Hint)
	}

	got, err := os.ReadFile(delivery.File.Path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", delivery.File.Path, err)
	}
	if string(got) != content {
		t.Fatalf("saved content differs: got %d bytes, want %d", len(got), len(content))
	}
	info, err := os.Stat(delivery.File.Path)
	if err != nil {
		t.Fatalf("Stat(%q): %v", delivery.File.Path, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %04o, want 0600", info.Mode().Perm())
	}
}

func TestPrepareFetchContentDeliverySpillBoundary(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	tempDir := setFetchDeliveryTempDir(t)

	t.Run("exactly 24 KiB remains inline", func(t *testing.T) {
		content := strings.Repeat("a", FetchContentSpillThreshold)
		rctx, _ := newFetchDeliveryTestRuntime(t, fetchDeliveryTestOptions{full: true})

		delivery, scan, err := PrepareFetchContentDelivery(rctx, map[string]any{"content": content}, content, testFetchContentJQPath)
		if err != nil {
			t.Fatalf("PrepareFetchContentDelivery() error = %v", err)
		}
		if scan.Blocked {
			t.Fatal("content safety scan unexpectedly blocked")
		}
		if !delivery.Inline() || delivery.Content != content {
			t.Fatalf("delivery = %#v, want exact inline content at threshold", delivery)
		}
	})

	t.Run("24 KiB plus one byte spills", func(t *testing.T) {
		content := strings.Repeat("b", FetchContentSpillThreshold+1)
		rctx, _ := newFetchDeliveryTestRuntime(t, fetchDeliveryTestOptions{full: true})

		delivery, scan, err := PrepareFetchContentDelivery(rctx, map[string]any{"content": content}, content, testFetchContentJQPath)
		if err != nil {
			t.Fatalf("PrepareFetchContentDelivery() error = %v", err)
		}
		if scan.Blocked {
			t.Fatal("content safety scan unexpectedly blocked")
		}
		t.Cleanup(func() { _ = os.Remove(delivery.File.Path) })
		assertFetchDeliveryFile(t, delivery, content)
		if filepath.Clean(filepath.Dir(delivery.File.Path)) != filepath.Clean(tempDir) {
			t.Errorf("temporary directory = %q, want %q", filepath.Dir(delivery.File.Path), tempDir)
		}
		if name := filepath.Base(delivery.File.Path); !strings.HasPrefix(name, "lark-cli-fetch-") || filepath.Ext(name) != ".md" {
			t.Errorf("temporary filename = %q, want lark-cli-fetch-*.md", name)
		}
		if len([]byte(delivery.Preview)) > fetchContentPreviewLimit {
			t.Errorf("preview = %d bytes, want at most %d", len([]byte(delivery.Preview)), fetchContentPreviewLimit)
		}
		if !utf8.ValidString(delivery.Preview) {
			t.Fatal("preview is not valid UTF-8")
		}
	})
}

func TestPrepareFetchContentDeliveryUnicodePreview(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	setFetchDeliveryTempDir(t)
	content := strings.Repeat("界", FetchContentSpillThreshold/len("界")+2)
	rctx, _ := newFetchDeliveryTestRuntime(t, fetchDeliveryTestOptions{full: true})

	delivery, _, err := PrepareFetchContentDelivery(rctx, map[string]any{"content": content}, content, testFetchContentJQPath)
	if err != nil {
		t.Fatalf("PrepareFetchContentDelivery() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(delivery.File.Path) })
	assertFetchDeliveryFile(t, delivery, content)
	if !utf8.ValidString(delivery.Preview) {
		t.Fatal("preview is not valid UTF-8")
	}
	if got := len([]byte(delivery.Preview)); got > fetchContentPreviewLimit {
		t.Errorf("preview = %d bytes, want at most %d", got, fetchContentPreviewLimit)
	}
	if !strings.HasSuffix(delivery.Preview, "…") {
		t.Errorf("preview suffix = %q, want ellipsis", delivery.Preview[len(delivery.Preview)-3:])
	}
}

func TestPrepareFetchContentDeliveryAutomaticSpillBypasses(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	tempDir := setFetchDeliveryTempDir(t)
	content := strings.Repeat("x", FetchContentSpillThreshold+1)
	tests := []struct {
		name string
		opts fetchDeliveryTestOptions
	}{
		{name: "non-full", opts: fetchDeliveryTestOptions{}},
		{name: "jq", opts: fetchDeliveryTestOptions{full: true, jq: ".data.content"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rctx, _ := newFetchDeliveryTestRuntime(t, tt.opts)
			delivery, _, err := PrepareFetchContentDelivery(rctx, map[string]any{"content": content}, content, testFetchContentJQPath)
			if err != nil {
				t.Fatalf("PrepareFetchContentDelivery() error = %v", err)
			}
			if !delivery.Inline() || delivery.Content != content {
				t.Fatalf("delivery = %#v, want exact inline content", delivery)
			}
		})
	}
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", tempDir, err)
	}
	if len(entries) != 0 {
		t.Fatalf("automatic-spill bypasses created files: %v", entries)
	}
}

func TestPrepareFetchContentDeliveryWithoutLocalTempSupportStaysInline(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	tempDir := setFetchDeliveryTempDir(t)
	content := strings.Repeat("x", FetchContentSpillThreshold+1)
	rctx, factory := newFetchDeliveryTestRuntime(t, fetchDeliveryTestOptions{full: true})
	factory.FileIOProvider = fetchDeliveryFileIOProvider{fileIO: fetchDeliveryUnsupportedFileIO{}}

	delivery, scan, err := PrepareFetchContentDelivery(rctx, map[string]any{"content": content}, content, testFetchContentJQPath)
	if err != nil {
		t.Fatalf("PrepareFetchContentDelivery() error = %v", err)
	}
	if scan.Blocked {
		t.Fatal("content safety scan unexpectedly blocked")
	}
	if !delivery.Inline() || delivery.Content != content {
		t.Fatalf("delivery = %#v, want complete inline content", delivery)
	}
	if want := fetchDeliveryFallbackHint(testFetchContentJQPath); delivery.InlineHint != want {
		t.Errorf("inline hint = %q, want %q", delivery.InlineHint, want)
	}
	entries, readErr := os.ReadDir(tempDir)
	if readErr != nil {
		t.Fatalf("ReadDir(%q): %v", tempDir, readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("unsupported runtime created temporary files: %v", entries)
	}
}

func fetchDeliveryFallbackHint(contentJQPath string) string {
	return "Content remains inline because temporary-file delivery failed and may be truncated. " +
		"If incomplete, rerun locally with --full --jq '" + contentJQPath +
		"' and redirect stdout to a new file; use --page-token only when shell redirection is unavailable."
}

func TestWriteFetchContentPrettyPrintsInlineFallbackHintFirst(t *testing.T) {
	delivery := FetchContentDelivery{Content: "body", InlineHint: "retry another way"}
	var out strings.Builder
	WriteFetchContentPretty(&out, delivery)
	if got, want := out.String(), "Hint: retry another way\n\nbody\n"; got != want {
		t.Fatalf("pretty output = %q, want %q", got, want)
	}
}

func TestPrepareFetchContentDeliverySafetyBlockCreatesNoFile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	tempDir := setFetchDeliveryTempDir(t)
	const tailMarker = "fetch-safety-tail-marker"
	provider := &fetchDeliverySafetyProvider{marker: tailMarker}
	extcs.Register(provider)
	t.Cleanup(func() { extcs.Register(nil) })
	content := strings.Repeat("blocked", FetchContentSpillThreshold) + tailMarker
	rctx, _ := newFetchDeliveryTestRuntime(t, fetchDeliveryTestOptions{full: true})

	delivery, scan, err := PrepareFetchContentDelivery(rctx, map[string]any{"content": content}, content, testFetchContentJQPath)
	if err == nil || !scan.Blocked {
		t.Fatalf("PrepareFetchContentDelivery() = (%#v, blocked=%t, %v), want content-safety block", delivery, scan.Blocked, err)
	}
	entries, readErr := os.ReadDir(tempDir)
	if readErr != nil {
		t.Fatalf("ReadDir(%q): %v", tempDir, readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("content-safety block created files: %v", entries)
	}
}

type fetchDeliverySafetyProvider struct{ marker string }

func (p *fetchDeliverySafetyProvider) Name() string { return "fetch-delivery-test" }

func (p *fetchDeliverySafetyProvider) Scan(_ context.Context, req extcs.ScanRequest) (*extcs.Alert, error) {
	raw, err := json.Marshal(req.Data)
	if err != nil || !strings.Contains(string(raw), p.marker) {
		return nil, err
	}
	return &extcs.Alert{
		Provider:     p.Name(),
		MatchedRules: []string{"blocked-content"},
	}, nil
}

type fetchDeliveryUnsupportedFileIO struct{}

func (fetchDeliveryUnsupportedFileIO) Open(string) (fileio.File, error) { return nil, fs.ErrNotExist }
func (fetchDeliveryUnsupportedFileIO) Stat(string) (fileio.FileInfo, error) {
	return nil, fs.ErrNotExist
}
func (fetchDeliveryUnsupportedFileIO) ResolvePath(path string) (string, error) { return path, nil }
func (fetchDeliveryUnsupportedFileIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	return nil, fs.ErrPermission
}

type fetchDeliveryFileIOProvider struct{ fileIO fileio.FileIO }

func (p fetchDeliveryFileIOProvider) Name() string { return "fetch-delivery-test" }
func (p fetchDeliveryFileIOProvider) ResolveFileIO(context.Context) fileio.FileIO {
	return p.fileIO
}

func TestPrepareFetchContentDeliveryTemporaryFileFailureStaysInline(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	notDirectory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(notDirectory, []byte("file"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", notDirectory, err)
	}
	t.Setenv("TMPDIR", notDirectory)
	t.Setenv("TEMP", notDirectory)
	t.Setenv("TMP", notDirectory)

	content := strings.Repeat("x", FetchContentSpillThreshold+1)
	rctx, _ := newFetchDeliveryTestRuntime(t, fetchDeliveryTestOptions{full: true})
	delivery, scan, err := PrepareFetchContentDelivery(rctx, map[string]any{"content": content}, content, testFetchContentJQPath)
	if err != nil {
		t.Fatalf("PrepareFetchContentDelivery() error = %v", err)
	}
	if scan.Blocked {
		t.Fatal("content safety scan unexpectedly blocked")
	}
	if !delivery.Inline() || delivery.Content != content {
		t.Fatalf("delivery = %#v, want complete inline content", delivery)
	}
	if want := fetchDeliveryFallbackHint(testFetchContentJQPath); delivery.InlineHint != want {
		t.Errorf("inline hint = %q, want %q", delivery.InlineHint, want)
	}
}
