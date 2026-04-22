// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "path/filepath"

// DefaultMinuteArtifactSubdir is the conventional top-level directory for
// minute-scoped artifacts (recording, transcript, ...).
const DefaultMinuteArtifactSubdir = "minutes"

// DefaultTranscriptFileName is the fixed file name used by `vc +notes` for the
// transcript artifact under the default layout. Recording files keep the
// server-provided name (Content-Disposition / Content-Type), so no constant is
// needed for them.
const DefaultTranscriptFileName = "transcript.txt"

// Artifact type enum surfaced in JSON output so Agents can index results by
// kind without parsing saved_path.
const (
	ArtifactTypeRecording  = "recording"
	ArtifactTypeTranscript = "transcript"
)

// DefaultMinuteArtifactDir returns the default output directory for an
// artifact keyed by minuteToken, used when the user omits --output /
// --output-dir. Both `minutes +download` and `vc +notes` land here so that
// recording and transcript of the same meeting end up in the same folder.
func DefaultMinuteArtifactDir(minuteToken string) string {
	return filepath.Join(DefaultMinuteArtifactSubdir, minuteToken)
}
