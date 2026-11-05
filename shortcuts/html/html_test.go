// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package html

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/html/payload"
)

func runHTML(t *testing.T, sc common.Shortcut, args []string, stub *httpmock.Stub, brands ...core.LarkBrand) (string, error) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	brand := core.BrandFeishu
	if len(brands) > 0 {
		brand = brands[0]
	}
	f, out, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-html", AppSecret: "test-secret", Brand: brand, UserOpenId: "ou_test"})
	if stub != nil {
		reg.Register(stub)
	}
	parent := &cobra.Command{Use: "html", SilenceErrors: true, SilenceUsage: true}
	sc.Mount(parent, f)
	parent.SetArgs(append(args, "--as", "user"))
	err := parent.ExecuteContext(context.Background())
	return out.String(), err
}
func writeFile(t *testing.T, name string, data []byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(name), 0700))
	require.NoError(t, os.WriteFile(name, data, 0600))
}
func fixture(t *testing.T) {
	t.Helper()
	cmdutil.TestChdir(t, t.TempDir())
	writeFile(t, "site/page.html", []byte(`<link rel="stylesheet" href="assets/style.css"><script src="assets/app.js"></script><img src="assets/p.png">`))
	writeFile(t, "site/assets/style.css", []byte(`body { color: red }`))
	writeFile(t, "site/assets/app.js", []byte(`console.log("hello")`))
	writeFile(t, "site/assets/p.png", []byte{0, 1, 2, 255})
}
func successPublishStub() *httpmock.Stub {
	return &httpmock.Stub{Method: http.MethodPost, URL: publishPath, Body: map[string]any{"code": 0, "data": map[string]any{"token": "html_test", "link_url": "https://example.test/page/html_test"}}}
}
func TestPublish_OrderedMultipart(t *testing.T) {
	fixture(t)
	stub := successPublishStub()
	out, err := runHTML(t, Publish, []string{"+publish", "--file-path", "site/page.html"}, stub)
	require.NoError(t, err)
	assert.Contains(t, out, `"token": "html_test"`)
	media, params, err := mime.ParseMediaType(stub.CapturedHeaders.Get("Content-Type"))
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", media)
	reader := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	var files []payload.HashFile
	fields := map[string][]string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		raw, err := io.ReadAll(part)
		require.NoError(t, err)
		if part.FormName() == "files" {
			// FileName strips subdirectories, so inspect the original MIME parameter.
			_, p, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			require.NoError(t, err)
			files = append(files, payload.HashFile{Path: p["filename"], Raw: raw})
		} else {
			fields[part.FormName()] = append(fields[part.FormName()], string(raw))
		}
	}
	assert.NotContains(t, fields, "token")
	paths := fields["paths"]
	assert.ElementsMatch(t, []string{"paths", "idempotent_key", "hash_tag"}, func() []string {
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		return names
	}())

	require.Len(t, files, 4)
	require.Len(t, paths, 4)
	require.Len(t, fields["idempotent_key"], 1)
	require.Len(t, fields["hash_tag"], 1)
	assert.Equal(t, "index.html", paths[0])
	for i, file := range files {
		assert.Equal(t, paths[i], file.Path)
		assert.NotContains(t, file.Path, "output/")
	}
	hash, err := payload.ContentHash(files)
	require.NoError(t, err)
	assert.Equal(t, hash, fields["hash_tag"][0])
	abs, err := filepath.Abs("site/page.html")
	require.NoError(t, err)
	abs, err = filepath.EvalSymlinks(abs)
	require.NoError(t, err)
	assert.Equal(t, abs, fields["idempotent_key"][0])
	for _, file := range files {
		if file.Path == "assets/p.png" {
			assert.Equal(t, []byte{0, 1, 2, 255}, file.Raw)
		}
	}
}
func TestPublish_DryRun(t *testing.T) {
	fixture(t)
	out, err := runHTML(t, Publish, []string{"+publish", "--file-path", "site/page.html", "--dry-run"}, nil)
	require.NoError(t, err)
	assert.Contains(t, out, publishPath)
	assert.Contains(t, out, "index.html")
	assert.Contains(t, out, "hash_tag")
	assert.NotContains(t, out, `"token":`)
	assert.NotContains(t, out, "console.log")
	assert.NotContains(t, out, "color: red")
}
func TestPublish_InvalidInput(t *testing.T) {
	for _, tc := range []struct{ name, entry, content string }{
		{"Missing", "absent.html", ""}, {"Empty", "empty.html", ""}, {"WrongExtension", "page.txt", "hello"},
		{"EmptyDependency", "page.html", `<script src="empty.js"></script>`},
		{"EntryConflict", "page.html", `<img src="index.html">`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmdutil.TestChdir(t, t.TempDir())
			if tc.name != "Missing" {
				writeFile(t, tc.entry, []byte(tc.content))
			}
			writeFile(t, "empty.js", nil)
			writeFile(t, "index.html", []byte("other"))
			_, err := runHTML(t, Publish, []string{"+publish", "--file-path", tc.entry}, nil)
			require.Error(t, err)
			p, ok := errs.ProblemOf(err)
			require.True(t, ok)
			assert.Equal(t, errs.CategoryValidation, p.Category)
		})
	}
}
func TestPublish_ResponseFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   any
	}{
		{"BusinessError", 200, map[string]any{"code": 99991403, "msg": "permission denied"}},
		{"HTTPError", 403, map[string]any{"code": 99991403, "msg": "permission denied"}},
		{"MissingData", 200, map[string]any{"code": 0, "data": map[string]any{}}},
		{"Malformed", 200, "unexpected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture(t)
			_, err := runHTML(t, Publish, []string{"+publish", "--file-path", "site/page.html"}, &httpmock.Stub{Method: "POST", URL: publishPath, Status: tc.status, Body: tc.body})
			require.Error(t, err)
			_, ok := errs.ProblemOf(err)
			assert.True(t, ok)
		})
	}
}
func archive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, err := z.Create("index.html")
	require.NoError(t, err)
	_, err = f.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, z.Close())
	return buf.Bytes()
}
func TestFetch_Output(t *testing.T) {
	for _, output := range []string{"", "downloads/source.zip"} {
		t.Run(strings.ReplaceAll(output, "/", "_"), func(t *testing.T) {
			cmdutil.TestChdir(t, t.TempDir())
			raw := archive(t)
			args := []string{"+fetch", "--token", "html_test"}
			expected := "html_test.zip"
			if output != "" {
				args = append(args, "--output", output)
				expected = output
			}
			out, err := runHTML(t, Fetch, args, &httpmock.Stub{Method: "GET", URL: fetchPath("html_test"), RawBody: raw, ContentType: "application/octet-stream"})
			require.NoError(t, err)
			saved, err := os.ReadFile(expected)
			require.NoError(t, err)
			assert.Equal(t, raw, saved)
			assert.Contains(t, out, `"size_bytes"`)
			_, err = os.Stat("index.html")
			assert.True(t, os.IsNotExist(err))
		})
	}
}
func TestFetch_DirectoryIsNotResolved(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	require.NoError(t, os.Mkdir("downloads", 0700))
	_, err := runHTML(t, Fetch, []string{"+fetch", "--token", "html_test", "--output", "downloads"}, &httpmock.Stub{Method: "GET", URL: fetchPath("html_test"), RawBody: archive(t)})
	require.Error(t, err)
	_, err = os.Stat("downloads/html_test.zip")
	assert.True(t, os.IsNotExist(err))
}
func TestFetch_ErrorDoesNotReplaceFile(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		raw    string
		ct     string
	}{
		{"JSONError", 200, `{"code":99991403,"msg":"permission denied"}`, "application/json"},
		{"MissingContentType", 200, `{"code":99991403,"msg":"permission denied"}`, ""},
		{"ZeroCodeJSON", 200, `{"code":0,"data":{}}`, "application/json"},
		{"HTTPError", 404, `{"code":404,"msg":"missing"}`, "application/json"},
		{"HTMLBody", 200, "<html>error</html>", "text/html"},
		{"EmptyBody", 200, "", "application/zip"},
		{"TruncatedSignature", 200, "PK\x03", "application/zip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmdutil.TestChdir(t, t.TempDir())
			writeFile(t, "existing.zip", []byte("keep"))
			_, err := runHTML(t, Fetch, []string{"+fetch", "--token", "html_test", "--output", "existing.zip"}, &httpmock.Stub{Method: "GET", URL: fetchPath("html_test"), Status: tc.status, RawBody: []byte(tc.raw), ContentType: tc.ct})
			require.Error(t, err)
			raw, e := os.ReadFile("existing.zip")
			require.NoError(t, e)
			assert.Equal(t, "keep", string(raw))
		})
	}
}
func TestFetch_InvalidFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--token", "../escape"}, {"--token", "https://example.test/page/token"}, {"--token", "."},
		{"--token", "token", "--output", "../source.zip"}, {"--token", "token", "--output", "/tmp/source.zip"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			_, err := runHTML(t, Fetch, append([]string{"+fetch"}, args...), nil)
			require.Error(t, err)
		})
	}
}
func TestFetch_DryRun(t *testing.T) {
	out, err := runHTML(t, Fetch, []string{"+fetch", "--token", "html_test", "--output", "downloads/src.zip", "--dry-run"}, nil)
	require.NoError(t, err)
	assert.Contains(t, out, fetchPath("html_test"))
	assert.Contains(t, out, "downloads/src.zip")
}
func TestHTML_RejectsBotIdentity(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-html", AppSecret: "test-secret", Brand: core.BrandLark})
	for _, sc := range Shortcuts() {
		t.Run(sc.Command, func(t *testing.T) {
			parent := &cobra.Command{Use: "html", SilenceErrors: true, SilenceUsage: true}
			sc.Mount(parent, f)
			args := []string{sc.Command, "--as", "bot"}
			if sc.Command == "+fetch" {
				args = append(args, "--token", "token")
			} else {
				args = append(args, "--file-path", "page.html")
			}
			parent.SetArgs(args)
			err := parent.ExecuteContext(context.Background())
			require.Error(t, err)
		})
	}
}

