// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"os"
	"strings"

	"github.com/larksuite/cli/errs"
)

// ClassifyError maps distribution transport, protocol, and local file failures
// to the CLI error contract while preserving the original cause.
func ClassifyError(message string, err error) errs.TypedError {
	var typed errs.TypedError
	if errors.As(err, &typed) {
		return typed
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errs.NewInternalError(errs.SubtypeFileIO, "%s", message).WithCause(err)
	}
	if status, ok := httpStatusCode(err); ok {
		subtype := errs.SubtypeNetworkProtocol
		retryable := false
		switch {
		case status == 408:
			subtype, retryable = errs.SubtypeNetworkTimeout, true
		case status >= 500:
			subtype, retryable = errs.SubtypeNetworkServer, true
		}
		networkErr := errs.NewNetworkError(subtype, "%s", message).WithCode(status).WithCause(err)
		if retryable {
			networkErr.WithRetryable()
		}
		return networkErr
	}

	subtype := errs.SubtypeNetworkProtocol
	retryable := false
	var netErr net.Error
	var dnsErr *net.DNSError
	var authorityErr x509.UnknownAuthorityError
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		subtype, retryable = errs.SubtypeNetworkTimeout, true
	case errors.As(err, &authorityErr), strings.Contains(lower, "x509:"), strings.Contains(lower, "tls:"):
		subtype = errs.SubtypeNetworkTLS
	case errors.As(err, &dnsErr):
		subtype, retryable = errs.SubtypeNetworkDNS, true
	case errors.As(err, &netErr):
		subtype, retryable = errs.SubtypeNetworkTransport, true
	}
	networkErr := errs.NewNetworkError(subtype, "%s", message).WithCause(err)
	if retryable && !errors.Is(err, context.Canceled) {
		networkErr.WithRetryable()
	}
	return networkErr
}
