// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

// The Lark business domains a command set may extend. The list is maintained by
// hand rather than generated from the shortcut registry: a domain exists once
// the CLI publishes it under `lark-cli --help`, which includes domains served
// only by typed and raw API commands. Generating from shortcuts.AllShortcuts
// would silently drop those. TestDomainConstantsCoverEveryService in
// internal/commandhost guards this list against the service registry.
const (
	// DomainApplication is the Application domain.
	DomainApplication DomainName = "application"
	// DomainApproval is the Approval domain.
	DomainApproval DomainName = "approval"
	// DomainApps is the Apps domain.
	DomainApps DomainName = "apps"
	// DomainAttendance is the Attendance domain.
	DomainAttendance DomainName = "attendance"
	// DomainBase is the Base domain.
	DomainBase DomainName = "base"
	// DomainCalendar is the Calendar domain.
	DomainCalendar DomainName = "calendar"
	// DomainContact is the Contacts domain.
	DomainContact DomainName = "contact"
	// DomainDocs is the Docs domain.
	DomainDocs DomainName = "docs"
	// DomainDrive is the Drive domain.
	DomainDrive DomainName = "drive"
	// DomainEvent is the Event domain.
	DomainEvent DomainName = "event"
	// DomainIm is the Messenger domain.
	DomainIm DomainName = "im"
	// DomainMail is the Mail domain.
	DomainMail DomainName = "mail"
	// DomainMarkdown is the Markdown domain.
	DomainMarkdown DomainName = "markdown"
	// DomainMindnotes is the Mindnote domain.
	DomainMindnotes DomainName = "mindnotes"
	// DomainMinutes is the Minutes domain.
	DomainMinutes DomainName = "minutes"
	// DomainNote is the Note domain.
	DomainNote DomainName = "note"
	// DomainOkr is the OKR domain.
	DomainOkr DomainName = "okr"
	// DomainSheets is the Sheets domain.
	DomainSheets DomainName = "sheets"
	// DomainSlides is the Slides domain.
	DomainSlides DomainName = "slides"
	// DomainTask is the Task domain.
	DomainTask DomainName = "task"
	// DomainVc is the VC domain.
	DomainVc DomainName = "vc"
	// DomainWhiteboard is the Whiteboard domain.
	DomainWhiteboard DomainName = "whiteboard"
	// DomainWiki is the Wiki domain.
	DomainWiki DomainName = "wiki"
)
