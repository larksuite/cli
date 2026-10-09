// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

type messageExportProvider struct{ fio fileio.FileIO }

func (p messageExportProvider) Name() string                                { return "message-export-test" }
func (p messageExportProvider) ResolveFileIO(context.Context) fileio.FileIO { return p.fio }

type messageExportFileInfo struct{ size int64 }

func (i messageExportFileInfo) Size() int64       { return i.size }
func (i messageExportFileInfo) IsDir() bool       { return false }
func (i messageExportFileInfo) Mode() fs.FileMode { return 0600 }

type messageExportMemoryIO struct {
	files      map[string][]byte
	saveErr    error
	resolveErr error
	options    fileio.SaveOptions
	nilResult  bool
}

func (f *messageExportMemoryIO) Open(string) (fileio.File, error) { return nil, fs.ErrNotExist }
func (f *messageExportMemoryIO) Stat(name string) (fileio.FileInfo, error) {
	if b, ok := f.files[name]; ok {
		return messageExportFileInfo{int64(len(b))}, nil
	}
	return nil, fs.ErrNotExist
}
func (f *messageExportMemoryIO) ResolvePath(name string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	return "workspace://" + name, nil
}
func (f *messageExportMemoryIO) Save(name string, opts fileio.SaveOptions, r io.Reader) (fileio.SaveResult, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if f.files == nil {
		f.files = make(map[string][]byte)
	}
	f.files[name] = b
	f.options = opts
	if f.nilResult {
		return nil, nil
	}
	return messageExportFileInfo{int64(len(b))}, nil
}
func (f *messageExportMemoryIO) SaveExclusive(name string, opts fileio.SaveOptions, r io.Reader) (fileio.SaveResult, error) {
	if _, exists := f.files[name]; exists {
		return nil, fs.ErrExist
	}
	return f.Save(name, opts, r)
}

func newMessageExportCommand(t *testing.T, shortcut common.Shortcut, fio fileio.FileIO, args ...string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	factory, stdout, stderr, registry := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})
	factory.FileIOProvider = messageExportProvider{fio}
	root := &cobra.Command{Use: "lark-cli", SilenceErrors: true, SilenceUsage: true}
	service := &cobra.Command{Use: "im"}
	root.AddCommand(service)
	shortcut.Mount(service, factory)
	baseArgs := []string{"im", shortcut.Command, "--as", "user", "--no-reactions", "--page-delay", "0"}
	if shortcut.Command == ImChatMessageList.Command {
		baseArgs = append(baseArgs, "--chat-id", "oc_test")
	} else {
		baseArgs = append(baseArgs, "--thread", "omt_test")
	}
	root.SetArgs(append(baseArgs, args...))
	return root, stdout, stderr, registry
}

func messageExportPage(registry *httpmock.Registry, id string, more bool, token string) *httpmock.Stub {
	items := []interface{}{}
	if id != "" {
		items = append(items, map[string]interface{}{
			"message_id": id, "msg_type": "text", "create_time": "0",
			"body": map[string]interface{}{"content": `{"text":"Complete message body <test> 中文"}`},
		})
	}
	stub := &httpmock.Stub{Method: http.MethodGet, URL: "/open-apis/im/v1/messages?", Body: map[string]interface{}{
		"code": 0, "data": map[string]interface{}{"items": items, "has_more": more, "page_token": token},
	}}
	registry.Register(stub)
	return stub
}

