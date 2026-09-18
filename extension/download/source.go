// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"net/http"
)

// Representation declares whether repeated range requests are guaranteed to
// address the same bytes.
type Representation string

const (
	// Mutable requires a strong ETag before Open combines multiple responses.
	Mutable Representation = "mutable"
	// Immutable permits multipart reads without an ETag because the caller
	// guarantees that the source identifier pins one representation.
	Immutable Representation = "immutable"
)

// Source binds a transport to its representation stability.
type Source struct {
	transport      Transport
	representation Representation
}

// ImmutableSource allows multipart reads without a validator.
func ImmutableSource(transport Transport) Source {
	return Source{transport: transport, representation: Immutable}
}

// MutableSource requires a strong ETag before combining responses.
func MutableSource(transport Transport) Source {
	return Source{transport: transport, representation: Mutable}
}

type representationSession struct {
	transport    Transport
	contract     Representation
	totalSize    int64
	validator    string
	hasValidator bool
}

func newRepresentationSession(source Source, first contentRange, header http.Header) *representationSession {
	validator, hasValidator := strongETag(header)
	return &representationSession{
		transport:    source.transport,
		contract:     source.representation,
		totalSize:    first.total,
		validator:    validator,
		hasValidator: hasValidator,
	}
}

func (s *representationSession) multipartAllowed() bool {
	return s.contract == Immutable || s.hasValidator
}

func (s *representationSession) request(byteRange ByteRange) Request {
	request := Request{Range: &byteRange}
	if s.hasValidator {
		request.IfRange = s.validator
	}
	return request
}

func (s *representationSession) validatePartial(header http.Header, got contentRange, requested ByteRange) error {
	if got.start != requested.Start {
		return protocolError("range response is %s, want it to start at byte %d", got, requested.Start)
	}
	if got.end > requested.End {
		return protocolError("range response is %s, outside requested %s", got, requested.HeaderValue())
	}
	if got.total != s.totalSize {
		if s.hasValidator {
			return protocolError("server ignored If-Range: range response is %s, want total %d", got, s.totalSize)
		}
		return representationChanged("resource size changed while downloading: range response is %s, want total %d", got, s.totalSize)
	}
	return s.observeValidator(header)
}

func (s *representationSession) validateFull(resp *http.Response) error {
	if resp.ContentLength >= 0 && resp.ContentLength != s.totalSize {
		return representationChanged("resource size changed while downloading: full response has %d bytes, want %d", resp.ContentLength, s.totalSize)
	}
	return s.observeValidator(resp.Header)
}

func (s *representationSession) observeValidator(header http.Header) error {
	got, ok := strongETag(header)
	if !s.hasValidator {
		if ok {
			s.validator = got
			s.hasValidator = true
		}
		return nil
	}
	if !ok {
		return protocolError("response carries no usable validator, so it cannot be tied to the %q the transfer started from", s.validator)
	}
	if got != s.validator {
		return representationChanged("response carries validator %q, want %q", got, s.validator)
	}
	return nil
}
