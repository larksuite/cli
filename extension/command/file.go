// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/larksuite/cli/extension/download"
)

// FileTarget names one invocation-scoped download destination. Name is passed
// through the active FileIO provider and must not be treated as an absolute
// local path.
type FileTarget struct {
	Name     string
	IfExists IfExistsPolicy

	_ struct{}
}

// IfExistsPolicy controls an existing download target.
type IfExistsPolicy string

const (
	// IfExistsFail preserves an existing file. It is also the zero-value policy.
	IfExistsFail IfExistsPolicy = "fail"
	// IfExistsOverwrite explicitly allows the provider to replace an existing file.
	IfExistsOverwrite IfExistsPolicy = "overwrite"
)

// Intent creates the matching dry-run file effect.
func (t FileTarget) Intent(content string) FileIntent {
	policy := t.IfExists
	if policy == "" {
		policy = IfExistsFail
	}
	return FileIntent{Name: t.Name, IfExists: policy, Content: content}
}

// Artifact is one file committed by the active host FileIO provider.
// It can be returned directly as typed command data.
type Artifact struct {
	Name        string `json:"name" schema:"required" doc:"logical artifact name"`
	Location    string `json:"location" schema:"required" doc:"host-resolved saved location"`
	Size        int64  `json:"size_bytes" schema:"required;minimum=0" doc:"committed byte count"`
	ContentType string `json:"content_type,omitempty" schema:"optional" doc:"response media type"`

	_ struct{}
}

// FileIntent describes a file effect in dry-run output without opening a
// stream or writing bytes.
type FileIntent struct {
	Name     string         `json:"name"`
	IfExists IfExistsPolicy `json:"if_exists"`
	Content  string         `json:"content,omitempty"`

	_ struct{}
}

// DownloadOptions selects the source-stability contract and the shared
// multipart engine settings. The zero value uses Mutable with production
// transfer defaults.
type DownloadOptions struct {
	Representation download.Representation
	Transfer       download.Options

	_ struct{}
}

// Download streams one authenticated OpenAPI GET response into the active
// invocation-scoped FileIO provider. The host owns range probing, bounded
// retries, response validation, body closure, provider-owned saving, and error
// typing. It is an Execute-hook capability and does not declare or register a
// CLI command.
func Download(ctx context.Context, command CommandContext, request Request, target FileTarget, options ...DownloadOptions) (Artifact, error) {
	if err := validateRequest(request); err != nil {
		return Artifact{}, err
	}
	view := InspectRequest(request)
	if view.Method != http.MethodGet {
		return Artifact{}, ValidationErrorf("file download requires GET, got %s", view.Method)
	}
	if view.Body != nil {
		return Artifact{}, ValidationErrorf("file download GET request must not contain a body")
	}
	target, resolvedOptions, err := prepareDownload(command, target, options)
	if err != nil {
		return Artifact{}, err
	}
	if command.download == nil {
		return Artifact{}, InternalErrorf("command host does not provide OpenAPI file downloads")
	}
	artifact, err := command.download(ctx, request, target, resolvedOptions)
	return validateDownloadArtifact(artifact, err)
}

// DownloadURL streams one HTTPS URL into the active invocation-scoped FileIO
// provider. The host applies external-request routing, SSRF protection, DNS/IP
// pinning, redirect validation, and the same multipart engine as Download. It
// is an Execute-hook capability, not a command definition.
func DownloadURL(ctx context.Context, command CommandContext, rawURL string, target FileTarget, options ...DownloadOptions) (Artifact, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || !strings.EqualFold(parsed.Scheme, "https") {
		return Artifact{}, ValidationErrorf("download URL must be an absolute HTTPS URL")
	}
	if rawURL != strings.TrimSpace(rawURL) {
		return Artifact{}, ValidationErrorf("download URL must be trimmed")
	}
	target, resolvedOptions, err := prepareDownload(command, target, options)
	if err != nil {
		return Artifact{}, err
	}
	if command.downloadURL == nil {
		return Artifact{}, InternalErrorf("command host does not provide URL file downloads")
	}
	artifact, err := command.downloadURL(ctx, rawURL, target, resolvedOptions)
	return validateDownloadArtifact(artifact, err)
}

func prepareDownload(command CommandContext, target FileTarget, options []DownloadOptions) (FileTarget, DownloadOptions, error) {
	if target.Name == "" || target.Name != strings.TrimSpace(target.Name) {
		return FileTarget{}, DownloadOptions{}, ValidationErrorf("download target name must be non-empty and trimmed")
	}
	normalizedName := strings.ReplaceAll(target.Name, `\`, "/")
	baseName := path.Base(normalizedName)
	if strings.HasSuffix(normalizedName, "/") || baseName == "." || baseName == ".." {
		return FileTarget{}, DownloadOptions{}, ValidationErrorf("download target %q must include a file name, not only a directory", target.Name)
	}
	if target.IfExists == "" {
		target.IfExists = IfExistsFail
	}
	if target.IfExists != IfExistsFail && target.IfExists != IfExistsOverwrite {
		return FileTarget{}, DownloadOptions{}, ValidationErrorf("unsupported download target conflict policy %q", target.IfExists)
	}
	if len(options) > 1 {
		return FileTarget{}, DownloadOptions{}, ValidationErrorf("file download accepts at most one DownloadOptions value")
	}
	resolvedOptions := DownloadOptions{Representation: download.Mutable}
	if len(options) == 1 {
		resolvedOptions = options[0]
		if resolvedOptions.Representation == "" {
			resolvedOptions.Representation = download.Mutable
		}
	}
	if resolvedOptions.Representation != download.Mutable && resolvedOptions.Representation != download.Immutable {
		return FileTarget{}, DownloadOptions{}, ValidationErrorf("unsupported download representation %q", resolvedOptions.Representation)
	}
	if command.inputStage {
		return FileTarget{}, DownloadOptions{}, ValidationErrorf("file downloads are unavailable in Normalize and Validate; move the download to Execute")
	}
	if command.dryRun {
		return FileTarget{}, DownloadOptions{}, ValidationErrorf("file downloads are unavailable during dry-run")
	}
	return target, resolvedOptions, nil
}

func validateDownloadArtifact(artifact Artifact, err error) (Artifact, error) {
	if err != nil {
		return Artifact{}, err
	}
	if artifact.Name == "" || artifact.Location == "" || artifact.Size < 0 {
		return Artifact{}, InternalErrorf("command host returned an invalid download artifact")
	}
	return artifact, nil
}
