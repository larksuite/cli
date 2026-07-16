// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/shortcuts/common"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/spf13/cobra"
)

// chdirTemp changes cwd to a fresh temp dir for the test duration.
func chdirTemp(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

// helper to build a RawEvent from event-level JSON and header fields.
func makeRawEvent(eventType string, eventJSON string) *RawEvent {
	return &RawEvent{
		Schema: "2.0",
		Header: larkevent.EventHeader{
			EventType: eventType,
			EventID:   "ev_test",
		},
		Event: json.RawMessage(eventJSON),
	}
}

func requireProblem(t *testing.T, err error, category errs.Category, subtype errs.Subtype, param string) {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf(%T) = false, error: %v", err, err)
	}
	if p.Category != category || p.Subtype != subtype {
		t.Fatalf("problem = %s/%s, want %s/%s", p.Category, p.Subtype, category, subtype)
	}
	if param != "" {
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("error %T is not *errs.ValidationError", err)
		}
		if ve.Param != param {
			t.Fatalf("Param = %q, want %q", ve.Param, param)
		}
	}
}

func TestEventTypedErrorHelpers(t *testing.T) {
	cause := errors.New("cause")

	validation := eventValidationError("bad input")
	requireProblem(t, validation, errs.CategoryValidation, errs.SubtypeInvalidArgument, "")

	paramErr := eventValidationParamErrorWithCause(cause, "--flag", "bad %s value", "flag")
	requireProblem(t, paramErr, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--flag")
	if got := paramErr.Error(); got != "bad flag value: cause" {
		t.Fatalf("message = %q, want %q", got, "bad flag value: cause")
	}
	if !errors.Is(paramErr, cause) {
		t.Fatal("validation error should preserve its cause")
	}

	fileErr := eventFileIOError(cause, "write failed")
	requireProblem(t, fileErr, errs.CategoryInternal, errs.SubtypeFileIO, "")
	if got := fileErr.Error(); got != "write failed: cause" {
		t.Fatalf("message = %q, want %q", got, "write failed: cause")
	}
	if !errors.Is(fileErr, cause) {
		t.Fatal("file_io error should preserve its cause")
	}

	networkErr := eventNetworkError(cause, "websocket failed")
	requireProblem(t, networkErr, errs.CategoryNetwork, errs.SubtypeNetworkTransport, "")
	if got := networkErr.Error(); got != "websocket failed: cause" {
		t.Fatalf("message = %q, want %q", got, "websocket failed: cause")
	}
	if !errors.Is(networkErr, cause) {
		t.Fatal("network error should preserve its cause")
	}
}

func newSubscribeTestRuntime(t *testing.T) *common.RuntimeContext {
	t.Helper()

	var out, errOut bytes.Buffer
	cmd := &cobra.Command{Use: "+subscribe"}
	cmd.Flags().String("event-types", "", "")
	cmd.Flags().String("filter", "", "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("compact", false, "")
	cmd.Flags().String("output-dir", "", "")
	cmd.Flags().Bool("quiet", false, "")
	cmd.Flags().StringArray("route", nil, "")
	cmd.Flags().Bool("force", false, "")

	return &common.RuntimeContext{
		Cmd: cmd,
		Config: &core.CliConfig{
			AppID:     "cli_event_test",
			AppSecret: "secret",
			Brand:     core.BrandFeishu,
		},
		Factory: &cmdutil.Factory{
			IOStreams: cmdutil.NewIOStreams(strings.NewReader(""), &out, &errOut),
		},
	}
}

// --- Registry ---

func TestRegistryLookup(t *testing.T) {
	r := DefaultRegistry()
	p := r.Lookup("im.message.receive_v1")
	if p.EventType() != "im.message.receive_v1" {
		t.Errorf("got %q", p.EventType())
	}
	p2 := r.Lookup("unknown.type")
	if p2.EventType() != "" {
		t.Errorf("fallback should have empty EventType, got %q", p2.EventType())
	}
}

func TestRegistryDuplicateReturnsError(t *testing.T) {
	r := NewProcessorRegistry(&GenericProcessor{})
	if err := r.Register(&ImMessageProcessor{}); err != nil {
		t.Fatalf("first register should succeed: %v", err)
	}
	err := r.Register(&ImMessageProcessor{})
	if err == nil {
		t.Error("expected error on duplicate registration")
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeUnknown, "")
}

// --- Filters ---

func TestEventTypeFilter(t *testing.T) {
	f := NewEventTypeFilter("im.message.receive_v1, drive.file.edit_v1")
	if !f.Allow("im.message.receive_v1") {
		t.Error("should allow")
	}
	if f.Allow("unknown.type") {
		t.Error("should reject")
	}
}

func TestEventTypeFilter_Empty(t *testing.T) {
	if f := NewEventTypeFilter(""); f != nil {
		t.Error("empty should return nil")
	}
}

func TestRegexFilter(t *testing.T) {
	f, err := NewRegexFilter("im\\.message\\..*")
	if err != nil {
		t.Fatal(err)
	}
	if !f.Allow("im.message.receive_v1") {
		t.Error("should match")
	}
	if f.Allow("drive.file.edit_v1") {
		t.Error("should not match")
	}
}

func TestRegexFilter_Invalid(t *testing.T) {
	_, err := NewRegexFilter("[invalid")
	if err == nil {
		t.Error("should error")
	}
}

func TestEventSubscribeExecuteRejectsUnsafeOutputDir(t *testing.T) {
	rt := newSubscribeTestRuntime(t)
	if err := rt.Cmd.Flags().Set("output-dir", "/tmp/events"); err != nil {
		t.Fatal(err)
	}
	err := EventSubscribe.Execute(context.Background(), rt)
	if err == nil {
		t.Fatal("expected unsafe output-dir error")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--output-dir")
	if errors.Unwrap(err) == nil {
		t.Fatal("unsafe output-dir error should preserve its cause")
	}
}

func TestEventSubscribeExecuteRejectsInvalidFilter(t *testing.T) {
	rt := newSubscribeTestRuntime(t)
	if err := rt.Cmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	if err := rt.Cmd.Flags().Set("filter", "[invalid"); err != nil {
		t.Fatal(err)
	}
	err := EventSubscribe.Execute(context.Background(), rt)
	if err == nil {
		t.Fatal("expected invalid filter error")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--filter")
	if errors.Unwrap(err) == nil {
		t.Fatal("invalid filter error should preserve its cause")
	}
}

func TestEventSubscribeExecuteRejectsInvalidRoute(t *testing.T) {
	rt := newSubscribeTestRuntime(t)
	if err := rt.Cmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	if err := rt.Cmd.Flags().Set("route", "no-equals-sign"); err != nil {
		t.Fatal(err)
	}
	err := EventSubscribe.Execute(context.Background(), rt)
	if err == nil {
		t.Fatal("expected invalid route error")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--route")
}

func TestFilterChain(t *testing.T) {
	etf := NewEventTypeFilter("im.message.receive_v1, drive.file.edit_v1")
	rf, _ := NewRegexFilter("im\\..*")
	chain := NewFilterChain(etf, rf)

	if !chain.Allow("im.message.receive_v1") {
		t.Error("both filters pass, should allow")
	}
	if chain.Allow("drive.file.edit_v1") {
		t.Error("regex rejects drive, should block")
	}

	empty := NewFilterChain()
	if !empty.Allow("anything") {
		t.Error("empty chain should allow all")
	}

	var nilChain *FilterChain
	if !nilChain.Allow("anything") {
		t.Error("nil chain should allow all")
	}
}

func TestEventTypeFilter_TypesSorted(t *testing.T) {
	f := NewEventTypeFilter("z.type,a.type,m.type")
	got := f.Types()
	want := []string{"a.type", "m.type", "z.type"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Types() = %v, want %v", got, want)
	}
}

// --- Processors ---

func TestImMessageProcessor_Raw(t *testing.T) {
	p := &ImMessageProcessor{}
	eventJSON := `{"message":{"id":"1"}}`
	raw := makeRawEvent("im.message.receive_v1", eventJSON)
	result, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent)
	if !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
	if result.Header.EventType != "im.message.receive_v1" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
	if result.Schema != "2.0" {
		t.Errorf("Schema = %v", result.Schema)
	}
}

func TestGenericProcessor_Compact(t *testing.T) {
	p := &GenericProcessor{}
	eventJSON := `{"file_token":"xxx"}`
	raw := makeRawEvent("drive.file.edit_v1", eventJSON)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(map[string]interface{})
	if !ok {
		t.Fatal("compact should return map[string]interface{}")
	}
	if result["file_token"] != "xxx" {
		t.Error("file_token should be preserved")
	}
	if result["type"] != "drive.file.edit_v1" {
		t.Errorf("type = %v, want drive.file.edit_v1", result["type"])
	}
	if result["event_id"] != "ev_test" {
		t.Errorf("event_id = %v, want ev_test", result["event_id"])
	}
}

func TestGenericProcessor_Raw(t *testing.T) {
	p := &GenericProcessor{}
	eventJSON := `{"schema":"2.0"}`
	raw := makeRawEvent("drive.file.edit_v1", eventJSON)
	result, ok := p.Transform(context.Background(), raw, TransformRaw).(*RawEvent)
	if !ok {
		t.Fatal("raw mode should return *RawEvent")
	}
	if result.Header.EventType != "drive.file.edit_v1" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
}

// --- Pipeline ---

func TestPipeline_Raw(t *testing.T) {
	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformRaw}, &out, &errOut)

	eventJSON := `{"file_token":"xxx"}`
	raw := makeRawEvent("drive.file.edit_v1", eventJSON)
	raw.Header.EventID = "ev_raw"
	raw.Header.CreateTime = "1700000000"
	raw.Header.AppID = "cli_test"
	p.Process(context.Background(), raw)

	// Raw output should be the complete original event (schema + header + event)
	var outputMap map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &outputMap); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if outputMap["schema"] != "2.0" {
		t.Errorf("schema = %v, want 2.0", outputMap["schema"])
	}
	header, ok := outputMap["header"].(map[string]interface{})
	if !ok {
		t.Fatal("raw output should contain header object")
	}
	if header["event_type"] != "drive.file.edit_v1" {
		t.Errorf("header.event_type = %v", header["event_type"])
	}
	if header["app_id"] != "cli_test" {
		t.Errorf("header.app_id = %v, want cli_test", header["app_id"])
	}
}

func TestPipeline_Filtered(t *testing.T) {
	filters := NewFilterChain(NewEventTypeFilter("im.message.receive_v1"))
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{}, &out, &errOut)

	raw := makeRawEvent("drive.file.edit_v1", `{}`)
	p.Process(context.Background(), raw)

	if p.EventCount() != 0 {
		t.Errorf("filtered event should not be counted")
	}
	if out.Len() != 0 {
		t.Error("filtered event should produce no output")
	}
}

