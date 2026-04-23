// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import "testing"

func TestDriveExportStatusLabelCoversKnownAndUnknownCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status driveExportStatus
		want   string
	}{
		{
			name:   "size limit",
			status: driveExportStatus{JobStatus: 107},
			want:   "export_size_limit",
		},
		{
			name:   "not exist",
			status: driveExportStatus{JobStatus: 123},
			want:   "docs_not_exist",
		},
		{
			name:   "unknown status",
			status: driveExportStatus{JobStatus: 999},
			want:   "status_999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.status.StatusLabel(); got != tt.want {
				t.Fatalf("StatusLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseDriveExportStatusWithoutResultKeepsTicket(t *testing.T) {
	t.Parallel()

	status := parseDriveExportStatus("ticket_export_test", map[string]interface{}{})
	if status.Ticket != "ticket_export_test" {
		t.Fatalf("ticket = %q, want %q", status.Ticket, "ticket_export_test")
	}
	if status.FileToken != "" {
		t.Fatalf("file token = %q, want empty", status.FileToken)
	}
}

func TestSanitizeExportFileNameAndEnsureExtension(t *testing.T) {
	t.Parallel()

	if got := sanitizeExportFileName("../quarterly:report?.pdf", "fallback.bin"); got != "quarterly_report_.pdf" {
		t.Fatalf("sanitizeExportFileName() = %q, want %q", got, "quarterly_report_.pdf")
	}
	if got := ensureExportFileExtension("meeting-notes", "markdown"); got != "meeting-notes.md" {
		t.Fatalf("ensureExportFileExtension() = %q, want %q", got, "meeting-notes.md")
	}
	if got := ensureExportFileExtension("report.pdf", "pdf"); got != "report.pdf" {
		t.Fatalf("ensureExportFileExtension() should preserve suffix, got %q", got)
	}
	if got := ensureExportFileExtension("crm", "base"); got != "crm.base" {
		t.Fatalf("ensureExportFileExtension() = %q, want %q", got, "crm.base")
	}
	if got := exportFileSuffix("base"); got != ".base" {
		t.Fatalf("exportFileSuffix(base) = %q, want %q", got, ".base")
	}
}