func TestMessageExportMatchesStdout(t *testing.T) {
	formats := []struct {
		name             string
		args             []string
		ext, contentType string
	}{
		{"json", nil, ".json", "application/json"},
		{"ndjson", []string{"--format", "ndjson"}, ".ndjson", "application/x-ndjson"},
		{"csv", []string{"--format", "csv"}, ".csv", "text/csv; charset=utf-8"},
		{"table", []string{"--format", "table"}, ".txt", "text/plain; charset=utf-8"},
		{"pretty", []string{"--format", "pretty"}, ".txt", "text/plain; charset=utf-8"},
		{"markdown", []string{"--concise"}, ".md", "text/markdown; charset=utf-8"},
		{"jq", []string{"--jq", ".data.messages[].message_id"}, ".txt", "text/plain; charset=utf-8"},
	}
	for _, shortcut := range []common.Shortcut{ImChatMessageList, ImThreadsMessagesList} {
		for _, format := range formats {
			t.Run(shortcut.Command+"/"+format.name, func(t *testing.T) {
				plain, want, _, registry := newMessageExportCommand(t, shortcut, nil, format.args...)
				messageExportPage(registry, "om_first", false, "")
				if err := plain.Execute(); err != nil {
					t.Fatal(err)
				}
				fio := &messageExportMemoryIO{}
				args := append(append([]string{}, format.args...), "--output-dir", "exports")
				cmd, stdout, _, registry := newMessageExportCommand(t, shortcut, fio, args...)
				messageExportPage(registry, "om_first", false, "")
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				id := "oc_test"
				if shortcut.Command == ImThreadsMessagesList.Command {
					id = "omt_test"
				}
				path := "exports/" + id + format.ext
				if !bytes.Equal(fio.files[path], want.Bytes()) {
					t.Fatalf("saved output differs from stdout:\n%s\nwant:\n%s", fio.files[path], want.Bytes())
				}
				var envelope struct {
					OK   bool                 `json:"ok"`
					Data messageExportSummary `json:"data"`
					Meta output.Meta          `json:"meta"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if !envelope.OK || envelope.Data.SavedPath != "workspace://"+path || envelope.Data.Total != 1 || envelope.Data.HasMore || envelope.Data.Format != format.name {
					t.Fatalf("summary = %+v", envelope)
				}
				if envelope.Data.SizeBytes != int64(want.Len()) || fio.options.ContentLength != int64(want.Len()) || fio.options.ContentType != format.contentType {
					t.Fatalf("size or MIME mismatch: %+v, %+v", envelope.Data, fio.options)
				}
				if strings.Contains(stdout.String(), "Complete message body") {
					t.Fatal("message content leaked to summary")
				}
				if envelope.Meta.Pagination == nil || !envelope.Meta.Pagination.Complete {
					t.Fatalf("missing pagination: %+v", envelope.Meta)
				}
			})
		}
	}
}

func TestMessageExportPaginationAndEmptyResult(t *testing.T) {
	for _, test := range []struct {
		name  string
		limit int
		empty bool
	}{
		{"complete", 2, false}, {"limited", 1, false}, {"empty", 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fio := &messageExportMemoryIO{}
			args := []string{"--output", "messages.json", "--page-all"}
			if test.limit == 1 {
				args = append(args, "--page-limit", "1")
			}
			cmd, stdout, _, registry := newMessageExportCommand(t, ImChatMessageList, fio, args...)
			if test.empty {
				messageExportPage(registry, "", false, "")
			} else {
				messageExportPage(registry, "om_first", true, "next-page")
				if test.limit == 2 {
					messageExportPage(registry, "om_second", false, "").OnMatch = func(req *http.Request) {
						if req.URL.Query().Get("page_token") != "next-page" {
							t.Errorf("missing cursor: %s", req.URL)
						}
					}
				}
			}
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var summary struct {
				Data messageExportSummary
				Meta output.Meta
			}
			if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
				t.Fatal(err)
			}
			var saved struct {
				Data struct {
					Messages  []map[string]interface{}
					Total     int
					HasMore   bool   `json:"has_more"`
					PageToken string `json:"page_token"`
				}
				Meta output.Meta
			}
			if err := json.Unmarshal(fio.files["messages.json"], &saved); err != nil {
				t.Fatal(err)
			}
			want := test.limit
			if test.empty {
				want = 0
			}
			if len(saved.Data.Messages) != want || summary.Data.Total != want {
				t.Fatalf("message counts: saved=%d summary=%d want=%d", len(saved.Data.Messages), summary.Data.Total, want)
			}
			if saved.Meta.Pagination.Complete != (test.limit == 2) || saved.Data.HasMore != summary.Data.HasMore || saved.Data.PageToken != summary.Data.PageToken {
				t.Fatalf("pagination: saved=%+v summary=%+v", saved, summary)
			}
			if test.limit == 1 && summary.Data.PageToken != "next-page" {
				t.Fatalf("lost cursor: %+v", summary.Data)
			}
		})
	}
}

func TestMessageExportValidationDoesNotFetchOrSave(t *testing.T) {
	for _, test := range []struct {
		name      string
		args      []string
		pathError bool
		param     string
	}{
		{"conflict", []string{"--output", "x.json", "--output-dir", "exports"}, false, ""},
		{"empty", []string{"--output", ""}, false, "--output"},
		{"blank directory", []string{"--output-dir", "  "}, false, "--output-dir"},
		{"overwrite alone", []string{"--overwrite"}, false, "--overwrite"},
		{"unsafe", []string{"--output", "../private.json"}, true, "--output"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fio := &messageExportMemoryIO{}
			if test.pathError {
				fio.resolveErr = &fileio.PathValidationError{Err: errors.New("outside workspace")}
			}
			cmd, stdout, _, _ := newMessageExportCommand(t, ImChatMessageList, fio, test.args...)
			err := cmd.Execute()
			var validation *errs.ValidationError
			if !errors.As(err, &validation) || validation.Param != test.param {
				t.Fatalf("error = %#v", err)
			}
			if test.pathError && !errors.Is(err, fileio.ErrPathValidation) {
				t.Fatal("path error cause lost")
			}
			if len(fio.files) != 0 || stdout.Len() != 0 {
				t.Fatal("failed validation produced output")
			}
		})
	}
}

func TestMessageExportExistingFileAndCommitRace(t *testing.T) {
	for _, mode := range []string{"existing", "race", "overwrite"} {
		t.Run(mode, func(t *testing.T) {
			fio := &messageExportMemoryIO{files: map[string][]byte{"messages.json": []byte("original")}}
			args := []string{"--output", "messages.json"}
			if mode == "overwrite" {
				args = append(args, "--overwrite")
			}
			if mode == "race" {
				delete(fio.files, "messages.json")
			}
			cmd, stdout, _, registry := newMessageExportCommand(t, ImChatMessageList, fio, args...)
			if mode != "existing" {
				messageExportPage(registry, "om_first", false, "").OnMatch = func(*http.Request) {
					if mode == "race" {
						fio.files["messages.json"] = []byte("original")
					}
				}
			}
			err := cmd.Execute()
			if mode == "overwrite" {
				if err != nil || string(fio.files["messages.json"]) == "original" {
					t.Fatalf("overwrite failed: %v", err)
				}
			} else {
				var validation *errs.ValidationError
				if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeFailedPrecondition || !errors.Is(err, fs.ErrExist) {
					t.Fatalf("error = %v", err)
				}
				if string(fio.files["messages.json"]) != "original" || stdout.Len() != 0 {
					t.Fatal("existing file changed or false success emitted")
				}
			}
		})
	}
}

func TestMessageExportFailures(t *testing.T) {
	for _, mode := range []string{"save", "nil result", "jq", "api", "later page"} {
		t.Run(mode, func(t *testing.T) {
			fio := &messageExportMemoryIO{}
			sentinel := errors.New("save unavailable")
			if mode == "save" {
				fio.saveErr = sentinel
			}
			fio.nilResult = mode == "nil result"
			args := []string{"--output", "messages.json"}
			if mode == "jq" {
				args = append(args, "--jq", `error("bad filter")`)
			}
			if mode == "later page" {
				args = append(args, "--page-all")
			}
			cmd, stdout, _, registry := newMessageExportCommand(t, ImChatMessageList, fio, args...)
			stub := messageExportPage(registry, "om_first", mode == "later page", "next-page")
			if mode == "later page" {
				stub = messageExportPage(registry, "", false, "")
			}
			if mode == "api" || mode == "later page" {
				stub.Body = map[string]interface{}{"code": 230001, "msg": "invalid request"}
			}
			err := cmd.Execute()
			if err == nil || stdout.Len() != 0 {
				t.Fatalf("expected error without summary, got %v: %s", err, stdout)
			}
			if _, typed := errs.ProblemOf(err); !typed {
				t.Fatalf("untyped error: %v", err)
			}
			if mode == "save" && !errors.Is(err, sentinel) {
				t.Fatal("save cause lost")
			}
			if mode != "nil result" && len(fio.files) != 0 {
				t.Fatal("failed request/render/save created an export")
			}
		})
	}
}

func TestMessageExportResolvedIDs(t *testing.T) {
	for _, shortcut := range []common.Shortcut{ImChatMessageList, ImThreadsMessagesList} {
		for _, unsafe := range []bool{false, true} {
			t.Run(shortcut.Command+map[bool]string{false: "/resolved", true: "/unsafe"}[unsafe], func(t *testing.T) {
				fio := &messageExportMemoryIO{}
				id := "oc_resolved"
				args := []string{"--chat-id", "", "--user-id", "ou_test"}
				lookup := &httpmock.Stub{Method: http.MethodPost, URL: "/open-apis/im/v1/chat_p2p/batch_query"}
				if shortcut.Command == ImThreadsMessagesList.Command {
					id = "omt_resolved"
					args = []string{"--thread", "om_test"}
					lookup = &httpmock.Stub{Method: http.MethodGet, URL: "/open-apis/im/v1/messages/om_test"}
				}
				if unsafe {
					id += "/../../outside"
				}
				data := map[string]interface{}{"p2p_chats": []interface{}{map[string]interface{}{"chat_id": id}}}
				if shortcut.Command == ImThreadsMessagesList.Command {
					data = map[string]interface{}{"items": []interface{}{map[string]interface{}{"thread_id": id}}}
				}
				lookup.Body = map[string]interface{}{"code": 0, "data": data}
				cmd, stdout, _, registry := newMessageExportCommand(t, shortcut, fio, append(args, "--output-dir", "exports")...)
				registry.Register(lookup)
				if !unsafe {
					messageExportPage(registry, "om_first", false, "").OnMatch = func(req *http.Request) {
						if req.URL.Query().Get("container_id") != id {
							t.Errorf("wrong resolved container: %s", req.URL)
						}
					}
				}
				err := cmd.Execute()
				if unsafe {
					var validation *errs.ValidationError
					if !errors.As(err, &validation) || validation.Param != "--output-dir" || len(fio.files) != 0 || stdout.Len() != 0 {
						t.Fatalf("unsafe filename was not rejected before fetch/save: %v", err)
					}
				} else if err != nil || len(fio.files["exports/"+id+".json"]) == 0 {
					t.Fatalf("resolved export missing: files=%v err=%v", fio.files, err)
				}
			})
		}
	}
}

func TestMessageExportProviderWithoutExclusiveWrites(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		fio := &messageExportMemoryIO{}
		// Expose only the required FileIO interface, hiding SaveExclusive.
		providerIO := struct{ fileio.FileIO }{fio}
		args := []string{"--output", "messages.json"}
		if overwrite {
			args = append(args, "--overwrite")
		}
		cmd, stdout, _, registry := newMessageExportCommand(t, ImChatMessageList, providerIO, args...)
		if overwrite {
			messageExportPage(registry, "om_first", false, "")
		}
		err := cmd.Execute()
		if overwrite {
			if err != nil || len(fio.files["messages.json"]) == 0 {
				t.Fatalf("explicit overwrite failed: %v", err)
			}
		} else {
			var validation *errs.ValidationError
			if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeFailedPrecondition || validation.Param != "--output" {
				t.Fatalf("expected unsupported no-clobber policy: %v", err)
			}
			if len(fio.files) != 0 || stdout.Len() != 0 {
				t.Fatal("unsupported policy wrote output")
			}
		}
	}
}

type blockingMessageExportScanner struct{}

func (blockingMessageExportScanner) Name() string { return "message-export-test" }
func (blockingMessageExportScanner) Scan(context.Context, extcs.ScanRequest) (*extcs.Alert, error) {
	return &extcs.Alert{Provider: "message-export-test", MatchedRules: []string{"blocked"}}, nil
}

func TestMessageExportScansBeforeSaving(t *testing.T) {
	previous := extcs.GetProvider()
	extcs.Register(blockingMessageExportScanner{})
	t.Cleanup(func() { extcs.Register(previous) })
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	for _, args := range [][]string{nil, {"--concise"}, {"--format", "ndjson"}, {"--jq", ".data.messages"}} {
		fio := &messageExportMemoryIO{}
		cmd, stdout, _, registry := newMessageExportCommand(t, ImChatMessageList, fio, append(args, "--output", "messages.txt")...)
		messageExportPage(registry, "om_first", false, "")
		var blocked *errs.ContentSafetyError
		if err := cmd.Execute(); !errors.As(err, &blocked) {
			t.Fatalf("expected content safety failure: %v", err)
		}
		if len(fio.files) != 0 || stdout.Len() != 0 {
			t.Fatal("blocked content escaped to disk/stdout")
		}
	}
}

func TestMessageExportDryRun(t *testing.T) {
	for _, shortcut := range []common.Shortcut{ImChatMessageList, ImThreadsMessagesList} {
		id, containerType := "oc_test", "chat"
		if shortcut.Command == ImThreadsMessagesList.Command {
			id, containerType = "omt_test", "thread"
		}
		fio := &messageExportMemoryIO{}
		cmd, stdout, _, _ := newMessageExportCommand(t, shortcut, fio, "--output-dir", "exports", "--overwrite", "--page-all", "--dry-run")
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Data struct {
				Files []cmdutil.DryRunFileIntent
				Calls []cmdutil.DryRunAPICall `json:"api"`
			}
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if len(envelope.Data.Files) != 1 || envelope.Data.Files[0].IfExists != "overwrite" || envelope.Data.Files[0].Name != "exports/"+id+".json" {
			t.Fatalf("files = %+v", envelope.Data.Files)
		}
		if len(envelope.Data.Calls) != 1 || envelope.Data.Calls[0].Method != "GET" || !strings.Contains(envelope.Data.Calls[0].URL, "/open-apis/im/v1/messages") {
			t.Fatalf("calls = %+v", envelope.Data.Calls)
		}
		params := envelope.Data.Calls[0].Params
		if params["container_id"] != id || params["container_id_type"] != containerType || envelope.Data.Calls[0].Body != nil {
			t.Fatalf("incorrect request: %+v", envelope.Data.Calls[0])
		}
		if len(fio.files) != 0 {
			t.Fatal("dry-run wrote an export")
		}
	}
}

func TestMessageExportLocalFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	cmd, stdout, _, registry := newMessageExportCommand(t, ImChatMessageList, &localfileio.LocalFileIO{}, "--output", "exports/chat.json")
	messageExportPage(registry, "om_first", false, "")
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "exports", "chat.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(content) || !strings.Contains(string(content), "Complete message body") {
		t.Fatalf("invalid export: %s", content)
	}
	var summary struct{ Data messageExportSummary }
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Data.SizeBytes != int64(len(content)) {
		t.Fatalf("wrong byte count: %+v", summary.Data)
	}
}