func TestDeduplicateKey(t *testing.T) {
	raw := makeRawEvent("im.message.receive_v1", `{}`)
	if k := (&ImMessageProcessor{}).DeduplicateKey(raw); k != "ev_test" {
		t.Errorf("ImMessageProcessor got %q, want ev_test", k)
	}
	if k := (&GenericProcessor{}).DeduplicateKey(raw); k != "ev_test" {
		t.Errorf("GenericProcessor got %q, want ev_test", k)
	}
}

func TestPipeline_Dedup(t *testing.T) {
	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformRaw}, &out, &errOut)

	raw := makeRawEvent("im.message.receive_v1", `{"message":{"id":"1"}}`)

	// First event should pass
	p.Process(context.Background(), raw)
	if p.EventCount() != 1 {
		t.Fatalf("EventCount = %d, want 1", p.EventCount())
	}
	firstLen := out.Len()
	if firstLen == 0 {
		t.Fatal("expected output from first event")
	}

	// Same event_id again should be deduped
	p.Process(context.Background(), raw)
	if p.EventCount() != 1 {
		t.Errorf("EventCount = %d, want 1 (deduped)", p.EventCount())
	}
	if out.Len() != firstLen {
		t.Error("duplicate event should produce no additional output")
	}

	// Different event_id should pass
	raw2 := makeRawEvent("im.message.receive_v1", `{"message":{"id":"2"}}`)
	raw2.Header.EventID = "ev_other"
	p.Process(context.Background(), raw2)
	if p.EventCount() != 2 {
		t.Errorf("EventCount = %d, want 2", p.EventCount())
	}
}

