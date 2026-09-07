// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	downloadcore "github.com/larksuite/cli/extension/download"
)

func TestDownloadUsesHostCapability(t *testing.T) {
	request := GET("/open-apis/drive/v1/files/file_1/download").Set("version", "7")
	target := FileTarget{Name: "reports/file.bin"}
	want := Artifact{Name: target.Name, Location: "/workspace/reports/file.bin", Size: 7, ContentType: "application/octet-stream"}
	called := false
	commandContext := NewCommandContext(ContextOptions{
		Identity: IdentityUser,
		Download: func(_ context.Context, gotRequest Request, gotTarget FileTarget, options DownloadOptions) (Artifact, error) {
			called = true
			if !reflect.DeepEqual(InspectRequest(gotRequest), InspectRequest(request)) || gotTarget.Name != target.Name || gotTarget.IfExists != IfExistsFail || options.Representation != downloadcore.Mutable {
				t.Fatalf("download input = %#v, %#v, %#v", InspectRequest(gotRequest), gotTarget, options)
			}
			return want, nil
		},
	})

	got, err := Download(context.Background(), commandContext, request, target)
	if err != nil {
		t.Fatal(err)
	}
	if !called || !reflect.DeepEqual(got, want) {
		t.Fatalf("Download() = %#v, called=%v", got, called)
	}
}

func TestDownloadRejectsUnavailableOrInvalidEffects(t *testing.T) {
	validRequest := GET("/open-apis/drive/v1/files/file_1/download")
	validTarget := FileTarget{Name: "file.bin"}
	tests := map[string]struct {
		command CommandContext
		request Request
		target  FileTarget
	}{
		"input stage":      {command: NewCommandContext(ContextOptions{InputStage: true}), request: validRequest, target: validTarget},
		"dry-run":          {command: NewCommandContext(ContextOptions{DryRun: true}), request: validRequest, target: validTarget},
		"missing host":     {command: NewCommandContext(ContextOptions{}), request: validRequest, target: validTarget},
		"write method":     {command: NewCommandContext(ContextOptions{}), request: POST("/open-apis/drive/v1/files/file_1/download"), target: validTarget},
		"request body":     {command: NewCommandContext(ContextOptions{}), request: validRequest.Body(map[string]any{"x": true}), target: validTarget},
		"empty target":     {command: NewCommandContext(ContextOptions{}), request: validRequest, target: FileTarget{}},
		"directory target": {command: NewCommandContext(ContextOptions{}), request: validRequest, target: FileTarget{Name: "reports/"}},
		"dot target":       {command: NewCommandContext(ContextOptions{}), request: validRequest, target: FileTarget{Name: "reports/."}},
		"bad policy":       {command: NewCommandContext(ContextOptions{}), request: validRequest, target: FileTarget{Name: "file.bin", IfExists: IfExistsPolicy("rename")}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Download(context.Background(), test.command, test.request, test.target); err == nil {
				t.Fatal("Download() error is nil")
			}
		})
	}
}

func TestDownloadPreservesHostErrorAndRejectsInvalidArtifact(t *testing.T) {
	want := errors.New("download failed")
	ctx := NewCommandContext(ContextOptions{Download: func(context.Context, Request, FileTarget, DownloadOptions) (Artifact, error) {
		return Artifact{}, want
	}})
	_, err := Download(context.Background(), ctx, GET("/open-apis/drive/v1/files/file_1/download"), FileTarget{Name: "file.bin"})
	if !errors.Is(err, want) {
		t.Fatalf("Download() error = %v", err)
	}

	ctx = NewCommandContext(ContextOptions{Download: func(context.Context, Request, FileTarget, DownloadOptions) (Artifact, error) {
		return Artifact{Name: "file.bin", Size: 1}, nil
	}})
	if _, err := Download(context.Background(), ctx, GET("/open-apis/drive/v1/files/file_1/download"), FileTarget{Name: "file.bin"}); err == nil {
		t.Fatal("invalid artifact was accepted")
	}
}

func TestDownloadForwardsImmutableMultipartOptions(t *testing.T) {
	wantOptions := DownloadOptions{
		Representation: downloadcore.Immutable,
		Transfer:       downloadcore.Options{PartSize: 4, MaxPartRetries: 2},
	}
	ctx := NewCommandContext(ContextOptions{Download: func(_ context.Context, _ Request, _ FileTarget, got DownloadOptions) (Artifact, error) {
		if !reflect.DeepEqual(got, wantOptions) {
			t.Fatalf("download options = %#v", got)
		}
		return Artifact{Name: "file.bin", Location: "file.bin", Size: 1}, nil
	}})
	if _, err := Download(context.Background(), ctx,
		GET("/open-apis/drive/v1/files/file_1/download"), FileTarget{Name: "file.bin"}, wantOptions); err != nil {
		t.Fatal(err)
	}
	if _, err := Download(context.Background(), ctx,
		GET("/open-apis/drive/v1/files/file_1/download"), FileTarget{Name: "file.bin"},
		DownloadOptions{Representation: downloadcore.Representation("unstable")}); err == nil {
		t.Fatal("invalid representation was accepted")
	}
	if _, err := Download(context.Background(), ctx,
		GET("/open-apis/drive/v1/files/file_1/download"), FileTarget{Name: "file.bin"}, DownloadOptions{}, DownloadOptions{}); err == nil {
		t.Fatal("multiple option values were accepted")
	}
}

func TestDownloadURLUsesHostCapabilityAndRejectsUnsafeSchemes(t *testing.T) {
	const sourceURL = "https://cdn.example.com/files/report.bin?signature=secret"
	target := FileTarget{Name: "report.bin"}
	options := DownloadOptions{Representation: downloadcore.Immutable}
	called := false
	ctx := NewCommandContext(ContextOptions{DownloadURL: func(_ context.Context, gotURL string, gotTarget FileTarget, gotOptions DownloadOptions) (Artifact, error) {
		called = true
		if gotURL != sourceURL || gotTarget.Name != target.Name || gotTarget.IfExists != IfExistsFail || !reflect.DeepEqual(gotOptions, options) {
			t.Fatalf("URL download input = %q, %#v, %#v", gotURL, gotTarget, gotOptions)
		}
		return Artifact{Name: target.Name, Location: target.Name, Size: 4}, nil
	}})
	if _, err := DownloadURL(context.Background(), ctx, sourceURL, target, options); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("DownloadURL did not reach host capability")
	}
	for _, invalid := range []string{
		"http://example.com/file", "file:///tmp/file", "/relative/file", " https://example.com/file",
		"https://user:password@example.com/file", "https://example.com/file#fragment",
	} {
		if _, err := DownloadURL(context.Background(), ctx, invalid, target); err == nil {
			t.Errorf("DownloadURL(%q) error is nil", invalid)
		}
	}
}

func TestDryRunCopiesFileIntents(t *testing.T) {
	target := FileTarget{Name: "file.bin"}
	dryRun := NewDryRun(GET("/open-apis/drive/v1/files/file_1/download")).File(target.Intent("OpenAPI response body"))
	first := InspectDryRun(dryRun)
	first.Files[0].Name = "mutated.bin"
	second := InspectDryRun(dryRun)
	if len(second.Files) != 1 || second.Files[0].Name != "file.bin" || second.Files[0].IfExists != IfExistsFail {
		t.Fatalf("file intents = %#v", second.Files)
	}
}
