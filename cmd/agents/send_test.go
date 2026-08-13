// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// sendCmdCtx builds a `lark-cli agents send` leaf command whose CommandPath() is
// non-empty (required for content-safety scanning) and whose --as flag is
// explicitly set to bot so ResolveAs honors it verbatim.
func sendCmdCtx(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "lark-cli"}
	group := &cobra.Command{Use: "agents"}
	leaf := &cobra.Command{Use: "send"}
	root.AddCommand(group)
	group.AddCommand(leaf)
	leaf.Flags().String("as", "", "identity")
	if err := leaf.Flags().Set("as", "bot"); err != nil {
		t.Fatal(err)
	}
	leaf.SetContext(context.Background())
	return leaf
}

// sendTestOpts wires a sendOptions against a real (test) Factory, addressing
// the scripted fakeflow agent agt_x under an explicit bot identity. The
// Factory's httpmock registry holds zero stubs, so any HTTP attempt fails the
// test — everything under test here is command-layer behavior over the
// scripted provider.
// mkSendFile chdirs to a temp dir and creates name there, so --file passes the
// relative-within-CWD + existence gate (validateSendFiles) in tests.
func mkSendFile(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sendTestOpts(t *testing.T) *sendOptions {
	t.Helper()
	registerScripted()
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	return &sendOptions{
		Factory: f,
		Cmd:     sendCmdCtx(t),
		Ref:     "fakeflow:agt_x",
		As:      "bot",
	}
}

// TestSendRequiresText pins that an empty or whitespace-only --text is a validation error
// (subtype invalid_argument) raised before any provider is built.
func TestSendRequiresText(t *testing.T) {
	for name, text := range map[string]string{
		"empty":            "",
		"spaces":           "   ",
		"tab":              "\t",
		"newline":          "\n",
		"full width space": "\u3000",
	} {
		t.Run(name, func(t *testing.T) {
			err := agentSendRun(&sendOptions{Ref: "fakeflow:agt_x", Text: text})
			if err == nil {
				t.Fatal("missing --text should raise a validation error")
			}
			if !errs.IsValidation(err) {
				t.Fatalf("want validation error, got %T", err)
			}
			p, ok := errs.ProblemOf(err)
			if !ok || p.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("subtype should be invalid_argument, got %+v", p)
			}
			// hint contract: a missing --text must carry a copy-pasteable remediation
			// hint, and the param uses the -- prefix.
			if !strings.Contains(p.Hint, "--text") {
				t.Errorf("hint should guide adding --text, got %q", p.Hint)
			}
			var verr *errs.ValidationError
			if !errors.As(err, &verr) || verr.Param != "--text" {
				t.Errorf("param should be --text, got %+v", verr)
			}
		})
	}
}