// --- Pipeline: OutputDir ---

func TestPipeline_OutputDir(t *testing.T) {
	dir := t.TempDir()
	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformCompact, OutputDir: dir}, &out, &errOut)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	eventJSON := `{
		"message": {
			"message_id": "msg_file", "chat_id": "oc_001",
			"chat_type": "group", "message_type": "text",
			"content": "{\"text\":\"file test\"}", "create_time": "1700000000"
		},
		"sender": {"sender_id": {"open_id": "ou_001"}}
	}`
	raw := makeRawEvent("im.message.receive_v1", eventJSON)
	raw.Header.EventID = "ev_file"
	raw.Header.CreateTime = "1700000000"
	p.Process(context.Background(), raw)

	// stdout should be empty (output goes to file)
	if out.Len() != 0 {
		t.Error("OutputDir mode should not write to stdout")
	}

	// Verify file was created
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}

	// Verify file content is valid JSON
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("file content is not valid JSON: %v", err)
	}
	if m["type"] != "im.message.receive_v1" {
		t.Errorf("type = %v", m["type"])
	}
}

func TestEventSubscribeExecuteRejectsHeldLock(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	lock, err := lockfile.ForSubscribe("cli_event_test")
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.TryLock(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Unlock() })

	rt := newSubscribeTestRuntime(t)
	execErr := EventSubscribe.Execute(context.Background(), rt)
	if execErr == nil {
		t.Fatal("expected lock-held error")
	}
	requireProblem(t, execErr, errs.CategoryValidation, errs.SubtypeFailedPrecondition, "")
	if !errors.Is(execErr, lockfile.ErrHeld) {
		t.Error("lock-held error should preserve lockfile.ErrHeld for errors.Is")
	}
	p, _ := errs.ProblemOf(execErr)
	if p.Hint == "" {
		t.Error("lock-held error should carry a recovery hint")
	}
	var ve *errs.ValidationError
	if errors.As(execErr, &ve) && ve.Param != "" {
		t.Errorf("lock contention names no offending flag; param = %q, want empty", ve.Param)
	}
}

