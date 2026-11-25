// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

func validationFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, out, errOut, _ := cmdutil.TestFactory(t, nil)
	f.APICatalog = apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{{
		Name: "fixture", Resources: map[string]meta.Resource{"items": {Methods: map[string]meta.Method{
			"create": {
				ID: "items.create", HTTPMethod: "POST", AccessTokens: []meta.Token{meta.TokenTenant},
				Parameters:  map[string]meta.Field{"folder_id": {Type: "string", Location: "path", Required: true}, "count": {Type: "integer", Location: "query", Min: "1", Max: "5"}},
				RequestBody: map[string]meta.Field{"title": {Type: "string", Required: true}, "status": {Type: "string", Enum: []any{"active", "done"}}, "items": {Type: "array", Properties: map[string]meta.Field{"id": {Type: "integer", Required: true}}}},
			},
		}}},
	}})
	f.LarkClient = nil
	f.HttpClient = nil // Any accidental API/auth transport use must fail this test.
	return f, out, errOut
}

const validInput = `{"params":{"folder_id":"PRIVATE_FOLDER"},"data":{"title":"PRIVATE_TITLE"}}`

func TestSchemaValidateFlagParsing(t *testing.T) {
	f, _, _ := validationFactory(t)
	for _, args := range [][]string{{"fixture.items.create", "--validate", validInput}, {"fixture.items.create", "--validate="}} {
		var got *SchemaOptions
		cmd := NewCmdSchema(f, func(opts *SchemaOptions) error { got = opts; return nil })
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if got == nil || !got.ValidateSet {
			t.Fatal("explicit validation flag not captured")
		}
	}
}

