// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/output"
)

// writeAppConfigWithoutUser installs a config holding an app but no logged-in
// user, which is what drives `auth check` down its exit-code-only signal path.
func writeAppConfigWithoutUser(t *testing.T, cfgDir string) {
	t.Helper()
	body := `{"apps":[{"name":"probe","appId":"cli_probe","appSecret":"probe-secret","brand":"feishu","users":[]}],"currentApp":"probe"}`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// captureShutdownErr registers a plugin whose only job is to record the error
// handed to the Shutdown lifecycle event.
func captureShutdownErr(t *testing.T, observed *error, fired *int) {
	t.Helper()
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	platform.Register(platform.NewPlugin("probe", "1.0").
		On(platform.Shutdown, "capture",
			func(_ context.Context, lc *platform.LifecycleContext) error {
				*fired++
				*observed = lc.Err
				return nil
			}).
		MustBuild())
}

func quietNotices(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("LARKSUITE_CLI_NO_SKILLS_NOTIFIER", "1")
}

// TestExitCodeOnlySignalSurvivesFullDispatch pins the contract that an
// exit-code-only signal keeps its exit code and writes nothing to stderr when
// it travels the whole way through ExecuteWithOptions. Classifying it instead
// would put a second, contradictory error envelope on stderr next to the
// result already on stdout, and replace exit 1 with the internal-fault code.
func TestExitCodeOnlySignalSurvivesFullDispatch(t *testing.T) {
	cfgDir := tmpHome(t)
	quietNotices(t)
	writeAppConfigWithoutUser(t, cfgDir)

	var observed error
	fired := 0
	captureShutdownErr(t, &observed, &fired)

	code, stdout, stderr := executeWithCapturedOS(t, nil,
		"auth", "check", "--scope", "im:message:send_as_bot")

	if stderr != "" {
		t.Errorf("stderr must stay empty for an exit-code-only signal, got %q", stderr)
	}
	if !strings.Contains(stdout, `"ok": false`) {
		t.Errorf("the result envelope belongs on stdout, got %q", stdout)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1 (the signal's own code)", code)
	}
	if fired != 1 {
		t.Fatalf("Shutdown handler fired %d times, want 1", fired)
	}
	if _, ok := errs.ProblemOf(observed); ok {
		t.Errorf("Shutdown observed a classified error %v; an exit-code-only "+
			"signal carries no Problem and must pass through unchanged", observed)
	}
	var bare *output.BareError
	if !errors.As(observed, &bare) {
		t.Errorf("Shutdown observed %T, want the original *output.BareError", observed)
	}
}

// TestShutdownHookCannotRewriteUserVisibleFailure pins that what the user got
// is settled before the Shutdown event fires. Typed errors carry exported
// fields, so a handler reaching through errs.ProblemOf really can write to the
// error it is given; ordering is what makes that harmless.
func TestShutdownHookCannotRewriteUserVisibleFailure(t *testing.T) {
	tmpHome(t)
	quietNotices(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	fired := 0
	platform.Register(platform.NewPlugin("tamper", "1.0").
		On(platform.Shutdown, "rewrite",
			func(_ context.Context, lc *platform.LifecycleContext) error {
				fired++
				if p, ok := errs.ProblemOf(lc.Err); ok {
					p.Category = errs.CategoryNetwork
					p.Subtype = "rewritten_by_plugin"
					p.Message = "rewritten by plugin"
					p.Hint = "rewritten by plugin"
				}
				return nil
			}).
		MustBuild())

	code, _, stderr := executeWithCapturedOS(t, nil, "definitely-not-a-command")

	// Without this, a Shutdown handler that never runs would leave the
	// envelope untouched and pass the assertions below for the wrong reason.
	if fired != 1 {
		t.Fatalf("Shutdown handler fired %d times, want 1", fired)
	}

	var envelope struct {
		Error struct {
			Category errs.Category `json:"type"`
			Subtype  errs.Subtype  `json:"subtype"`
			Message  string        `json:"message"`
			Hint     string        `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, stderr)
	}
	if envelope.Error.Category != errs.CategoryValidation ||
		envelope.Error.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("plugin rewrote the envelope: %s/%s",
			envelope.Error.Category, envelope.Error.Subtype)
	}
	if strings.Contains(envelope.Error.Message, "rewritten") ||
		strings.Contains(envelope.Error.Hint, "rewritten") {
		t.Errorf("plugin rewrote user-visible text: message=%q hint=%q",
			envelope.Error.Message, envelope.Error.Hint)
	}
	if code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d; a Shutdown handler must not change it",
			code, output.ExitValidation)
	}
}

// TestCobraValidationFailuresAreUserErrors covers every shape cobra rejects a
// command line with before any command body runs. All of them are mistakes in
// what the user typed, so none may be reported as an internal fault — that
// would tell the user the tool broke and would count their typo against
// service health.
func TestCobraValidationFailuresAreUserErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"unknown command", []string{"definitely-not-a-command"}},
		{"unknown subcommand", []string{"sheets", "+definitely-nope"}},
		{"missing required flag", []string{"auth", "check"}},
		{"wrong argument count", []string{"profile", "remove", "first", "second"}},
		{"positional arg on a shortcut", []string{"wiki", "+space-list", "stray"}},
		{"flag group one-required", []string{
			"sheets", "+csv-put", "--spreadsheet-token", "Xxxxxxxxxxx",
			"--sheet-id", "abc", "--csv", "a,b"}},
		{"flag group mutually exclusive", []string{
			"sheets", "+csv-put", "--spreadsheet-token", "Xxxxxxxxxxx",
			"--sheet-id", "abc", "--csv", "a,b", "--start-cell", "A1", "--range", "A1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpHome(t)
			quietNotices(t)

			var observed error
			fired := 0
			captureShutdownErr(t, &observed, &fired)

			code, _, stderr := executeWithCapturedOS(t, nil, tc.args...)

			var envelope struct {
				Error struct {
					Category errs.Category `json:"type"`
					Subtype  errs.Subtype  `json:"subtype"`
					Message  string        `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
				t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, stderr)
			}
			if envelope.Error.Category != errs.CategoryValidation ||
				envelope.Error.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("reported as %s/%s, want %s/%s",
					envelope.Error.Category, envelope.Error.Subtype,
					errs.CategoryValidation, errs.SubtypeInvalidArgument)
			}
			if code != output.ExitValidation {
				t.Errorf("exit code = %d, want %d", code, output.ExitValidation)
			}

			// The Shutdown handler must agree with what the user was told.
			if fired != 1 {
				t.Fatalf("Shutdown handler fired %d times, want 1", fired)
			}
			problem, ok := errs.ProblemOf(observed)
			if !ok {
				t.Fatalf("Shutdown observed unclassified %T (%v)", observed, observed)
			}
			if problem.Category != envelope.Error.Category ||
				problem.Subtype != envelope.Error.Subtype ||
				problem.Message != envelope.Error.Message {
				t.Errorf("Shutdown observed %s/%s %q, envelope reports %s/%s %q",
					problem.Category, problem.Subtype, problem.Message,
					envelope.Error.Category, envelope.Error.Subtype, envelope.Error.Message)
			}
			if got := output.ExitCodeOf(observed); got != code {
				t.Errorf("Shutdown observed exit code %d, process exited %d", got, code)
			}
		})
	}
}

