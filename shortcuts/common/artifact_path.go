// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "path/filepath"

// DefaultMinuteArtifactSubdir is the top-level directory for minute-scoped
// artifacts under the default layout.
const DefaultMinuteArtifactSubdir = "minutes"

// DefaultTranscriptFileName is the fixed transcript filename under the
// default layout. Recording files keep the server-provided name.
const DefaultTranscriptFileName = "transcript.txt"

// Artifact type values emitted in JSON output so that callers can index
// results by kind without parsing saved_path.
const (
	ArtifactTypeRecording  = "recording"
	ArtifactTypeTranscript = "transcript"
)

// DefaultMinuteArtifactDir returns the default output directory for an
// artifact keyed by minuteToken. The same path is shared across commands so
// that related artifacts of one meeting land together.
func DefaultMinuteArtifactDir(minuteToken string) string {
	return filepath.Join(DefaultMinuteArtifactSubdir, minuteToken)
}
