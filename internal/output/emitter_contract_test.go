// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/output"
)

type contractFailingWriter struct {
	err error
}

func (w contractFailingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

type contractSafetyProvider struct {
	alert *extcs.Alert
}

func (p *contractSafetyProvider) Name() string {
	return "emitter-contract"
}

func (p *contractSafetyProvider) Scan(context.Context, extcs.ScanRequest) (*extcs.Alert, error) {
	return p.alert, nil
}

func TestEmitterSuccessWritesAllBytes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	stdout := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
		Identity:    "bot",
	})
	data := map[string]interface{}{"id": "1"}

	err := emitter.Success(data, output.EmitOptions{Format: "json"})
	if err != nil {
		t.Fatalf("Emitter.Success() error = %v", err)
	}
	want, marshalErr := json.MarshalIndent(output.Envelope{OK: true, Identity: "bot", Data: data}, "", "  ")
	if marshalErr != nil {
		t.Fatalf("marshal expected envelope: %v", marshalErr)
	}
	want = append(want, '\n')
	if !bytes.Equal(stdout.Bytes(), want) {
		t.Fatalf("stdout bytes = %q, want %q", stdout.Bytes(), want)
	}
}

func TestEmitterPaginationMetadataByFormat(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	data := map[string]interface{}{
		"items": []interface{}{map[string]interface{}{"id": "1", "name": "first"}},
	}
	meta := &output.Meta{
		Count: 1,
		Pagination: &output.PaginationMeta{
			Complete:  false,
			Pages:     2,
			Items:     1,
			NextToken: "resume-token",
		},
	}

	t.Run("json envelope", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		emitter := output.NewEmitter(output.EmitterConfig{
			Out: stdout, ErrOut: stderr, CommandPath: "lark-cli fixture +emit",
		})
		if err := emitter.Success(data, output.EmitOptions{Format: "json", Meta: meta}); err != nil {
			t.Fatalf("Emitter.Success() error = %v", err)
		}
		var envelope output.Envelope
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("decode stdout: %v", err)
		}
		if envelope.Meta == nil || !reflect.DeepEqual(envelope.Meta.Pagination, meta.Pagination) {
			t.Fatalf("pagination meta = %#v, want %#v", envelope.Meta, meta.Pagination)
		}
		if stderr.Len() != 0 {
			t.Fatalf("json wrote pagination diagnostic to stderr: %q", stderr.String())
		}
	})

	t.Run("unknown format falls back to the same json envelope", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		emitter := output.NewEmitter(output.EmitterConfig{
			Out: stdout, ErrOut: stderr, CommandPath: "lark-cli fixture +emit",
		})
		if err := emitter.Success(data, output.EmitOptions{Format: "yaml", Meta: meta}); err != nil {
			t.Fatalf("Emitter.Success() error = %v", err)
		}
		var envelope output.Envelope
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatalf("fallback stdout is not one complete JSON envelope: %v\n%s", err, stdout.String())
		}
		if envelope.Meta == nil || !reflect.DeepEqual(envelope.Meta.Pagination, meta.Pagination) {
			t.Fatalf("fallback pagination meta = %#v, want %#v", envelope.Meta, meta.Pagination)
		}
		if !strings.Contains(stderr.String(), `warning: unknown format "yaml", falling back to json`) {
			t.Fatalf("fallback stderr = %q, want unknown-format warning", stderr.String())
		}
		if strings.Contains(stderr.String(), `"_diagnostic":"pagination"`) {
			t.Fatalf("fallback emitted a second pagination contract: %q", stderr.String())
		}
	})

	for _, tc := range []struct {
		name   string
		format string
		pretty output.PrettyRenderer
	}{
		{name: "pretty", format: "pretty", pretty: func(w io.Writer, _ bool) error {
			_, err := io.WriteString(w, "first\n")
			return err
		}},
		{name: "table", format: "table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			emitter := output.NewEmitter(output.EmitterConfig{
				Out: stdout, ErrOut: stderr, CommandPath: "lark-cli fixture +emit",
			})
			if err := emitter.Success(data, output.EmitOptions{Format: tc.format, Pretty: tc.pretty, Meta: meta}); err != nil {
				t.Fatalf("Emitter.Success() error = %v", err)
			}
			for _, want := range []string{"Pagination: incomplete", "2 page(s)", "1 item(s)", `resume token: "resume-token"`} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout = %q, want %q", stdout.String(), want)
				}
			}
			if stderr.Len() != 0 {
				t.Fatalf("%s wrote pagination diagnostic to stderr: %q", tc.format, stderr.String())
			}
		})
	}

	for _, format := range []string{"ndjson", "csv"} {
		t.Run(format, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			emitter := output.NewEmitter(output.EmitterConfig{
				Out: stdout, ErrOut: stderr, CommandPath: "lark-cli fixture +emit",
			})
			if err := emitter.Success(data, output.EmitOptions{Format: format, Meta: meta}); err != nil {
				t.Fatalf("Emitter.Success() error = %v", err)
			}
			if strings.Contains(stdout.String(), "_diagnostic") || strings.Contains(stdout.String(), "resume-token") {
				t.Fatalf("%s stdout was polluted by pagination metadata: %q", format, stdout.String())
			}
			var diagnostic struct {
				Diagnostic string `json:"_diagnostic"`
				Complete   bool   `json:"complete"`
				Pages      int    `json:"pages"`
				Items      int    `json:"items"`
				NextToken  string `json:"next_token"`
			}
			if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &diagnostic); err != nil {
				t.Fatalf("decode pagination diagnostic %q: %v", stderr.String(), err)
			}
			if diagnostic.Diagnostic != "pagination" || diagnostic.Complete || diagnostic.Pages != 2 || diagnostic.Items != 1 || diagnostic.NextToken != "resume-token" {
				t.Fatalf("pagination diagnostic = %+v", diagnostic)
			}
		})
	}
}

