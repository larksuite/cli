// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// This file defines artifact-path conventions shared between
// `minutes +download` and `vc +notes`. Callers outside those two shortcuts
// should not take a dependency on these symbols.

package common

import "path/filepath"

// DefaultMinuteArtifactSubdir is the top-level directory for minute-scoped
// artifacts under the default layout.
const DefaultMinuteArtifactSubdir = "minutes"

// DefaultTranscriptFileName is the fixed transcript filename under the
// default layout. Recording files keep the server-provided name.
const DefaultTranscriptFileName = "transcript.txt"

// ArtifactTypeRecording is the artifact_type value emitted by
// `minutes +download` so that callers can index results by kind without
// parsing saved_path.
const ArtifactTypeRecording = "recording"

// DefaultMinuteArtifactDir returns the default output directory for an
// artifact keyed by minuteToken. The same path is shared across commands so
// that related artifacts of one meeting land together.
func DefaultMinuteArtifactDir(minuteToken string) string {
	return filepath.Join(DefaultMinuteArtifactSubdir, minuteToken)
}
