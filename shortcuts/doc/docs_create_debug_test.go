// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

type debugEvent struct {
	Event   string                  `json:"event"`
	Status  string                  `json:"status"`
	Mode    string                  `json:"mode"`
	Stage   string                  `json:"stage"`
	TaskID  string                  `json:"task_id"`
	Timings []docsCreateDebugTiming `json:"timings"`
}

func readCreateDebugEvents(t *testing.T, text string) []debugEvent {
	t.Helper()
	var events []debugEvent
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, docsCreateDebugPrefix) {
			continue
		}
		var event debugEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, docsCreateDebugPrefix)), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func TestDocsCreateDebugIsOptInAndPreservesStdout(t *testing.T) {
	var outputs []string
	for _, enabled := range []string{"", "1"} {
		t.Run("enabled="+enabled, func(t *testing.T) {
			t.Setenv(docsCreateDebugEnv, enabled)
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			cfg := docsCreateTestConfig(t, "")
			f, stdout, stderr, reg := cmdutil.TestFactory(t, cfg)
			registerDocsCreateAPIStub(reg, map[string]interface{}{"document": map[string]interface{}{
				"document_id": "doxcn_debug", "revision_id": 1, "url": "https://example.com/docx/doxcn_debug",
			}})
			if err := runDocsCreateShortcut(t, f, stdout, []string{"+create", "--content", "<title>PrivateTitle</title><p>PrivateBody</p>", "--as", "user"}); err != nil {
				t.Fatal(err)
			}
			outputs = append(outputs, stdout.String())
			events := readCreateDebugEvents(t, stderr.String())
			if enabled == "" {
				if len(events) != 0 {
					t.Fatalf("debug enabled by default: %s", stderr)
				}
				return
			}
			for _, forbidden := range []string{"PrivateTitle", "PrivateBody", cfg.AppSecret, "Authorization", "test-token"} {
				if strings.Contains(stderr.String(), forbidden) {
					t.Fatalf("debug leaked %q", forbidden)
				}
			}
			if len(events) == 0 || events[0].Event != "start" || events[len(events)-1].Event != "summary" {
				t.Fatalf("events=%+v", events)
			}
			var steps []string
			for _, event := range events {
				if event.Event == "create_mode" && event.Mode != "direct_response" {
					t.Fatalf("mode=%s", event.Mode)
				}
				if strings.HasSuffix(event.Event, ".start") {
					steps = append(steps, strings.TrimSuffix(event.Event, ".start"))
				}
			}
			want := "prepare_input,validate_remote_sources,create_request,wait_task,permission,document_url,resources,output"
			if strings.Join(steps, ",") != want {
				t.Fatalf("steps=%v", steps)
			}
			summary := events[len(events)-1]
			if summary.Status != "completed" || len(summary.Timings) != len(steps) {
				t.Fatalf("summary=%+v", summary)
			}
			for _, timing := range summary.Timings {
				if timing.DurationMS < 0 || timing.Percent < 0 || timing.Percent > 100 {
					t.Fatalf("timing=%+v", timing)
				}
			}
		})
	}
	if len(outputs) != 2 || outputs[0] != outputs[1] {
		t.Fatalf("debug changed stdout: %q", outputs)
	}
}

func TestDocsCreateDebugReportsAsyncFailureAndSkipsFollowup(t *testing.T) {
	t.Setenv(docsCreateDebugEnv, "1")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsCreateTestConfig(t, ""))
	registerDocsCreateAPIStub(reg, map[string]interface{}{"task": map[string]interface{}{
		"task_id": "task_debug", "status": "processing", "stage": "writing_content",
	}})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/async_tasks/task_debug", Body: map[string]interface{}{
		"code": 0, "data": map[string]interface{}{"task": map[string]interface{}{
			"task_id": "task_debug", "status": "failed", "failure": map[string]interface{}{"code": "execution_interrupted", "message": "PrivateErrorBody"},
		}},
	}})
	err := runDocsCreateShortcut(t, f, stdout, []string{"+create", "--content", "<title>Debug</title><p>Body</p>", "--as", "user"})
	if err == nil || stdout.Len() != 0 {
		t.Fatalf("err=%v stdout=%s", err, stdout)
	}
	events := readCreateDebugEvents(t, stderr.String())
	var observed []string
	for _, event := range events {
		if event.Event == "permission.start" || event.Event == "output.start" {
			t.Fatalf("followup ran after failure: %+v", events)
		}
		if event.Event == "task_observed" {
			observed = append(observed, event.TaskID+":"+event.Status+":"+event.Stage)
		}
	}
	if strings.Join(observed, ",") != "task_debug:processing:writing_content,task_debug:failed:" {
		t.Fatalf("observed=%v", observed)
	}
	if events[len(events)-1].Status != "failed" {
		t.Fatalf("summary=%+v", events[len(events)-1])
	}
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.HasPrefix(line, docsCreateDebugPrefix) && strings.Contains(line, "PrivateErrorBody") {
			t.Fatal("trace leaked error body")
		}
	}
}