func TestEmitterRedactedFallbackSkipsBlockedPresentationScan(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	extcs.Register(&contractSafetyProvider{alert: &extcs.Alert{
		Provider:     "emitter-contract",
		MatchedRules: []string{"blocked-presentation"},
	}})
	t.Cleanup(func() { extcs.Register(nil) })
	stdout := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		ErrOut:      io.Discard,
		CommandPath: "lark-cli im fixture",
	})

	err := emitter.RedactedFallback(output.Envelope{
		OK:    false,
		Data:  map[string]interface{}{"completion": map[string]interface{}{"status": "complete"}},
		Error: errs.NewAPIError(errs.SubtypeUnknown, "Output failed after the IM write completed"),
	})
	if err != nil {
		t.Fatalf("Emitter.RedactedFallback() error = %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode fallback: %v", err)
	}
	if env.OK || env.Error == nil {
		t.Fatalf("fallback = %#v, want redacted failure envelope", env)
	}
}

func TestEmitterMarshalFailureReturnsTypedErrorWithoutOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	stdout := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
	})

	err := emitter.Success(map[string]interface{}{"unsupported": func() {}}, output.EmitOptions{Format: "json"})
	if err == nil {
		t.Fatal("Emitter.Success() error = nil, want marshal failure")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal {
		t.Fatalf("Emitter.Success() problem = %#v, %v; want internal typed error", problem, ok)
	}
	var unsupported *json.UnsupportedTypeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("Emitter.Success() error = %v, want json.UnsupportedTypeError cause", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Emitter.Success() stdout = %q, want empty", stdout.String())
	}
}

func TestEmitterWriterFailurePreservesCause(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	sentinel := errors.New("write failed")
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         contractFailingWriter{err: sentinel},
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
	})

	err := emitter.Success(map[string]interface{}{"id": "1"}, output.EmitOptions{Format: "json"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Emitter.Success() error = %v, want preserved writer cause", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal {
		t.Fatalf("Emitter.Success() problem = %#v, %v; want internal typed error", problem, ok)
	}
}

func TestEmitterPrettyRendererFailurePreservesCause(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	sentinel := errors.New("pretty render failed")
	stdout := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
	})

	err := emitter.Success(map[string]interface{}{"id": "1"}, output.EmitOptions{
		Format: "pretty",
		Pretty: func(io.Writer, bool) error {
			return sentinel
		},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Emitter.Success() error = %v, want preserved renderer cause", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal {
		t.Fatalf("Emitter.Success() problem = %#v, %v; want internal typed error", problem, ok)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Emitter.Success() stdout = %q, want empty", stdout.String())
	}
}

func TestEmitterAlertWarningFailurePreservesCause(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	extcs.Register(&contractSafetyProvider{alert: &extcs.Alert{
		Provider:     "emitter-contract",
		MatchedRules: []string{"fixture-rule"},
	}})
	t.Cleanup(func() { extcs.Register(nil) })
	sentinel := errors.New("warning write failed")
	stdout := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		ErrOut:      contractFailingWriter{err: sentinel},
		CommandPath: "lark-cli fixture +emit",
	})

	err := emitter.Success([]interface{}{map[string]interface{}{"id": "1"}}, output.EmitOptions{Format: "table"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Emitter.Success() error = %v, want preserved warning writer cause", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal {
		t.Fatalf("Emitter.Success() problem = %#v, %v; want internal typed error", problem, ok)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Emitter.Success() stdout = %q, want empty", stdout.String())
	}
}

func TestNewEmitterDefaultsNilErrOutToDiscard(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	extcs.Register(&contractSafetyProvider{alert: &extcs.Alert{
		Provider:     "emitter-contract",
		MatchedRules: []string{"fixture-rule"},
	}})
	t.Cleanup(func() { extcs.Register(nil) })
	stdout := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		CommandPath: "lark-cli fixture +emit",
	})

	if err := emitter.Success([]interface{}{map[string]interface{}{"id": "1"}}, output.EmitOptions{Format: "table"}); err != nil {
		t.Fatalf("Emitter.Success() error = %v", err)
	}
	if stdout.Len() == 0 {
		t.Fatal("Emitter.Success() stdout is empty")
	}
}

