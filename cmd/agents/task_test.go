// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

// taskCmdCtx builds a `lark-cli agents task <leaf>` command whose CommandPath()
// is non-empty (required for content-safety scanning) and whose --as flag is set
// to bot so ResolveAs honors it verbatim.
func taskCmdCtx(t *testing.T, leaf string) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "lark-cli"}
	group := &cobra.Command{Use: "agents"}
	task := &cobra.Command{Use: "task"}
	l := &cobra.Command{Use: leaf}
	root.AddCommand(group)
	group.AddCommand(task)
	task.AddCommand(l)
	l.Flags().String("as", "", "identity")
	if err := l.Flags().Set("as", "bot"); err != nil {
		t.Fatal(err)
	}
	l.SetContext(context.Background())
	return l
}

// taskTestOpts wires a taskOptions against a real (test) Factory, addressing
// the scripted fakeflow agent agt_x under an explicit bot identity. The
// Factory's httpmock registry holds zero stubs, so any HTTP attempt fails the
// test; provider behavior is scripted via setScripted.
func taskTestOpts(t *testing.T, leaf string) (*taskOptions, *httpmock.Registry) {
	t.Helper()
	registerScripted()
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	return &taskOptions{
		Factory:  f,
		Cmd:      taskCmdCtx(t, leaf),
		Ref:      "fakeflow:agt_x",
		TaskID:   "chat_1",
		As:       "bot",
		PageSize: defaultPageSize,
	}, reg
}