func TestHTMLBrandRouting(t *testing.T) {
	for _, brand := range []core.LarkBrand{core.BrandFeishu, core.BrandLark} {
		t.Run(string(brand), func(t *testing.T) {
			cmdutil.TestChdir(t, t.TempDir())
			stub := &httpmock.Stub{Method: "GET", URL: fetchPath("html_test"), RawBody: archive(t), OnMatch: func(req *http.Request) {
				assert.True(t, strings.HasPrefix(req.URL.String(), core.ResolveOpenBaseURL(brand)+"/open-apis/"))
			}}
			_, err := runHTML(t, Fetch, []string{"+fetch", "--token", "html_test"}, stub, brand)
			require.NoError(t, err)
		})
	}
}

type brokenArchiveReader struct{}

func (brokenArchiveReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type archiveTransport struct{ raw []byte }

func (tr archiveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/zip"}}, Body: io.NopCloser(io.MultiReader(bytes.NewReader(tr.raw), brokenArchiveReader{})), ContentLength: -1, Request: req}, nil
}
func TestFetch_PeekErrorDoesNotReplaceFile(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	writeFile(t, "existing.zip", []byte("keep"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-html", AppSecret: "test-secret", Brand: core.BrandFeishu, UserOpenId: "ou_test"})
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: archiveTransport{raw: []byte("PK")}}, nil
	}
	parent := &cobra.Command{Use: "html", SilenceErrors: true, SilenceUsage: true}
	Fetch.Mount(parent, f)
	parent.SetArgs([]string{"+fetch", "--token", "html_test", "--output", "existing.zip", "--as", "user"})
	err := parent.ExecuteContext(context.Background())
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	problem, ok := errs.ProblemOf(err)
	require.True(t, ok)
	assert.Equal(t, errs.SubtypeNetworkTransport, problem.Subtype)
	raw, readErr := os.ReadFile("existing.zip")
	require.NoError(t, readErr)
	assert.Equal(t, "keep", string(raw))
	entries, readErr := os.ReadDir(".")
	require.NoError(t, readErr)
	require.Len(t, entries, 1)
}

func TestFetch_InterruptedDownloadIsAtomic(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	writeFile(t, "existing.zip", []byte("keep"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-html", AppSecret: "test-secret", Brand: core.BrandFeishu, UserOpenId: "ou_test"})
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: archiveTransport{raw: []byte("PK\x03\x04partial")}}, nil
	}
	parent := &cobra.Command{Use: "html", SilenceErrors: true, SilenceUsage: true}
	Fetch.Mount(parent, f)
	parent.SetArgs([]string{"+fetch", "--token", "html_test", "--output", "existing.zip", "--as", "user"})
	err := parent.ExecuteContext(context.Background())
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	problem, ok := errs.ProblemOf(err)
	require.True(t, ok)
	assert.Equal(t, errs.SubtypeFileIO, problem.Subtype)
	raw, e := os.ReadFile("existing.zip")
	require.NoError(t, e)
	assert.Equal(t, "keep", string(raw))
	entries, e := os.ReadDir(".")
	require.NoError(t, e)
	require.Len(t, entries, 1)
}

func TestPublishPlanDoesNotOutliveInvocation(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(fmt.Sprint(dryRun), func(t *testing.T) {
			fixture(t)
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			factory, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-html", AppSecret: "test-secret", Brand: core.BrandFeishu, UserOpenId: "ou_test"})
			if !dryRun {
				reg.Register(successPublishStub())
			}
			parent := &cobra.Command{Use: "html", SilenceErrors: true, SilenceUsage: true}
			Publish.Mount(parent, factory)
			args := []string{"+publish", "--file-path", "site/page.html", "--as", "user"}
			if dryRun {
				args = append(args, "--dry-run")
			}
			parent.SetArgs(args)
			require.NoError(t, parent.ExecuteContext(context.Background()))
			child, _, err := parent.Find([]string{"+publish"})
			require.NoError(t, err)
			assert.Nil(t, child.Context().Value(publishPlanKey{}), "file snapshots must be released after execution and dry-run")
		})
	}
}

func TestPublish_TargetToken(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		name := "Multipart"
		if dryRun {
			name = "Dry_Run"
		}
		t.Run(name, func(t *testing.T) {
			fixture(t)
			const token = "HTML_Target_MixedCase"
			args := []string{"+publish", "--file-path", "site/page.html", "--token", token}
			var stub *httpmock.Stub
			if dryRun {
				args = append(args, "--dry-run")
			} else {
				stub = successPublishStub()
			}
			out, err := runHTML(t, Publish, args, stub)
			require.NoError(t, err)
			if dryRun {
				assert.Contains(t, out, `"token": "`+token+`"`)
				assert.Contains(t, out, "idempotent_key")
				return
			}
			_, params, err := mime.ParseMediaType(stub.CapturedHeaders.Get("Content-Type"))
			require.NoError(t, err)
			reader := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
			form, err := reader.ReadForm(maxPublishBytes)
			require.NoError(t, err)
			defer form.RemoveAll()
			assert.Equal(t, []string{token}, form.Value["token"])
			assert.Len(t, form.Value["idempotent_key"], 1)
			assert.Len(t, form.File["files"], 4)
		})
	}
}
