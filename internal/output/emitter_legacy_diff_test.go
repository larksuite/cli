// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

type emitterCapture struct {
	stdout string
	stderr string
	err    error
}

type emitterSafetyProvider struct {
	alert *extcs.Alert
	err   error
}

func (p *emitterSafetyProvider) Name() string { return "emitter-oracle" }

func (p *emitterSafetyProvider) Scan(context.Context, extcs.ScanRequest) (*extcs.Alert, error) {
	return p.alert, p.err
}

func TestEmitterMatchesRuntimeContextLegacyOracle(t *testing.T) {
	previousNotice := output.PendingNotice
	t.Cleanup(func() {
		output.PendingNotice = previousNotice
		extcs.Register(nil)
	})

	type oracleCase struct {
		name        string
		data        func() interface{}
		raw         bool
		ok          bool
		meta        *output.Meta
		jq          string
		format      string
		useFormat   bool
		pretty      bool
		notice      map[string]interface{}
		safetyMode  string
		safetyAlert *extcs.Alert
		safetyErr   error
	}

	cases := []oracleCase{
		{
			name: "json_object",
			data: func() interface{} {
				return map[string]interface{}{"id": "1", "enabled": true}
			},
			ok: true,
		},
		{
			name: "raw_json_preserves_html",
			data: func() interface{} {
				return map[string]interface{}{"html": "<p>a&b</p>"}
			},
			raw: true,
			ok:  true,
		},
		{
			name: "partial_failure_ok_false",
			data: func() interface{} {
				return map[string]interface{}{"succeeded": 1, "failed": 1}
			},
			ok: false,
		},
		{
			name: "metadata",
			data: func() interface{} {
				return []interface{}{map[string]interface{}{"id": "1"}}
			},
			ok:   true,
			meta: &output.Meta{Count: 1, Rollback: "lark-cli fixture rollback"},
		},
		{
			name: "jq_scalar",
			data: func() interface{} {
				return map[string]interface{}{"name": "Alice", "age": 30}
			},
			ok: true,
			jq: ".data.name",
		},
		{
			name: "raw_jq_complex",
			data: func() interface{} {
				return map[string]interface{}{"document": map[string]interface{}{"html": "<p>a&b</p>"}}
			},
			raw: true,
			ok:  true,
			jq:  ".data.document",
		},
		{
			name: "notice",
			data: func() interface{} {
				return map[string]interface{}{"id": "1"}
			},
			ok:     true,
			notice: map[string]interface{}{"update": map[string]interface{}{"latest": "9.9.9"}},
		},
		{
			name: "pretty",
			data: func() interface{} {
				return map[string]interface{}{"name": "Alice"}
			},
			ok:        true,
			format:    "pretty",
			useFormat: true,
			pretty:    true,
		},
		{
			name: "ndjson",
			data: func() interface{} {
				return map[string]interface{}{"items": []interface{}{
					map[string]interface{}{"id": "1"},
					map[string]interface{}{"id": "2"},
				}}
			},
			ok:        true,
			format:    "ndjson",
			useFormat: true,
		},
		{
			name: "table_with_safety_warning",
			data: func() interface{} {
				return []interface{}{map[string]interface{}{"id": "1", "name": "Alice"}}
			},
			ok:         true,
			format:     "table",
			useFormat:  true,
			safetyMode: "warn",
			safetyAlert: &extcs.Alert{
				Provider:     "emitter-oracle",
				MatchedRules: []string{"fixture-rule"},
			},
		},
		{
			name: "csv",
			data: func() interface{} {
				return []interface{}{
					map[string]interface{}{"id": "1", "name": "Alice"},
					map[string]interface{}{"id": "2", "name": "Bob"},
				}
			},
			ok:        true,
			format:    "csv",
			useFormat: true,
		},
		{
			name: "jq_safety_alert_without_stderr_warning",
			data: func() interface{} {
				return map[string]interface{}{"id": "1"}
			},
			ok:         true,
			jq:         ".data.id",
			safetyMode: "warn",
			safetyAlert: &extcs.Alert{
				Provider:     "emitter-oracle",
				MatchedRules: []string{"fixture-rule"},
			},
		},
		{
			name: "scanner_error_fails_open",
			data: func() interface{} {
				return map[string]interface{}{"id": "1"}
			},
			ok:         true,
			safetyMode: "warn",
			safetyErr:  errors.New("scanner unavailable"),
		},
		{
			name: "scanner_block",
			data: func() interface{} {
				return map[string]interface{}{"id": "blocked"}
			},
			ok:         true,
			safetyMode: "block",
			safetyAlert: &extcs.Alert{
				Provider:     "emitter-oracle",
				MatchedRules: []string{"fixture-rule"},
			},
		},
		{
			name: "unknown_format_data_envelope_notice",
			data: func() interface{} {
				return map[string]interface{}{"ok": true, "value": "fixture"}
			},
			ok:        true,
			format:    "yaml",
			useFormat: true,
			notice:    map[string]interface{}{"skills": map[string]interface{}{"current": "1.0.0"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode := tc.safetyMode
			if mode == "" {
				mode = "off"
			}
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", mode)
			extcs.Register(&emitterSafetyProvider{alert: tc.safetyAlert, err: tc.safetyErr})
			t.Cleanup(func() { extcs.Register(nil) })

			notice := tc.notice
			output.PendingNotice = func() map[string]interface{} { return notice }

			legacy := runRuntimeContextOracle(t, tc.data(), runtimeOracleOptions{
				raw:       tc.raw,
				ok:        tc.ok,
				meta:      tc.meta,
				jq:        tc.jq,
				format:    tc.format,
				useFormat: tc.useFormat,
				pretty:    tc.pretty,
			})
			current := runEmitterSuccess(tc.data(), output.EmitterConfig{
				CommandPath:    "lark-cli fixture +emit",
				Identity:       "bot",
				NoticeProvider: func() map[string]interface{} { return notice },
			}, output.EmitOptions{
				Raw:    tc.raw,
				OK:     tc.ok,
				Meta:   tc.meta,
				Format: tc.format,
				JQ:     tc.jq,
				Pretty: emitterPrettyRenderer(tc.pretty),
			})

			assertEmitterBytes(t, legacy, current)
			if tc.safetyMode == "block" {
				var safetyErr *errs.ContentSafetyError
				if !errors.As(current.err, &safetyErr) {
					t.Fatalf("Emitter.Success() error = %T, want *errs.ContentSafetyError", current.err)
				}
			}
		})
	}
}

type runtimeOracleOptions struct {
	raw       bool
	ok        bool
	meta      *output.Meta
	jq        string
	format    string
	useFormat bool
	pretty    bool
}

func runRuntimeContextOracle(t *testing.T, data interface{}, opts runtimeOracleOptions) emitterCapture {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	parent := &cobra.Command{Use: "lark-cli"}
	cmd := &cobra.Command{Use: "fixture"}
	leaf := &cobra.Command{Use: "+emit"}
	parent.AddCommand(cmd)
	cmd.AddCommand(leaf)

	factory := &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{Out: stdout, ErrOut: stderr}}
	runtime := common.TestNewRuntimeContextForAPI(
		context.Background(), leaf, &core.CliConfig{Brand: core.BrandFeishu}, factory, core.AsBot,
	)
	runtime.Format = opts.format
	runtime.JqExpr = opts.jq

	pretty := func(w io.Writer) {
		fmt.Fprintln(w, "pretty:fixture")
	}
	if !opts.pretty {
		pretty = nil
	}

	var err error
	switch {
	case opts.useFormat && opts.raw:
		runtime.OutFormatRaw(data, opts.meta, pretty)
	case opts.useFormat:
		runtime.OutFormat(data, opts.meta, pretty)
	case !opts.ok:
		err = runtime.OutPartialFailure(data, opts.meta)
	case opts.raw:
		runtime.OutRaw(data, opts.meta)
	default:
		runtime.Out(data, opts.meta)
	}

	return emitterCapture{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func runEmitterSuccess(data interface{}, config output.EmitterConfig, opts output.EmitOptions) emitterCapture {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	config.Out = stdout
	config.ErrOut = stderr
	err := output.NewEmitter(config).Success(data, opts)
	return emitterCapture{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func emitterPrettyRenderer(enabled bool) output.PrettyRenderer {
	if !enabled {
		return nil
	}
	return func(w io.Writer, _ bool) error {
		_, err := fmt.Fprintln(w, "pretty:fixture")
		return err
	}
}

func TestEmitterMatchesWriteSuccessEnvelopeLegacyOracle(t *testing.T) {
	previousNotice := output.PendingNotice
	t.Cleanup(func() {
		output.PendingNotice = previousNotice
		extcs.Register(nil)
	})

	type oracleCase struct {
		name        string
		data        func() interface{}
		dryRun      bool
		jq          string
		notice      map[string]interface{}
		safetyMode  string
		safetyAlert *extcs.Alert
	}
	cases := []oracleCase{
		{
			name: "json",
			data: func() interface{} { return map[string]interface{}{"id": "1"} },
		},
		{
			name:   "dry_run",
			data:   func() interface{} { return map[string]interface{}{"api": []interface{}{}} },
			dryRun: true,
		},
		{
			name: "jq",
			data: func() interface{} { return map[string]interface{}{"id": "1"} },
			jq:   ".data.id",
		},
		{
			name:   "notice",
			data:   func() interface{} { return map[string]interface{}{"id": "1"} },
			notice: map[string]interface{}{"update": map[string]interface{}{"latest": "9.9.9"}},
		},
		{
			name:       "jq_safety_warning",
			data:       func() interface{} { return map[string]interface{}{"id": "1"} },
			jq:         ".data.id",
			safetyMode: "warn",
			safetyAlert: &extcs.Alert{
				Provider:     "emitter-oracle",
				MatchedRules: []string{"fixture-rule"},
			},
		},
		{
			name:       "scanner_block",
			data:       func() interface{} { return map[string]interface{}{"id": "blocked"} },
			safetyMode: "block",
			safetyAlert: &extcs.Alert{
				Provider:     "emitter-oracle",
				MatchedRules: []string{"fixture-rule"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode := tc.safetyMode
			if mode == "" {
				mode = "off"
			}
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", mode)
			extcs.Register(&emitterSafetyProvider{alert: tc.safetyAlert})
			t.Cleanup(func() { extcs.Register(nil) })
			notice := tc.notice
			output.PendingNotice = func() map[string]interface{} { return notice }

			legacy := runWriteSuccessEnvelopeOracle(tc.data(), tc.dryRun, tc.jq)
			current := runEmitterSuccess(tc.data(), output.EmitterConfig{
				CommandPath:    "lark-cli fixture +emit",
				Identity:       "bot",
				NoticeProvider: func() map[string]interface{} { return notice },
			}, output.EmitOptions{
				OK:              true,
				Format:          "json",
				JQ:              tc.jq,
				DryRun:          tc.dryRun,
				JQSafetyWarning: true,
			})

			assertEmitterBytes(t, legacy, current)
			assertEquivalentError(t, legacy.err, current.err)
		})
	}
}

func runWriteSuccessEnvelopeOracle(data interface{}, dryRun bool, jq string) emitterCapture {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := output.WriteSuccessEnvelope(data, output.SuccessEnvelopeOptions{
		CommandPath: "lark-cli fixture +emit",
		Identity:    "bot",
		DryRun:      dryRun,
		JqExpr:      jq,
		Out:         stdout,
		ErrOut:      stderr,
	})
	return emitterCapture{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func TestEmitterStreamPageMatchesPaginationLegacyOracle(t *testing.T) {
	t.Cleanup(func() { extcs.Register(nil) })

	type oracleCase struct {
		name        string
		format      output.Format
		safetyMode  string
		safetyAlert *extcs.Alert
	}
	cases := []oracleCase{
		{name: "ndjson", format: output.FormatNDJSON},
		{name: "table", format: output.FormatTable},
		{name: "csv", format: output.FormatCSV},
		{
			name:       "warn",
			format:     output.FormatNDJSON,
			safetyMode: "warn",
			safetyAlert: &extcs.Alert{
				Provider:     "emitter-oracle",
				MatchedRules: []string{"fixture-rule"},
			},
		},
		{
			name:       "block",
			format:     output.FormatTable,
			safetyMode: "block",
			safetyAlert: &extcs.Alert{
				Provider:     "emitter-oracle",
				MatchedRules: []string{"fixture-rule"},
			},
		},
	}

	pages := []interface{}{
		[]interface{}{map[string]interface{}{"id": "1", "name": "Alice"}},
		[]interface{}{map[string]interface{}{"id": "2", "name": "Bob", "ignored": true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode := tc.safetyMode
			if mode == "" {
				mode = "off"
			}
			t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", mode)
			extcs.Register(&emitterSafetyProvider{alert: tc.safetyAlert})
			t.Cleanup(func() { extcs.Register(nil) })

			legacy := runPaginationOracle(pages, tc.format)
			current := runEmitterStreamPages(pages, tc.format.String())

			assertEmitterBytes(t, legacy, current)
			assertEquivalentError(t, legacy.err, current.err)
		})
	}
}

func runPaginationOracle(pages []interface{}, format output.Format) emitterCapture {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	formatter := output.NewPaginatedFormatter(stdout, format)
	var emitErr error
	for _, page := range pages {
		scanResult := output.ScanForSafety("lark-cli fixture +emit", page, stderr)
		if scanResult.Blocked {
			emitErr = scanResult.BlockErr
			break
		}
		if scanResult.Alert != nil {
			output.WriteAlertWarning(stderr, scanResult.Alert)
		}
		formatter.FormatPage(page)
	}
	return emitterCapture{stdout: stdout.String(), stderr: stderr.String(), err: emitErr}
}

func runEmitterStreamPages(pages []interface{}, format string) emitterCapture {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		ErrOut:      stderr,
		CommandPath: "lark-cli fixture +emit",
		Identity:    "bot",
	})
	var emitErr error
	for _, page := range pages {
		if emitErr = emitter.StreamPage(page, output.EmitOptions{OK: true, Format: format}); emitErr != nil {
			break
		}
	}
	return emitterCapture{stdout: stdout.String(), stderr: stderr.String(), err: emitErr}
}

func TestEmitterCapturesNoticeAndColorDependencies(t *testing.T) {
	previousNotice := output.PendingNotice
	output.PendingNotice = func() map[string]interface{} {
		return map[string]interface{}{"source": "global"}
	}
	t.Cleanup(func() { output.PendingNotice = previousNotice })
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	colorSeen := false
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:          stdout,
		ErrOut:       stderr,
		CommandPath:  "lark-cli fixture +emit",
		Identity:     "bot",
		ColorEnabled: true,
		NoticeProvider: func() map[string]interface{} {
			return map[string]interface{}{"source": "captured"}
		},
	})
	if err := emitter.Success(map[string]interface{}{"id": "1"}, output.EmitOptions{
		OK:     true,
		Format: "json",
	}); err != nil {
		t.Fatalf("Emitter.Success() error = %v", err)
	}
	if strings.Contains(stdout.String(), "global") || !strings.Contains(stdout.String(), "captured") {
		t.Fatalf("notice source was not captured by Emitter:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := emitter.Success(map[string]interface{}{"id": "1"}, output.EmitOptions{
		OK:     true,
		Format: "pretty",
		Pretty: func(w io.Writer, colorEnabled bool) error {
			colorSeen = colorEnabled
			_, err := fmt.Fprintln(w, "pretty")
			return err
		},
	}); err != nil {
		t.Fatalf("Emitter.Success(pretty) error = %v", err)
	}
	if !colorSeen {
		t.Fatal("PrettyRenderer did not receive captured ColorEnabled value")
	}

	stdout.Reset()
	if err := emitter.Success(map[string]interface{}{"ok": true, "id": "1"}, output.EmitOptions{
		OK:     true,
		Format: "yaml",
	}); err != nil {
		t.Fatalf("Emitter.Success(unknown format) error = %v", err)
	}
	if strings.Contains(stdout.String(), "global") || !strings.Contains(stdout.String(), "captured") {
		t.Fatalf("legacy JSON fallback consulted global notice:\n%s", stdout.String())
	}
}

type failingEmitterWriter struct {
	err error
}

func (w failingEmitterWriter) Write([]byte) (int, error) { return 0, w.err }

func TestEmitterPropagatesOutputError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	sentinel := errors.New("write failed")
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         failingEmitterWriter{err: sentinel},
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
	})
	err := emitter.Success(map[string]interface{}{"id": "1"}, output.EmitOptions{
		Raw:    true,
		OK:     true,
		Format: "json",
		JQ:     ".data",
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Emitter.Success() error = %v, want preserved writer cause", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal {
		t.Fatalf("Emitter.Success() problem = %#v, %v; want internal typed error", problem, ok)
	}
}

func TestEmitterRawJSONPropagatesWriteError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	sentinel := errors.New("write failed")
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         failingEmitterWriter{err: sentinel},
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
	})
	// Raw JSON without jq must surface the encoder write error, not discard it.
	err := emitter.Success(map[string]interface{}{"id": "1"}, output.EmitOptions{
		Raw:    true,
		OK:     true,
		Format: "json",
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Emitter.Success() error = %v, want preserved writer cause", err)
	}
}