// TestEveryArgsValidatorProducesTypedErrors is the guard that keeps a newly
// added positional validator from silently reporting a user's mistake as an
// internal fault: instrumentErrorBoundaries must reach every command in the
// tree, so no Args validator can return an unclassified error. Empty, single,
// and oversized probes cover both lower- and upper-bound validators.
func TestEveryArgsValidatorProducesTypedErrors(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	_, root, _ := buildInternal(context.Background(), buildInvocationForTest(t), WithoutPlugins())

	oversized := make([]string, 64)
	for i := range oversized {
		oversized[i] = fmt.Sprintf("stray%d", i+1)
	}
	probes := []struct {
		name string
		args []string
	}{
		{name: "empty"},
		{name: "single", args: []string{"stray"}},
		{name: "oversized", args: oversized},
	}
	hits := make([]int, len(probes))
	var unguarded []string
	forEachCommand(root, func(c *cobra.Command) {
		if c.Args == nil {
			return
		}
		for i, probe := range probes {
			err := c.Args(c, probe.args)
			if err == nil {
				continue
			}
			hits[i]++
			problem, ok := errs.ProblemOf(err)
			if !ok {
				unguarded = append(unguarded,
					fmt.Sprintf("%s/%s (unclassified)", c.CommandPath(), probe.name))
				continue
			}
			// A typed error is not enough: rejecting what the user typed must be
			// reported as a user error, never as an internal fault.
			if problem.Category != errs.CategoryValidation ||
				problem.Subtype != errs.SubtypeInvalidArgument {
				unguarded = append(unguarded, fmt.Sprintf("%s/%s (%s/%s)",
					c.CommandPath(), probe.name, problem.Category, problem.Subtype))
			}
		}
	})

	for i, hit := range hits {
		if hit == 0 {
			t.Errorf("no Args validator rejected the %s probe; the walk did not cover that shape", probes[i].name)
		}
	}
	if len(unguarded) > 0 {
		t.Errorf("these commands misreport a positional-argument mistake: %v", unguarded)
	}
}

