// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package deviceinfo collects the platform hardware product model and the
// platform values used by device-related risk-control headers.
package riskcontrol

import (
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/http/httpguts"
)

// OSType is the server-side risk-control operating-system enum.
type OSType string

// OS type enum values for X-Agent-Os-Type.
const (
	OSTypeUnknown = "0"
	OSTypeWindows = "1"
	OSTypeLinux   = "2"
	OSTypeMacOS   = "3"
)

const (
	// TerminalTypePC is the fixed X-Agent-Terminal-Type value for the CLI.
	TerminalTypePC = "1"

	// Unknown is used when the hardware product model cannot be collected.
	Unknown = "Unknown"

	// deviceModelMaxBytes bounds the value added to X-Agent-Device-Type.
	// Device models are short identifiers; a larger value is treated as
	// malformed rather than truncated so the header never misrepresents it.
	deviceModelMaxBytes = 256
)

// Snapshot contains the deliberately small risk-control signal set.
// ProductModel is omitted when the platform cannot provide a safe value.
type Snapshot struct {
	OSType       OSType
	ProductModel string
}

// Source supplies one immutable process-level snapshot.
type Source interface {
	Snapshot() Snapshot
}

// HostSource lazily reads host signals once, after outbound policy authorizes
// the first request. Failed probes are cached and are not retried per request.
type HostSource struct {
	once      sync.Once
	value     Snapshot
	readModel func() string
}

// NewHostSource creates the production host signal source.
func NewHostSource() *HostSource {
	return &HostSource{readModel: readDeviceModel}
}

// Snapshot returns the cached host signal snapshot.
func (s *HostSource) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.once.Do(func() {
		readModel := s.readModel
		if readModel == nil {
			readModel = readDeviceModel
		}
		s.value = Snapshot{
			OSType:       GetOSType(OSName()),
			ProductModel: normalizeDeviceModel(readModel()),
		}
	})
	return s.value
}

// normalizeModel removes non-printable characters and returns a model only
// when the remaining text is safe to use as an HTTP header value. Input that
// cannot produce a valid model is rejected so Get can fall back to Unknown.
func normalizeDeviceModel(model string) string {
	if !utf8.ValidString(model) {
		return ""
	}
	model = strings.Map(func(r rune) rune {
		switch {
		case r == '\r' || r == '\n' || r == '\x00':
			return -1
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsPrint(r):
			return r
		default:
			return -1
		}
	}, model)

	model = strings.Join(strings.Fields(model), " ")

	if model == "" || len(model) > deviceModelMaxBytes {
		return ""
	}
	if !httpguts.ValidHeaderFieldValue(model) {
		return ""
	}
	return model
}

// GetOSType maps a platform name to the X-Agent-Os-Type enum.
func GetOSType(osName string) OSType {
	switch osName {
	case "Windows":
		return OSTypeWindows
	case "Linux":
		return OSTypeLinux
	case "MacOS":
		return OSTypeMacOS
	default:
		return OSTypeUnknown
	}
}

// OSName returns the platform name used by GetOSType.
func OSName() string {
	switch runtime.GOOS {
	case "darwin":
		return "MacOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}