func TestEmitterDoesNotMutateCallerMap(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	data := map[string]interface{}{"ok": true, "value": "fixture"}
	want := map[string]interface{}{"ok": true, "value": "fixture"}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         &bytes.Buffer{},
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
		NoticeProvider: func() map[string]interface{} {
			return map[string]interface{}{"update": "available"}
		},
	})

	if err := emitter.Success(data, output.EmitOptions{Format: "yaml"}); err != nil {
		t.Fatalf("Emitter.Success() error = %v", err)
	}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("caller map = %#v, want unchanged %#v", data, want)
	}
}

func TestEmitterDoesNotOverwriteCallerNotice(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	existing := map[string]interface{}{"source": "caller"}
	data := map[string]interface{}{"ok": true, "_notice": existing}
	stdout := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
		NoticeProvider: func() map[string]interface{} {
			return map[string]interface{}{"source": "provider"}
		},
	})

	if err := emitter.Success(data, output.EmitOptions{Format: "yaml"}); err != nil {
		t.Fatalf("Emitter.Success() error = %v", err)
	}
	if got := data["_notice"]; !reflect.DeepEqual(got, existing) {
		t.Fatalf("caller _notice = %#v, want unchanged %#v", got, existing)
	}
	var emitted map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &emitted); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}
	if got := emitted["_notice"]; !reflect.DeepEqual(got, map[string]interface{}{"source": "provider"}) {
		t.Fatalf("emitted _notice = %#v, want provider notice", got)
	}
}

func TestEmitterReadsNoticeProviderAtMostOncePerEmission(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	calls := 0
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         &bytes.Buffer{},
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
		NoticeProvider: func() map[string]interface{} {
			calls++
			return map[string]interface{}{"source": "provider"}
		},
	})

	if err := emitter.Success(map[string]interface{}{"ok": true}, output.EmitOptions{Format: "yaml"}); err != nil {
		t.Fatalf("Emitter.Success() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("notice provider calls = %d, want 1", calls)
	}
}

func TestEmitterRawJSONPropagatesWriteError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	sentinel := errors.New("write failed")
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         contractFailingWriter{err: sentinel},
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
	})
	err := emitter.Success(map[string]interface{}{"id": "1"}, output.EmitOptions{
		Raw: true, Format: "json",
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Emitter.Success() error = %v, want preserved writer cause", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal {
		t.Fatalf("Emitter.Success() problem = %#v, %v; want internal typed error", problem, ok)
	}
}

func TestEmitterInvalidJQReturnsErrorWithoutStderr(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	stderr := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         &bytes.Buffer{},
		ErrOut:      stderr,
		CommandPath: "lark-cli fixture +emit",
	})
	err := emitter.Success(map[string]interface{}{"id": "1"}, output.EmitOptions{
		Format: "json",
		JQ:     "this is not valid jq (((",
	})
	if err == nil {
		t.Fatal("Success() with invalid jq = nil, want error")
	}
	if stderr.Len() != 0 {
		t.Fatalf("Success() with invalid jq wrote stderr %q, want empty", stderr.String())
	}
}

func TestEmitterJQRuntimeErrorPreservesTypedError(t *testing.T) {
	// A valid expression that fails at runtime must surface jq's own typed error
	// (an api error), not a wrapped internal output error, and must emit no
	// partial stdout.
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	stdout := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
	})
	err := emitter.Success(map[string]interface{}{"id": "1"}, output.EmitOptions{
		Format: "json",
		JQ:     `error("boom")`,
	})
	if err == nil {
		t.Fatal("Success() with a runtime jq error = nil, want error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category == errs.CategoryInternal {
		t.Fatalf("Success() jq runtime error problem = %#v, %v; want jq's own typed error, not internal", problem, ok)
	}
	if !strings.Contains(err.Error(), "jq error") {
		t.Fatalf("Success() jq runtime error = %v, want jq's own error message preserved", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("Success() jq runtime error wrote stdout %q, want empty", stdout.String())
	}
}

func TestEmitterUnknownFormatStructKeepsNotice(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	type payload struct {
		OK    bool   `json:"ok"`
		Value string `json:"value"`
	}
	stdout := &bytes.Buffer{}
	emitter := output.NewEmitter(output.EmitterConfig{
		Out:         stdout,
		ErrOut:      io.Discard,
		CommandPath: "lark-cli fixture +emit",
		NoticeProvider: func() map[string]interface{} {
			return map[string]interface{}{"update": map[string]interface{}{"latest": "9.9.9"}}
		},
	})
	if err := emitter.Success(payload{OK: true, Value: "fixture"}, output.EmitOptions{Format: "yaml"}); err != nil {
		t.Fatalf("Success() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "_notice") {
		t.Fatalf("struct payload on unknown-format fallback dropped _notice:\n%s", stdout.String())
	}
}