// forEachCommand visits root and every command beneath it.
func forEachCommand(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, sub := range cmd.Commands() {
		forEachCommand(sub, visit)
	}
}

// unrenderableTypedError is a problem carrier the envelope writer cannot
// serialize: the exported func field makes json.Marshal fail. A plugin's Wrap
// chain returning a value like this is the realistic way to reach the
// dispatcher's last-resort branch.
type unrenderableTypedError struct {
	*errs.Problem
	Leak func() `json:"leak"`
}

type silentShortWriter struct{}

func (silentShortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

// TestUnrenderableTypedErrorStillReachesStderr pins why the last-resort branch
// rebuilds the error instead of reusing it: the value it receives has just
// failed to serialize, so handing the same value to the writer again would
// leave the user with a non-zero exit and a silent stderr.
//
// The probe deliberately carries a category other than internal. With an
// internal one, a fallback that discarded the producer's category would land
// on the same answer by coincidence and the assertions below would hold for
// the wrong reason.
func TestUnrenderableTypedErrorStillReachesStderr(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	broken := &unrenderableTypedError{
		Problem: &errs.Problem{
			Category: errs.CategoryNetwork,
			Subtype:  errs.SubtypeNetworkTimeout,
			Message:  "upstream blew up",
		},
		Leak: func() {},
	}

	// Precondition: this value really cannot render itself.
	if output.WriteTypedErrorEnvelope(io.Discard, broken, "user") {
		t.Fatal("precondition failed: the probe error serialized successfully")
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	errOut := &bytes.Buffer{}
	f.IOStreams.ErrOut = errOut

	exit := handleRootError(f, broken, nil)

	if errOut.Len() == 0 {
		t.Fatal("stderr is empty; a non-zero exit must always be explained")
	}
	var envelope struct {
		Error struct {
			Category errs.Category `json:"type"`
			Message  string        `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(errOut.Bytes(), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, errOut.String())
	}
	if envelope.Error.Message != "upstream blew up" {
		t.Errorf("message = %q, want the original text preserved", envelope.Error.Message)
	}
	if want := output.ExitCodeOf(broken); exit != want {
		t.Errorf("exit = %d, want %d; the rebuilt error must keep the original category's exit code",
			exit, want)
	}
}

// TestShortcutPositionalErrorPointsAtTheFlags pins the diagnostics the shortcut
// framework adds over the generic Args wrapper: the stray word the user typed,
// and where to look for the flags to use instead.
//
// It also pins what the envelope must not carry. `param` names the parameter
// the caller has to correct and agents read it literally; a shortcut declares
// no positional parameter, so echoing the stray word there would send an agent
// looking for a flag by that name. It would also put unbounded user input in a
// field the contract classes as stable per subtype.
func TestShortcutPositionalErrorPointsAtTheFlags(t *testing.T) {
	tmpHome(t)
	quietNotices(t)

	_, _, stderr := executeWithCapturedOS(t, nil, "wiki", "+space-list", "stray")

	var envelope struct {
		Error struct {
			Message string              `json:"message"`
			Hint    string              `json:"hint"`
			Param   string              `json:"param"`
			Params  []errs.InvalidParam `json:"params"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, stderr)
	}
	if !strings.Contains(envelope.Error.Message, "stray") {
		t.Errorf("message = %q, want the stray word shown", envelope.Error.Message)
	}
	if !strings.Contains(envelope.Error.Hint, "--help") {
		t.Errorf("hint = %q, want it to point at --help", envelope.Error.Hint)
	}
	if envelope.Error.Param != "" {
		t.Errorf("param = %q, want it absent: a shortcut has no positional parameter to name",
			envelope.Error.Param)
	}
	if len(envelope.Error.Params) != 0 {
		t.Errorf("params = %v, want none", envelope.Error.Params)
	}
}

// TestErrorClassificationFollowsBoundaryNotText keeps text matching from
// creeping back into classification. The exact same message is a user error
// when an Args validator returns it and an internal fault when application
// execution returns it. Every error-returning Cobra callback is covered.
func TestErrorClassificationFollowsBoundaryNotText(t *testing.T) {
	const sharedText = `required flag(s) "csv" not set`
	causes := map[string]error{}
	newCause := func(name string) error {
		causes[name] = errors.New(sharedText)
		return causes[name]
	}

	cmd := &cobra.Command{
		Use: "probe",
		Args: func(*cobra.Command, []string) error {
			return newCause("Args")
		},
		PersistentPreRunE: func(*cobra.Command, []string) error {
			return newCause("PersistentPreRunE")
		},
		PreRunE: func(*cobra.Command, []string) error {
			return newCause("PreRunE")
		},
		RunE: func(*cobra.Command, []string) error {
			return newCause("RunE")
		},
		PostRunE: func(*cobra.Command, []string) error {
			return newCause("PostRunE")
		},
		PersistentPostRunE: func(*cobra.Command, []string) error {
			return newCause("PersistentPostRunE")
		},
	}
	instrumentErrorBoundaries(cmd)

	assertClassified := func(name string, got error, category errs.Category) {
		t.Helper()
		problem, ok := errs.ProblemOf(got)
		if !ok {
			t.Fatalf("%s returned unclassified %T (%v)", name, got, got)
		}
		wantSubtype := errs.SubtypeUnknown
		if category == errs.CategoryValidation {
			wantSubtype = errs.SubtypeInvalidArgument
		}
		if problem.Category != category || problem.Subtype != wantSubtype {
			t.Errorf("%s classified as %s/%s, want %s/%s",
				name, problem.Category, problem.Subtype, category, wantSubtype)
		}
		if !errors.Is(got, causes[name]) {
			t.Errorf("%s lost its original cause", name)
		}
	}

	assertClassified("Args", cmd.Args(cmd, nil), errs.CategoryValidation)
	for name, invoke := range map[string]func() error{
		"PersistentPreRunE":  func() error { return cmd.PersistentPreRunE(cmd, nil) },
		"PreRunE":            func() error { return cmd.PreRunE(cmd, nil) },
		"RunE":               func() error { return cmd.RunE(cmd, nil) },
		"PostRunE":           func() error { return cmd.PostRunE(cmd, nil) },
		"PersistentPostRunE": func() error { return cmd.PersistentPostRunE(cmd, nil) },
	} {
		assertClassified(name, invoke(), errs.CategoryInternal)
	}
}

func TestErrorBoundaryPassesTypedErrorsAndExitSignalsUnchanged(t *testing.T) {
	instrumentErrorBoundaries(nil) // must not panic

	typed := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "timed out")
	bare := fmt.Errorf("wrapped bare: %w", output.ErrBare(7))
	partial := fmt.Errorf("wrapped partial: %w", output.PartialFailure(8))
	for name, err := range map[string]error{
		"typed": typed, "bare": bare, "partial": partial,
	} {
		if got := typedCommandError(err); got != err {
			t.Errorf("command %s identity changed: got %T %v, want %T %v", name, got, got, err, err)
		}
		if got := typedArgsError(err); got != err {
			t.Errorf("Args %s identity changed: got %T %v, want %T %v", name, got, got, err, err)
		}
	}
}

// TestRepeatedExecutionDoesNotLeakClassification proves classification is
// attached to the callback result, not retained as mutable process/tree state.
func TestRepeatedExecutionDoesNotLeakClassification(t *testing.T) {
	bodyCause := errors.New("body failed")
	root := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
	body := &cobra.Command{
		Use:  "body",
		RunE: func(*cobra.Command, []string) error { return bodyCause },
	}
	validation := &cobra.Command{
		Use:  "validation",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	validation.Flags().String("required", "", "required probe")
	if err := validation.MarkFlagRequired("required"); err != nil {
		t.Fatal(err)
	}
	root.AddCommand(body, validation)
	instrumentErrorBoundaries(root)

	root.SetArgs([]string{"body"})
	first := root.Execute()
	if p, ok := errs.ProblemOf(first); !ok || p.Category != errs.CategoryInternal || !errors.Is(first, bodyCause) {
		t.Fatalf("first execution = %T %v, want internal preserving body cause", first, first)
	}

	root.SetArgs([]string{"validation"})
	second := normalizeRootError(root.Execute())
	if p, ok := errs.ProblemOf(second); !ok || p.Category != errs.CategoryValidation {
		t.Fatalf("second execution = %T %v, want residual Cobra validation independent of the first run", second, second)
	}
}

// TestFinalBuiltHelpCommandIsInstrumented pins build ordering. Concealment
// installs a custom help RunE after plugin hooks; a raw failure inside that
// late command is application execution, not a malformed command line.
func TestFinalBuiltHelpCommandIsInstrumented(t *testing.T) {
	tmpHome(t)
	quietNotices(t)
	registerRestriction(t, []string{"skills/read"}, nil)

	_, root, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		ConcealRestrictedCommands(),
	)
	want := errors.New("usage renderer failed")
	root.SetOut(io.Discard)
	root.SetUsageFunc(func(*cobra.Command) error { return want })
	root.SetArgs([]string{"help", "definitely-missing"})

	got := root.Execute()
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("late help failure = %T %v, want internal/unknown", got, got)
	}
	if !errors.Is(got, want) {
		t.Error("late help failure lost the usage renderer cause")
	}
}

