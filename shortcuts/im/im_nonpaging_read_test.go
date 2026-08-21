// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/shortcuts/common"
)

// nonPagingReadCase describes one non-paginating IM read shortcut and the
// minimal environment needed to drive it end to end.
type nonPagingReadCase struct {
	command  string
	shortcut common.Shortcut
	flags    map[string]string
	// chdirTemp switches the working directory to t.TempDir() for commands
	// that write files.
	chdirTemp bool
	respond   func(req *http.Request) (*http.Response, error)
	// wantDataKey must exist in envelope.data, proving the payload actually
	// passed through rather than an empty shell envelope being emitted.
	wantDataKey string
}

func nonPagingReadCases() []nonPagingReadCase {
	return []nonPagingReadCase{
		{
			command:  "+messages-mget",
			shortcut: ImMessagesMGet,
			flags:    map[string]string{"message-ids": "om_probe", "no-reactions": "true"},
			respond: func(*http.Request) (*http.Response, error) {
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items": []interface{}{map[string]interface{}{
							"message_id": "om_probe",
							"msg_type":   "text",
							"body":       map[string]interface{}{"content": `{"text":"hello"}`},
						}},
					},
				}), nil
			},
			wantDataKey: "messages",
		},
		{
			command:  "+feed-group-query-item",
			shortcut: ImFeedGroupQueryItem,
			flags:    map[string]string{"feed-group-id": "ofg_probe", "feed-id": "oc_probe"},
			respond: func(*http.Request) (*http.Response, error) {
				return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items": []interface{}{map[string]interface{}{
							"feed_id":   "oc_probe",
							"feed_type": "chat",
						}},
					},
				}), nil
			},
			wantDataKey: "items",
		},
		{
			command:   "+messages-resources-download",
			shortcut:  ImMessagesResourcesDownload,
			flags:     map[string]string{"message-id": "om_probe", "file-key": "img_probe", "type": "image"},
			chdirTemp: true,
			respond: func(*http.Request) (*http.Response, error) {
				return shortcutRawResponse(http.StatusOK, []byte("probe-bytes"), http.Header{
					"Content-Type": []string{"image/png"},
				}), nil
			},
			wantDataKey: "saved_path",
		},
	}
}

// readRuntimeOutputErr reads the private outputErr, which decides the process
// exit code. Asserting only on stdout would miss a valid envelope paired with
// a non-zero exit.
func readRuntimeOutputErr(t *testing.T, runtime *common.RuntimeContext) error {
	t.Helper()
	field := reflect.ValueOf(runtime).Elem().FieldByName("outputErr")
	if !field.IsValid() {
		t.Fatal("RuntimeContext has no outputErr field")
	}
	value := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	if value == nil {
		return nil
	}
	err, _ := value.(error)
	return err
}

// attachIMReadSession attaches the command's real contract read session.
func attachIMReadSession(t *testing.T, runtime *common.RuntimeContext, command string) {
	t.Helper()
	contract, ok := imcontract.Lookup(imcontract.ContractKey("im " + command))
	if !ok {
		t.Fatalf("no IM contract for %s", command)
	}
	session, err := imcontract.NewReadSession(contract, imcontract.ReadOptions{})
	if err != nil {
		t.Fatalf("NewReadSession() error = %v", err)
	}
	setRuntimeField(t, runtime, "readSession", session)
}

// TestNonPagingIMReadsEmitValidEnvelope drives every non-paginating IM read
// through its real contract session and a mock transport, asserting it emits a
// valid envelope and exits zero. --dry-run is deliberately not used: it skips
// emitFinalized, which is exactly the path this regression lived on.
func TestNonPagingIMReadsEmitValidEnvelope(t *testing.T) {
	for _, tc := range nonPagingReadCases() {
		t.Run(tc.command, func(t *testing.T) {
			if tc.chdirTemp {
				cmdutil.TestChdir(t, t.TempDir())
			}
			runtime := newUserShortcutRuntime(t, shortcutRoundTripFunc(tc.respond))
			runtime.Cmd = newListPageAllCommand(t, tc.shortcut, tc.flags)
			runtime.Format = "json"
			attachIMReadSession(t, runtime, tc.command)

			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v, want nil", err)
			}
			if err := readRuntimeOutputErr(t, runtime); err != nil {
				t.Fatalf("outputErr = %v, want nil (non-zero exit despite a successful call)", err)
			}

			out, ok := runtime.IO().Out.(*bytes.Buffer)
			if !ok {
				t.Fatal("stdout is not a bytes.Buffer")
			}
			stdout := out.String()
			if strings.TrimSpace(stdout) == "" {
				t.Fatal("stdout is empty; the command produced no envelope at all")
			}
			var envelope map[string]interface{}
			if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
				t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
			}
			if ok, _ := envelope["ok"].(bool); !ok {
				t.Fatalf("envelope ok = false, want true: %s", stdout)
			}
			data, _ := envelope["data"].(map[string]interface{})
			if _, exists := data[tc.wantDataKey]; !exists {
				t.Fatalf("envelope data has no %q; payload did not pass through: %s", tc.wantDataKey, stdout)
			}
		})
	}
}

// imNonPagingReadShortcuts derives the non-paginating read shortcut set from
// the registry rather than a hand-written list.
func imNonPagingReadShortcuts(t *testing.T) []common.Shortcut {
	t.Helper()
	var out []common.Shortcut
	for _, sc := range Shortcuts() {
		contract, ok := imcontract.Lookup(imcontract.ContractKey(sc.Service + " " + sc.Command))
		if !ok || !contract.Strategy.Kind.IsRead() {
			continue
		}
		session, err := imcontract.NewReadSession(contract, imcontract.ReadOptions{})
		if err != nil {
			t.Fatalf("NewReadSession(%s) error = %v", sc.Command, err)
		}
		if !session.RequiresPagination() {
			out = append(out, sc)
		}
	}
	return out
}

// TestNonPagingIMReadCoverageIsComplete keeps the case table in sync with the
// registry. Adding a non-paginating read shortcut fails this test until a case
// is added, so the class cannot silently regress again.
func TestNonPagingIMReadCoverageIsComplete(t *testing.T) {
	var want []string
	for _, sc := range imNonPagingReadShortcuts(t) {
		want = append(want, sc.Command)
	}
	if len(want) == 0 {
		t.Fatal("no non-paginating read shortcuts found; the assertion would be vacuous")
	}
	var got []string
	for _, tc := range nonPagingReadCases() {
		got = append(got, tc.command)
	}
	sort.Strings(want)
	sort.Strings(got)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("non-paginating read coverage drifted:\n  registry: %v\n  test table: %v\nadd the missing command to nonPagingReadCases()", want, got)
	}
}

// TestNonPagingIMReadsDeclareNoPaginationFlags guards against a mis-registered
// contract kind. A command that genuinely paginates registers pagination
// flags; declaring no pagination while carrying them signals the kind is
// wrong, which would silently skip that command's truncation facts.
func TestNonPagingIMReadsDeclareNoPaginationFlags(t *testing.T) {
	for _, sc := range imNonPagingReadShortcuts(t) {
		t.Run(sc.Command, func(t *testing.T) {
			for _, flag := range sc.Flags {
				if flag.Name == "page-all" || flag.Name == "page-limit" {
					t.Errorf("%s is registered as a non-paginating read contract but declares --%s; the contract kind is likely mis-registered", sc.Command, flag.Name)
				}
			}
		})
	}
}
