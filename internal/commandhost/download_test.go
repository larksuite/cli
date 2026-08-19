// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandhost

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/download"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/spf13/cobra"
)

type backupArgs struct {
	FileToken string `flag:"file-token" schema:"required;minLength=1" doc:"file token"`
	Output    string `flag:"output" schema:"required;minLength=1" doc:"logical output name"`
	Overwrite bool   `flag:"overwrite" schema:"optional;default=false" doc:"replace an existing output"`
}

func backupDownloadOptions() command.DownloadOptions {
	return command.DownloadOptions{
		Representation: download.Immutable,
		Transfer:       download.Options{PartSize: 4, MaxPartRetries: 1},
	}
}

func externalBackupCommand() command.Command {
	return externalBackupCommandWithOptions("+external-backup", backupDownloadOptions())
}

func externalBackupCommandWithOptions(name string, options ...command.DownloadOptions) command.Command {
	request := func(args *backupArgs) command.Request {
		return command.GET("/open-apis/drive/v1/files/" + command.PathSegment(args.FileToken) + "/download")
	}
	target := func(args *backupArgs) command.FileTarget {
		result := command.FileTarget{Name: args.Output}
		if args.Overwrite {
			result.IfExists = command.IfExistsOverwrite
		}
		return result
	}
	return command.Define(command.Definition[backupArgs, command.Artifact]{
		Metadata: command.CommandMetadata{
			Service: command.DomainDrive, Command: name, Description: "Download a file", Risk: command.RiskWrite,
			Authorization: command.AuthorizationDefinition{Identities: map[command.Identity]command.IdentityAuthorization{
				command.IdentityUser: {RequiredScopes: []string{"drive:file:download"}},
			}},
		},
		Hooks: command.Hooks[backupArgs, command.Artifact]{
			DryRun: func(_ context.Context, _ command.CommandContext, args *backupArgs) *command.DryRun {
				return command.NewDryRun(request(args)).File(target(args).Intent("OpenAPI response body"))
			},
			Execute: func(ctx context.Context, commandContext command.CommandContext, args *backupArgs) (command.Result[command.Artifact], error) {
				artifact, err := command.Download(ctx, commandContext, request(args), target(args), options...)
				if err != nil {
					return command.Result[command.Artifact]{}, err
				}
				return command.Success(artifact), nil
			},
		},
	})
}

func TestExternalDownloadPreservesExistingFileBeforeNetwork(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	if err := vfs.WriteFile("file.bin", []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	root, factory := mountExternalBackup(t)
	called := false
	factoryTestRegistry(t, factory).Register(&httpmock.Stub{
		Method: http.MethodGet, URL: "/open-apis/drive/v1/files/file_1/download",
		RawBody: []byte("replacement"), ContentType: "application/octet-stream", Optional: true,
		OnMatch: func(*http.Request) { called = true },
	})

	root.SetArgs([]string{"drive", "+external-backup", "--file-token", "file_1", "--output", "file.bin", "--as", "user"})
	_, err := root.ExecuteC()
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("download error = %#v", err)
	}
	content, readErr := vfs.ReadFile("file.bin")
	if readErr != nil || string(content) != "original" || called {
		t.Fatalf("existing file = %q, readErr=%v, networkCalled=%v", content, readErr, called)
	}
}

func TestExternalDownloadRejectsTraversalBeforeNetwork(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	root, factory := mountExternalBackup(t)
	called := false
	factoryTestRegistry(t, factory).Register(&httpmock.Stub{
		Method: http.MethodGet, URL: "/open-apis/drive/v1/files/file_1/download",
		RawBody: []byte("payload"), ContentType: "application/octet-stream", Optional: true,
		OnMatch: func(*http.Request) { called = true },
	})

	root.SetArgs([]string{"drive", "+external-backup", "--file-token", "file_1", "--output", "../escape/file.bin", "--as", "user"})
	_, err := root.ExecuteC()
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("download error = %#v", err)
	}
	if called {
		t.Fatal("unsafe output path reached the network")
	}
}