func TestEventSubscribeDryRunEchoesFlags(t *testing.T) {
	rt := newSubscribeTestRuntime(t)
	for flag, value := range map[string]string{
		"event-types": "im.message.receive_v1",
		"filter":      "^im\\.",
		"output-dir":  "events_out",
	} {
		if err := rt.Cmd.Flags().Set(flag, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := rt.Cmd.Flags().Set("route", "^im\\.message=dir:./messages"); err != nil {
		t.Fatal(err)
	}

	d := EventSubscribe.DryRun(context.Background(), rt)
	if d == nil {
		t.Fatal("DryRun returned nil")
	}
	payload, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"command":"event +subscribe"`,
		`"app_id":"cli_event_test"`,
		`"event_types":"im.message.receive_v1"`,
		`"output_dir":"events_out"`,
	} {
		if !strings.Contains(string(payload), want) {
			t.Errorf("dry-run payload missing %s\ngot: %s", want, payload)
		}
	}
}

func TestPipeline_EnsureDirsRouteDirFileIOError(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("blocked", []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	router, err := ParseRoutes([]string{`^im\.=dir:./blocked/child`})
	if err != nil {
		t.Fatalf("ParseRoutes: %v", err)
	}
	p := NewEventPipeline(DefaultRegistry(), NewFilterChain(),
		PipelineConfig{Mode: TransformCompact, Router: router}, io.Discard, io.Discard)
	err = p.EnsureDirs()
	if err == nil {
		t.Fatal("expected file_io error for route dir blocked by a file")
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeFileIO, "")
}

func TestPipeline_EnsureDirsFileIOError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	p := NewEventPipeline(DefaultRegistry(), NewFilterChain(),
		PipelineConfig{Mode: TransformCompact, OutputDir: filepath.Join(path, "child")}, io.Discard, io.Discard)
	err := p.EnsureDirs()
	if err == nil {
		t.Fatal("expected file_io error")
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeFileIO, "")
	if errors.Unwrap(err) == nil {
		t.Fatal("file_io error should preserve its cause")
	}
}

// --- Pipeline: JsonFlag ---

func TestPipeline_JsonFlag(t *testing.T) {
	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformRaw, JsonFlag: true}, &out, &errOut)

	raw := makeRawEvent("drive.file.edit_v1", `{"key":"val"}`)
	p.Process(context.Background(), raw)

	// --json output should be pretty-printed (contain newlines + indentation)
	output := out.String()
	if !strings.Contains(output, "\n") {
		t.Error("--json output should be pretty-printed")
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(output), &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
}

// --- Pipeline: Quiet ---

func TestPipeline_Quiet(t *testing.T) {
	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformRaw, Quiet: true}, &out, &errOut)

	raw := makeRawEvent("im.message.receive_v1", `{}`)
	p.Process(context.Background(), raw)

	if errOut.Len() != 0 {
		t.Errorf("quiet mode should suppress stderr, got: %s", errOut.String())
	}
}

// --- writeEventFile ---

func TestWriteEventFile(t *testing.T) {
	dir := t.TempDir()
	header := larkevent.EventHeader{
		EventType:  "im.message.receive_v1",
		EventID:    "ev_write",
		CreateTime: "1700000000",
	}
	data := map[string]string{"hello": "world"}

	path, err := writeEventFile(dir, data, header)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "ev_write") {
		t.Errorf("path should contain event ID, got: %s", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"hello"`) {
		t.Error("file should contain data")
	}
}

func TestWriteEventFile_EmptyFields(t *testing.T) {
	dir := t.TempDir()
	header := larkevent.EventHeader{EventType: "test.type"}
	_, err := writeEventFile(dir, "data", header)
	if err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("expected 1 file")
	}
	name := entries[0].Name()
	if !strings.Contains(name, "unknown") {
		t.Errorf("empty EventID should fallback to 'unknown', got: %s", name)
	}
}

