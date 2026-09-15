// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/registry"
	"github.com/spf13/cobra"
)

func schemaTestFactory(t *testing.T, config *core.CliConfig) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	f, out, errOut, in := cmdutil.TestFactory(t, config)
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	f.APICatalog = snapshot.Catalog()
	return f, out, errOut, in
}

// schemaTestCatalog is the full catalog the resolve-error tests navigate. They
// exercise rendering and hints directly rather than through a command, so they
// need the catalog alone rather than a whole Factory.
func schemaTestCatalog(t *testing.T) apicatalog.Catalog {
	t.Helper()
	f, _, _, _ := schemaTestFactory(t, nil)
	return f.APICatalog
}

func TestSchemaCmd_FlagParsing(t *testing.T) {
	f, _, _, _ := schemaTestFactory(t, nil)

	var gotOpts *SchemaOptions
	cmd := NewCmdSchema(f, func(opts *SchemaOptions) error {
		gotOpts = opts
		return nil
	})
	cmd.SetArgs([]string{"calendar.events.list"})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotOpts.Args) != 1 || gotOpts.Args[0] != "calendar.events.list" {
		t.Errorf("expected args [calendar.events.list], got %v", gotOpts.Args)
	}
}

func TestSchemaCmd_APICatalogCompletionAndRun(t *testing.T) {
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	catalog := apicatalog.Filter(snapshot.Catalog(), func(svc meta.Service) (meta.Service, bool) {
		return svc, svc.Name == "drive"
	})
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	f.APICatalog = catalog
	cmd := NewCmdSchema(f, nil)

	completions, _ := cmd.ValidArgsFunction(cmd, nil, "")
	if len(completions) != 1 || completions[0] != "drive." {
		t.Fatalf("completion = %v, want only drive.", completions)
	}
	cmd.SetArgs([]string{"drive"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	// A service target renders the method index, whose rows are dotted paths.
	if !strings.Contains(stdout.String(), `"path": "drive.`) {
		t.Fatalf("drive schema output missing drive methods: %s", stdout.String())
	}
}

func TestSchemaCmd_OutputFlagsAcceptedForCompat(t *testing.T) {
	// Agents are habituated to --format/--json/--as from api/service commands.
	// schema must accept them without erroring and always emit the JSON envelope —
	// its output is structured JSON and identity-independent, so the values have
	// no effect.
	argSets := [][]string{
		{"--format", "json"},
		{"--format", "pretty"},
		{"--format", "table"}, // no table rendering for a nested schema -> JSON
		{"--format", "csv"},
		{"--json"},
		{"--json", "--format", "ndjson"},
		{"--as", "user"},
		{"--as", "bot"},
		{"--as", "user", "--json"},
	}
	for _, extra := range argSets {
		f, stdout, _, _ := schemaTestFactory(t, nil)
		cmd := NewCmdSchema(f, nil)
		cmd.SetArgs(append([]string{"im.images.create"}, extra...))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("args %v should be accepted, got error: %v", extra, err)
		}
		var env map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
			t.Fatalf("args %v: output is not a JSON envelope: %v\n%s", extra, err, stdout.String())
		}
		if env["name"] != "im images create" {
			t.Errorf("args %v: expected the im images create envelope, got name=%v", extra, env["name"])
		}
	}
}