func TestSchemaValidateSuccessUsesEnvelopeWithoutEchoingInput(t *testing.T) {
	for _, path := range [][]string{{"fixture.items.create"}, {"fixture", "items", "create"}} {
		f, out, errOut := validationFactory(t)
		cmd := NewCmdSchema(f, nil)
		cmd.SetArgs(append(path, "--validate", validInput))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			OK   bool                   `json:"ok"`
			Data inputValidationSuccess `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if !envelope.OK || !envelope.Data.Valid || envelope.Data.SchemaPath != "fixture.items.create" {
			t.Fatalf("output=%s", out)
		}
		if strings.Contains(out.String(), "PRIVATE_") || errOut.Len() != 0 {
			t.Fatalf("stdout=%s stderr=%s", out, errOut)
		}
	}
}

func TestSchemaValidateReportsTypedFieldErrors(t *testing.T) {
	f, out, _ := validationFactory(t)
	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"fixture.items.create", "--validate", `{"params":{"folder_id":null,"count":6},"data":{"status":"PRIVATE_STATUS","items":[{}]}}`})
	err := cmd.Execute()
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("want validation, got %T %v", err, err)
	}
	if validation.Param != "--validate" || validation.Subtype != errs.SubtypeInvalidArgument || output.ExitCodeOf(err) != output.ExitValidation {
		t.Fatalf("typed error=%+v", validation)
	}
	want := []errs.InvalidParam{
		{Name: "/data/title", Reason: "required property is missing"},
		{Name: "/data/items/0/id", Reason: "required property is missing"},
		{Name: "/data/status", Reason: "must equal one of the declared enum values"},
		{Name: "/params/count", Reason: "must be less than or equal to maximum"},
		{Name: "/params/folder_id", Reason: "expected string"},
	}
	if !reflect.DeepEqual(validation.Params, want) {
		t.Fatalf("params=%+v want=%+v", validation.Params, want)
	}
	if out.Len() != 0 {
		t.Fatalf("failure polluted stdout: %s", out)
	}
	wire, marshalErr := json.Marshal(validation)
	if marshalErr != nil || strings.Contains(string(wire), "PRIVATE_STATUS") {
		t.Fatalf("error wire=%s err=%v", wire, marshalErr)
	}
}

func TestSchemaValidateMalformedAndEmptyInputPreservesCause(t *testing.T) {
	for _, input := range []string{"", " ", `{`, `{} {}`, `{"params":{},"params":{}}`} {
		f, out, _ := validationFactory(t)
		cmd := NewCmdSchema(f, nil)
		cmd.SetArgs([]string{"fixture.items.create", "--validate=" + input})
		err := cmd.Execute()
		var validation *errs.ValidationError
		if !errors.As(err, &validation) || validation.Param != "--validate" || errors.Unwrap(validation) == nil {
			t.Fatalf("input=%q err=%#v", input, err)
		}
		if out.Len() != 0 {
			t.Fatalf("unexpected stdout %s", out)
		}
	}
}

func TestSchemaValidateStandardInputAndLocalFile(t *testing.T) {
	for _, source := range []string{"stdin", "file", "quoted"} {
		t.Run(source, func(t *testing.T) {
			f, out, _ := validationFactory(t)
			input := "-"
			if source == "stdin" {
				f.IOStreams.In = strings.NewReader(validInput + "\n")
			}
			if source == "quoted" {
				input = "'" + validInput + "'"
			}
			if source == "file" {
				dir := t.TempDir()
				cmdutil.TestChdir(t, dir)
				if err := os.WriteFile(filepath.Join(dir, "input.json"), []byte(validInput), 0600); err != nil {
					t.Fatal(err)
				}
				f.FileIOProvider = &localfileio.Provider{}
				input = "@input.json"
			}
			cmd := NewCmdSchema(f, nil)
			cmd.SetArgs([]string{"fixture.items.create", "--validate", input})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), `"valid": true`) {
				t.Fatal(out.String())
			}
		})
	}
}

type validationReader struct {
	err  error
	read bool
}

func (r *validationReader) Read([]byte) (int, error) { r.read = true; return 0, r.err }

func TestSchemaValidateResolvesPathAndVisibilityBeforeReadingInput(t *testing.T) {
	for _, args := range [][]string{{}, {"fixture"}, {"fixture.items"}, {"missing.items.create"}, {"fixture.items.create"}} {
		f, out, _ := validationFactory(t)
		input := &validationReader{err: errors.New("should not read")}
		f.IOStreams.In = input
		visible := func(path []string) bool { return strings.Join(path, ".") != "fixture.items.create" }
		cmd := NewCmdSchemaWithVisibility(f, visible, nil)
		cmd.SetArgs(append(args, "--validate", "-"))
		if err := cmd.Execute(); !errs.IsValidation(err) {
			t.Fatalf("args=%v err=%v", args, err)
		}
		if input.read || out.Len() > 0 {
			t.Fatalf("read input or emitted output for unavailable selector %v", args)
		}
	}
}

func TestSchemaValidateReaderFailurePreservesCause(t *testing.T) {
	f, out, _ := validationFactory(t)
	cause := errors.New("PRIVATE_READ_FAILURE")
	f.IOStreams.In = &validationReader{err: cause}
	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"fixture.items.create", "--validate", "-"})
	err := cmd.Execute()
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || !errors.Is(err, cause) || validation.Param != "--validate" {
		t.Fatalf("error=%#v", err)
	}
	if strings.Contains(validation.Message, "PRIVATE_") || out.Len() > 0 {
		t.Fatalf("error=%+v stdout=%s", validation, out)
	}
}

func TestSchemaValidateHonorsStrictIdentityBeforeReadingInput(t *testing.T) {
	f, out, _ := validationFactory(t)
	strict, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{SupportedIdentities: 1})
	f.Credential = strict.Credential
	input := &validationReader{err: errors.New("should not read")}
	f.IOStreams.In = input
	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"fixture.items.create", "--as", "bot", "--validate", "-"})
	err := cmd.Execute()
	if !errs.IsValidation(err) || input.read || out.Len() > 0 {
		t.Fatalf("strict identity must hide bot method before reading: err=%v read=%v out=%s", err, input.read, out)
	}
}

type validationCatalogLoader struct{ err error }

func (l validationCatalogLoader) Names() []string { return []string{"fixture"} }
func (l validationCatalogLoader) Load(string) (meta.Service, error) {
	return meta.Service{}, l.err
}

func TestSchemaValidatePreservesCatalogFailureBeforeReadingInput(t *testing.T) {
	f, out, _ := validationFactory(t)
	cause := errors.New("catalog fixture failure")
	loadErr := errs.NewInternalError(errs.SubtypeSDKError, "cannot load catalog").WithCause(cause)
	f.APICatalog = apicatalog.NewLazy(apicatalog.SourceEmbedded, validationCatalogLoader{err: loadErr})
	input := &validationReader{err: io.ErrUnexpectedEOF}
	f.IOStreams.In = input
	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"fixture.items.create", "--validate", "-"})
	err := cmd.Execute()
	if err != loadErr || !errors.Is(err, cause) || input.read || out.Len() > 0 {
		t.Fatalf("catalog failure changed or input consumed: err=%v read=%v out=%s", err, input.read, out)
	}
}

type validationFile struct {
	*strings.Reader
	closed bool
}

func (f *validationFile) Close() error { f.closed = true; return nil }

type validationFileIO struct {
	fileio.FileIO
	file *validationFile
	path string
	err  error
}

func (f *validationFileIO) Open(path string) (fileio.File, error) {
	f.path = path
	return f.file, f.err
}

type validationProvider struct {
	fileio  fileio.FileIO
	context context.Context
}

func (p *validationProvider) Name() string { return "validation-test" }
func (p *validationProvider) ResolveFileIO(ctx context.Context) fileio.FileIO {
	p.context = ctx
	return p.fileio
}

func TestSchemaValidateUsesInvocationFileIOAndClosesInput(t *testing.T) {
	f, out, _ := validationFactory(t)
	file := &validationFile{Reader: strings.NewReader(validInput)}
	backend := &validationFileIO{file: file}
	provider := &validationProvider{fileio: backend}
	f.FileIOProvider = provider
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"fixture.items.create", "--validate", "@remote/input.json"})
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
	if backend.path != "remote/input.json" || !file.closed || provider.context != ctx || out.Len() == 0 {
		t.Fatalf("custom FileIO not honored: %+v", backend)
	}
}

func TestSchemaValidatePreservesFilePolicyFailure(t *testing.T) {
	f, _, _ := validationFactory(t)
	policyErr := &fileio.PathValidationError{Err: errors.New("denied")}
	f.FileIOProvider = &validationProvider{fileio: &validationFileIO{err: policyErr}}
	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"fixture.items.create", "--validate", "@outside.json"})
	err := cmd.Execute()
	if !errors.Is(err, fileio.ErrPathValidation) || !errs.IsValidation(err) {
		t.Fatalf("lost path failure: %v", err)
	}
}

func TestSchemaValidateTruncationIsExplicit(t *testing.T) {
	f, _, _ := validationFactory(t)
	cmd := NewCmdSchema(f, nil)
	input := `{"params":{"folder_id":"x"},"data":{"title":"x","items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 101), ",") + `]}}`
	cmd.SetArgs([]string{"fixture.items.create", "--validate", input})
	err := cmd.Execute()
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || len(validation.Params) != 100 || !strings.Contains(validation.Hint, "first 100") {
		t.Fatalf("error=%+v", validation)
	}
}

func TestSchemaValidateDoesNotOpenDeclaredUploadPaths(t *testing.T) {
	f, out, _, _ := schemaTestFactory(t, nil)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f.FileIOProvider = &validationProvider{fileio: &validationFileIO{err: errors.New("must not open upload")}}
	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"im.images.create", "--validate", `{"data":{"image_type":"message"},"file":{"image":"/does/not/exist"}}`})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"valid": true`) {
		t.Fatal(out.String())
	}
}

func TestSchemaValidateHelpDoesNotReadInput(t *testing.T) {
	f, _, _ := validationFactory(t)
	input := &validationReader{err: io.ErrUnexpectedEOF}
	f.IOStreams.In = input
	cmd := NewCmdSchema(f, nil)
	var help bytes.Buffer
	cmd.SetOut(&help)
	cmd.SetArgs([]string{"--help", "--validate", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if input.read || !strings.Contains(help.String(), "--validate") || !strings.Contains(help.String(), "offline") {
		t.Fatal(help.String())
	}
}

func TestSchemaValidateDoesNotChangeInspectionOutput(t *testing.T) {
	f, out, _ := validationFactory(t)
	cmd := NewCmdSchema(f, nil)
	cmd.SetArgs([]string{"fixture.items.create"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["name"] != "fixture items create" || envelope["ok"] != nil || envelope["inputSchema"] == nil {
		t.Fatal(out.String())
	}
}