func TestLazyCompletionArgsFailureIsValidation(t *testing.T) {
	tmpHome(t)
	_, root, _ := buildInternal(
		context.Background(), buildInvocationForTest(t), WithoutPlugins(), WithoutServiceCommands(),
	)
	root.SetArgs([]string{"__complete"})

	got := normalizeRootError(root.Execute())
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("lazy completion Args failure = %T %v, want validation/invalid_argument", got, got)
	}
}

func TestVersionWriterFailureIsInternal(t *testing.T) {
	tmpHome(t)
	want := &failingWriter{limit: 0}
	_, root, _ := buildInternal(
		context.Background(),
		buildInvocationForTest(t),
		WithIO(strings.NewReader(""), want, io.Discard),
		WithoutPlugins(),
		WithoutServiceCommands(),
	)
	root.SetArgs([]string{"--version"})

	got := normalizeRootError(root.Execute())
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("version writer failure = %T %v, want internal/unknown", got, got)
	}
	if !errors.Is(got, io.ErrShortWrite) {
		t.Error("version writer failure lost io.ErrShortWrite")
	}
}

func TestInternalErrorWriterTypesSilentShortWrite(t *testing.T) {
	n, got := (internalErrorWriter{Writer: silentShortWriter{}}).Write([]byte("version"))
	if n != len("version")-1 {
		t.Fatalf("bytes written = %d, want %d", n, len("version")-1)
	}
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeUnknown {
		t.Fatalf("silent short write = %T %v, want internal/unknown", got, got)
	}
	if !errors.Is(got, io.ErrShortWrite) {
		t.Error("silent short write did not preserve io.ErrShortWrite")
	}
}