func TestSchemaCmd_NoArgs_RendersServiceIndex(t *testing.T) {
	f, stdout, _, _ := schemaTestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(stdout.String())
	// The bare form renders a service index object, not the former array of
	// every method's full envelope — that shape exceeded any practical
	// single-response budget.
	if !strings.HasPrefix(out, "{") {
		head := out
		if len(head) > 80 {
			head = head[:80]
		}
		t.Errorf("expected JSON object root, first 80 chars:\n%s", head)
	}
	var idx struct {
		Kind     string `json:"kind"`
		Services []struct {
			Name string `json:"name"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(out), &idx); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if idx.Kind != "service_index" {
		t.Errorf("kind = %q, want service_index", idx.Kind)
	}
	// Every service the catalog knows must be listed — this index is the entry
	// point for discovering them, so a missing one is unreachable.
	if want := len(f.APICatalog.Services()); len(idx.Services) != want {
		t.Errorf("services count = %d, want %d", len(idx.Services), want)
	}
}

func TestSchemaCmd_JSONIsEnvelope(t *testing.T) {
	f, stdout, _, _ := schemaTestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im.images.create"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, stdout.String())
	}
	if env["name"] != "im images create" {
		t.Errorf("name = %v, want \"im images create\"", env["name"])
	}
	for _, key := range []string{"description", "inputSchema", "outputSchema", "_meta"} {
		if _, ok := env[key]; !ok {
			t.Errorf("missing top-level key: %s", key)
		}
	}
	meta, _ := env["_meta"].(map[string]interface{})
	if meta["envelope_version"] != "1.0" {
		t.Errorf("envelope_version = %v, want \"1.0\"", meta["envelope_version"])
	}
}

func TestSchemaCmd_LargeIntegerBoundStaysExact(t *testing.T) {
	f, stdout, _, _ := schemaTestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"slides.xml_presentations.create", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"maximum": 9223372036854775807`) {
		t.Fatalf("schema output does not preserve MaxInt64 bound:\n%s", out)
	}
	if strings.Contains(out, "9223372036854776000") {
		t.Fatalf("schema output contains float64-rounded bound:\n%s", out)
	}
}

func TestSchemaCmd_LargeIntegerExampleStaysExact(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	f.APICatalog = apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{{
		Name: "fixture",
		Resources: map[string]meta.Resource{
			"items": {
				Methods: map[string]meta.Method{
					"get": {
						ID:         "items.get",
						HTTPMethod: "GET",
						Parameters: map[string]meta.Field{
							"cursor": {
								Type:     "integer",
								Location: "query",
								Example:  json.Number("7342342398472398471"),
							},
						},
					},
				},
			},
		},
	}})

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"fixture.items.get", "--format", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"example": 7342342398472398471`) {
		t.Fatalf("schema output does not preserve exact large integer example:\n%s", out)
	}
	if strings.Contains(out, "7342342398472398848") {
		t.Fatalf("schema output contains float64-rounded large integer example:\n%s", out)
	}
}

func TestSchemaCmd_SpaceSeparatedPath_EqualsDotted(t *testing.T) {
	f1, out1, _, _ := schemaTestFactory(t, nil)
	cmd1 := NewCmdSchema(f1, nil)
	cmd1.SetArgs([]string{"im", "images", "create"})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("space form failed: %v", err)
	}

	f2, out2, _, _ := schemaTestFactory(t, nil)
	cmd2 := NewCmdSchema(f2, nil)
	cmd2.SetArgs([]string{"im.images.create"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("dotted form failed: %v", err)
	}

	if out1.String() != out2.String() {
		t.Errorf("space and dotted forms produced different output")
	}
}

func TestSchemaCmd_ServiceRendersMethodIndex(t *testing.T) {
	f, stdout, _, _ := schemaTestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var idx struct {
		Kind    string `json:"kind"`
		Service string `json:"service"`
		Methods []struct {
			Path string `json:"path"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &idx); err != nil {
		t.Fatalf("unmarshal failed: %v\n%s", err, stdout.String())
	}
	if idx.Kind != "method_index" || idx.Service != "im" {
		t.Errorf("kind/service = %q/%q, want method_index/im", idx.Kind, idx.Service)
	}
	if len(idx.Methods) == 0 {
		t.Fatal("expected non-empty method index for service im")
	}
	// Scoping to one service must not leak another service's methods.
	for _, m := range idx.Methods {
		if !strings.HasPrefix(m.Path, "im.") {
			t.Errorf("method path %q does not start with \"im.\"", m.Path)
		}
	}
}

func TestSchemaCmd_HighRiskYesInjection(t *testing.T) {
	f, stdout, _, _ := schemaTestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im.messages.delete"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	is, _ := env["inputSchema"].(map[string]interface{})
	props, _ := is["properties"].(map[string]interface{})
	if _, ok := props["yes"]; !ok {
		t.Errorf("inputSchema.properties.yes missing for high-risk-write command")
	}
}

func TestSchemaCmd_NoYesForReadRisk(t *testing.T) {
	f, stdout, _, _ := schemaTestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im.reactions.list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	is, _ := env["inputSchema"].(map[string]interface{})
	props, _ := is["properties"].(map[string]interface{})
	if _, ok := props["yes"]; ok {
		t.Errorf("yes property should not appear for risk=read command")
	}
}

