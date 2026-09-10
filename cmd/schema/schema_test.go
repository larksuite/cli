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
	if !strings.Contains(stdout.String(), `"name": "drive `) {
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

func TestSchemaCmd_NoArgs_JSON_IsArray(t *testing.T) {
	f, stdout, _, _ := schemaTestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(out, "[") {
		head := out
		if len(head) > 80 {
			head = head[:80]
		}
		t.Errorf("expected JSON array root, first 80 chars:\n%s", head)
	}
	var envs []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &envs); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(envs) < 193 {
		t.Errorf("envelopes count = %d, want >= 193", len(envs))
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

func TestSchemaCmd_ServiceListIsArray(t *testing.T) {
	f, stdout, _, _ := schemaTestFactory(t, nil)

	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var envs []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &envs); err != nil {
		t.Fatalf("unmarshal failed: %v\n%s", err, stdout.String())
	}
	if len(envs) == 0 {
		t.Fatal("expected non-empty array for service im")
	}
	for _, e := range envs {
		name, _ := e["name"].(string)
		if !strings.HasPrefix(name, "im ") {
			t.Errorf("envelope name %q does not start with \"im \"", name)
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
	if !strings.Contains(err.Error(), "Unknown service") {
		t.Errorf("expected 'Unknown service' error, got: %v", err)
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

// Base completion navigation (dotted + space forms, strict-mode filtering,
// dotted-resource handling) lives in internal/apicatalog. The tests below pin
// cmd/schema's build-local surface projection around that navigator.

func TestSchemaSurfaceProjectionFiltersExecutionListingAndCompletion(t *testing.T) {
	catalog := schemaSurfaceCatalog()
	visible := func(path []string) bool {
		return strings.Join(path, "/") != "mail/user_mailbox.messages/list"
	}

	var out bytes.Buffer
	if err := runSchemaCatalog(&out, nil, core.StrictModeOff, catalog, nil, visible); err != nil {
		t.Fatalf("broad schema failed: %v", err)
	}
	var envelopes []map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &envelopes); err != nil {
		t.Fatalf("broad schema output is not JSON: %v\n%s", err, out.String())
	}
	names := make(map[string]bool, len(envelopes))
	for _, envelope := range envelopes {
		name, _ := envelope["name"].(string)
		names[name] = true
	}
	if names["mail user_mailbox.messages list"] {
		t.Error("broad schema retained concealed mail messages list")
	}
	for _, want := range []string{"mail user_mailbox.messages get", "im messages list"} {
		if !names[want] {
			t.Errorf("broad schema lost visible method %q: %v", want, names)
		}
	}

	out.Reset()
	err := runSchemaCatalog(
		&out,
		[]string{"mail", "user_mailbox", "messages", "list"},
		core.StrictModeOff,
		catalog,
		nil,
		visible,
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
	if err := runSchemaCatalog(&defaultOut, nil, core.StrictModeOff, catalog, nil, nil); err != nil {
		t.Fatalf("default schema failed: %v", err)
	}
	if err := runSchemaCatalog(&projectedOut, nil, core.StrictModeOff, catalog, nil, allVisible); err != nil {
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
