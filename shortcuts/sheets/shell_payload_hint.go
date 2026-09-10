// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	goruntime "runtime"
)

// ─── shell-correct payload prescriptions ──────────────────────────────
//
// Every composite flag in this domain takes its payload three ways: inline,
// as a relative @file, or on stdin. Which of the three a prescription should
// name depends on the shell the caller is in, and getting that wrong costs
// the same round trip the prescription was written to save.
//
// PowerShell has no input redirection: `<` is a reserved operator, so
// `--cells - < cells.json` fails with "The '<' operator is reserved for
// future use" — an error about the shell, on a line the CLI told the caller
// to run. It also splits an argument on the quotes and commas inside a JSON
// literal, which single quotes do not prevent, so inlining a payload there is
// not a style choice but a defect: 08-29..31 reflow, 89 rejections whose
// message was some flavor of "invalid JSON: invalid character '待'" or
// "positional arguments are not supported", every one of them on windows.
//
// The pipe is not the answer there either: PowerShell 5 re-encodes non-ASCII
// bytes on the way through, which turns a CJK payload into the same invalid
// JSON by a different route (the windows-compat skill reference rules both
// out). So on windows every prescription lands on @file, quoted — `@` starts
// a splatting expression unquoted — and stdin is named only where the shell
// actually has it.

// payloadFileForm is quoted on windows: bare `@` opens a splatting
// expression there, so the shell rejects the argument before the CLI sees it.
func payloadFileFormFor(flag, goos string) string {
	if goos == "windows" {
		return fmt.Sprintf("--%s \"@./payload.json\"", flag)
	}
	return fmt.Sprintf("--%s @./payload.json", flag)
}

// payloadStdinFormFor spells "read this flag from stdin" for the host shell,
// and reports "" where the shell has no usable form.
func payloadStdinFormFor(flag, goos string) string {
	if goos == "windows" {
		return "" // no redirection operator, and the pipe re-encodes
	}
	return fmt.Sprintf("--%s - < payload.json", flag)
}

// mangledPayloadHint prescribes the fix for a payload the shell damaged
// before the CLI saw it. On windows it also names the cause, because the
// caller quoted the argument and has no reason to suspect the quoting.
func mangledPayloadHint(flag string) string { return mangledPayloadHintFor(flag, goruntime.GOOS) }

func mangledPayloadHintFor(flag, goos string) string {
	if goos == "windows" {
		return fmt.Sprintf(
			"PowerShell splits a JSON argument on the quotes and commas inside it, and single quotes do not prevent that — never inline JSON on this shell. Write the payload to a file and pass %s; the quotes are required because bare @ starts a splatting expression. Do not pipe it in either: PowerShell 5 re-encodes non-ASCII on the way through",
			payloadFileFormFor(flag, goos))
	}
	return fmt.Sprintf(
		"if the payload contains formulas / quotes / commas, pass it as a relative @file (%s) or on stdin (%s) so the shell cannot mangle it",
		payloadFileFormFor(flag, goos), payloadStdinFormFor(flag, goos))
}

// outOfTreeFileHint prescribes how to read a file the @file policy will not
// take, in the host shell's spelling.
func outOfTreeFileHint(flag string) string { return outOfTreeFileHintFor(flag, goruntime.GOOS) }

func outOfTreeFileHintFor(flag, goos string) string {
	if goos == "windows" {
		// Neither redirection nor the pipe is usable here, so the only route
		// left is to make the file's directory the working directory.
		return fmt.Sprintf("cd to the file's directory first, then name it relatively: --%s \"@./<name>\"", flag)
	}
	return fmt.Sprintf("pipe it in instead: --%s - < <path>", flag)
}