// TestSendTaskIDRequiresContextID pins that --task-id without --context-id is a
// validation error, raised before any provider is built.
func TestSendTaskIDRequiresContextID(t *testing.T) {
	err := agentSendRun(&sendOptions{Ref: "fakeflow:agt_x", Text: "x", TaskID: "t1"})
	if err == nil {
		t.Fatal("--task-id without --context-id should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("want validation error, got %T", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype should be invalid_argument, got %+v", p)
	}
	// hint contract: state the next step clearly (--task-id must be provided
	// together with --context-id).
	if !strings.Contains(p.Hint, "--context-id") {
		t.Errorf("hint should note it must be used with --context-id, got %q", p.Hint)
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.Param != "--task-id" {
		t.Errorf("param should be --task-id, got %+v", verr)
	}
}

// TestSendAnswerGroup pins the structured input_required answer path: --answer
// entries need no --text, and they reach the provider hook as the map
// encoding — keys verbatim (bare vs .text), values in argv order, multi-select
// accumulated, exact duplicates deduplicated.
func TestSendAnswerGroup(t *testing.T) {
	opts := sendTestOpts(t)
	opts.ContextID = "sess_1"
	opts.TaskID = "task_1"
	opts.Answers = []string{
		"q1_a8=by_region",
		"q2_a8.text=full year 2024",
		"q3_a8=east", "q3_a8=north", "q3_a8=east", // exact dup → deduped
	}
	// deliberately no opts.Text — the answers ARE the message.
	var got iagents.SendInput
	setScripted(t, scriptedHooks{send: func(in iagents.SendInput) (*iagents.AgentTask, error) {
		got = in
		return &iagents.AgentTask{TaskID: "task_1", ContextID: "sess_1", State: iagents.StateCompleted}, nil
	}})
	if err := agentSendRun(opts); err != nil {
		t.Fatalf("answering a group should not require --text: %v", err)
	}
	if v := got.Answers["q1_a8"]; len(v) != 1 || v[0] != "by_region" {
		t.Errorf("bare answer should reach the hook as-is, got %v", got.Answers["q1_a8"])
	}
	if v := got.Answers["q2_a8.text"]; len(v) != 1 || v[0] != "full year 2024" {
		t.Errorf(".text key should stay verbatim in the map, got %v", got.Answers["q2_a8.text"])
	}
	if v := got.Answers["q3_a8"]; len(v) != 2 || v[0] != "east" || v[1] != "north" {
		t.Errorf("multi-select should accumulate in argv order and dedupe exact repeats, got %v", got.Answers["q3_a8"])
	}
}

// TestSendAnswerRequiresTaskContext pins that answering a group needs the
// task/context it belongs to (mode-first guard, before key parsing).
func TestSendAnswerRequiresTaskContext(t *testing.T) {
	err := agentSendRun(&sendOptions{Ref: "fakeflow:agt_x", Answers: []string{"q1=by_region"}})
	if err == nil {
		t.Fatal("--answer without --context-id/--task-id should error")
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.Param != "--answer" {
		t.Errorf("param should be --answer, got %+v", verr)
	}
}

// TestSendAnswerGrammar pins the offline --answer key/value grammar in one
// collect-all pass: a non-key=value entry, a near-miss suffix (.txt), a
// flag-lookalike key, an empty value, and a duplicated .text entry are ALL
// reported in one error; none of them reaches any provider.
func TestSendAnswerGrammar(t *testing.T) {
	err := agentSendRun(&sendOptions{Ref: "fakeflow:agt_x", ContextID: "sess_1", TaskID: "task_1",
		Answers: []string{
			"noequals",               // not key=value
			"q1.txt=x",               // misspelled suffix: invalid key
			"--text=x",               // flag-shaped key: illegal first character
			"q2=",                    // empty value
			"q3.text=a", "q3.text=b", // .text does not accumulate
		}})
	if err == nil {
		t.Fatal("illegal --answer entries should error offline")
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) || verr.Param != "--answer" {
		t.Fatalf("param should be --answer, got %+v", verr)
	}
	for _, frag := range []string{"noequals", "q1.txt", "--text", "q2", "q3.text"} {
		if !strings.Contains(verr.Problem.Message, frag) {
			t.Errorf("collect-all message should name %q, got %q", frag, verr.Problem.Message)
		}
	}
}

// workingTask is the canonical non-terminal task the scripted Send returns for
// the happy-path tests.
func workingTask() *iagents.AgentTask {
	return &iagents.AgentTask{TaskID: "chat_1", ContextID: "sess_1", State: iagents.StateWorking}
}

// TestSendPrettyFormat pins that `send --format pretty` renders the
// resulting task as key: value lines (previously the flag was registered but
// silently ignored).
func TestSendPrettyFormat(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Text = "analyze sales"
	opts.Format = "pretty"
	setScripted(t, scriptedHooks{send: func(iagents.SendInput) (*iagents.AgentTask, error) {
		return workingTask(), nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentSendRun(opts); err != nil {
		t.Fatalf("send --format pretty should not error: %v", err)
	}
	text := string(out.Bytes())
	for _, want := range []string{"state: working", "task_id: chat_1", "context_id: sess_1"} {
		if !strings.Contains(text, want) {
			t.Errorf("pretty output should contain %q, got:\n%s", want, text)
		}
	}
	var env output.Envelope
	if json.Unmarshal(out.Bytes(), &env) == nil && env.OK {
		t.Errorf("pretty should not be a JSON envelope: %s", text)
	}
}

// TestSendDryRunPrettyFormat pins that --dry-run also consumes --format pretty
// (key: value preview) instead of silently emitting JSON.
func TestSendDryRunPrettyFormat(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Text = "analyze sales"
	opts.DryRun = true
	opts.Format = "pretty"
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentSendRun(opts); err != nil {
		t.Fatalf("dry-run pretty should not error: %v", err)
	}
	text := string(out.Bytes())
	for _, want := range []string{"dry_run: true", "ref: fakeflow:agt_x", "text: analyze sales"} {
		if !strings.Contains(text, want) {
			t.Errorf("pretty output should contain %q, got:\n%s", want, text)
		}
	}
	var env output.Envelope
	if json.Unmarshal(out.Bytes(), &env) == nil && env.OK {
		t.Errorf("pretty should not be a JSON envelope: %s", text)
	}
}

// TestSendDryRunPrettyNeutralizesInjection pins F2: the dry-run pretty preview
// runs context_id/task_id through kvValue (like every other pretty face), so a
// value carrying a newline cannot forge an adjacent "key: value" field row.
func TestSendDryRunPrettyNeutralizesInjection(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Text = "hi"
	opts.DryRun = true
	opts.Format = "pretty"
	opts.ContextID = "ctx1\nstate: completed"
	opts.TaskID = "task1\ndeleted: true"
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentSendRun(opts); err != nil {
		t.Fatalf("dry-run pretty should not error: %v", err)
	}
	text := string(out.Bytes())
	// The raw newline must not survive into a forged adjacent row.
	if strings.Contains(text, "context_id: ctx1\nstate: completed") {
		t.Errorf("context_id newline not neutralized, forged a field row:\n%s", text)
	}
	if strings.Contains(text, "task_id: task1\ndeleted: true") {
		t.Errorf("task_id newline not neutralized, forged a field row:\n%s", text)
	}
	// kvValue collapses the newline to a space, keeping the value on one line.
	if !strings.Contains(text, "context_id: ctx1 state: completed") {
		t.Errorf("context_id should collapse to one line, got:\n%s", text)
	}
	if !strings.Contains(text, "task_id: task1 deleted: true") {
		t.Errorf("task_id should collapse to one line, got:\n%s", text)
	}
}

// TestSendNoParamsRequired pins card v2: the scripted card declares no
// parameters, so a send without any --param passes card validation — asserted
// via --dry-run so no provider Send fires. A malformed --param is still a
// validation error.
func TestSendNoParamsRequired(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Text = "analyze sales"
	opts.Params = nil
	opts.DryRun = true
	if err := agentSendRun(opts); err != nil {
		t.Fatalf("card has no required params, send without --param should pass validation: %v", err)
	}

	opts2 := sendTestOpts(t)
	opts2.Text = "analyze sales"
	opts2.Params = []string{"noequals"} // a --param without '=' should still raise validation
	opts2.DryRun = true
	err := agentSendRun(opts2)
	if err == nil {
		t.Fatal("malformed --param should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("want validation error, got %T", err)
	}
}

// TestSendUnknownParamRejected pins, against an empty-parameters card, that
// any --param key is unknown → invalid_argument with a hint pointing at
// `agents card`, raised before any provider Send (asserted via --dry-run with
// no send hook installed).
func TestSendUnknownParamRejected(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Text = "analyze sales"
	opts.Params = []string{"app_id=app_1"}
	opts.DryRun = true
	err := agentSendRun(opts)
	if err == nil {
		t.Fatal("card did not declare app_id, --param app_id should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("want validation error, got %T", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype should be invalid_argument, got %+v", p)
	}
	if !strings.Contains(p.Hint, "agents card") {
		t.Fatalf("hint should point to agents card, got %q", p.Hint)
	}
}

// TestSendDryRun pins that --dry-run prints a would_send preview and never
// calls the provider (no send hook installed → a Send would panic).
func TestSendDryRun(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Text = "analyze sales"
	opts.DryRun = true
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentSendRun(opts); err != nil {
		t.Fatalf("dry-run should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("dry-run output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	if !env.OK {
		t.Errorf("ok should be true: %+v", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data should be an object, got %T", env.Data)
	}
	if data["dry_run"] != true {
		t.Errorf("data.dry_run should be true, got %v", data["dry_run"])
	}
	would, ok := data["would_send"].(map[string]interface{})
	if !ok {
		t.Fatalf("data.would_send should be an object, got %T", data["would_send"])
	}
	if would["text"] != "analyze sales" {
		t.Errorf("would_send.text should echo the text, got %v", would["text"])
	}
}

// TestSendDryRunRejectsProviderUnsupportedIdentity ensures --dry-run obeys the
// same provider identity contract as a live send while remaining network-free.
func TestSendDryRunRejectsProviderUnsupportedIdentity(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Ref = "fakeuseronly:agt_x"
	opts.Text = "analyze sales"
	opts.DryRun = true

	err := agentSendRun(opts)
	if err == nil {
		t.Fatal("bot dry-run should be rejected by a user-only provider")
	}
	p, ok := errs.ProblemOf(err)
	var validationErr *errs.ValidationError
	if !ok || p.Subtype != errs.SubtypeInvalidArgument || !errors.As(err, &validationErr) || validationErr.Param != "--as" {
		t.Fatalf("bot dry-run should fail as invalid_argument for --as, got problem=%+v err=%v", p, err)
	}
}

// TestSendStartsTask pins the happy path: a single Send fires and returns the
// submitted / working task in a success envelope immediately (no polling), with
// a meta.next hint pointing at task get --watch.
func TestSendStartsTask(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Text = "analyze sales"
	var gotText string
	setScripted(t, scriptedHooks{send: func(in iagents.SendInput) (*iagents.AgentTask, error) {
		gotText = in.Text
		return workingTask(), nil
	}})
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentSendRun(opts); err != nil {
		t.Fatalf("send should not error: %v", err)
	}
	if gotText != "analyze sales" {
		t.Errorf("provider should receive the original text, got %q", gotText)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	data, _ := env.Data.(map[string]interface{})
	if data["task_id"] != "chat_1" {
		t.Errorf("task_id should be chat_1, got %v", data["task_id"])
	}
	if data["state"] != string(iagents.StateWorking) {
		t.Errorf("state should be working, got %v", data["state"])
	}
	// meta.next should suggest polling / continuing.
	if !strings.Contains(string(out.Bytes()), `"next"`) {
		t.Errorf("non-terminal should provide meta.next follow-up: %s", string(out.Bytes()))
	}
}

// TestSendSendError surfaces a provider Send failure unchanged.
func TestSendSendError(t *testing.T) {
	opts := sendTestOpts(t)
	opts.Text = "x"
	setScripted(t, scriptedHooks{send: func(iagents.SendInput) (*iagents.AgentTask, error) {
		return nil, errs.NewAPIError(errs.SubtypeUnknown, "app ticket invalid").WithCode(99991663)
	}})
	if err := agentSendRun(opts); err == nil {
		t.Fatal("Send error should propagate")
	}
}

// TestSendInvalidRef surfaces a malformed ref as a validation error after the
// text/task-id guards pass.
func TestSendInvalidRef(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu})
	err := agentSendRun(&sendOptions{Ref: "no-colon", Text: "x", Cmd: sendCmdCtx(t), As: "bot", Factory: f})
	if err == nil {
		t.Fatal("malformed ref should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("want validation error, got %T", err)
	}
}

// TestNewCmdAgentSend_WriteRiskAndArgs pins ExactArgs(1), write risk, and the
// presence of the send-specific flags.
func TestNewCmdAgentSend_WriteRiskAndArgs(t *testing.T) {
	cmd := NewCmdAgentSend(nil, nil)
	if level, ok := cmdutil.GetRisk(cmd); !ok || level != cmdutil.RiskWrite {
		t.Errorf("agents send should be marked write risk, got level=%q ok=%v", level, ok)
	}
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Error("agents send missing ref should raise an args error (ExactArgs 1)")
	}
	if err := cmd.Args(cmd, []string{"fakeflow:x"}); err != nil {
		t.Errorf("agents send with a single ref should be valid: %v", err)
	}
	for _, name := range []string{"text", "file", "param", "context-id", "task-id", "dry-run", "as", "format", "jq"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("agents send should have --%s flag", name)
		}
	}
	if cmd.Flags().Lookup("wait") != nil {
		t.Error("agents send --wait should be removed (polling goes through task get --watch)")
	}
	// The --file help must point out that files are sent off to the remote
	// provider (file-egress requirement).
	fileFlag := cmd.Flags().Lookup("file")
	if fileFlag != nil && !strings.Contains(fileFlag.Usage, "uploaded") && !strings.Contains(fileFlag.Usage, "leaves this machine") {
		t.Errorf("--file help should note files are sent out to the remote provider, got %q", fileFlag.Usage)
	}
}

// TestNewCmdAgentSend_RunFOverride confirms the injected runF hook is used
// instead of the production path (construction-time seam).
func TestNewCmdAgentSend_RunFOverride(t *testing.T) {
	called := false
	var captured *sendOptions
	cmd := NewCmdAgentSend(nil, func(opts *sendOptions) error {
		called = true
		captured = opts
		return nil
	})
	cmd.SetArgs([]string{"fakeflow:agt_x", "--text", "hi"})
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute should not error: %v", err)
	}
	if !called {
		t.Fatal("runF should be called")
	}
	if captured.Ref != "fakeflow:agt_x" || captured.Text != "hi" {
		t.Errorf("opts not populated correctly: %+v", captured)
	}
}

// TestSend_FileRequiresYes pins the --file exfil confirmation gate: a real send
// carrying --file to a provider that supports file upload (the scripted card has
// file_input=true) requires --yes, so without it the command returns
// confirmation_required (exit 10) BEFORE reaching the provider — the unset send
// hook is a tripwire that would panic if the gate let the upload through.
func TestSend_FileRequiresYes(t *testing.T) {
	mkSendFile(t, "local.txt")
	opts := sendTestOpts(t)
	opts.Text = "hi"
	opts.Files = []string{"local.txt"} // no --yes

	err := agentSendRun(opts)
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeConfirmationRequired {
		t.Fatalf("send --file without --yes should be confirmation_required, got %+v (err=%v)", p, err)
	}
	if output.ExitCodeOf(err) != output.ExitConfirmationRequired {
		t.Fatalf("exit should be %d, got %d", output.ExitConfirmationRequired, output.ExitCodeOf(err))
	}
}

// TestSend_FileWithYesProceeds pins that --yes satisfies the --file gate: the
// send reaches the provider, which receives the file path.
func TestSend_FileWithYesProceeds(t *testing.T) {
	mkSendFile(t, "local.txt")
	opts := sendTestOpts(t)
	sent := false
	setScripted(t, scriptedHooks{send: func(in iagents.SendInput) (*iagents.AgentTask, error) {
		sent = true
		if len(in.Files) != 1 || in.Files[0] != "local.txt" {
			t.Errorf("provider should receive the --file path, got %v", in.Files)
		}
		return &iagents.AgentTask{TaskID: "t1", State: iagents.StateCompleted, IsTerminal: true}, nil
	}})
	opts.Text = "hi"
	opts.Files = []string{"local.txt"}
	opts.Yes = true

	if err := agentSendRun(opts); err != nil {
		t.Fatalf("send --file --yes should proceed: %v", err)
	}
	if !sent {
		t.Error("provider Send should be reached after --yes")
	}
}

// TestSend_FileDryRunNotGated pins that --dry-run with --file is exempt from the
// CONFIRMATION gate (dry-run never uploads), so a file_input=TRUE provider needs
// no --yes and never reaches the provider (unset send hook stays a tripwire).
// The capability gate is a separate matter — see TestSend_DryRunGatedByCapability.
func TestSend_FileDryRunNotGated(t *testing.T) {
	mkSendFile(t, "local.txt")
	opts := sendTestOpts(t) // fakeflow declares file_input=true
	opts.Text = "hi"
	opts.Files = []string{"local.txt"}
	opts.DryRun = true // no --yes

	if err := agentSendRun(opts); err != nil {
		t.Fatalf("dry-run --file should not be gated: %v", err)
	}
}

// TestSend_DryRunGatedByCapability pins that --dry-run is NOT a capability
// bypass: against an agent that declares file_input=false / input_required=false,
// --file / --answer are rejected with unsupported_capability under dry-run — the
// SAME verdict a real send would give. This closes the "dry-run false green
// light" gap, where a preview returned ok for a send the provider could never
// accept, misleading a caller that uses dry-run to pre-check a command.
func TestSend_DryRunGatedByCapability(t *testing.T) {
	wantUnsupported := func(t *testing.T, err error) {
		t.Helper()
		p, ok := errs.ProblemOf(err)
		if !ok || p.Subtype != errs.Subtype("unsupported_capability") {
			t.Fatalf("want unsupported_capability under dry-run, got problem=%+v err=%v", p, err)
		}
	}
	// --file against file_input=false (fakemin declares neither capability).
	t.Run("file", func(t *testing.T) {
		mkSendFile(t, "local.txt")
		opts := sendTestOpts(t)
		opts.Ref = "fakemin:agt_x"
		opts.Text = "hi"
		opts.Files = []string{"local.txt"}
		opts.DryRun = true
		wantUnsupported(t, agentSendRun(opts))
	})
	// --answer against input_required=false.
	t.Run("answer", func(t *testing.T) {
		opts := sendTestOpts(t)
		opts.Ref = "fakemin:agt_x"
		opts.ContextID = "ctx_x"
		opts.TaskID = "task_x"
		opts.Answers = []string{"q1=a1"}
		opts.DryRun = true
		wantUnsupported(t, agentSendRun(opts))
	})
}