// --- stderrLogger ---

func TestStderrLogger(t *testing.T) {
	var buf bytes.Buffer
	l := &stderrLogger{w: &buf, quiet: false}

	l.Debug(context.Background(), "debug msg")
	if buf.Len() != 0 {
		t.Error("Debug should always be suppressed")
	}

	l.Info(context.Background(), "info msg")
	if !strings.Contains(buf.String(), "info msg") {
		t.Error("Info should print when not quiet")
	}
	buf.Reset()

	l.Warn(context.Background(), "warn msg")
	if !strings.Contains(buf.String(), "warn msg") {
		t.Error("Warn should always print")
	}
	buf.Reset()

	l.Error(context.Background(), "error msg")
	if !strings.Contains(buf.String(), "error msg") {
		t.Error("Error should always print")
	}
}

func TestStderrLogger_Quiet(t *testing.T) {
	var buf bytes.Buffer
	l := &stderrLogger{w: &buf, quiet: true}

	l.Info(context.Background(), "info msg")
	if buf.Len() != 0 {
		t.Error("Info should be suppressed when quiet")
	}

	l.Warn(context.Background(), "warn msg")
	if !strings.Contains(buf.String(), "warn msg") {
		t.Error("Warn should print even when quiet")
	}
}

// --- RegexFilter.String ---

