// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/workflowrun"
	"github.com/larksuite/cli/shortcuts/common"
)

func runLogFixture(t *testing.T) *workflowrun.Fixture {
	t.Helper()
	b, e := os.ReadFile("../../internal/workflowrun/testdata/basic.json")
	if e != nil {
		t.Fatal(e)
	}
	var f workflowrun.Fixture
	if e = json.Unmarshal(b, &f); e != nil {
		t.Fatal(e)
	}
	return &f
}
func runLogArgs(command string) []string {
	return []string{command, "--base-token", "base-fixture", "--workflow-id", "wf1"}
}
func TestWorkflowRunCommandPipeline(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
		want     int
	}{{"list", BaseWorkflowRunList, []string{"--start-time", "0", "--end-time", "172800000"}, 2}, {"get", BaseWorkflowRunGet, []string{"--run-id", "E1"}, 2}, {"button", BaseWorkflowButtonLogList, []string{"--run-id", "E1", "--button-key", "message_btn_12345678"}, 4}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			b, _ := json.Marshal(runLogFixture(t))
			args := append(runLogArgs(tt.shortcut.Command), tt.args...)
			args = append(args, "--mock-fixture", string(b))
			if e := runShortcut(t, tt.shortcut, args, factory, stdout); e != nil {
				t.Fatal(e)
			}
			var envelope struct {
				OK   bool               `json:"ok"`
				Data workflowrun.Result `json:"data"`
			}
			if e := json.Unmarshal(stdout.Bytes(), &envelope); e != nil {
				t.Fatal(e)
			}
			if envelope.Data.DataSource != "mock:fixture:workflow-run-log-basic" {
				t.Fatalf("missing fixture source %s", stdout)
			}
			for _, i := range envelope.Data.Items {
				if i.DataSource != envelope.Data.DataSource {
					t.Fatal("record source not marked")
				}
			}
			for _, p := range envelope.Data.Pages {
				if p.DataSource != envelope.Data.DataSource {
					t.Fatal("page source not marked")
				}
			}
			if !envelope.OK || envelope.Data.Total != tt.want {
				t.Fatalf("output %s", stdout)
			}
			if tt.name == "get" && len(envelope.Data.Items[0].Groups) != 2 {
				t.Fatal("button indexes missing without payload")
			}
		})
	}
}
func TestWorkflowRunDryRun(t *testing.T) {
	for _, shortcut := range []common.Shortcut{BaseWorkflowRunList, BaseWorkflowRunGet, BaseWorkflowButtonLogList, BaseWorkflowRunExport} {
		t.Run(shortcut.Command, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			args := runLogArgs(shortcut.Command)
			if shortcut.Command == BaseWorkflowRunGet.Command || shortcut.Command == BaseWorkflowButtonLogList.Command {
				args = append(args, "--run-id", "E1")
			}
			if shortcut.Command == BaseWorkflowButtonLogList.Command {
				args = append(args, "--button-key", "message_btn_12345678")
			}
			if shortcut.Command == BaseWorkflowRunExport.Command {
				args = append(args, "--output", "dry-run-never-written")
			}
			args = append(args, "--dry-run")
			if e := runShortcut(t, shortcut, args, factory, stdout); e != nil {
				t.Fatal(e)
			}
			if !strings.Contains(stdout.String(), "mock_only") || !strings.Contains(stdout.String(), "GET") {
				t.Fatalf("plan %s", stdout)
			}
			if shortcut.Risk != "read" {
				t.Fatal("not read-only")
			}
		})
	}
}
func TestWorkflowRunExportExitMatrix(t *testing.T) {
	for _, tt := range []struct {
		name     string
		coverage string
		fault    *workflowrun.Fault
		max      string
		require  bool
		want     int
	}{{"AC15_guaranteed", workflowrun.Guaranteed, nil, "", true, 0}, {"AC16_expired", workflowrun.Guaranteed, &workflowrun.Fault{Plane: workflowrun.Runs, Result: "data_expired", Count: -1}, "", true, 1}, {"AC17_unverified", workflowrun.Unverified, nil, "", true, 1}, {"AC07_permission", workflowrun.Guaranteed, &workflowrun.Fault{Plane: workflowrun.Runs, HTTP: 403, Count: -1}, "", false, 3}, {"AC07_request", workflowrun.Guaranteed, &workflowrun.Fault{Plane: workflowrun.Runs, HTTP: 503, Count: -1}, "", false, 4}, {"AC06_max_records", workflowrun.Guaranteed, nil, "1", false, 7}} {
		t.Run(tt.name, func(t *testing.T) {
			f := runLogFixture(t)
			dir := t.TempDir()
			withBaseWorkingDir(t, dir)
			factory, stdout, _ := newExecuteFactory(t)
			f.Coverage = tt.coverage
			if tt.fault != nil {
				f.Faults = []workflowrun.Fault{*tt.fault}
			}
			b, _ := json.Marshal(f)
			args := append(runLogArgs(BaseWorkflowRunExport.Command), "--start-time", "0", "--end-time", "172800000", "--output", "export", "--mock-fixture", string(b))
			if tt.require {
				args = append(args, "--require-complete")
			}
			if tt.max != "" {
				args = append(args, "--max-records", tt.max)
			}
			e := runShortcut(t, BaseWorkflowRunExport, args, factory, stdout)
			code := 0
			if e != nil {
				code = output.ExitCodeOf(e)
			}
			if code != tt.want {
				t.Fatalf("code=%d want=%d err=%v stdout=%s", code, tt.want, e, stdout)
			}
			manifestBytes, _ := os.ReadFile(filepath.Join(dir, "export/manifest.json"))
			var manifest workflowrun.Manifest
			_ = json.Unmarshal(manifestBytes, &manifest)
			if manifest.DataSource != "mock:fixture:workflow-run-log-basic" {
				t.Fatal("manifest source not marked")
			}
			if _, e = os.Stat(filepath.Join(dir, "export/manifest.json")); e != nil {
				t.Fatal(e)
			}
			if tt.want != 0 && !strings.Contains(stdout.String(), `"ok": false`) {
				t.Fatalf("partial envelope not false %s", stdout)
			}
			if tt.max != "" {
				args = []string{BaseWorkflowRunExport.Command, "--base-token", "base-fixture", "--resume", "export", "--mock-fixture", string(b), "--require-complete"}
				if e = runShortcut(t, BaseWorkflowRunExport, args, factory, stdout); e != nil {
					t.Fatalf("budget resume %v", e)
				}
			}
		})
	}
}
func TestWorkflowRunResumeConflictNoWrites(t *testing.T) {
	f := runLogFixture(t)
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, _ := newExecuteFactory(t)
	b, _ := json.Marshal(f)
	args := append(runLogArgs(BaseWorkflowRunExport.Command), "--start-time", "0", "--end-time", "172800000", "--output", "export", "--mock-fixture", string(b))
	if e := runShortcut(t, BaseWorkflowRunExport, args, factory, stdout); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile("export/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	args = []string{BaseWorkflowRunExport.Command, "--base-token", "base-fixture", "--resume", "export", "--status", "running", "--mock-fixture", string(b)}
	e = runShortcut(t, BaseWorkflowRunExport, args, factory, stdout)
	if !errs.IsValidation(e) || output.ExitCodeOf(e) != 2 {
		t.Fatalf("conflict %v", e)
	}
	after, _ := os.ReadFile("export/manifest.json")
	if !bytes.Equal(before, after) {
		t.Fatal("conflicting resume wrote manifest")
	}
	args = []string{BaseWorkflowRunExport.Command, "--base-token", "base-fixture", "--resume", "export", "--mock-fixture", string(b)}
	if e = runShortcut(t, BaseWorkflowRunExport, args, factory, stdout); e != nil {
		t.Fatal(e)
	}
	after, _ = os.ReadFile("export/manifest.json")
	var m workflowrun.Manifest
	_ = json.Unmarshal(after, &m)
	if m.SelectionWindow.Start != 0 || m.SelectionWindow.End != 172800000 {
		t.Fatal("default recent days recomputed")
	}
}
func TestWorkflowRunCommandHTTP(t *testing.T) {
	f := runLogFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, e := f.RoundTrip(r)
		if e != nil {
			t.Error(e)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer server.Close()
	factory, stdout, _ := newExecuteFactory(t)
	args := append(runLogArgs(BaseWorkflowRunList.Command), "--start-time", "1970-01-01T00:00:00Z", "--end-time", "1970-01-03T00:00:00Z", "--mock-url", server.URL)
	if e := runShortcut(t, BaseWorkflowRunList, args, factory, stdout); e != nil {
		t.Fatal(e)
	}
	if len(f.Requests) != 2 {
		t.Fatalf("HTTP windows %v", f.Requests)
	}
	args = append(args, "--jq", ".data.total")
	if e := runShortcut(t, BaseWorkflowRunList, args, factory, stdout); e != nil || strings.TrimSpace(stdout.String()) != "2" {
		t.Fatalf("jq %s %v", stdout, e)
	}
}
func TestWorkflowRunSelectorsAndHelp(t *testing.T) {
	for _, args := range [][]string{{"--button-key", "node_message_btn_12345678"}, {"--button-key", "message_btn_12345678", "--button-ref", "fi2"}, {}} {
		factory, stdout, _ := newExecuteFactory(t)
		base := append(runLogArgs(BaseWorkflowButtonLogList.Command), "--run-id", "E1")
		e := runShortcut(t, BaseWorkflowButtonLogList, append(base, args...), factory, stdout)
		if !errs.IsValidation(e) {
			t.Fatalf("selector accepted %v", args)
		}
	}
	found := []string{}
	for _, s := range Shortcuts() {
		if strings.Contains(s.Command, "workflow-run") || s.Command == BaseWorkflowButtonLogList.Command {
			found = append(found, s.Command)
		}
	}
	if len(found) != 4 {
		t.Fatalf("registration %v", found)
	}
	if slices.Contains(found, "+workflow-run-sync") {
		t.Fatal("unapproved followup command")
	}
	factory, stdout, _ := newExecuteFactory(t)
	if e := runShortcut(t, BaseWorkflowRunExport, []string{BaseWorkflowRunExport.Command, "--help"}, factory, stdout); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(stdout.String(), "--refresh") || strings.Contains(stdout.String(), "--sync") {
		t.Fatal("implied followup capability")
	}
}
func TestWorkflowRunCancelled(t *testing.T) {
	f := runLogFixture(t)
	c, _ := workflowrun.NewMockClient(f, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := (workflowrun.Engine{Client: c}).Collect(ctx, workflowrun.Runs, workflowrun.Query{Base: "b", Workflow: "wf1", Window: workflowrun.Window{Start: 0, End: 1}}, 0)
	if e == nil {
		t.Fatal("cancelled request executed")
	}
}

func TestWorkflowRunMissingFixtureCoverage(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	args := append(runLogArgs(BaseWorkflowRunList.Command), "--start-time", "0", "--end-time", "1", "--mock-fixture", `{"runs":[]}`)
	if e := runShortcut(t, BaseWorkflowRunList, args, factory, stdout); e != nil {
		t.Fatal(e)
	}
	var out struct {
		Data workflowrun.Result `json:"data"`
	}
	if e := json.Unmarshal(stdout.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	if out.Data.Completeness != "partial" {
		t.Fatal("missing coverage inherited guaranteed")
	}
}

func workflowRunValidArgs(s common.Shortcut) []string {
	args := runLogArgs(s.Command)
	switch s.Command {
	case BaseWorkflowRunGet.Command, BaseWorkflowButtonLogList.Command:
		args = append(args, "--run-id", "E1")
	}
	if s.Command == BaseWorkflowButtonLogList.Command {
		args = append(args, "--button-key", "message_btn_12345678")
	}
	if s.Command == BaseWorkflowRunExport.Command {
		args = append(args, "--output", "export")
	}
	return args
}
func TestWorkflowRunDefaultNeverReturnsMock(t *testing.T) {
	for _, s := range []common.Shortcut{BaseWorkflowRunList, BaseWorkflowRunGet, BaseWorkflowButtonLogList, BaseWorkflowRunExport} {
		for _, disabled := range []bool{false, true} {
			t.Run(s.Command+"/"+map[bool]string{true: "explicit_false", false: "no_mock_flag"}[disabled], func(t *testing.T) {
				dir := t.TempDir()
				withBaseWorkingDir(t, dir)
				factory, stdout, _ := newExecuteFactory(t)
				args := workflowRunValidArgs(s)
				if disabled {
					args = append(args, "--mock=false")
				}
				e := runShortcut(t, s, args, factory, stdout)
				p, ok := errs.ProblemOf(e)
				if !ok || p.Subtype != errs.SubtypeFailedPrecondition || !strings.Contains(e.Error(), "not implemented") {
					t.Fatalf("default returned wrong result %v", e)
				}
				if stdout.Len() != 0 {
					t.Fatalf("default emitted data %s", stdout)
				}
				if _, e = os.Stat("export"); !os.IsNotExist(e) {
					t.Fatal("default export wrote files")
				}
			})
		}
	}
}
func TestWorkflowRunExplicitEmptyAndAnonymousSources(t *testing.T) {
	for _, tt := range []struct {
		name   string
		args   []string
		prefix string
	}{{"empty", []string{"--mock"}, "mock:fixture:empty"}, {"anonymous", []string{"--mock-fixture", `{"runs":[],"coverage":"guaranteed"}`}, "mock:fixture:sha256:"}} {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			args := append(workflowRunValidArgs(BaseWorkflowRunList), tt.args...)
			if e := runShortcut(t, BaseWorkflowRunList, args, factory, stdout); e != nil {
				t.Fatal(e)
			}
			var envelope struct {
				Data workflowrun.Result `json:"data"`
			}
			if e := json.Unmarshal(stdout.Bytes(), &envelope); e != nil {
				t.Fatal(e)
			}
			if !strings.HasPrefix(envelope.Data.DataSource, tt.prefix) {
				t.Fatalf("source %s", stdout)
			}
		})
	}
}
func TestWorkflowRunDryRunSources(t *testing.T) {
	for _, s := range []common.Shortcut{BaseWorkflowRunList, BaseWorkflowRunGet, BaseWorkflowButtonLogList, BaseWorkflowRunExport} {
		for _, mock := range []bool{false, true} {
			t.Run(s.Command+"/"+map[bool]string{true: "mock", false: "real"}[mock], func(t *testing.T) {
				factory, stdout, _ := newExecuteFactory(t)
				args := append(workflowRunValidArgs(s), "--dry-run")
				if mock {
					args = append(args, "--mock")
				}
				if e := runShortcut(t, s, args, factory, stdout); e != nil {
					t.Fatal(e)
				}
				var envelope struct {
					Data struct {
						DataSource string `json:"data_source"`
						Mock       bool   `json:"mock_only"`
						Available  bool   `json:"production_api_available"`
					} `json:"data"`
				}
				if e := json.Unmarshal(stdout.Bytes(), &envelope); e != nil {
					t.Fatal(e)
				}
				expected := "real"
				if mock {
					expected = "mock:fixture:empty"
				}
				if envelope.Data.DataSource != expected || envelope.Data.Mock != mock || envelope.Data.Available {
					t.Fatalf("dry-run provenance %s", stdout)
				}
			})
		}
	}
}
func TestWorkflowRunMockFailureNeverFallsBack(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(503) }))
	defer server.Close()
	factory, stdout, _ := newExecuteFactory(t)
	args := append(workflowRunValidArgs(BaseWorkflowRunList), "--mock-url", server.URL)
	e := runShortcut(t, BaseWorkflowRunList, args, factory, stdout)
	if !errs.IsNetwork(e) || requests != 3 || stdout.Len() != 0 {
		t.Fatalf("failure was replaced with mock data: requests=%d error=%v stdout=%s", requests, e, stdout)
	}
}
func TestWorkflowRunExplicitSourcesRejectConflicts(t *testing.T) {
	for _, suffix := range [][]string{{"--mock-fixture", ""}, {"--mock-url", ""}, {"--mock-fixture", `{}`, "--mock-url", "http://127.0.0.1:1"}} {
		factory, stdout, _ := newExecuteFactory(t)
		e := runShortcut(t, BaseWorkflowRunList, append(workflowRunValidArgs(BaseWorkflowRunList), suffix...), factory, stdout)
		if !errs.IsValidation(e) || stdout.Len() != 0 {
			t.Fatalf("invalid source accepted %v", e)
		}
	}
}
func TestWorkflowRunResumeRequiresSameSource(t *testing.T) {
	f := runLogFixture(t)
	b, _ := json.Marshal(f)
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	factory, stdout, _ := newExecuteFactory(t)
	args := append(workflowRunValidArgs(BaseWorkflowRunExport), "--mock-fixture", string(b))
	if e := runShortcut(t, BaseWorkflowRunExport, args, factory, stdout); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile("export/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, suffix := range [][]string{{"--mock"}, {}} {
		args = []string{BaseWorkflowRunExport.Command, "--base-token", "base-fixture", "--resume", "export"}
		e = runShortcut(t, BaseWorkflowRunExport, append(args, suffix...), factory, stdout)
		if !errs.IsValidation(e) {
			t.Fatalf("source conflict accepted %v", e)
		}
		after, _ := os.ReadFile("export/manifest.json")
		if !bytes.Equal(before, after) {
			t.Fatal("source conflict rewrote manifest")
		}
	}
	var m workflowrun.Manifest
	_ = json.Unmarshal(before, &m)
	m.DataSource = ""
	legacy, _ := json.Marshal(m)
	if e = os.WriteFile("export/manifest.json", legacy, 0600); e != nil {
		t.Fatal(e)
	}
	args = []string{BaseWorkflowRunExport.Command, "--base-token", "base-fixture", "--resume", "export", "--mock-fixture", string(b)}
	if e = runShortcut(t, BaseWorkflowRunExport, args, factory, stdout); !errs.IsValidation(e) {
		t.Fatal("unlabeled export silently adopted")
	}
	after, _ := os.ReadFile("export/manifest.json")
	if !bytes.Equal(legacy, after) {
		t.Fatal("unlabeled export relabeled")
	}
	m.Params.FormatVersion = 1
	legacy, _ = json.Marshal(m)
	_ = os.WriteFile("export/manifest.json", legacy, 0600)
	if e = runShortcut(t, BaseWorkflowRunExport, args, factory, stdout); !errs.IsValidation(e) {
		t.Fatal("legacy format silently adopted")
	}
}
func TestWorkflowRunLocalServerSource(t *testing.T) {
	f := runLogFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, e := f.RoundTrip(r)
		if e != nil {
			t.Error(e)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer server.Close()
	factory, stdout, _ := newExecuteFactory(t)
	args := append(workflowRunValidArgs(BaseWorkflowRunList), "--mock-url", server.URL, "--start-time", "0", "--end-time", "172800000")
	if e := runShortcut(t, BaseWorkflowRunList, args, factory, stdout); e != nil {
		t.Fatal(e)
	}
	var out struct {
		Data workflowrun.Result `json:"data"`
	}
	_ = json.Unmarshal(stdout.Bytes(), &out)
	if out.Data.DataSource != "mock:server:"+server.URL {
		t.Fatalf("server source %s", stdout)
	}
}

func TestMockServerCannotMasqueradeAsReal(t *testing.T) {
	f := runLogFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, e := f.RoundTrip(r)
		if e != nil {
			t.Error(e)
			return
		}
		defer resp.Body.Close()
		var p workflowrun.Page
		if e = json.NewDecoder(resp.Body).Decode(&p); e != nil {
			t.Error(e)
			return
		}
		p.DataSource = "real"
		for n := range p.Items {
			p.Items[n].DataSource = "real"
		}
		w.WriteHeader(resp.StatusCode)
		_ = json.NewEncoder(w).Encode(p)
	}))
	defer server.Close()
	factory, stdout, _ := newExecuteFactory(t)
	args := append(workflowRunValidArgs(BaseWorkflowRunList), "--start-time", "0", "--end-time", "172800000", "--mock-url", server.URL)
	if e := runShortcut(t, BaseWorkflowRunList, args, factory, stdout); e != nil {
		t.Fatal(e)
	}
	var out struct {
		Data workflowrun.Result `json:"data"`
	}
	_ = json.Unmarshal(stdout.Bytes(), &out)
	for _, p := range out.Data.Pages {
		if p.DataSource != out.Data.DataSource {
			t.Fatal("server changed provenance")
		}
		for _, i := range p.Items {
			if i.DataSource != out.Data.DataSource {
				t.Fatal("server changed record provenance")
			}
		}
	}
}