// TestWrapperFailureIsOurFaultNotTheUsers pins that a plugin wrapper failing
// before it delegates is attributed to us. The wrapper chain is part of
// executing the command, so its failure is never a mistake in what the user
// typed — reporting it as invalid input would send the user looking at their
// own command line for a fault that is not there.
func TestWrapperFailureIsOurFaultNotTheUsers(t *testing.T) {
	tmpHome(t)
	quietNotices(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	platform.Register(platform.NewPlugin("backend", "1.0").
		Wrap("gate", platform.All(), func(next platform.Handler) platform.Handler {
			return func(context.Context, platform.Invocation) error {
				// Fails without delegating, and without using AbortError.
				return errors.New("plugin backend unavailable")
			}
		}).FailOpen().MustBuild())

	code, _, stderr := executeWithCapturedOS(t, nil, "profile", "list")

	var envelope struct {
		Error struct {
			Category errs.Category `json:"type"`
			Subtype  errs.Subtype  `json:"subtype"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("stderr is not a JSON envelope: %v\n%s", err, stderr)
	}
	if envelope.Error.Category != errs.CategoryInternal ||
		envelope.Error.Subtype != errs.SubtypeUnknown {
		t.Errorf("reported as %s/%s, want %s/%s",
			envelope.Error.Category, envelope.Error.Subtype,
			errs.CategoryInternal, errs.SubtypeUnknown)
	}
	if code != output.ExitInternal {
		t.Errorf("exit code = %d, want %d", code, output.ExitInternal)
	}
}

// TestShutdownHookCannotRewriteBareExit pins that the user-visible exit is
// settled before Shutdown. Clone isolation for both exit signal types is
// covered directly in internal/hook.
func TestShutdownHookCannotRewriteBareExit(t *testing.T) {
	cfgDir := tmpHome(t)
	quietNotices(t)
	writeAppConfigWithoutUser(t, cfgDir)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	rewrote := false
	platform.Register(platform.NewPlugin("tamper", "1.0").
		On(platform.Shutdown, "rewrite",
			func(_ context.Context, lc *platform.LifecycleContext) error {
				var bare *output.BareError
				if errors.As(lc.Err, &bare) {
					bare.Code = 0
					rewrote = true
				}
				return nil
			}).
		MustBuild())

	code, _, _ := executeWithCapturedOS(t, nil,
		"auth", "check", "--scope", "im:message:send_as_bot")

	if !rewrote {
		t.Fatal("the handler never saw an exit-code-only signal; the test no longer covers its own premise")
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1; a Shutdown handler must not be able to change it", code)
	}
}

// TestShutdownHandlersDoNotSeeEachOthersEdits pins that handlers are isolated
// from one another. They run in registration order against the same failure, so
// sharing one error value would let whichever runs first decide what every
// later audit handler records.
func TestShutdownHandlersDoNotSeeEachOthersEdits(t *testing.T) {
	tmpHome(t)
	quietNotices(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	var secondSaw errs.Category
	secondRan := false
	platform.Register(platform.NewPlugin("tamper", "1.0").
		On(platform.Shutdown, "rewrite",
			func(_ context.Context, lc *platform.LifecycleContext) error {
				if p, ok := errs.ProblemOf(lc.Err); ok {
					p.Category = errs.CategoryNetwork
					p.Subtype = "rewritten_by_first_handler"
				}
				return nil
			}).
		MustBuild())
	platform.Register(platform.NewPlugin("audit", "1.0").
		On(platform.Shutdown, "observe",
			func(_ context.Context, lc *platform.LifecycleContext) error {
				secondRan = true
				if p, ok := errs.ProblemOf(lc.Err); ok {
					secondSaw = p.Category
				}
				return nil
			}).
		MustBuild())

	executeWithCapturedOS(t, nil, "definitely-not-a-command")

	if !secondRan {
		t.Fatal("the second handler never ran; the test no longer covers its own premise")
	}
	if secondSaw != errs.CategoryValidation {
		t.Errorf("second handler observed %q, want %q — the first handler's edit leaked",
			secondSaw, errs.CategoryValidation)
	}
}

// TestFailingShutdownHookLeavesTheEnvelopeLastOnStderr pins the byte order of
// the two independent writers that share stderr. The hook layer warns about a
// handler that fails or is skipped; the dispatcher writes the JSON envelope.
// A reader recovers the envelope by scanning back from the end of stderr for a
// JSON object, so a warning printed after it would hide it — the envelope is
// pretty-printed across several lines and its own braces are indented, leaving
// nothing for that scan to land on.
//
// The handler is also given a mutable failure to prove the ordering costs
// nothing: what the user is told is decided before the event fires, so the
// handler still cannot rewrite it.
func TestFailingShutdownHookLeavesTheEnvelopeLastOnStderr(t *testing.T) {
	tmpHome(t)
	quietNotices(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	// The hook layer captured os.Stderr at package init, so point its warnings
	// at the same buffer the factory writes the envelope to. In production both
	// are the one stderr file descriptor.
	var stderr bytes.Buffer
	t.Cleanup(hook.SetStderrForTesting(&stderr))

	handlerRan := false
	platform.Register(platform.NewPlugin("audit", "1.0").
		On(platform.Shutdown, "explode",
			func(_ context.Context, lc *platform.LifecycleContext) error {
				handlerRan = true
				if p, ok := errs.ProblemOf(lc.Err); ok {
					p.Subtype = "rewritten_by_shutdown_handler"
				}
				return errors.New("audit sink unreachable")
			}).
		MustBuild())

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"lark-cli", "definitely-not-a-command"}

	code := ExecuteWithOptions(WithIO(strings.NewReader(""), io.Discard, &stderr))

	if !handlerRan {
		t.Fatal("the Shutdown handler never ran; the test no longer covers its own premise")
	}
	out := stderr.String()
	if !strings.Contains(out, "warning: shutdown hook") {
		t.Fatalf("no hook warning on stderr; the test no longer covers its own premise:\n%s", out)
	}

	payload := trailingJSONObject(out)
	if payload == "" {
		t.Fatalf("no JSON object at the end of stderr; the envelope is unreachable:\n%s", out)
	}
	var envelope struct {
		Error struct {
			Category errs.Category `json:"type"`
			Subtype  errs.Subtype  `json:"subtype"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatalf("trailing content is not the error envelope: %v\n%s", err, payload)
	}
	if envelope.Error.Category != errs.CategoryValidation ||
		envelope.Error.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("envelope reports %s/%s, want %s/%s — a Shutdown handler rewrote it",
			envelope.Error.Category, envelope.Error.Subtype,
			errs.CategoryValidation, errs.SubtypeInvalidArgument)
	}
	if code != output.ExitValidation {
		t.Errorf("exit code = %d, want %d", code, output.ExitValidation)
	}
}

// trailingJSONObject mirrors how a reader recovers the envelope from a stderr
// stream that may also carry framework warnings: take the last line that starts
// a JSON object and everything after it. Leading noise is tolerated; trailing
// noise is not, which is the property the test above exists to protect.
func trailingJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if json.Valid([]byte(raw)) {
		return raw
	}
	start := strings.LastIndex(raw, "\n{")
	if start < 0 {
		return ""
	}
	candidate := raw[start+1:]
	if !json.Valid([]byte(candidate)) {
		return ""
	}
	return candidate
}
