// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"encoding/json"
)

// DiagnosticMetadata carries optional producer diagnostics without changing
// the field layout of Problem or any concrete typed error. Keeping this
// metadata in a wrapper preserves source compatibility for callers that use
// positional literals of the existing exported error structs.
type DiagnosticMetadata struct {
	Origin         string
	ProxyRequestID string
}

type diagnosticMetadataWrapper struct {
	err      error
	typed    error
	metadata DiagnosticMetadata
}

func (e *diagnosticMetadataWrapper) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *diagnosticMetadataWrapper) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *diagnosticMetadataWrapper) ProblemDetail() *Problem {
	if e == nil {
		return nil
	}
	problem, _ := ProblemOf(e.typed)
	return problem
}

func (e *diagnosticMetadataWrapper) DiagnosticMetadata() DiagnosticMetadata {
	if e == nil {
		return DiagnosticMetadata{}
	}
	return e.metadata
}

// MarshalJSON preserves the concrete typed error's extension fields and adds
// the optional diagnostics as sibling fields in the existing error object.
func (e *diagnosticMetadataWrapper) MarshalJSON() ([]byte, error) {
	raw, err := json.Marshal(e.typed)
	if err != nil {
		return nil, err
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if e.metadata.Origin != "" {
		origin, err := json.Marshal(e.metadata.Origin)
		if err != nil {
			return nil, err
		}
		object["origin"] = origin
	}
	if e.metadata.ProxyRequestID != "" {
		requestID, err := json.Marshal(e.metadata.ProxyRequestID)
		if err != nil {
			return nil, err
		}
		object["proxy_request_id"] = requestID
	}
	return json.Marshal(object)
}

// WithDiagnosticMetadata attaches optional wire diagnostics to a typed error.
// Empty metadata is a no-op. The returned wrapper still participates in
// errors.Is/errors.As and TypedError routing through Unwrap and ProblemDetail.
func WithDiagnosticMetadata(err error, metadata DiagnosticMetadata) error {
	if err == nil || (metadata.Origin == "" && metadata.ProxyRequestID == "") {
		return err
	}

	typed, ok := firstProblemCarrier(err)
	if !ok {
		return err
	}
	if existing, ok := diagnosticMetadataWrapperForProducer(typed); ok {
		merged := existing.metadata
		if metadata.Origin != "" {
			merged.Origin = metadata.Origin
		}
		if metadata.ProxyRequestID != "" {
			merged.ProxyRequestID = metadata.ProxyRequestID
		}
		return &diagnosticMetadataWrapper{err: err, typed: existing.typed, metadata: merged}
	}
	return &diagnosticMetadataWrapper{err: err, typed: typed, metadata: metadata}
}

// DiagnosticMetadataOf returns optional diagnostics attached to the first
// typed producer in err's wrap chain. Metadata on a typed cause belongs to
// that inner producer and must not be projected onto an outer typed error.
func DiagnosticMetadataOf(err error) (DiagnosticMetadata, bool) {
	typed, ok := firstProblemCarrier(err)
	if !ok {
		return DiagnosticMetadata{}, false
	}
	carrier, ok := diagnosticMetadataWrapperForProducer(typed)
	if !ok {
		return DiagnosticMetadata{}, false
	}
	metadata := carrier.DiagnosticMetadata()
	if metadata.Origin == "" && metadata.ProxyRequestID == "" {
		return DiagnosticMetadata{}, false
	}
	return metadata, true
}

// diagnosticMetadataWrapperForProducer deliberately checks only the selected
// typed producer. errors.As must not be used here because it would traverse
// into an inner typed cause and associate that cause's metadata with the outer
// producer.
func diagnosticMetadataWrapperForProducer(typed error) (*diagnosticMetadataWrapper, bool) {
	wrapper, ok := typed.(*diagnosticMetadataWrapper) //nolint:errorlint // Exact producer identity is the invariant being enforced.
	return wrapper, ok
}