func TestEmitterStreamPageRejectsJQ(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         io.Discard,
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
	})
	err := emitter.StreamPage([]interface{}{map[string]interface{}{"id": "1"}}, output.EmitOptions{
		OK:     true,
		Format: "ndjson",
		JQ:     ".data",
	})
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("StreamPage() error = %T, want *errs.ValidationError", err)
	}
	if verr.Param != "--jq" {
		t.Errorf("Param = %q, want --jq", verr.Param)
	}
}

func assertEmitterBytes(t *testing.T, legacy, current emitterCapture) {
	t.Helper()
	if legacy.stdout != current.stdout {
		t.Fatalf("stdout byte mismatch\nlegacy (%d bytes):\n%q\nEmitter (%d bytes):\n%q",
			len(legacy.stdout), legacy.stdout, len(current.stdout), current.stdout)
	}
	if legacy.stderr != current.stderr {
		t.Fatalf("stderr byte mismatch\nlegacy (%d bytes):\n%q\nEmitter (%d bytes):\n%q",
			len(legacy.stderr), legacy.stderr, len(current.stderr), current.stderr)
	}
}

func assertEquivalentError(t *testing.T, legacy, current error) {
	t.Helper()
	if (legacy == nil) != (current == nil) {
		t.Fatalf("error presence mismatch: legacy=%v Emitter=%v", legacy, current)
	}
	if legacy == nil {
		return
	}
	legacyProblem, legacyOK := errs.ProblemOf(legacy)
	currentProblem, currentOK := errs.ProblemOf(current)
	if legacyOK != currentOK {
		t.Fatalf("typed error mismatch: legacy=%T Emitter=%T", legacy, current)
	}
	if legacyOK && !reflect.DeepEqual(legacyProblem, currentProblem) {
		t.Fatalf("problem mismatch:\nlegacy: %#v\nEmitter: %#v", legacyProblem, currentProblem)
	}
}
