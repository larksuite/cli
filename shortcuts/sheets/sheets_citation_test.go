// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestWorkbookReadCitations(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, sc := range []common.Shortcut{WorkbookInfo, CsvGet} {
		for _, tc := range []struct {
			name, gate, payload, wantURL, wantTitle, wantTime string
			tokenOnly                                         bool
		}{
			{name: "native metadata", gate: "1", payload: `{"title":"A & B","url":"https://example.feishu.cn/sheets/native","modified_time":"2026-10-22T01:02:03Z"}`, wantURL: "https://example.feishu.cn/sheets/native", wantTitle: "A &amp; B", wantTime: "2026-10-22T01:02:03Z"},
			{name: "input URL", gate: "1", payload: `{"title":"Workbook","modified_time":"2026-10-22T01:02:03Z"}`, wantURL: testURL, wantTitle: "Workbook", wantTime: "2026-10-22T01:02:03Z"},
			{name: "token with native URL", gate: "1", payload: `{"title":"Workbook","url":"https://example.feishu.cn/sheets/native"}`, tokenOnly: true, wantURL: "https://example.feishu.cn/sheets/native", wantTitle: "Workbook"},
			{name: "metadata unavailable", gate: "1", payload: `{}`, wantURL: testURL},
			{name: "token without URL", gate: "1", payload: `{"title":"Workbook"}`, tokenOnly: true},
			{name: "disabled", gate: "0", payload: `{"title":"Workbook","url":"https://example.feishu.cn/sheets/native"}`},
			{name: "unset", payload: `{}`},
		} {
			t.Run(sc.Command+"/"+tc.name, func(t *testing.T) {
				t.Setenv(envvars.CliCitation, tc.gate)
				args := []string{"--url", testURL}
				if tc.tokenOnly {
					args = []string{"--spreadsheet-token", testToken}
				}
				if sc.Command == CsvGet.Command {
					args = append(args, "--sheet-id", testSheetID, "--range", "A1")
				}
				out, err := runShortcutWithStubs(t, sc, args, toolOutputStub(testToken, "read", tc.payload))
				if err != nil {
					t.Fatalf("execute: %v\n%s", err, out)
				}
				var env struct {
					Citations []string `json:"citations"`
				}
				if err := json.Unmarshal([]byte(out), &env); err != nil {
					t.Fatal(err)
				}
				var want []string
				if tc.wantURL != "" {
					doc := `<document reference_id="` + tc.wantURL + `"><title>` + tc.wantTitle + `</title><source_type>6</source_type><url>` + tc.wantURL + `</url>`
					if tc.wantTime != "" {
						doc += `<publish_time>` + tc.wantTime + `</publish_time>`
					}
					want = []string{doc + `</document>`}
				}
				if !reflect.DeepEqual(env.Citations, want) {
					t.Fatalf("citations = %#v, want %#v", env.Citations, want)
				}
			})
		}
	}
}

func TestCsvGetCitationsSurvivePaginationAndOffload(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv(envvars.CliCitation, "1")
	const nativeURL = testURL + "?sheet=" + testSheetID
	for _, mode := range []string{"inline", "output-path", "auto-offload"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:A2"}
			path := ""
			if mode == "output-path" {
				// Keep user output inside the invocation workspace on every OS.
				dir, err := os.MkdirTemp(".", "csv-citation-")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.RemoveAll(dir) })
				path = filepath.Join(dir, "read.json")
				args = append(args, "--output-path", path)
			}
			value := "first"
			if mode == "auto-offload" {
				value = strings.Repeat("x", stdoutInlineLimit+1)
			}
			first, err := json.Marshal(map[string]interface{}{
				"title": "Workbook", "url": nativeURL, "modified_time": "2026-10-22T01:02:03Z",
				"actual_range": "A1:A1", "row_count": 1, "has_more": true, "annotated_csv": "[row=1] " + value,
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := runShortcutWithStubs(t, CsvGet, args,
				toolOutputStub(testToken, "read", string(first)),
				toolOutputStub(testToken, "read", `{"actual_range":"A2:A2","row_count":1,"has_more":false,"annotated_csv":"[row=2] second"}`))
			if err != nil {
				t.Fatalf("execute: %v\n%s", err, out)
			}
			var env struct {
				Citations []string               `json:"citations"`
				Data      map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal([]byte(out), &env); err != nil {
				t.Fatal(err)
			}
			want := `<document reference_id="` + nativeURL + `"><title>Workbook</title><source_type>6</source_type><url>` + nativeURL + `</url><publish_time>2026-10-22T01:02:03Z</publish_time></document>`
			if len(env.Citations) != 1 || env.Citations[0] != want {
				t.Fatalf("citations = %#v, want %q", env.Citations, want)
			}
			data := env.Data
			if mode != "inline" {
				if data["url"] != nativeURL {
					t.Fatalf("receipt url = %#v, want native URL %q", data["url"], nativeURL)
				}
				path, _ = data["output_path"].(string)
				if path == "" {
					t.Fatal("missing offload path")
				}
				if mode == "auto-offload" {
					t.Cleanup(func() { _ = os.Remove(path) })
				}
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(b, &data); err != nil {
					t.Fatal(err)
				}
			}
			if data["csv"] != value+"\nsecond" || data["complete"] != true || data["batches"] != float64(2) {
				t.Fatal("paginated CSV data or completeness changed")
			}
		})
	}
}