// TestTaskCancelUnsupportedGated pins that cancel against an agent whose spec
// does not wire CancelTask (task_cancel=false, fakecat:min) is gated offline —
// it returns an unsupported_capability validation error before any network
// access (the httpmock registry has zero stubs, so any request would fail
// differently).
func TestTaskCancelUnsupportedGated(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	err := agentTaskCancelRun(&taskOptions{
		Factory: f, Cmd: taskCmdCtx(t, "cancel"), Ref: "fakecat:min", TaskID: "t1", As: "bot",
	})
	if err == nil {
		t.Fatal("task cancel with task_cancel=false should report unsupported_capability")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("want validation error, got %T", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.Subtype("unsupported_capability") {
		t.Fatalf("subtype should be unsupported_capability, got %+v", p)
	}
	if output.ExitCodeOf(err) != output.ExitValidation {
		t.Fatalf("exit should be %d, got %d", output.ExitValidation, output.ExitCodeOf(err))
	}
}

// TestArtifactRequiresOutput pins that `task get --artifact` without -o is a
// validation error raised before any provider is built (client-side guard).
func TestArtifactRequiresOutput(t *testing.T) {
	err := agentTaskGetRun(&taskOptions{Ref: "fakeflow:agt_x", TaskID: "t1", ArtifactID: "a1", Output: ""})
	if err == nil {
		t.Fatal("--artifact without -o should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("want validation error, got %T", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype should be invalid_argument, got %+v", p)
	}
	// hint contract: --artifact without -o must carry a remediation hint, and
	// the param uses the -- prefix.
	if !strings.Contains(p.Hint, "-o") {
		t.Errorf("hint should guide adding -o, got %q", p.Hint)
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.Param != "--output" {
		t.Errorf("param should be --output, got %+v", verr)
	}
}

// TestTaskGetSingle pins the single get (no --watch): GetTask returns the
// provider's task and the command emits it in a success envelope, exit 0.
func TestTaskGetSingle(t *testing.T) {
	opts, _ := taskTestOpts(t, "get")
	setScripted(t, scriptedHooks{getTask: func(taskID string) (*iagents.AgentTask, error) {
		return &iagents.AgentTask{TaskID: taskID, ContextID: "sess_1", State: iagents.StateCompleted, IsTerminal: true}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentTaskGetRun(opts); err != nil {
		t.Fatalf("get should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	data, _ := env.Data.(map[string]interface{})
	if data["state"] != string(iagents.StateCompleted) {
		t.Errorf("state should be completed, got %v", data["state"])
	}
}

// TestTaskGetError surfaces a provider GetTask failure unchanged.
func TestTaskGetError(t *testing.T) {
	opts, _ := taskTestOpts(t, "get")
	setScripted(t, scriptedHooks{getTask: func(string) (*iagents.AgentTask, error) {
		return nil, errs.NewAPIError(errs.SubtypeNotFound, "not found").WithCode(1254000)
	}})
	if err := agentTaskGetRun(opts); err == nil {
		t.Fatal("GetTask error should propagate")
	}
}

// TestTaskGetWatchTerminalFailureExit pins that a --watch that lands on a failed
// terminal state emits the envelope but returns the silent exit-1.
func TestTaskGetWatchTerminalFailureExit(t *testing.T) {
	restore := swapSleep()
	defer restore()

	opts, _ := taskTestOpts(t, "get")
	opts.Watch = true
	setScripted(t, scriptedHooks{getTask: func(taskID string) (*iagents.AgentTask, error) {
		return &iagents.AgentTask{TaskID: taskID, State: iagents.StateFailed, IsTerminal: true}, nil
	}})
	if err := agentTaskGetRun(opts); output.ExitCodeOf(err) != 1 {
		t.Fatalf("failed terminal state should return exit 1, got %d (err=%v)", output.ExitCodeOf(err), err)
	}
}

// TestTaskGetWatchTerminalSuccessExit pins that a --watch reaching a successful
// terminal state emits the task and exits 0.
func TestTaskGetWatchTerminalSuccessExit(t *testing.T) {
	restore := swapSleep()
	defer restore()

	opts, _ := taskTestOpts(t, "get")
	opts.Watch = true
	setScripted(t, scriptedHooks{getTask: func(taskID string) (*iagents.AgentTask, error) {
		return &iagents.AgentTask{TaskID: taskID, State: iagents.StateCompleted, IsTerminal: true}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	err := agentTaskGetRun(opts)
	if output.ExitCodeOf(err) != output.ExitOK {
		t.Fatalf("completed terminal state should exit 0, got %d (err=%v)", output.ExitCodeOf(err), err)
	}
	var env output.Envelope
	if uerr := json.Unmarshal(out.Bytes(), &env); uerr != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", uerr, string(out.Bytes()))
	}
	data, _ := env.Data.(map[string]interface{})
	if data["state"] != string(iagents.StateCompleted) {
		t.Errorf("--watch should poll to completed, got %v", data["state"])
	}
}

// TestTaskGetTimeoutRequiresWatch pins the combo guard: --timeout is meaningful
// only with --watch, so passing it alone is a client-side validation error
// (exit 2, param --timeout) before any network access.
func TestTaskGetTimeoutRequiresWatch(t *testing.T) {
	opts, _ := taskTestOpts(t, "get")
	opts.Timeout = 30 * time.Second // no --watch
	err := agentTaskGetRun(opts)
	if err == nil {
		t.Fatal("--timeout without --watch should error")
	}
	if !errs.IsValidation(err) || output.ExitCodeOf(err) != output.ExitValidation {
		t.Fatalf("want validation error (exit 2), got %T exit=%d", err, output.ExitCodeOf(err))
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--timeout" {
		t.Fatalf("param should be --timeout, got %+v", ve)
	}
	if !strings.Contains(ve.Message, "--timeout must be used together with --watch") {
		t.Errorf("message should explain --timeout must be used with --watch, got %q", ve.Message)
	}
}

// TestTaskGetWatchBoundedTimeout pins the bounded watch: with --watch and a
// short --timeout against a task that never terminates (always Running), the
// poll deadline fires and the command degrades gracefully — it prints the
// current (working) state, exits 0 (a timeout is not a failure), and its
// meta.next suggests a fresh bounded watch (`--watch --timeout`). A real (short)
// deadline is used so the context.WithTimeout wiring itself is exercised; the
// backoff sleep is cut short by the deadline.
func TestTaskGetWatchBoundedTimeout(t *testing.T) {
	opts, _ := taskTestOpts(t, "get")
	opts.Watch = true
	opts.Timeout = 40 * time.Millisecond
	// The initial get + each poll iteration all observe a still-working task.
	setScripted(t, scriptedHooks{getTask: func(taskID string) (*iagents.AgentTask, error) {
		return &iagents.AgentTask{TaskID: taskID, State: iagents.StateWorking}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	err := agentTaskGetRun(opts)
	if output.ExitCodeOf(err) != output.ExitOK {
		t.Fatalf("watch timeout should exit 0 (timeout is not a failure), got %d (err=%v)", output.ExitCodeOf(err), err)
	}
	var env output.Envelope
	if uerr := json.Unmarshal(out.Bytes(), &env); uerr != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", uerr, string(out.Bytes()))
	}
	data, _ := env.Data.(map[string]interface{})
	if data["state"] != string(iagents.StateWorking) {
		t.Errorf("timeout should return the current working state, got %v", data["state"])
	}
	if env.Meta == nil || len(env.Meta.Next) == 0 {
		t.Fatalf("non-terminal should provide a meta.next continue-watch command, got %+v", env.Meta)
	}
	if cmd := env.Meta.Next[0].Command; !strings.Contains(cmd, "--watch --timeout") {
		t.Errorf("meta.next should suggest a bounded watch (--watch --timeout), got %q", cmd)
	}
}

// TestTaskListEmitsCount pins that `task list` returns {tasks:[...]} with a
// meta.count reflecting the number of tasks.
func TestTaskListEmitsCount(t *testing.T) {
	opts, _ := taskTestOpts(t, "list")
	setScripted(t, scriptedHooks{listTasks: func(contextID string, _ iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
		return []iagents.TaskSummary{
			{TaskID: "chat_1", State: iagents.StateCompleted, IsTerminal: true},
			{TaskID: "chat_2", State: iagents.StateWorking},
		}, iagents.PageInfo{}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentTaskListRun(opts); err != nil {
		t.Fatalf("list should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	data, _ := env.Data.(map[string]interface{})
	tasks, ok := data["tasks"].([]interface{})
	if !ok || len(tasks) != 2 {
		t.Fatalf("data.tasks should have 2, got %v", data["tasks"])
	}
	if env.Meta == nil || env.Meta.Pagination == nil || env.Meta.Pagination.Items != 2 {
		t.Errorf("meta.pagination.items should be 2, got %+v", env.Meta)
	}
}

// TestTaskListEmptyEmitsArray pins the array convention: an empty task list
// serializes as [] (never null), matching Card.Parameters.
func TestTaskListEmptyEmitsArray(t *testing.T) {
	opts, _ := taskTestOpts(t, "list")
	setScripted(t, scriptedHooks{listTasks: func(string, iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
		return nil, iagents.PageInfo{}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentTaskListRun(opts); err != nil {
		t.Fatalf("list should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	data, _ := env.Data.(map[string]interface{})
	v, present := data["tasks"]
	if !present {
		t.Fatal("data.tasks key should be present")
	}
	if _, ok := v.([]interface{}); !ok {
		t.Errorf("empty task list should emit a JSON array (not null), got %T: %v", v, v)
	}
	if env.Meta != nil {
		t.Errorf("empty list should omit meta entirely (no ambiguous {} shape), got %+v", env.Meta)
	}
}

// TestTaskListError surfaces a provider ListTasks failure.
func TestTaskListError(t *testing.T) {
	opts, _ := taskTestOpts(t, "list")
	setScripted(t, scriptedHooks{listTasks: func(string, iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
		return nil, iagents.PageInfo{}, errs.NewAPIError(errs.SubtypeUnknown, "app ticket invalid").WithCode(99991663)
	}})
	if err := agentTaskListRun(opts); err == nil {
		t.Fatal("ListTasks error should propagate")
	}
}

// TestTaskGetArtifactURLDownload pins the --artifact URL path: DownloadArtifact
// returns a URL, which is SSRF-validated, fetched, and written to -o via vfs.
func TestTaskGetArtifactURLDownload(t *testing.T) {
	// A local test server stands in for the artifact host; its loopback address
	// is allowlisted for the test via the download client seam below.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("REPORT-BYTES"))
	}))
	defer srv.Close()

	restore := swapArtifactFetch(func(_ context.Context, _ *cmdutil.Factory, rawURL string) ([]byte, error) {
		resp, err := http.Get(rawURL) //nolint:gosec,noctx // test-only fetch of a loopback httptest server
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		buf := make([]byte, 0, 32)
		tmp := make([]byte, 32)
		for {
			n, rerr := resp.Body.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if rerr != nil {
				break
			}
		}
		return buf, nil
	})
	defer restore()

	// SafeOutputPath requires a relative path within the CWD, so run from a
	// temp dir with a relative -o.
	chdirTemp(t)
	outPath := "report.txt"

	opts, _ := taskTestOpts(t, "get")
	opts.ArtifactID = "art_1"
	opts.Output = outPath
	// name is untrusted server input, deliberately set to a path-traversal value:
	// if it participates in local path construction, the artifact would escape
	// the -o path and be caught by the assertions below. This runs the real
	// resolveDownload path (resolveProvider → provider.DownloadArtifact), with
	// the provider returning a url-type artifact.
	setScripted(t, scriptedHooks{downloadArtifact: func(taskID, artifactID string) (*iagents.ArtifactData, error) {
		return &iagents.ArtifactData{Name: "../escape.txt", URL: srv.URL}, nil
	}})

	if err := agentTaskGetRun(opts); err != nil {
		t.Fatalf("artifact download should not error: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("artifact should be written to the -o path (the response name must not rewrite the on-disk location): %v", err)
	}
	if string(got) != "REPORT-BYTES" {
		t.Errorf("artifact content should be the downloaded bytes, got %q", string(got))
	}
	if _, err := os.Stat("../escape.txt"); !os.IsNotExist(err) {
		t.Errorf("the response name must not participate in path construction: ../escape.txt should not exist (stat err=%v)", err)
	}
	// name is echoed to suggested_name as advisory info (for the AI to pick -o),
	// but never affects the on-disk path: above already proved the file is at
	// report.txt and no escape file was created. This proves it is surfaced (honesty).
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })
	if !strings.Contains(string(out.Bytes()), `"suggested_name": "../escape.txt"`) {
		t.Errorf("download output should echo the server-suggested name to suggested_name, got: %s", out.Bytes())
	}
}

// TestTaskGetArtifactInlineWrite pins the --artifact inline path: an artifact
// with inline Bytes (no URL) is written straight to -o without any network.
func TestTaskGetArtifactInlineWrite(t *testing.T) {
	fetched := false
	restore := swapArtifactFetch(func(context.Context, *cmdutil.Factory, string) ([]byte, error) {
		fetched = true
		return nil, nil
	})
	defer restore()

	chdirTemp(t)
	outPath := "inline.bin"

	opts, _ := taskTestOpts(t, "get")
	opts.ArtifactID = "art_inline"
	opts.Output = outPath
	// Swap the provider seam so DownloadArtifact yields inline bytes.
	restoreP := swapResolveDownload(func(*taskOptions) (*iagents.ArtifactData, error) {
		return &iagents.ArtifactData{Mime: "application/octet-stream", Bytes: []byte("INLINE")}, nil
	})
	defer restoreP()

	if err := agentTaskGetRun(opts); err != nil {
		t.Fatalf("writing an inline artifact should not error: %v", err)
	}
	if fetched {
		t.Error("an inline artifact should not trigger a URL download")
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("inline artifact should be written to the -o path: %v", err)
	}
	if string(got) != "INLINE" {
		t.Errorf("inline artifact content mismatch, got %q", string(got))
	}
}

// TestNewCmdAgentTask_GroupHasSubcommands pins that the task group registers
// get/list/cancel and has no RunE of its own.
func TestNewCmdAgentTask_GroupHasSubcommands(t *testing.T) {
	cmd := NewCmdAgentTask(nil)
	if cmd.RunE != nil {
		t.Error("task should be a pure group with no RunE")
	}
	want := map[string]bool{"get": false, "list": false, "cancel": false}
	for _, sub := range cmd.Commands() {
		want[sub.Name()] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("task should contain subcommand %q", name)
		}
	}
}

// TestNewCmdAgentTaskGet_ReadRiskArgsFlags pins ExactArgs(2), read risk and the
// get-specific flag surface.
func TestNewCmdAgentTaskGet_ReadRiskArgsFlags(t *testing.T) {
	cmd := NewCmdAgentTaskGet(nil)
	if level, ok := cmdutil.GetRisk(cmd); !ok || level != cmdutil.RiskRead {
		t.Errorf("task get should be marked read risk, got level=%q ok=%v", level, ok)
	}
	if err := cmd.Args(cmd, []string{"fakeflow:x"}); err == nil {
		t.Error("task get missing task-id should raise an args error (ExactArgs 2)")
	}
	if err := cmd.Args(cmd, []string{"fakeflow:x", "t1"}); err != nil {
		t.Errorf("task get with two positional args should be valid: %v", err)
	}
	for _, name := range []string{"watch", "timeout", "artifact", "output", "as", "format", "jq"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("task get should have --%s flag", name)
		}
	}
	if cmd.Flags().ShorthandLookup("o") == nil {
		t.Error("task get --output should have the -o shorthand")
	}
	// flag cleanup: --until had only one valid value and was never read, so it
	// is removed.
	if cmd.Flags().Lookup("until") != nil {
		t.Error("task get --until should be removed")
	}
	if strings.Contains(cmd.Long, "--until") {
		t.Errorf("help text should no longer mention --until, got %q", cmd.Long)
	}
}

// TestTaskGetPrettyFormat pins that `task get --format pretty` renders
// key: value lines (state / task_id / context_id / first text message
// truncated / artifacts count) with agent text stripped of ANSI escapes.
func TestTaskGetPrettyFormat(t *testing.T) {
	opts, _ := taskTestOpts(t, "get")
	opts.Format = "pretty"
	setScripted(t, scriptedHooks{getTask: func(taskID string) (*iagents.AgentTask, error) {
		return &iagents.AgentTask{
			TaskID: taskID, ContextID: "sess_1", State: iagents.StateCompleted, IsTerminal: true,
			Messages: []iagents.Message{{Role: "agent", Parts: []iagents.Part{
				{Type: "text", Text: "\x1b[31manalysis complete\x1b[0m"},
			}}},
			Artifacts: []iagents.Artifact{{ID: "art_1", Kind: "file"}},
		}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentTaskGetRun(opts); err != nil {
		t.Fatalf("task get --format pretty should not error: %v", err)
	}
	text := string(out.Bytes())
	for _, want := range []string{"state: completed", "task_id: chat_1", "context_id: sess_1", "reply: analysis complete", "artifacts: 1"} {
		if !strings.Contains(text, want) {
			t.Errorf("pretty output should contain %q, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b") {
		t.Errorf("the agent body ANSI sequences must be stripped: %q", text)
	}
	var env output.Envelope
	if json.Unmarshal(out.Bytes(), &env) == nil && env.OK {
		t.Errorf("pretty should not be a JSON envelope: %s", text)
	}
}

// TestTaskListPrettyFormat pins list-class pretty: header TSV whose columns
// mirror the json fields (now including UPDATED_AT + SUMMARY), and a data row
// carrying the timestamp and (flattened) summary.
func TestTaskListPrettyFormat(t *testing.T) {
	opts, _ := taskTestOpts(t, "list")
	opts.Format = "pretty"
	setScripted(t, scriptedHooks{listTasks: func(string, iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
		return []iagents.TaskSummary{
			{TaskID: "chat_1", ContextID: "sess_1", State: iagents.StateCompleted, IsTerminal: true,
				UpdatedAt: "2026-07-05T12:00:00Z", Summary: "analysis complete"},
		}, iagents.PageInfo{}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentTaskListRun(opts); err != nil {
		t.Fatalf("task list --format pretty should not error: %v", err)
	}
	text := string(out.Bytes())
	if !strings.HasPrefix(text, "TASK_ID\tCONTEXT_ID\tSTATE\tIS_TERMINAL\tUPDATED_AT\tSUMMARY\tBIZ_ERR_CODE\tBIZ_ERR_MESSAGE\n") {
		t.Errorf("pretty output should start with a header row, got %q", text)
	}
	if !strings.Contains(text, "chat_1\tsess_1\tcompleted\ttrue\t2026-07-05T12:00:00Z\tanalysis complete") {
		t.Errorf("pretty output should contain a data row with updated_at + summary, got %q", text)
	}
}

// TestTaskListPrettySanitizesStateAndTimestamp pins that the agent-controlled
// State and UpdatedAt fields are ANSI-stripped on the pretty/TSV path, not just
// the ids/summary — a malicious provider must not inject terminal escapes via a
// forged state string or timestamp.
func TestTaskListPrettySanitizesStateAndTimestamp(t *testing.T) {
	opts, _ := taskTestOpts(t, "list")
	opts.Format = "pretty"
	setScripted(t, scriptedHooks{listTasks: func(string, iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
		return []iagents.TaskSummary{
			{TaskID: "chat_1", State: iagents.TaskState("completed\x1b[2J"), IsTerminal: true,
				UpdatedAt: "2026-07-05T12:00:00Z\x1b]0;pwned\x07"},
		}, iagents.PageInfo{}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentTaskListRun(opts); err != nil {
		t.Fatalf("task list --format pretty should not error: %v", err)
	}
	if text := string(out.Bytes()); strings.Contains(text, "\x1b") {
		t.Errorf("ANSI/OSC sequences in State/UpdatedAt must be stripped, got %q", text)
	}
}

// TestTaskListSortedByUpdatedAtDesc pins the ordering + enriched-field
// contract: the provider returns tasks in most-recent-first order (its
// contract), and the command emits them verbatim while carrying updated_at +
// summary on each.
func TestTaskListSortedByUpdatedAtDesc(t *testing.T) {
	opts, _ := taskTestOpts(t, "list")
	setScripted(t, scriptedHooks{listTasks: func(string, iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
		return []iagents.TaskSummary{
			{TaskID: "new", State: iagents.StateInputRequired, UpdatedAt: "2026-07-05T12:00:00Z", Summary: "more input needed"},
			{TaskID: "mid", State: iagents.StateCompleted, UpdatedAt: "2026-07-05T11:00:00Z", Summary: "round two"},
			{TaskID: "old", State: iagents.StateCompleted, UpdatedAt: "2026-07-05T10:00:00Z", Summary: "round one"},
		}, iagents.PageInfo{}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentTaskListRun(opts); err != nil {
		t.Fatalf("task list should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	data, _ := env.Data.(map[string]interface{})
	tasks, ok := data["tasks"].([]interface{})
	if !ok || len(tasks) != 3 {
		t.Fatalf("data.tasks should have 3 entries, got %v", data["tasks"])
	}
	want := []string{"new", "mid", "old"}
	for i, w := range want {
		m, _ := tasks[i].(map[string]interface{})
		if m["task_id"] != w {
			t.Errorf("tasks[%d].task_id should be %q (newest-first), got %v", i, w, m["task_id"])
		}
	}
	first, _ := tasks[0].(map[string]interface{})
	if first["updated_at"] != "2026-07-05T12:00:00Z" {
		t.Errorf("tasks[0].updated_at should be carried, got %v", first["updated_at"])
	}
	if first["summary"] != "more input needed" {
		t.Errorf("tasks[0].summary should be carried, got %v", first["summary"])
	}
}

// TestNewCmdAgentTaskCancel_WriteRisk pins ExactArgs(2) and write risk on cancel.
func TestNewCmdAgentTaskCancel_WriteRisk(t *testing.T) {
	cmd := NewCmdAgentTaskCancel(nil)
	if level, ok := cmdutil.GetRisk(cmd); !ok || level != cmdutil.RiskWrite {
		t.Errorf("task cancel should be marked write risk, got level=%q ok=%v", level, ok)
	}
	if err := cmd.Args(cmd, []string{"fakeflow:x"}); err == nil {
		t.Error("task cancel missing task-id should raise an args error (ExactArgs 2)")
	}
}

// TestNewCmdAgentTaskList_ReadRisk pins ExactArgs(1), read risk and --context-id.
func TestNewCmdAgentTaskList_ReadRisk(t *testing.T) {
	cmd := NewCmdAgentTaskList(nil)
	if level, ok := cmdutil.GetRisk(cmd); !ok || level != cmdutil.RiskRead {
		t.Errorf("task list should be marked read risk, got level=%q ok=%v", level, ok)
	}
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Error("task list missing ref should raise an args error (ExactArgs 1)")
	}
	if cmd.Flags().Lookup("context-id") == nil {
		t.Error("task list should have --context-id flag")
	}
}

// chdirTemp switches the working directory to a fresh temp dir for the test
// (restored on cleanup), so a relative --output stays within the CWD as
// SafeOutputPath requires. (Go 1.23 has no t.Chdir.)
func chdirTemp(t *testing.T) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// swapArtifactFetch replaces the package-level URL fetch seam with a stub for
// tests, returning a restore func.
func swapArtifactFetch(fn func(context.Context, *cmdutil.Factory, string) ([]byte, error)) func() {
	orig := artifactFetch
	artifactFetch = fn
	return func() { artifactFetch = orig }
}

// swapResolveDownload replaces the package-level DownloadArtifact seam with a
// stub for tests, returning a restore func.
func swapResolveDownload(fn func(*taskOptions) (*iagents.ArtifactData, error)) func() {
	orig := resolveDownload
	resolveDownload = fn
	return func() { resolveDownload = orig }
}

// TestNewCmdAgentTaskGet_ArtifactHelpMentionsOutput ensures the --artifact help
// tells the user -o is required (discoverability of the client-side guard).
func TestNewCmdAgentTaskGet_ArtifactHelpMentionsOutput(t *testing.T) {
	cmd := NewCmdAgentTaskGet(nil)
	f := cmd.Flags().Lookup("artifact")
	if f == nil {
		t.Fatal("should have --artifact flag")
	}
	if !strings.Contains(f.Usage, "-o") && !strings.Contains(f.Usage, "output") {
		t.Errorf("--artifact help should note it must be used with -o, got %q", f.Usage)
	}
}

// publicArtifactURL is a public IP-literal artifact URL. Using an IP literal
// (not a hostname) makes ValidateDownloadSourceURL skip DNS entirely: it parses
// the IP, sees it is public (203.0.113.7 is RFC 5737 TEST-NET-3, which
// isRestrictedDownloadIP does not block), and returns nil — so the SSRF guard
// passes deterministically with zero network. The httpmock transport then
// intercepts the request before any real dial to that (non-routable) address.
const publicArtifactURL = "https://203.0.113.7/artifacts/report.txt"

// swapHardenDownloadClient replaces the download-client-build seam so tests can
// feed the interceptable httpmock base client straight into fetchArtifactURL
// (the production hardened client clones a real *http.Transport and would drop
// the mock). Returns a restore func.
func swapHardenDownloadClient(fn func(*http.Client) *http.Client) func() {
	orig := hardenDownloadClient
	hardenDownloadClient = fn
	return func() { hardenDownloadClient = orig }
}

// passthroughClient is the hardenDownloadClient stub: it returns the base client
// unchanged so the httpmock RoundTripper survives to intercept the request.
func passthroughClient(base *http.Client) *http.Client { return base }

// TestFetchArtifactURL_SSRFBlocked pins the SSRF guard: a loopback URL is
// rejected by ValidateDownloadSourceURL before any client is built or request
// is made, surfaced as an invalid_argument validation error.
func TestFetchArtifactURL_SSRFBlocked(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	_, err := fetchArtifactURL(context.Background(), f, "http://127.0.0.1/secret")
	if err == nil {
		t.Fatal("loopback URL should be blocked by SSRF")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("SSRF block should be a validation error, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype should be invalid_argument, got %+v", p)
	}
}

// TestFetchArtifactURL_MalformedURLBlocked pins that a malformed URL string is
// caught by the SSRF/URL guard (ValidateDownloadSourceURL parses first), not by
// the downstream request builder: it is an invalid_argument validation error.
func TestFetchArtifactURL_MalformedURLBlocked(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	_, err := fetchArtifactURL(context.Background(), f, "ftp://example.com/x")
	if err == nil {
		t.Fatal("a non-http/https URL should be rejected")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("a malformed URL should be a validation error, got %T: %v", err, err)
	}
}

// TestFetchArtifactURL_HttpRejected pins the https-only enforcement, distinct
// from the SSRF guard: a PUBLIC http:// URL passes the SSRF check (routable,
// http-family) but is still rejected because artifact bytes must travel over
// https (no plaintext download). No request is made.
func TestFetchArtifactURL_HttpRejected(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)

	_, err := fetchArtifactURL(context.Background(), f, "http://203.0.113.7/artifacts/report.txt")
	if err == nil {
		t.Fatal("a public plain-http artifact URL should be rejected by the https-only check")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("https-only rejection should be a validation error, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("error should state the https requirement, got %v", err)
	}
}

// TestFetchArtifactURL_HttpClientError pins the HttpClient() failure branch: a
// public (SSRF-passing) URL whose Factory cannot mint an http client surfaces as
// an internal sdk_error. No request is made.
func TestFetchArtifactURL_HttpClientError(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	f.HttpClient = func() (*http.Client, error) { return nil, errors.New("boom no client") }

	_, err := fetchArtifactURL(context.Background(), f, publicArtifactURL)
	if err == nil {
		t.Fatal("HttpClient failure should propagate")
	}
	if !errs.IsInternal(err) {
		t.Fatalf("HttpClient failure should be an internal error, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeSDKError {
		t.Fatalf("subtype should be sdk_error, got %+v", p)
	}
}

// TestFetchArtifactURL_NetworkError pins the client.Do failure branch: the
// injected transport returns a transport-level error, surfaced as a network
// transport error.
func TestFetchArtifactURL_NetworkError(t *testing.T) {
	restore := swapHardenDownloadClient(passthroughClient)
	defer restore()

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	// Point the Factory's base client at a transport that always errors.
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: errRoundTripper{err: errors.New("dial refused")}}, nil
	}

	_, err := fetchArtifactURL(context.Background(), f, publicArtifactURL)
	if err == nil {
		t.Fatal("transport error should propagate")
	}
	if !errs.IsNetwork(err) {
		t.Fatalf("transport error should be a network error, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("subtype should be transport, got %+v", p)
	}
}

// TestFetchArtifactURL_HTTPStatusError pins the non-200 branch: an HTTP 503 is
// surfaced as a network server error carrying the status code.
func TestFetchArtifactURL_HTTPStatusError(t *testing.T) {
	restore := swapHardenDownloadClient(passthroughClient)
	defer restore()

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "203.0.113.7/artifacts/report.txt",
		Status:  503,
		RawBody: []byte("Service Unavailable"),
	})

	_, err := fetchArtifactURL(context.Background(), f, publicArtifactURL)
	if err == nil {
		t.Fatal("HTTP 503 should propagate")
	}
	if !errs.IsNetwork(err) {
		t.Fatalf("an HTTP status error should be a network error, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeNetworkServer {
		t.Fatalf("subtype should be server_error, got %+v", p)
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("error message should contain status code 503, got %v", err)
	}
}

// TestFetchArtifactURL_Success pins the happy path: a 200 response body is read
// through the LimitReader and returned verbatim.
func TestFetchArtifactURL_Success(t *testing.T) {
	restore := swapHardenDownloadClient(passthroughClient)
	defer restore()

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "203.0.113.7/artifacts/report.txt",
		Status:  200,
		RawBody: []byte("REPORT-BYTES"),
	})

	got, err := fetchArtifactURL(context.Background(), f, publicArtifactURL)
	if err != nil {
		t.Fatalf("download should not error: %v", err)
	}
	if string(got) != "REPORT-BYTES" {
		t.Errorf("artifact bytes should be returned verbatim, got %q", string(got))
	}
}

// TestFetchArtifactURL_LimitEnforced pins the size-cap guard: a body larger than
// maxArtifactBytes is REJECTED with a typed error rather than silently truncated
// onto disk (the fetch reads max+1 to detect the overflow), so a hostile host can
// neither stream an unbounded body nor slip a corrupt partial file past as
// success. Uses a streaming RoundTripper that emits one byte past the cap.
func TestFetchArtifactURL_LimitEnforced(t *testing.T) {
	restore := swapHardenDownloadClient(passthroughClient)
	defer restore()

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	// A body one byte longer than the cap must be refused, not truncated.
	oversized := int64(maxArtifactBytes) + 1
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: streamRoundTripper{n: oversized, b: 'A'}}, nil
	}

	_, err := fetchArtifactURL(context.Background(), f, publicArtifactURL)
	if err == nil {
		t.Fatal("an oversized artifact should be rejected with an error, not truncated to a partial file")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("oversized artifact should be a validation error, got %T: %v", err, err)
	}
}

// TestFetchArtifactURL_ReadError pins the response-body read-error branch: a 200
// response whose Body.Read fails mid-stream surfaces as a network transport
// error (not a partial success).
func TestFetchArtifactURL_ReadError(t *testing.T) {
	restore := swapHardenDownloadClient(passthroughClient)
	defer restore()

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: readErrRoundTripper{err: errors.New("body read failed")}}, nil
	}

	_, err := fetchArtifactURL(context.Background(), f, publicArtifactURL)
	if err == nil {
		t.Fatal("response body read failure should propagate")
	}
	if !errs.IsNetwork(err) {
		t.Fatalf("a read failure should be a network error, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("subtype should be transport, got %+v", p)
	}
}

// TestDownloadArtifact_SafeOutputPathError pins that an unsafe -o (absolute path)
// is rejected by SafeOutputPath before any provider resolution or write, as an
// invalid_argument validation error carrying the output param.
func TestDownloadArtifact_SafeOutputPathError(t *testing.T) {
	fetched := false
	restore := swapResolveDownload(func(*taskOptions) (*iagents.ArtifactData, error) {
		fetched = true
		return &iagents.ArtifactData{Bytes: []byte("x")}, nil
	})
	defer restore()

	opts, _ := taskTestOpts(t, "get")
	opts.ArtifactID = "art_1"
	opts.Output = "/etc/passwd" // absolute → SafeOutputPath rejects

	err := downloadArtifact(opts)
	if err == nil {
		t.Fatal("an absolute -o should be rejected by SafeOutputPath")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("want validation error, got %T: %v", err, err)
	}
	if fetched {
		t.Error("path validation failure should not trigger artifact resolution/download")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--output" {
		t.Fatalf("validation error should carry Param=--output, got %T %+v", err, err)
	}
}

// TestDownloadArtifact_ResolveDownloadError pins that a DownloadArtifact
// (provider) failure is propagated unchanged (the descriptor fetch is the seam).
func TestDownloadArtifact_ResolveDownloadError(t *testing.T) {
	restore := swapResolveDownload(func(*taskOptions) (*iagents.ArtifactData, error) {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkServer, "artifact fetch failed")
	})
	defer restore()

	chdirTemp(t)
	opts, _ := taskTestOpts(t, "get")
	opts.ArtifactID = "art_1"
	opts.Output = "out.bin"

	err := downloadArtifact(opts)
	if err == nil {
		t.Fatal("DownloadArtifact error should propagate")
	}
	if !errs.IsNetwork(err) {
		t.Fatalf("should propagate the provider's network error, got %T: %v", err, err)
	}
}

// TestDownloadArtifact_URLFetchError pins that a failure from the URL-fetch seam
// (a URL-type artifact whose download fails) is propagated unchanged.
func TestDownloadArtifact_URLFetchError(t *testing.T) {
	restore := swapResolveDownload(func(*taskOptions) (*iagents.ArtifactData, error) {
		return &iagents.ArtifactData{Mime: "text/plain", URL: publicArtifactURL}, nil
	})
	defer restore()
	restoreFetch := swapArtifactFetch(func(context.Context, *cmdutil.Factory, string) ([]byte, error) {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "download blew up")
	})
	defer restoreFetch()

	chdirTemp(t)
	opts, _ := taskTestOpts(t, "get")
	opts.ArtifactID = "art_1"
	opts.Output = "out.bin"

	err := downloadArtifact(opts)
	if err == nil {
		t.Fatal("URL download failure should propagate")
	}
	if !errs.IsNetwork(err) {
		t.Fatalf("should propagate the fetch network error, got %T: %v", err, err)
	}
	if _, statErr := os.Stat(filepath.Join(".", "out.bin")); !os.IsNotExist(statErr) {
		t.Errorf("should not write a file on download failure, stat err=%v", statErr)
	}
}

// TestDownloadArtifact_WriteFileError pins the vfs.WriteFile failure branch: the
// path passes SafeOutputPath but its parent directory does not exist, so the
// write fails and is surfaced as an internal file_io error.
func TestDownloadArtifact_WriteFileError(t *testing.T) {
	restore := swapResolveDownload(func(*taskOptions) (*iagents.ArtifactData, error) {
		return &iagents.ArtifactData{Mime: "application/octet-stream", Bytes: []byte("INLINE")}, nil
	})
	defer restore()

	chdirTemp(t)
	opts, _ := taskTestOpts(t, "get")
	opts.ArtifactID = "art_1"
	// Nonexistent parent dir → passes SafeOutputPath (resolveNearestAncestor),
	// but vfs.WriteFile has no parent to write into and fails.
	opts.Output = filepath.Join("no_such_dir", "out.bin")

	err := downloadArtifact(opts)
	if err == nil {
		t.Fatal("a write failure should propagate")
	}
	if !errs.IsInternal(err) {
		t.Fatalf("a write failure should be an internal error, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeFileIO {
		t.Fatalf("subtype should be file_io, got %+v", p)
	}
}

// errRoundTripper is a transport that always fails, used to exercise the
// client.Do error branch of fetchArtifactURL.
type errRoundTripper struct{ err error }

// RoundTrip always returns the configured error.
func (e errRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, e.err }

// readErrRoundTripper returns a 200 response whose body fails on Read, used to
// exercise the io.ReadAll error branch of fetchArtifactURL.
type readErrRoundTripper struct{ err error }

// RoundTrip returns a 200 response backed by a reader that always errors.
func (r readErrRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(errReader{err: r.err}),
		Request:    req,
	}, nil
}

// errReader always fails on Read with its configured error.
type errReader struct{ err error }

// Read always returns the configured error.
func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// streamRoundTripper returns a 200 response whose body streams n copies of byte
// b, used to exercise the io.LimitReader truncation in fetchArtifactURL without
// materializing the whole body in memory up front.
type streamRoundTripper struct {
	n int64
	b byte
}

// RoundTrip returns a 200 response backed by an n-byte constant reader.
func (s streamRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(&constReader{remaining: s.n, b: s.b}),
		Request:    req,
	}, nil
}

// constReader yields `remaining` bytes of a constant value, then io.EOF. It lets
// the LimitReader test drive an arbitrarily large body cheaply.
type constReader struct {
	remaining int64
	b         byte
}

// Read fills p with the constant byte until `remaining` is exhausted.
func (c *constReader) Read(p []byte) (int, error) {
	if c.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > c.remaining {
		n = int(c.remaining)
	}
	for i := 0; i < n; i++ {
		p[i] = c.b
	}
	c.remaining -= int64(n)
	if c.remaining <= 0 {
		return n, io.EOF
	}
	return n, nil
}

// TestDownloadArtifact_RefusesOverwriteWithoutForce pins the -o overwrite guard:
// an existing target is a high-risk clobber, so without --force the download is
// refused with confirmation_required (exit 10) BEFORE any fetch (the
// resolveDownload seam must not be reached), and the file is left untouched.
func TestDownloadArtifact_RefusesOverwriteWithoutForce(t *testing.T) {
	fetched := false
	restore := swapResolveDownload(func(*taskOptions) (*iagents.ArtifactData, error) {
		fetched = true
		return &iagents.ArtifactData{Bytes: []byte("NEW")}, nil
	})
	defer restore()

	chdirTemp(t)
	if err := os.WriteFile("out.bin", []byte("OLD"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, _ := taskTestOpts(t, "get")
	opts.ArtifactID = "a1"
	opts.Output = "out.bin" // already exists, no --force

	err := downloadArtifact(opts)
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeConfirmationRequired {
		t.Fatalf("overwrite without --force should be confirmation_required, got %+v (err=%v)", p, err)
	}
	if output.ExitCodeOf(err) != output.ExitConfirmationRequired {
		t.Fatalf("exit should be %d, got %d", output.ExitConfirmationRequired, output.ExitCodeOf(err))
	}
	if fetched {
		t.Error("must not fetch/download when refusing to overwrite")
	}
	if b, _ := os.ReadFile("out.bin"); string(b) != "OLD" {
		t.Errorf("existing file must be left untouched, got %q", b)
	}
}

// TestDownloadArtifact_ForceOverwrites pins that --force satisfies the overwrite
// guard: an existing target is replaced with the downloaded bytes.
func TestDownloadArtifact_ForceOverwrites(t *testing.T) {
	restore := swapResolveDownload(func(*taskOptions) (*iagents.ArtifactData, error) {
		return &iagents.ArtifactData{Bytes: []byte("NEW")}, nil
	})
	defer restore()

	chdirTemp(t)
	if err := os.WriteFile("out.bin", []byte("OLD"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts, _ := taskTestOpts(t, "get")
	opts.ArtifactID = "a1"
	opts.Output = "out.bin"
	opts.Force = true

	if err := downloadArtifact(opts); err != nil {
		t.Fatalf("--force should overwrite an existing target: %v", err)
	}
	if b, _ := os.ReadFile("out.bin"); string(b) != "NEW" {
		t.Errorf("--force should have overwritten with downloaded bytes, got %q", b)
	}
}

// TestTaskListPaginationMeta pins the command-level pagination envelope: a
// provider that returns a page plus PageInfo{HasMore,NextToken} surfaces as
// meta.count / meta.has_more / meta.page_token, and meta.next carries a "next page"
// action whose command replays --page-size / --page-token.
func TestTaskListPaginationMeta(t *testing.T) {
	opts, _ := taskTestOpts(t, "list")
	opts.PageSize = 2
	setScripted(t, scriptedHooks{listTasks: func(_ string, page iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
		if page.Size != 2 {
			t.Errorf("the hook should receive the requested page size 2, got %d", page.Size)
		}
		return []iagents.TaskSummary{
				{TaskID: "chat_1", State: iagents.StateCompleted, UpdatedAt: "2026-07-05T12:00:00Z"},
				{TaskID: "chat_2", State: iagents.StateCompleted, UpdatedAt: "2026-07-05T11:00:00Z"},
			},
			iagents.PageInfo{NextToken: "2", HasMore: true}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentTaskListRun(opts); err != nil {
		t.Fatalf("paged task list should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	if env.Meta == nil {
		t.Fatal("a paged list should carry meta")
	}
	if env.Meta.Pagination == nil {
		t.Fatal("a paged list should carry meta.pagination")
	}
	if env.Meta.Pagination.Complete {
		t.Error("meta.pagination.complete should be false while a next page exists")
	}
	if env.Meta.Pagination.NextToken != "2" {
		t.Errorf("meta.pagination.next_token should be the next cursor \"2\", got %q", env.Meta.Pagination.NextToken)
	}
	found := false
	for _, n := range env.Meta.Next {
		if n.Label == "next page" && strings.Contains(n.Command, "--page-token 2") && strings.Contains(n.Command, "--page-size 2") {
			found = true
		}
	}
	if !found {
		t.Errorf("meta.next should contain a next page action replaying --page-size/--page-token, got %+v", env.Meta.Next)
	}
}

// TestTaskListPaginationUnsafeCursorDropsNextKeepsToken pins the injection-drop
// branch: an unsafe server cursor still rides meta.pagination.next_token verbatim
// (it is DATA the caller can inspect), but it fails the safeNextID whitelist so no
// executable "next page" command is emitted with it interpolated.
func TestTaskListPaginationUnsafeCursorDropsNextKeepsToken(t *testing.T) {
	opts, _ := taskTestOpts(t, "list")
	opts.PageSize = 2
	setScripted(t, scriptedHooks{listTasks: func(_ string, _ iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
		return []iagents.TaskSummary{
				{TaskID: "chat_1", State: iagents.StateCompleted, UpdatedAt: "2026-07-05T12:00:00Z"},
			},
			iagents.PageInfo{NextToken: "2 && evil", HasMore: true}, nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentTaskListRun(opts); err != nil {
		t.Fatalf("paged task list should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	if env.Meta == nil {
		t.Fatal("a paged list should carry meta")
	}
	if env.Meta.Pagination == nil {
		t.Fatal("a paged list should carry meta.pagination")
	}
	if env.Meta.Pagination.NextToken != "2 && evil" {
		t.Errorf("meta.pagination.next_token should preserve the raw cursor as data, got %q", env.Meta.Pagination.NextToken)
	}
	for _, n := range env.Meta.Next {
		if n.Label == "next page" {
			t.Errorf("an unsafe cursor must drop the executable next page command, got %+v", n)
		}
	}
}

// TestValidatePageSize pins the [1,100] range guard: 0 and 101 are rejected as
// invalid_argument validation errors carrying the --page-size param, while the
// in-range values pass.
func TestValidatePageSize(t *testing.T) {
	for _, n := range []int{0, 101} {
		err := validatePageSize(n)
		if err == nil {
			t.Fatalf("page-size %d should be rejected", n)
		}
		if !errs.IsValidation(err) {
			t.Fatalf("page-size %d should be a validation error, got %T", n, err)
		}
		p, ok := errs.ProblemOf(err)
		if !ok || p.Subtype != errs.SubtypeInvalidArgument {
			t.Fatalf("page-size %d should be invalid_argument, got %+v", n, p)
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) || ve.Param != "--page-size" {
			t.Errorf("page-size %d error should carry param --page-size, got %+v", n, ve)
		}
	}
	for _, n := range []int{1, 20, 100} {
		if err := validatePageSize(n); err != nil {
			t.Errorf("page-size %d should be valid, got %v", n, err)
		}
	}
}
