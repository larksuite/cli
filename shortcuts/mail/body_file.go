// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"io"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

// bodyFileFlag is the shared `--body-file` flag declaration reused by every
// compose shortcut (+send / +draft-create / +reply / +reply-all / +forward).
// All six shortcuts honour the same mutual-exclusion contract with `--body`
// and the cwd-subtree path safety rule. The flag is intentionally NOT
// shared with `+lint-html` because that command's description differs
// ("HTML to lint" vs "email body") in a way that is more readable when
// authored per-shortcut. `+draft-edit` does not expose `--body-file` either
// — its body ops flow through `--patch-file` JSON whose `value` field is
// the natural file-based entry point for large bodies.
var bodyFileFlag = common.Flag{
	Name:  "body-file",
	Desc:  "Path (relative, within cwd subtree) to a file containing the email body HTML. Mutually exclusive with --body. Size capped at 32 MB.",
	Input: []string{common.File},
}

// maxBodyFileSize caps the size of a `--body-file` HTML input. The compose
// path's downstream EML limit is 25 MB (helpers.go MAX_EML_BYTES); we allow a
// bit more headroom here (32 MB) so a body close to the limit still loads
// before the downstream check fires with a clearer error message. The cap
// prevents an `io.ReadAll` from blowing memory on a misdirected gigabyte
// file.
const maxBodyFileSize = 32 * 1024 * 1024 // 32 MB

// validateBodyFileMutex enforces the `--body` / `--body-file` mutual
// exclusion + cwd-subtree path safety. Compose shortcuts call this in
// their Validate phase so AI / users see a clear error before any work
// runs. Pass the shortcut's RuntimeContext-resolved flag values directly:
// `bodyFlag` is the `--body` value (may be empty), `bodyFile` is the
// trimmed `--body-file` value, and `validatePath` is the
// runtime.ValidatePath bound function used to enforce the relative-path
// rule (cwd-subtree only; no absolute / `..` traversal).
//
// Returns an ErrValidation error when either invariant is violated, nil
// otherwise. The "exactly one of {--body, --body-file}" check is
// shortcut-specific (some shortcuts allow neither, e.g. `+forward` with
// no explicit body) and is therefore left to the caller.
func validateBodyFileMutex(bodyFlag, bodyFile string, validatePath func(string) error) error {
	bodyEmpty := strings.TrimSpace(bodyFlag) == ""
	if !bodyEmpty && bodyFile != "" {
		return mailValidationError("--body and --body-file are mutually exclusive; pass exactly one").
			WithParams(
				mailInvalidParam("--body", "mutually exclusive with --body-file"),
				mailInvalidParam("--body-file", "mutually exclusive with --body"),
			)
	}
	if bodyFile != "" {
		if err := validatePath(bodyFile); err != nil {
			return mailValidationParamError("--body-file", "--body-file: %v", err).WithCause(err)
		}
	}
	return nil
}

// resolveBodyFromFlags returns the body content from --body or --body-file.
// Validate has already enforced mutual exclusion via validateBodyFileMutex,
// so exactly one is set (or neither when a template / parent message
// supplies the body). Returns ("", nil) when neither flag is set so
// downstream code can decide whether the empty body is allowed.
func resolveBodyFromFlags(runtime *common.RuntimeContext) (string, error) {
	if body := runtime.Str("body"); strings.TrimSpace(body) != "" {
		return body, nil
	}
	path := strings.TrimSpace(runtime.Str("body-file"))
	if path == "" {
		return "", nil
	}
	return readBodyFile(runtime.FileIO(), path)
}

func validateRequiredResolvedBody(body string, hasTemplate bool, message string) error {
	if !hasTemplate && strings.TrimSpace(body) == "" {
		return mailValidationError("%s", message)
	}
	return nil
}

// readBodyFile loads --body-file content with a size cap. Returns an
// ErrValidation error if the file exceeds maxBodyFileSize or any IO error
// occurs. The size check uses io.LimitReader(maxBodyFileSize+1) so any
// over-cap byte is observable without reading the whole file.
//
// Callers MUST have run runtime.ValidatePath(path) on `path` first — the
// helper only opens the file via the supplied FileIO and does not repeat
// the cwd-subtree safety check.
func readBodyFile(fio fileio.FileIO, path string) (string, error) {
	f, err := fio.Open(path)
	if err != nil {
		return "", mailValidationParamError("--body-file", "open --body-file %s: %v", path, err).WithCause(mailInputStatError(err))
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, maxBodyFileSize+1))
	if err != nil {
		return "", mailValidationParamError("--body-file", "read --body-file %s: %v", path, err).WithCause(err)
	}
	if len(buf) > maxBodyFileSize {
		return "", mailValidationParamError("--body-file", "--body-file: file exceeds %d MB limit", maxBodyFileSize/1024/1024)
	}
	return string(buf), nil
}