func TestSchemaCmd_UnknownService(t *testing.T) {
	f, _, _, _ := schemaTestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"nonexistent_service"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for unknown service")
	}
	// A name that is no domain at all must be called unknown. "No API methods
	// for" asserts the name exists, which is the wording reserved for
	// shortcut-only domains (see TestResolveError_ShortcutOnlyDomainPointsAtHelp).
	if !strings.Contains(err.Error(), "Unknown service") {
		t.Errorf("expected 'Unknown service' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent_service") {
		t.Errorf("error must name the rejected service, got: %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if !strings.Contains(ve.Hint, "Available:") {
		t.Errorf("expected hint listing available services, got: %q", ve.Hint)
	}
	// The hint must not hand back a command that fails the same way the rejected
	// call just did — that trades one dead end for another.
	if strings.Contains(ve.Hint, "lark-cli nonexistent_service") {
		t.Errorf("hint must not suggest running the rejected name as a command, got: %q", ve.Hint)
	}
	if !strings.Contains(ve.Hint, "lark-cli --help") {
		t.Errorf("hint must offer a usable next step, got: %q", ve.Hint)
	}
}

// TestSchemaCmd_UnknownMethod_TypedValidation pins the typed envelope for the
// JSON-mode unknown-method path: *errs.ValidationError with
// subtype invalid_argument and a hint listing the available methods.
func TestSchemaCmd_UnknownMethod_TypedValidation(t *testing.T) {
	f, _, _, _ := schemaTestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	})

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"calendar.events.nonexistent_method"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if !strings.Contains(err.Error(), "Unknown method") {
		t.Errorf("expected 'Unknown method' error, got: %v", err)
	}
	if !strings.Contains(ve.Hint, "Available:") {
		t.Errorf("expected hint listing available methods, got: %q", ve.Hint)
	}
}

// Completion candidate generation (dotted + space forms, strict-mode filtering,
// dotted-resource handling) now lives in internal/apicatalog and is covered by
// apicatalog's TestComplete. cmd/schema only adapts catalog.Complete to cobra.

func TestResolveError_ShortcutPathPointsAtHelp(t *testing.T) {
	var buf bytes.Buffer
	err := runSchemaCatalog(&buf, []string{"im", "+messages-send"}, core.StrictModeOff, schemaTestCatalog(t), nil, nil, "", nil, nil)
	if err == nil {
		t.Fatal("a +shortcut path must not resolve")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("error must carry a problem envelope")
	}
	for _, want := range []string{
		"shortcuts are documented in --help, not schema",
		"lark-cli im +messages-send --help",
		"lark-cli im --help",
	} {
		if !strings.Contains(problem.Hint, want) {
			t.Errorf("hint must contain %q, got %q", want, problem.Hint)
		}
	}
}

func TestResolveError_UnknownResourceAlsoPointsAtSchemaIndex(t *testing.T) {
	var buf bytes.Buffer
	err := runSchemaCatalog(&buf, []string{"mail", "nonexist"}, core.StrictModeOff, schemaTestCatalog(t), nil, nil, "", nil, nil)
	if err == nil {
		t.Fatal("an unknown resource must not resolve")
	}
	problem, _ := errs.ProblemOf(err)
	// The candidate list stays; only the guidance sentence is added.
	if !strings.Contains(problem.Hint, "Available:") {
		t.Errorf("hint must keep the candidate list, got %q", problem.Hint)
	}
	if !strings.Contains(problem.Hint, "lark-cli schema mail") {
		t.Errorf("hint must point at the method index, got %q", problem.Hint)
	}
}

func TestResolveError_SanitizesEchoedInput(t *testing.T) {
	var buf bytes.Buffer
	err := runSchemaCatalog(&buf, []string{"im", "+bad\x1b[31mname"}, core.StrictModeOff, schemaTestCatalog(t), nil, nil, "", nil, nil)
	if err == nil {
		t.Fatal("must not resolve")
	}
	problem, _ := errs.ProblemOf(err)
	if strings.Contains(problem.Hint, "\x1b") {
		t.Errorf("hint must not echo control characters, got %q", problem.Hint)
	}
}