func TestRegexFilter_String(t *testing.T) {
	f, _ := NewRegexFilter("im\\..*")
	if f.String() != "im\\..*" {
		t.Errorf("String() = %v", f.String())
	}
}

// --- WindowStrategy ---

func TestWindowStrategy(t *testing.T) {
	im := &ImMessageProcessor{}
	if im.WindowStrategy() != (WindowConfig{}) {
		t.Error("should return zero WindowConfig")
	}
	gen := &GenericProcessor{}
	if gen.WindowStrategy() != (WindowConfig{}) {
		t.Error("should return zero WindowConfig")
	}
}

// --- Shortcuts ---

func TestShortcuts(t *testing.T) {
	s := Shortcuts()
	if len(s) == 0 {
		t.Fatal("should return at least one shortcut")
	}
	if s[0].Command != "+subscribe" {
		t.Errorf("first shortcut command = %q", s[0].Command)
	}
}

// --- Compact unmarshal error fallback ---

func TestImMessageProcessor_CompactUnmarshalError(t *testing.T) {
	p := &ImMessageProcessor{}
	raw := makeRawEvent("im.message.receive_v1", `not valid json`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent)
	if !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
	if result.Header.EventType != "im.message.receive_v1" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
}

func TestImMessageProcessor_CompactInteractiveFallsBackToRaw(t *testing.T) {
	p := &ImMessageProcessor{}
	raw := makeRawEvent("im.message.receive_v1", `{
		"message": {
			"message_id": "om_interactive",
			"message_type": "interactive",
			"content": "{\"type\":\"template\"}"
		}
	}`)

	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = origStderr
	}()

	result, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent)
	if err := w.Close(); err != nil {
		t.Fatalf("stderr close error = %v", err)
	}
	hint, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("ReadAll(stderr) error = %v", readErr)
	}
	if !ok {
		t.Fatal("interactive compact conversion should fallback to *RawEvent")
	}
	if result != raw {
		t.Fatal("interactive compact conversion should return the original raw event")
	}
	if !strings.Contains(string(hint), "interactive") || !strings.Contains(string(hint), "returning raw event data") {
		t.Fatalf("stderr hint = %q, want interactive fallback message", string(hint))
	}
}
func TestGenericProcessor_CompactUnmarshalError(t *testing.T) {
	p := &GenericProcessor{}
	raw := makeRawEvent("some.type", `not valid json`)
	result, ok := p.Transform(context.Background(), raw, TransformCompact).(*RawEvent)
	if !ok {
		t.Fatal("unmarshal error should fallback to *RawEvent")
	}
	if result.Header.EventType != "some.type" {
		t.Errorf("EventType = %v", result.Header.EventType)
	}
}

// --- Router ---

