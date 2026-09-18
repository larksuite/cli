// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandhost

import (
	"context"
	"errors"
	"io/fs"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/command"
	"github.com/larksuite/cli/extension/download"
	"github.com/larksuite/cli/extension/fileio"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/commandbridge"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/downloadtransport"
	internaltransport "github.com/larksuite/cli/internal/transport"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// downloadCommand streams one logical OpenAPI file into the invocation's
// FileIO. MutableSource is the safe default: multipart transfer is used only
// when a strong validator binds every response to the same representation;
// immutable endpoints can gain an explicit fast path after a real need appears.
func downloadCommand(ctx context.Context, host commandbridge.RuntimeContext, request command.Request, target command.FileTarget, options command.DownloadOptions) (command.Artifact, error) {
	view := command.InspectRequest(request)
	if err := command.ValidateRequestView(view); err != nil {
		return command.Artifact{}, err
	}
	if view.Method != http.MethodGet || view.Body != nil {
		return command.Artifact{}, command.ValidationErrorf("file download requires a bodyless GET request")
	}

	transport := func(ctx context.Context, part download.Request) (*http.Response, error) {
		apiRequest := &larkcore.ApiReq{
			HttpMethod:  http.MethodGet,
			ApiPath:     view.Path,
			QueryParams: queryParams(view.Query),
		}
		return doCommandAPIStream(ctx, host, apiRequest,
			client.WithHeaders(part.Headers()), client.WithReplaySafe())
	}
	return downloadToFile(ctx, host, transport, target, options)
}

func downloadURLCommand(ctx context.Context, host commandbridge.RuntimeContext, rawURL string, target command.FileTarget, options command.DownloadOptions) (command.Artifact, error) {
	if err := validate.ValidateDownloadSourceURL(ctx, rawURL); err != nil {
		return command.Artifact{}, errs.NewSecurityPolicyError(errs.SubtypeAccessDenied,
			"blocked download URL: %v", err).WithCause(err)
	}
	apiClient, err := commandAPIClient(host)
	if err != nil {
		return command.Artifact{}, err
	}
	if apiClient.HTTP == nil {
		return command.Artifact{}, errs.NewInternalError(errs.SubtypeUnknown, "command host has no HTTP client for URL downloads")
	}
	externalClient := internaltransport.ClientForRequestClass(apiClient.HTTP, exttransport.RequestClassExternal)
	safeClient := validate.NewDownloadHTTPClient(externalClient, validate.DownloadHTTPClientOptions{})
	return downloadToFile(ctx, host, downloadtransport.URL(safeClient, rawURL), target, options)
}

func downloadToFile(ctx context.Context, host commandbridge.RuntimeContext, transport download.Transport, target command.FileTarget, options command.DownloadOptions) (command.Artifact, error) {
	fileIO := host.FileIO()
	if fileIO == nil {
		return command.Artifact{}, errs.NewInternalError(errs.SubtypeFileIO, "command host has no file I/O provider")
	}
	location, err := host.ResolveSavePath(target.Name)
	if err != nil {
		return command.Artifact{}, common.WrapSaveErrorTyped(err)
	}
	// The fail policy is enforced by the commit, not by a preceding existence
	// check: the download happens between check and commit, so a target created
	// in that window would be overwritten by a check-then-save sequence. A
	// provider that cannot commit exclusively is refused here rather than served
	// with a guarantee it does not implement.
	var exclusive fileio.ExclusiveFileIO
	if target.IfExists == command.IfExistsFail {
		capable, ok := fileIO.(fileio.ExclusiveFileIO)
		if !ok {
			return command.Artifact{}, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"the configured file provider cannot refuse an existing download target %q", target.Name).
				WithHint("use the overwrite policy explicitly, or configure a provider that commits exclusively")
		}
		exclusive = capable
		// Report the common case before spending the download: a target that
		// already exists now will still exist at commit time.
		if _, statErr := fileIO.Stat(target.Name); statErr == nil {
			return command.Artifact{}, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"download target %q already exists", target.Name).
				WithHint("choose another target or explicitly use the overwrite policy")
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return command.Artifact{}, errs.NewInternalError(errs.SubtypeFileIO,
				"inspect download target %q: %v", target.Name, statErr).WithCause(statErr)
		}
	}

	var source download.Source
	switch options.Representation {
	case download.Mutable:
		source = download.MutableSource(transport)
	case download.Immutable:
		source = download.ImmutableSource(transport)
	default:
		return command.Artifact{}, errs.NewInternalError(errs.SubtypeUnknown,
			"command host received unsupported download representation %q", options.Representation)
	}
	stream, err := download.Open(ctx, source, options.Transfer)
	if err != nil {
		return command.Artifact{}, err
	}
	defer stream.Body.Close()

	contentType := stream.Header.Get("Content-Type")
	saveOptions := fileio.SaveOptions{ContentType: contentType, ContentLength: stream.ContentLength}
	var saved fileio.SaveResult
	if exclusive != nil {
		saved, err = exclusive.SaveExclusive(target.Name, saveOptions, stream.Body)
		if errors.Is(err, fs.ErrExist) {
			return command.Artifact{}, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"download target %q already exists", target.Name).
				WithHint("choose another target or explicitly use the overwrite policy")
		}
	} else {
		saved, err = fileIO.Save(target.Name, saveOptions, stream.Body)
	}
	if err != nil {
		return command.Artifact{}, common.WrapSaveErrorTyped(err)
	}
	if stream.ContentLength >= 0 && saved.Size() != stream.ContentLength {
		return command.Artifact{}, errs.NewInternalError(errs.SubtypeFileIO,
			"download provider committed %d bytes, expected %d", saved.Size(), stream.ContentLength)
	}
	return command.Artifact{
		Name: target.Name, Location: location, Size: saved.Size(), ContentType: contentType,
	}, nil
}

func doCommandAPIStream(ctx context.Context, host commandbridge.RuntimeContext, request *larkcore.ApiReq, options ...client.Option) (*http.Response, error) {
	apiClient, err := commandAPIClient(host)
	if err != nil {
		return nil, err
	}
	base := []client.Option{client.WithHeaders(cmdutil.BaseSecurityHeaders())}
	if headers := cmdutil.ShortcutHeaders(ctx); headers != nil {
		base = append(base, client.WithHeaders(headers))
	}
	return apiClient.DoStream(ctx, request, core.Identity(host.Identity()), append(base, options...)...)
}

func commandAPIClient(host commandbridge.RuntimeContext) (*client.APIClient, error) {
	apiClient, err := host.APIClient()
	if err != nil {
		if _, typed := errs.ProblemOf(err); typed {
			return nil, err
		}
		return nil, errs.WrapInternal(err)
	}
	return apiClient, nil
}