// The rejected segment appears in both the message and the hint, so both go
// through the same whitelist — a raw bidi override in the message could still
// reorder how the rejection reads.
func TestResolveError_SanitizesShortcutMessageToo(t *testing.T) {
	var buf bytes.Buffer
	err := runSchemaCatalog(&buf, []string{"im", "+bad‮name"}, core.StrictModeOff, schemaTestCatalog(t), nil, nil, "", nil, nil)
	if err == nil {
		t.Fatal("must not resolve")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("error must carry a problem envelope")
	}
	if strings.ContainsRune(problem.Message, '‮') {
		t.Errorf("message must not echo bidi controls, got %q", problem.Message)
	}
	if strings.ContainsRune(problem.Hint, '‮') {
		t.Errorf("hint must not echo bidi controls, got %q", problem.Hint)
	}
}

// The shortcut and service branches were sanitized from the start; the resource,
// method and path rejections echoed their subject raw. Every branch shares one
// envelope that an agent parses, so each has to reach it through the same
// whitelist — a bidi override surviving in any one of them can reorder how the
// whole rejection reads.
//
// The parts are passed pre-split, exactly as apicatalog.ParsePath would hand
// them over, because a single dotted argument resolves as a service name and
// would test the service branch instead — which was never the unsanitized one.
// wantPrefix pins which branch each case actually reaches, so the test cannot
// silently drift back onto an already-safe path.
func TestResolveError_SanitizesEveryRejectedSubject(t *testing.T) {
	const bidi = "\u202e"
	for _, tc := range []struct {
		name       string
		parts      []string
		wantPrefix string
	}{
		{"resource", []string{"im", "chat" + bidi + "members", "get"}, "Unknown resource:"},
		{"method", []string{"im", "chats", "ge" + bidi + "t"}, "Unknown method:"},
		{"control in resource", []string{"im", "chat\x1b[31mmembers", "get"}, "Unknown resource:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := runSchemaCatalog(&buf, tc.parts, core.StrictModeOff, schemaTestCatalog(t), nil, nil, "", nil, nil)
			if err == nil {
				t.Fatal("must not resolve")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatal("error must carry a problem envelope")
			}
			if !strings.HasPrefix(problem.Message, tc.wantPrefix) {
				t.Fatalf("case reached the wrong branch: message %q, want prefix %q", problem.Message, tc.wantPrefix)
			}
			for _, field := range []struct{ label, text string }{
				{"message", problem.Message},
				{"hint", problem.Hint},
			} {
				if strings.Contains(field.text, bidi) {
					t.Errorf("%s must not echo bidi controls, got %q", field.label, field.text)
				}
				if strings.Contains(field.text, "\x1b") {
					t.Errorf("%s must not echo control characters, got %q", field.label, field.text)
				}
			}
		})
	}
}

// A name that is absent from the API catalog but present in the command tree —
// a +shortcut-only domain, or a CLI command like `auth` — must not be called
// unknown, and the rejection must point back at the help tree.
func TestResolveError_ExistingCommandWithoutAPIPointsAtHelp(t *testing.T) {
	var buf bytes.Buffer
	exists := func(name string) bool { return name == "docs" }
	err := runSchemaCatalog(&buf, []string{"docs"}, core.StrictModeOff, schemaTestCatalog(t), nil, nil, "", exists, nil)
	if err == nil {
		t.Fatal("a shortcut-only domain has no API methods and must not resolve")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatal("error must carry a problem envelope")
	}
	if strings.Contains(problem.Message, "Unknown service") {
		t.Errorf("message must not claim the domain is unknown, got %q", problem.Message)
	}
	if !strings.Contains(problem.Message, "docs") {
		t.Errorf("message must name the rejected domain, got %q", problem.Message)
	}
	for _, want := range []string{"lark-cli docs --help", "lark-cli --help"} {
		if !strings.Contains(problem.Hint, want) {
			t.Errorf("hint must offer %q, got %q", want, problem.Hint)
		}
	}
}