func TestParseRoutes(t *testing.T) {
	routes, err := ParseRoutes([]string{
		`^im\.message=dir:./messages/`,
		`^contact\.=dir:./contacts/`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routes == nil {
		t.Fatal("expected non-nil router")
	}
	if len(routes.routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(routes.routes))
	}
}

func TestParseRoutes_Empty(t *testing.T) {
	routes, err := ParseRoutes(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routes != nil {
		t.Error("expected nil router for empty input")
	}

	routes2, err2 := ParseRoutes([]string{})
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if routes2 != nil {
		t.Error("expected nil router for empty slice")
	}
}

func TestParseRoutes_MissingEquals(t *testing.T) {
	_, err := ParseRoutes([]string{"no-equals-sign"})
	if err == nil {
		t.Error("expected error for missing =")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--route")
}

func TestParseRoutes_InvalidRegex(t *testing.T) {
	_, err := ParseRoutes([]string{"[invalid=dir:./foo/"})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--route")
	if errors.Unwrap(err) == nil {
		t.Fatal("invalid regex error should preserve its cause")
	}
}

func TestParseRoutes_MissingPrefix(t *testing.T) {
	_, err := ParseRoutes([]string{`^im\.message=./messages/`})
	if err == nil {
		t.Error("expected error for missing dir: prefix")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--route")
	if !strings.Contains(err.Error(), "dir:") {
		t.Errorf("error should mention dir: prefix, got: %v", err)
	}
}

func TestParseRoutes_EmptyPath(t *testing.T) {
	_, err := ParseRoutes([]string{`^im\.message=dir:`})
	if err == nil {
		t.Error("expected error for empty path")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--route")
}

func TestParseRoutes_RejectsAbsolutePath(t *testing.T) {
	_, err := ParseRoutes([]string{`^test=dir:/tmp/evil`})
	if err == nil {
		t.Error("expected error for absolute path in route")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--route")
}

func TestParseRoutes_RejectsTraversal(t *testing.T) {
	_, err := ParseRoutes([]string{`^test=dir:../../etc/evil`})
	if err == nil {
		t.Error("expected error for path traversal in route")
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, "--route")
}

func TestParseRoutes_PathSafety(t *testing.T) {
	routes, err := ParseRoutes([]string{`^test=dir:./foo/../bar/`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dir := routes.routes[0].dir
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %s", dir)
	}
	if strings.Contains(dir, "..") {
		t.Errorf("expected cleaned path without .., got %s", dir)
	}
}

func TestEventRouter_Match(t *testing.T) {
	chdirTemp(t)

	router, err := ParseRoutes([]string{
		`^im\.message=dir:./test_messages`,
		`^contact\.=dir:./test_contacts`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Single match
	dirs := router.Match("im.message.receive_v1")
	if len(dirs) != 1 {
		t.Errorf("expected 1 match, got %v", dirs)
	}

	dirs = router.Match("contact.user.created_v3")
	if len(dirs) != 1 {
		t.Errorf("expected 1 match, got %v", dirs)
	}

	// No match
	dirs = router.Match("drive.file.edit_v1")
	if len(dirs) != 0 {
		t.Errorf("expected no match, got %v", dirs)
	}
}

func TestEventRouter_Match_FanOut(t *testing.T) {
	chdirTemp(t)

	router, err := ParseRoutes([]string{
		`^im\.=dir:./test_im`,
		`message=dir:./test_msg`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// "im.message.receive_v1" matches both patterns
	dirs := router.Match("im.message.receive_v1")
	if len(dirs) != 2 {
		t.Errorf("expected 2 matches (fan-out), got %d: %v", len(dirs), dirs)
	}
}

// --- Pipeline: Route ---

func TestPipeline_Route(t *testing.T) {
	chdirTemp(t)
	router, err := ParseRoutes([]string{
		`^im\.message=dir:./route_out`,
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := router.routes[0].dir

	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformCompact, Router: router}, &out, &errOut)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	eventJSON := `{
		"message": {
			"message_id": "msg_route", "chat_id": "oc_001",
			"chat_type": "group", "message_type": "text",
			"content": "{\"text\":\"routed\"}", "create_time": "1700000000"
		},
		"sender": {"sender_id": {"open_id": "ou_001"}}
	}`
	raw := makeRawEvent("im.message.receive_v1", eventJSON)
	raw.Header.EventID = "ev_route"
	raw.Header.CreateTime = "1700000000"
	p.Process(context.Background(), raw)

	// stdout should be empty — output goes to route dir
	if out.Len() != 0 {
		t.Errorf("routed event should not appear on stdout, got: %s", out.String())
	}

	// Verify file was created in route dir
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in route dir, got %d", len(entries))
	}

	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("file content is not valid JSON: %v", err)
	}
	if m["type"] != "im.message.receive_v1" {
		t.Errorf("type = %v", m["type"])
	}
}

func TestPipeline_Route_NoMatch(t *testing.T) {
	chdirTemp(t)
	fallbackDir := t.TempDir()

	router, err := ParseRoutes([]string{
		`^im\.message=dir:./route_dir`,
	})
	if err != nil {
		t.Fatal(err)
	}
	routeDir := router.routes[0].dir

	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformCompact, Router: router, OutputDir: fallbackDir}, &out, &errOut)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	// Send an event that does NOT match the route
	raw := makeRawEvent("drive.file.edit_v1", `{"file_token":"xxx"}`)
	raw.Header.EventID = "ev_nomatch"
	raw.Header.CreateTime = "1700000000"
	p.Process(context.Background(), raw)

	// stdout should be empty
	if out.Len() != 0 {
		t.Errorf("should not appear on stdout, got: %s", out.String())
	}

	// Route dir should be empty
	routeEntries, _ := os.ReadDir(routeDir)
	if len(routeEntries) != 0 {
		t.Errorf("route dir should be empty, got %d files", len(routeEntries))
	}

	// Fallback dir should have the file
	fallbackEntries, _ := os.ReadDir(fallbackDir)
	if len(fallbackEntries) != 1 {
		t.Fatalf("fallback dir should have 1 file, got %d", len(fallbackEntries))
	}
}

func TestPipeline_Route_NoMatch_Stdout(t *testing.T) {
	chdirTemp(t)

	router, err := ParseRoutes([]string{
		`^im\.message=dir:./route_dir`,
	})
	if err != nil {
		t.Fatal(err)
	}
	routeDir := router.routes[0].dir

	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	// No OutputDir — unmatched events should go to stdout
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformRaw, Router: router}, &out, &errOut)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	raw := makeRawEvent("drive.file.edit_v1", `{"file_token":"xxx"}`)
	raw.Header.EventID = "ev_stdout"
	raw.Header.CreateTime = "1700000000"
	p.Process(context.Background(), raw)

	// Route dir should be empty
	routeEntries, _ := os.ReadDir(routeDir)
	if len(routeEntries) != 0 {
		t.Errorf("route dir should be empty, got %d files", len(routeEntries))
	}

	// stdout should have the event
	if out.Len() == 0 {
		t.Error("unmatched event should fall through to stdout")
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}
}

func TestPipeline_Route_FanOut(t *testing.T) {
	chdirTemp(t)

	router, err := ParseRoutes([]string{
		`^im\.=dir:./fanout1`,
		`message=dir:./fanout2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	dir1 := router.routes[0].dir
	dir2 := router.routes[1].dir

	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformCompact, Router: router}, &out, &errOut)
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}

	eventJSON := `{
		"message": {
			"message_id": "msg_fanout", "chat_id": "oc_001",
			"chat_type": "group", "message_type": "text",
			"content": "{\"text\":\"fanout\"}", "create_time": "1700000000"
		},
		"sender": {"sender_id": {"open_id": "ou_001"}}
	}`
	raw := makeRawEvent("im.message.receive_v1", eventJSON)
	raw.Header.EventID = "ev_fanout"
	raw.Header.CreateTime = "1700000000"
	p.Process(context.Background(), raw)

	// stdout should be empty
	if out.Len() != 0 {
		t.Errorf("fan-out event should not appear on stdout, got: %s", out.String())
	}

	// Both dirs should have a file
	entries1, _ := os.ReadDir(dir1)
	entries2, _ := os.ReadDir(dir2)
	if len(entries1) != 1 {
		t.Errorf("dir1 should have 1 file, got %d", len(entries1))
	}
	if len(entries2) != 1 {
		t.Errorf("dir2 should have 1 file, got %d", len(entries2))
	}
}

// --- cleanupSeen ---

func TestCleanupSeen(t *testing.T) {
	filters := NewFilterChain()
	var out, errOut bytes.Buffer
	p := NewEventPipeline(DefaultRegistry(), filters,
		PipelineConfig{Mode: TransformRaw}, &out, &errOut)

	// Insert an expired entry directly
	p.seen.Store("old_key", time.Now().Add(-10*time.Minute))
	p.seen.Store("fresh_key", time.Now())

	p.cleanupSeen(time.Now())

	if _, ok := p.seen.Load("old_key"); ok {
		t.Error("expired key should be cleaned up")
	}
	if _, ok := p.seen.Load("fresh_key"); !ok {
		t.Error("fresh key should be kept")
	}
}