func TestExternalDownloadExplicitOverwriteReplacesExistingFile(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	if err := vfs.WriteFile("file.bin", []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	root, factory := mountExternalBackup(t)
	factoryTestRegistry(t, factory).Register(&httpmock.Stub{
		Method: http.MethodGet, URL: "/open-apis/drive/v1/files/file_1/download",
		RawBody: []byte("replacement"), ContentType: "application/octet-stream",
	})

	root.SetArgs([]string{"drive", "+external-backup", "--file-token", "file_1", "--output", "file.bin", "--overwrite", "--as", "user"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	content, err := vfs.ReadFile("file.bin")
	if err != nil || string(content) != "replacement" {
		t.Fatalf("overwritten file = %q, %v", content, err)
	}
}

func mountExternalBackup(t *testing.T) (*cobra.Command, *cmdutil.Factory) {
	return mountDownloadCommand(t, externalBackupCommand())
}

func mountDownloadCommand(t *testing.T, declaration command.Command) (*cobra.Command, *cmdutil.Factory) {
	t.Helper()
	compiled, err := CompileSets([]command.Set{{
		Domain: command.ExtendDomain(command.DomainDrive), Commands: []command.Command{declaration},
	}})
	if err != nil {
		t.Fatal(err)
	}
	factory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "app-id", AppSecret: "app-secret"})
	root := &cobra.Command{Use: "lark-cli", SilenceErrors: true, SilenceUsage: true}
	service := &cobra.Command{Use: "drive"}
	root.AddCommand(service)
	compiled[0].Mount(service, factory)
	return root, factory
}

func TestExternalDownloadDefaultsToMutableRepresentation(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	root, factory := mountDownloadCommand(t, externalBackupCommandWithOptions("+external-backup-mutable", command.DownloadOptions{
		Transfer: download.Options{PartSize: 4},
	}))
	registry := factoryTestRegistry(t, factory)
	probe := &httpmock.Stub{
		Method: http.MethodGet, URL: "/open-apis/drive/v1/files/file_1/download",
		Status: http.StatusPartialContent, RawBody: []byte("payl"),
		Headers: http.Header{
			"Content-Type":  {"application/octet-stream"},
			"Content-Range": {"bytes 0-3/7"},
		},
	}
	full := &httpmock.Stub{
		Method: http.MethodGet, URL: "/open-apis/drive/v1/files/file_1/download",
		RawBody: []byte("payload"), ContentType: "application/octet-stream",
	}
	registry.Register(probe)
	registry.Register(full)

	root.SetArgs([]string{"drive", "+external-backup-mutable", "--file-token", "file_1", "--output", "file.bin", "--as", "user"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	content, err := vfs.ReadFile("file.bin")
	if err != nil || string(content) != "payload" {
		t.Fatalf("downloaded content = %q, %v", content, err)
	}
	if probe.CapturedHeaders.Get("Range") != "bytes=0-3" || full.CapturedHeaders.Get("Range") != "" {
		t.Fatalf("mutable requests = probe %#v, full %#v", probe.CapturedHeaders, full.CapturedHeaders)
	}
}

func TestExternalDownloadStreamsThroughCommonDownloadAndFileIO(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	root, factory := mountExternalBackup(t)
	registry := factoryTestRegistry(t, factory)
	first := &httpmock.Stub{
		Method: http.MethodGet, URL: "/open-apis/drive/v1/files/file_1/download",
		Status: http.StatusPartialContent, RawBody: []byte("payl"),
		Headers: http.Header{
			"Content-Type":  {"application/octet-stream"},
			"Content-Range": {"bytes 0-3/7"},
		},
	}
	second := &httpmock.Stub{
		Method: http.MethodGet, URL: "/open-apis/drive/v1/files/file_1/download",
		Status: http.StatusPartialContent, RawBody: []byte("oad"),
		Headers: http.Header{
			"Content-Type":  {"application/octet-stream"},
			"Content-Range": {"bytes 4-6/7"},
		},
	}
	registry.Register(first)
	registry.Register(second)

	root.SetArgs([]string{"drive", "+external-backup", "--file-token", "file_1", "--output", "reports/file.bin", "--as", "user"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	content, err := vfs.ReadFile("reports/file.bin")
	if err != nil || string(content) != "payload" {
		t.Fatalf("downloaded content = %q, %v", content, err)
	}
	if first.CapturedHeaders.Get("Range") != "bytes=0-3" || second.CapturedHeaders.Get("Range") != "bytes=4-6" ||
		first.CapturedHeaders.Get("Accept-Encoding") != "identity" || second.CapturedHeaders.Get("Accept-Encoding") != "identity" {
		t.Fatalf("download headers = first %#v, second %#v", first.CapturedHeaders, second.CapturedHeaders)
	}
	if first.CapturedHeaders.Get("Authorization") != "Bearer test-token" || second.CapturedHeaders.Get("Authorization") != "Bearer test-token" {
		t.Fatalf("authorization = first %q, second %q", first.CapturedHeaders.Get("Authorization"), second.CapturedHeaders.Get("Authorization"))
	}
}

func TestExternalDownloadDryRunReportsFileWithoutWriting(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	compiled, err := CompileSets([]command.Set{{
		Domain: command.ExtendDomain(command.DomainDrive), Commands: []command.Command{externalBackupCommand()},
	}})
	if err != nil {
		t.Fatal(err)
	}
	factory, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "app-id", AppSecret: "app-secret"})
	root := &cobra.Command{Use: "lark-cli", SilenceErrors: true, SilenceUsage: true}
	service := &cobra.Command{Use: "drive"}
	root.AddCommand(service)
	compiled[0].Mount(service, factory)
	root.SetArgs([]string{"drive", "+external-backup", "--file-token", "file_1", "--output", "reports/file.bin", "--as", "user", "--dry-run"})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatal(err)
	}
	if output := stdout.String(); !strings.Contains(output, `"files"`) || !strings.Contains(output, `"name": "reports/file.bin"`) {
		t.Fatalf("dry-run output = %s", output)
	}
	if _, err := vfs.Stat("reports/file.bin"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("dry-run created output: %v", err)
	}
}

func TestConvertDryRunValidatesFileIntent(t *testing.T) {
	if _, err := convertDryRun(command.NewDryRun().File(command.FileIntent{})); err == nil {
		t.Fatal("empty dry-run file intent was accepted")
	}
	if _, err := convertDryRun(command.NewDryRun().File(command.FileIntent{
		Name: "file.bin", IfExists: command.IfExistsPolicy("rename"),
	})); err == nil {
		t.Fatal("unsupported dry-run conflict policy was accepted")
	}
}

func TestURLDownloadBlocksLocalTargetBeforeHostCapabilities(t *testing.T) {
	_, err := downloadURLCommand(context.Background(), stubHost{}, "https://127.0.0.1/file", command.FileTarget{
		Name: "file.bin", IfExists: command.IfExistsFail,
	}, command.DownloadOptions{Representation: download.Mutable})
	var policy *errs.SecurityPolicyError
	if !errors.As(err, &policy) || policy.Subtype != errs.SubtypeAccessDenied {
		t.Fatalf("URL download error = %#v", err)
	}
}

// factoryTestRegistry returns the registry installed by TestFactory's HTTP
// client without adding another production seam.
func factoryTestRegistry(t *testing.T, factory *cmdutil.Factory) *httpmock.Registry {
	t.Helper()
	client, err := factory.HttpClient()
	if err != nil {
		t.Fatal(err)
	}
	registry, ok := client.Transport.(*httpmock.Registry)
	if !ok {
		t.Fatalf("test HTTP transport = %T", client.Transport)
	}
	return registry
}

// The fail policy must survive a target that appears while the download is in
// flight. The existence check happens before the network, so only an exclusive
// commit can refuse the file at that point -- a check-then-save sequence would
// overwrite whatever the other writer put there.
func TestExternalDownloadRefusesTargetCreatedDuringTransfer(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	root, factory := mountExternalBackup(t)
	factoryTestRegistry(t, factory).Register(&httpmock.Stub{
		Method: http.MethodGet, URL: "/open-apis/drive/v1/files/file_1/download",
		RawBody: []byte("replacement"), ContentType: "application/octet-stream",
		OnMatch: func(*http.Request) {
			// Another writer wins the name after the pre-flight check passed.
			if err := vfs.WriteFile("file.bin", []byte("concurrent"), 0600); err != nil {
				t.Fatalf("simulate concurrent writer: %v", err)
			}
		},
	})

	root.SetArgs([]string{"drive", "+external-backup", "--file-token", "file_1", "--output", "file.bin", "--as", "user"})
	_, err := root.ExecuteC()
	var validation *errs.ValidationError
	if !errors.As(err, &validation) || validation.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("download error = %#v, want a failed-precondition refusal", err)
	}
	content, readErr := vfs.ReadFile("file.bin")
	if readErr != nil || string(content) != "concurrent" {
		t.Fatalf("concurrently created file = %q (readErr=%v), want it preserved", content, readErr)
	}
}