// The command tree, not the shortcut registry, decides which branch a name
// takes: CLI commands such as `auth` provide no API methods and no +shortcuts,
// yet `lark-cli auth --help` works, so calling them unknown misleads just as
// much as it does for a shortcut-only domain.
func TestSchemaCmd_CLICommandIsNotCalledUnknown(t *testing.T) {
	for _, name := range []string{"auth", "config", "whoami"} {
		f, _, _, _ := schemaTestFactory(t, nil)
		root := &cobra.Command{Use: "lark-cli"}
		root.AddCommand(&cobra.Command{Use: name, RunE: func(*cobra.Command, []string) error { return nil }})
		root.AddCommand(NewCmdSchema(f, nil))
		root.SetArgs([]string{"schema", name})
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		err := root.Execute()
		if err == nil {
			t.Fatalf("%s has no API methods and must not resolve", name)
		}
		problem, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("%s: error must carry a problem envelope", name)
		}
		if strings.Contains(problem.Message, "Unknown service") {
			t.Errorf("%s: message must not claim the command is unknown, got %q", name, problem.Message)
		}
		if !strings.Contains(problem.Hint, "lark-cli "+name+" --help") {
			t.Errorf("%s: hint must point at the command's own help, got %q", name, problem.Hint)
		}
	}
}

// Base completion navigation (dotted + space forms, strict-mode filtering,
// dotted-resource handling) lives in internal/apicatalog. The tests below pin
// cmd/schema's build-local surface projection around that navigator.

