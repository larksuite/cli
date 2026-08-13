// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"io"
	"net/http"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/requestcontext"
)

// RemoteFile is a service-returned file reference that has passed the active
// runtime's data-plane policy. Its URL is intentionally private so callers
// cannot construct a trusted reference without Validate.
type RemoteFile struct {
	rawURL string
}

// URL returns the original service-provided URL verbatim.
func (f RemoteFile) URL() string { return f.rawURL }

// NewRequest constructs a request that remains bound to this validated file
// reference. Business shortcuts do not construct raw URL requests themselves.
func (f RemoteFile) NewRequest(ctx context.Context, method string, body io.Reader) (*http.Request, error) {
	if f.rawURL == "" {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"cannot build a request for an empty remote file reference")
	}
	req, err := http.NewRequestWithContext(ctx, method, f.rawURL, body)
	if err != nil {
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"build remote file request: %v", err).WithCause(err)
	}
	return req, nil
}

// DirectRemoteFileValidator applies a caller-specific policy to direct-mode
// URLs. Proxy handles are validated by the managed data-plane policy instead.
type DirectRemoteFileValidator func(context.Context, string) error

// HTTPClientProvider resolves the runtime's configured HTTP client without
// exposing raw client construction to business shortcuts.
type HTTPClientProvider func() (*http.Client, error)

// RemoteFilePolicy is the source-neutral runtime contract for validating and
// routing service-returned file references.
type RemoteFilePolicy interface {
	ValidateRemoteFile(rawURL string) error
	UsesManagedFilePlane() bool
}

// RemoteFiles is the runtime boundary for service-returned file references.
// It keeps credential mode and edition routing out of business shortcuts.
// Every service-returned file byte transfer must pass through Validate,
// RemoteFile.NewRequest, and Do so the active data-plane policy is enforced.
type RemoteFiles struct {
	policy        RemoteFilePolicy
	managedClient HTTPClientProvider
	identity      core.Identity
}

// NewRemoteFiles creates a file boundary for one resolved runtime identity.
func NewRemoteFiles(
	policy RemoteFilePolicy,
	managedClient HTTPClientProvider,
	identity core.Identity,
) *RemoteFiles {
	return &RemoteFiles{
		policy:        policy,
		managedClient: managedClient,
		identity:      identity,
	}
}

// Validate checks a known service-returned file URL and returns an opaque,
// typed reference. A directValidator, when supplied, is applied only to the
// ordinary/direct data plane; proxy handles use the configured proxy policy.
func (r *RemoteFiles) Validate(
	ctx context.Context,
	rawURL string,
	directValidator ...DirectRemoteFileValidator,
) (RemoteFile, error) {
	if r == nil {
		return RemoteFile{}, errs.NewInternalError(errs.SubtypeUnknown, "remote file runtime is unavailable")
	}
	if r.policy != nil {
		if err := r.policy.ValidateRemoteFile(rawURL); err != nil {
			return RemoteFile{}, err
		}
	}
	if !usesManagedFilePlane(r.policy) && len(directValidator) > 0 && directValidator[0] != nil {
		if err := directValidator[0](ctx, rawURL); err != nil {
			return RemoteFile{}, err
		}
	}
	return RemoteFile{rawURL: rawURL}, nil
}

// RequirePortableURL rejects commands that would return a managed file handle
// for use outside this CLI. The check is capability-based, so callers do not
// branch on a credential mode.
func (r *RemoteFiles) RequirePortableURL(param string) error {
	if r == nil || !usesManagedFilePlane(r.policy) {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"%s is unavailable because this credential source requires CLI-managed file routing", param).
		WithParam(param).
		WithHint("omit %s and let lark-cli transfer the file", param)
}

// Do executes a request for a validated remote file. Ordinary/direct runtimes
// use directClient unchanged; a nil directClient gets the same isolated
// DefaultTransport client historically used by presigned Apps transfers.
// Managed runtimes select the proxy-aware client and copy the caller's
// timeout/redirect policy so existing transfer semantics remain intact.
func (r *RemoteFiles) Do(req *http.Request, file RemoteFile, directClient *http.Client) (*http.Response, error) {
	if r == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "remote file runtime is unavailable")
	}
	if req == nil || req.URL == nil || file.rawURL == "" || req.URL.String() != file.rawURL {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"remote file request does not match its validated reference")
	}

	if directClient == nil {
		directClient = &http.Client{Transport: http.DefaultTransport}
	}
	selected := directClient
	managedPlane := usesManagedFilePlane(r.policy)
	if managedPlane {
		if r.managedClient == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"managed remote file runtime has no HTTP client")
		}
		managed, err := r.managedClient()
		if err != nil {
			return nil, err
		}
		if managed == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"managed remote file runtime returned no HTTP client")
		}
		if directClient != nil && managed != directClient {
			cloned := *managed
			cloned.Timeout = directClient.Timeout
			if directClient.CheckRedirect != nil {
				cloned.CheckRedirect = directClient.CheckRedirect
			}
			selected = &cloned
		} else {
			selected = managed
		}
	}
	if !managedPlane {
		return selected.Do(req)
	}

	requestCtx := requestcontext.WithIdentity(req.Context(), r.identity)
	return selected.Do(req.Clone(requestCtx))
}

func usesManagedFilePlane(policy RemoteFilePolicy) bool {
	return policy != nil && policy.UsesManagedFilePlane()
}