func TestSchemaSurfaceProjectionFiltersExecutionListingAndCompletion(t *testing.T) {
	catalog := schemaSurfaceCatalog()
	visible := func(path []string) bool {
		return strings.Join(path, "/") != "mail/user_mailbox.messages/list"
	}

	// The bare form renders the service index, which names services rather than
	// methods. Both services survive projection here because each keeps at least
	// one visible method.
	var out bytes.Buffer
	if err := runSchemaCatalog(&out, nil, core.StrictModeOff, catalog, nil, visible, "", nil, nil); err != nil {
		t.Fatalf("broad schema failed: %v", err)
	}
	var index struct {
		Kind     string `json:"kind"`
		Services []struct {
			Name string `json:"name"`
		} `json:"services"`
	}
	if err := json.Unmarshal(out.Bytes(), &index); err != nil {
		t.Fatalf("broad schema output is not JSON: %v\n%s", err, out.String())
	}
	if index.Kind != "service_index" {
		t.Errorf("kind = %q, want service_index", index.Kind)
	}
	services := make(map[string]bool, len(index.Services))
	for _, svc := range index.Services {
		services[svc.Name] = true
	}
	for _, want := range []string{"mail", "im"} {
		if !services[want] {
			t.Errorf("service index lost %q: %v", want, services)
		}
	}

	// A concealed method can only surface in the method index, so that is where
	// listing-side projection has to be asserted.
	out.Reset()
	if err := runSchemaCatalog(&out, []string{"mail"}, core.StrictModeOff, catalog, nil, visible, "", nil, nil); err != nil {
		t.Fatalf("mail method index failed: %v", err)
	}
	var methodIndex struct {
		Methods []struct {
			Path string `json:"path"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(out.Bytes(), &methodIndex); err != nil {
		t.Fatalf("mail method index is not JSON: %v\n%s", err, out.String())
	}
	paths := make(map[string]bool, len(methodIndex.Methods))
	for _, m := range methodIndex.Methods {
		paths[m.Path] = true
	}
	if paths["mail.user_mailbox.messages.list"] {
		t.Error("method index retained concealed mail messages list")
	}
	if !paths["mail.user_mailbox.messages.get"] {
		t.Errorf("method index lost visible sibling: %v", paths)
	}

	out.Reset()
	err := runSchemaCatalog(
		&out,
		[]string{"mail", "user_mailbox", "messages", "list"},
		core.StrictModeOff,
		catalog,
		nil,
		visible,
		"",
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("concealed exact method unexpectedly resolved")
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("concealed exact method error = %T %v, want validation/invalid_argument", err, err)
	}
	if strings.Contains(validationErr.Hint, "list") || !strings.Contains(validationErr.Hint, "get") {
		t.Errorf("resolve candidates were not surface-projected: %q", validationErr.Hint)
	}
	if out.Len() != 0 {
		t.Errorf("concealed exact method wrote schema output: %s", out.String())
	}

	projected := projectSchemaCatalog(catalog, visible)
	if got, _ := projected.Complete(nil, "mail.user_mailbox.messages.l", nil); len(got) != 0 {
		t.Errorf("dotted completion exposed concealed method: %v", got)
	}
	if got, _ := projected.Complete(nil, "mail.user_mailbox.messages.g", nil); !reflect.DeepEqual(got, []string{"mail.user_mailbox.messages.get"}) {
		t.Errorf("dotted completion lost visible sibling: %v", got)
	}
	if got, _ := projected.Complete([]string{"mail", "user_mailbox", "messages"}, "l", nil); len(got) != 0 {
		t.Errorf("space completion exposed concealed method: %v", got)
	}
	if got, _ := projected.Complete([]string{"mail", "user_mailbox", "messages"}, "g", nil); !reflect.DeepEqual(got, []string{"get"}) {
		t.Errorf("space completion lost visible sibling: %v", got)
	}
}

func TestSchemaSurfaceProjectionDropsServiceWhenGlobConcealsAllDescendants(t *testing.T) {
	catalog := schemaSurfaceCatalog()
	// Mirrors a policy that retains the top-level schema command and mail group
	// but conceals mail/**.
	visible := func(path []string) bool {
		return !strings.HasPrefix(strings.Join(path, "/"), "mail/")
	}
	projected := projectSchemaCatalog(catalog, visible)

	if _, ok := projected.Service("mail"); ok {
		t.Fatal("mail survived as an empty schema namespace after mail/** was concealed")
	}
	if _, ok := projected.Service("im"); !ok {
		t.Fatal("unrelated visible service im was removed")
	}
	if got, _ := projected.Complete(nil, "ma", nil); len(got) != 0 {
		t.Errorf("root dotted completion exposed concealed mail service: %v", got)
	}
	if got, _ := projected.Complete(nil, "im.m", nil); !reflect.DeepEqual(got, []string{"im.messages."}) {
		t.Errorf("root dotted completion lost visible im service: %v", got)
	}

	_, err := projected.Resolve([]string{"mail", "messages", "get"})
	var resolveErr *apicatalog.ResolveError
	if !errors.As(err, &resolveErr) || resolveErr.Kind != apicatalog.ErrService {
		t.Fatalf("concealed mail resolve error = %T %v, want unknown service", err, err)
	}
	if strings.Contains(strings.Join(resolveErr.Candidates, ","), "mail") {
		t.Errorf("unknown-service candidates exposed concealed mail: %v", resolveErr.Candidates)
	}
}

func TestSchemaSurfaceProjectionPreservesDefaultAndDeniedVisibleCatalog(t *testing.T) {
	catalog := schemaSurfaceCatalog()
	allVisible := func([]string) bool { return true }

	var defaultOut, projectedOut bytes.Buffer
	if err := runSchemaCatalog(&defaultOut, nil, core.StrictModeOff, catalog, nil, nil, "", nil, nil); err != nil {
		t.Fatalf("default schema failed: %v", err)
	}
	if err := runSchemaCatalog(&projectedOut, nil, core.StrictModeOff, catalog, nil, allVisible, "", nil, nil); err != nil {
		t.Fatalf("all-visible schema failed: %v", err)
	}
	if defaultOut.String() != projectedOut.String() {
		t.Errorf("all-referenceable surface changed default schema output\ndefault: %s\nprojected: %s", defaultOut.String(), projectedOut.String())
	}
}

func schemaSurfaceCatalog() apicatalog.Catalog {
	service := func(name string, methods map[string]interface{}) meta.Service {
		resourceName := "messages"
		if name == "mail" {
			resourceName = "user_mailbox.messages"
		}
		return meta.ServiceFromMap(map[string]interface{}{
			"name":        name,
			"version":     "v1",
			"servicePath": "/open-apis/" + name + "/v1",
			"resources": map[string]interface{}{
				resourceName: map[string]interface{}{
					"methods": methods,
				},
			},
		})
	}
	method := func(id, description string) map[string]interface{} {
		return map[string]interface{}{
			"id":           id,
			"path":         "/open-apis/fixture/v1/messages",
			"httpMethod":   "GET",
			"description":  description,
			"risk":         "read",
			"accessTokens": []interface{}{"tenant"},
		}
	}
	return apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{
		service("mail", map[string]interface{}{
			"get":  method("mail.user_mailbox.messages.get", "visible mail method"),
			"list": method("mail.user_mailbox.messages.list", "concealable mail method"),
		}),
		service("im", map[string]interface{}{
			"list": method("im.messages.list", "visible im method"),
		}),
	})
}
