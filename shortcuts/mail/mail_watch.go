// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/larksuite/cli/errs"
	eventmail "github.com/larksuite/cli/events/mail"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/event/consume"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

type mailWatchLogger struct {
	w io.Writer
}

func (l *mailWatchLogger) Debug(_ context.Context, _ ...interface{}) {}
func (l *mailWatchLogger) Info(_ context.Context, args ...interface{}) {
	fmt.Fprintln(l.w, append([]interface{}{"[SDK Info]"}, args...)...)
}

type mailWatchEnvelopeWriter struct {
	out      io.Writer
	identity string
}

func (w *mailWatchEnvelopeWriter) Write(p []byte) (int, error) {
	trimmed := bytes.TrimSpace(p)
	if len(trimmed) == 0 {
		return len(p), nil
	}
	output.PrintNdjson(w.out, output.Envelope{
		OK:       true,
		Identity: w.identity,
		Data:     json.RawMessage(trimmed),
	})
	return len(p), nil
}

// handleMailWatchSignal processes a shutdown signal: logs status, unsubscribes
// mailbox events, restores default signal behavior for forced termination, and
// cancels the watch context.
func handleMailWatchSignal(errOut io.Writer, sig os.Signal, eventCount int64, unsubscribeWithLog func(), stopSignals func(), cancel context.CancelFunc) {
	fmt.Fprintf(errOut, "\nShutting down (signal: %v)... (received %d events)\n", sig, eventCount)
	// Restore default signal behavior so a second Ctrl+C can force terminate.
	stopSignals()
	signal.Reset(os.Interrupt, syscall.SIGTERM)
	unsubscribeWithLog()
	cancel()
}

const mailEventType = "mail.user_mailbox.event.message_received_v1"

// promptInjectionPatterns lists known prompt injection trigger phrases.
var promptInjectionPatterns = []string{
	"ignore all previous",
	"ignore previous instructions",
	"disregard",
	"you are now",
	"system prompt",
	"jailbreak",
	"act as if",
	"new instructions",
}

// detectPromptInjection reports whether content contains known prompt injection patterns.
// Content is normalized first to strip zero-width characters and other dangerous Unicode
// that could be used to bypass keyword matching (e.g. U+200B inserted inside a phrase).
func detectPromptInjection(content string) bool {
	normalized := strings.ToLower(sanitizeForTerminal(content))
	for _, p := range promptInjectionPatterns {
		if strings.Contains(normalized, p) {
			return true
		}
	}
	return false
}

var MailWatch = common.Shortcut{
	Service:     "mail",
	Command:     "+watch",
	Description: "Watch for incoming mail events via the unified event framework (requires scope mail:event and event mail.user_mailbox.event.message_received_v1 enabled). Run with --print-output-schema to see per-format field reference before parsing output.",
	Risk:        "read",
	Scopes:      []string{"mail:event", "mail:user_mailbox.event.mail_address:read", "mail:user_mailbox:readonly", "mail:user_mailbox.message:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "format", Default: "data", Desc: "json: NDJSON stream with ok/data envelope; data: bare NDJSON stream"},
		{Name: "msg-format", Default: "metadata", Desc: "message payload mode: metadata(headers + meta, for triage/notification) | minimal(IDs and state only, no headers, for tracking read/folder changes) | plain_text_full(all metadata fields + full plain-text body) | event(raw event envelope, no API call, for debug) | full(full message including HTML body and attachments)"},
		{Name: "output-dir", Desc: "Write each message as a JSON file (always full payload, regardless of --msg-format)"},
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "labels", Desc: "filter: label names JSON array, e.g. [\"important\",\"team-label\"]"},
		{Name: "folders", Desc: "filter: folder names JSON array, e.g. [\"inbox\",\"news\"]"},
		{Name: "label-ids", Desc: "filter: label IDs JSON array, e.g. [\"FLAGGED\",\"IMPORTANT\"]"},
		{Name: "folder-ids", Desc: "filter: folder IDs JSON array, e.g. [\"INBOX\",\"SENT\"]"},
		{Name: "max-events", Type: "int", Desc: "Exit after N successful emits (0 = unlimited). Non-TTY stdin EOF also stops the run."},
		{Name: "timeout", Default: "0", Desc: "Exit after DURATION (e.g. 30s, 2m). 0 = no timeout. Non-TTY stdin EOF exits earlier with reason: signal."},
		{Name: "print-output-schema", Type: "bool", Desc: "Print output field reference per --msg-format (run this first to learn field names before parsing output)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailbox := resolveMailboxID(runtime)
		msgFormat := runtime.Str("msg-format")
		labelIDsInput := runtime.Str("label-ids")
		folderIDsInput := runtime.Str("folder-ids")
		labelsInput := runtime.Str("labels")
		foldersInput := runtime.Str("folders")
		outputDir := runtime.Str("output-dir")

		resolvedFolderIDs, folderDeferred := resolveWatchFilterIDsForDryRun(folderIDsInput, foldersInput, false, resolveFolderSystemAliasOrID)
		resolvedLabelIDs, labelDeferred := resolveWatchFilterIDsForDryRun(labelIDsInput, labelsInput, false, resolveLabelSystemID)

		outputDirDisplay := "(stdout)"
		if outputDir != "" {
			outputDirDisplay = outputDir
		}
		effectiveFolderDisplay := strings.Join(resolvedFolderIDs, ",")
		if effectiveFolderDisplay == "" {
			effectiveFolderDisplay = "(none)"
		}
		effectiveLabelDisplay := strings.Join(resolvedLabelIDs, ",")
		if effectiveLabelDisplay == "" {
			effectiveLabelDisplay = "(none)"
		}

		dryRunDesc := "Step 1: subscribe mailbox events; Step 2: consume via unified event framework (long-running)"
		if folderDeferred || labelDeferred {
			dryRunDesc += "; non-system folder/label names are resolved to IDs during execution"
		}
		d := common.NewDryRunAPI().
			Desc(dryRunDesc).
			Set("command", "mail +watch").
			Set("app_id", runtime.Config.AppID).
			Set("msg_format", msgFormat).
			Set("output_dir", outputDirDisplay).
			Set("input_folder_ids", folderIDsInput).
			Set("input_folders", foldersInput).
			Set("input_label_ids", labelIDsInput).
			Set("input_labels", labelsInput).
			Set("effective_folder_ids", resolvedFolderIDs).
			Set("effective_label_ids", resolvedLabelIDs)

		d.POST(mailboxPath(mailbox, "event", "subscribe")).
			Desc(fmt.Sprintf("Subscribe mailbox events (effective_folder_ids=%s, effective_label_ids=%s)", effectiveFolderDisplay, effectiveLabelDisplay)).
			Body(map[string]interface{}{"event_type": 1})

		if mailbox == "me" {
			d.GET(mailboxPath("me", "profile")).
				Desc("Resolve mailbox address for event filtering (requires scope mail:user_mailbox:readonly)")
		}

		if len(resolvedLabelIDs) > 0 {
			d.Set("filter_label_ids", strings.Join(resolvedLabelIDs, ","))
		}
		if len(resolvedFolderIDs) > 0 {
			d.Set("filter_folder_ids", strings.Join(resolvedFolderIDs, ","))
		}
		// When outputting message payload (or when label/folder filtering is enabled),
		// +watch will fetch message details by message_id.
		if msgFormat != "event" || len(resolvedLabelIDs) > 0 || len(resolvedFolderIDs) > 0 {
			params := map[string]interface{}{
				"format": watchFetchFormat(msgFormat, len(resolvedLabelIDs) > 0 || len(resolvedFolderIDs) > 0),
			}
			d.GET(mailboxPath(mailbox, "messages", "{message_id}")).
				Params(params)
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-output-schema") {
			printWatchOutputSchema(runtime)
			return nil
		}
		mailbox := resolveMailboxID(runtime)
		hintIdentityFirst(runtime, mailbox)
		outFormat := runtime.Str("format")
		switch outFormat {
		case "json", "data", "":
		default:
			return mailValidationParamError("--format", "invalid --format %q: must be json or data", outFormat)
		}
		msgFormat := runtime.Str("msg-format")
		outputDir := runtime.Str("output-dir")
		if outputDir != "" {
			// Reject all tilde-prefixed paths — SafeOutputPath treats "~/x" as a
			// literal relative path (creating a directory named "~"), which is
			// confusing. This also covers ~user/path forms.
			if strings.HasPrefix(outputDir, "~") {
				return mailValidationParamError("--output-dir", "--output-dir does not support ~ expansion; use a relative path like ./output instead")
			}
			// Enforce CWD containment: reject absolute paths, path traversal,
			// and symlink escapes. SafeOutputPath returns a resolved absolute path
			// under CWD, preventing writes to arbitrary system directories.
			safePath, err := validate.SafeOutputPath(outputDir)
			if err != nil {
				return mailValidationParamError("--output-dir", "invalid --output-dir %q: %v", outputDir, err).WithCause(err)
			}
			outputDir = safePath
			if err := vfs.MkdirAll(outputDir, 0700); err != nil {
				return mailFileIOError("cannot create output directory %q: %v", err, outputDir, err)
			}
		}
		params := map[string]string{
			"mailbox":    mailbox,
			"msg_format": msgFormat,
			"folder_ids": runtime.Str("folder-ids"),
			"folders":    runtime.Str("folders"),
			"label_ids":  runtime.Str("label-ids"),
			"labels":     runtime.Str("labels"),
		}
		if outputDir != "" {
			params["msg_format"] = "full"
		}
		consumeOut := runtime.IO().Out
		if outFormat == "json" || outFormat == "" {
			consumeOut = &mailWatchEnvelopeWriter{out: runtime.IO().Out, identity: string(runtime.As())}
		}
		timeout, err := parseMailWatchTimeout(runtime.Str("timeout"))
		if err != nil {
			return err
		}
		opts := mailWatchConsumeOptions(runtime, params, outputDir, consumeOut, timeout, mailWatchStdinClosed(runtime))
		return consume.Run(ctx, transport.New(), runtime.Config.AppID, runtime.Config.ProfileName, core.ResolveEndpoints(runtime.Config.Brand).Open, opts)
	},
}

func parseMailWatchTimeout(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, mailValidationParamError("--timeout", "invalid --timeout %q: expected duration like 30s or 2m", value).WithCause(err)
	}
	return timeout, nil
}

func mailWatchStdinClosed(runtime *common.RuntimeContext) <-chan struct{} {
	streams := runtime.IO()
	if streams == nil || streams.IsTerminal || streams.In == nil {
		return nil
	}
	stdinClosed := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, streams.In)
		fmt.Fprintln(streams.ErrOut, "[event] stdin closed — shutting down. "+
			"mail +watch treats stdin EOF as exit signal in non-TTY mode. "+
			"To keep running until --max-events/--timeout/SIGTERM: keep stdin open "+
			"(e.g. `< /dev/tty` interactive, `< <(tail -f /dev/null)` script), "+
			"or stop via SIGTERM instead of closing stdin.")
		close(stdinClosed)
	}()
	return stdinClosed
}

func mailWatchConsumeOptions(runtime *common.RuntimeContext, params map[string]string, outputDir string, consumeOut io.Writer, timeout time.Duration, stdinClosed <-chan struct{}) consume.Options {
	apiRuntime := eventmail.NewRuntime(runtime)
	return consume.Options{
		EventKey:        mailEventType,
		Params:          params,
		Quiet:           false,
		OutputDir:       outputDir,
		Runtime:         apiRuntime,
		Out:             consumeOut,
		ErrOut:          runtime.IO().ErrOut,
		RemoteAPIClient: apiRuntime,
		MaxEvents:       runtime.Int("max-events"),
		Timeout:         timeout,
		IsTTY:           runtime.IO().IsTerminal,
		StdinClosed:     stdinClosed,
	}
}

// extractMailEventBody extracts the event body from the Lark event envelope.
func extractMailEventBody(data map[string]interface{}) map[string]interface{} {
	// V2 envelope: { header: {...}, event: { mail_address, message_id, ... } }
	if event, ok := data["event"].(map[string]interface{}); ok {
		return event
	}
	return data
}

func parseJSONArrayFlag(input, flagName string) ([]string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		return nil, mailValidationParamError("--"+flagName, "invalid --%s: expected JSON array of strings, e.g. [\"INBOX\",\"SENT\"]", flagName).WithCause(err)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

func parseJSONArrayFlagLoose(input string) []string {
	values, err := parseJSONArrayFlag(input, "")
	if err != nil {
		return nil
	}
	return values
}

func mergeIDSet(set map[string]bool, ids []string) map[string]bool {
	if len(ids) == 0 {
		return set
	}
	if set == nil {
		set = make(map[string]bool, len(ids))
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		set[id] = true
	}
	return set
}

func setKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func resolveWatchFilterIDsForDryRun(explicitIDsInput, namesInput string, namesCanUseSystemIDs bool, systemResolver func(string) (string, bool)) ([]string, bool) {
	explicitIDs := parseJSONArrayFlagLoose(explicitIDsInput)
	names := parseJSONArrayFlagLoose(namesInput)
	set := make(map[string]bool)
	for _, raw := range explicitIDs {
		if id, ok := systemResolver(raw); ok {
			set[id] = true
			continue
		}
		set[raw] = true
	}
	deferred := false
	for _, raw := range names {
		if namesCanUseSystemIDs {
			if id, ok := systemResolver(raw); ok {
				set[id] = true
				continue
			}
		}
		if strings.TrimSpace(raw) != "" {
			deferred = true
		}
	}
	return setKeys(set), deferred
}

func resolveWatchNames(
	runtime *common.RuntimeContext,
	mailboxID, input, flagName string,
	resolveNames func(*common.RuntimeContext, string, []string) ([]string, error),
	systemResolver func(string) (string, bool),
) ([]string, error) {
	names, err := parseJSONArrayFlag(input, flagName)
	if err != nil {
		return nil, err
	}
	resolvedNames := make([]string, 0, len(names))
	for _, raw := range names {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if id, ok := systemResolver(value); ok {
			resolvedNames = append(resolvedNames, id)
			continue
		}
	}
	remainingNames := make([]string, 0, len(names))
	for _, raw := range names {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, ok := systemResolver(value); ok {
			continue
		}
		remainingNames = append(remainingNames, value)
	}
	rest, err := resolveNames(runtime, mailboxID, remainingNames)
	if err != nil {
		return nil, err
	}
	return append(resolvedNames, rest...), nil
}

func resolveWatchFilterIDs(
	runtime *common.RuntimeContext,
	mailboxID, explicitIDsInput, namesInput string,
	resolveExplicitID func(*common.RuntimeContext, string, string) (string, error),
	resolveNames func(*common.RuntimeContext, string, []string) ([]string, error),
	systemResolver func(string) (string, bool),
	explicitFlagName, namesFlagName, kind string,
) ([]string, error) {
	explicitIDs, err := parseJSONArrayFlag(explicitIDsInput, explicitFlagName)
	if err != nil {
		return nil, err
	}
	resolvedNames, err := resolveWatchNames(runtime, mailboxID, namesInput, namesFlagName, resolveNames, systemResolver)
	if err != nil {
		return nil, err
	}

	set := make(map[string]bool)
	for _, raw := range explicitIDs {
		resolved := strings.TrimSpace(raw)
		if id, ok := systemResolver(resolved); ok {
			set[id] = true
			continue
		}
		var err error
		resolved, err = resolveExplicitID(runtime, mailboxID, resolved)
		if err != nil {
			return nil, err
		}
		if resolved != "" {
			set[resolved] = true
		}
	}
	return setKeys(mergeIDSet(set, resolvedNames)), nil
}

func watchFetchFormat(msgFormat string, forceMetadata bool) string {
	if forceMetadata && msgFormat == "event" {
		return "metadata"
	}
	switch msgFormat {
	case "metadata", "plain_text_full", "full":
		return msgFormat
	case "minimal":
		return "metadata"
	default:
		return "metadata"
	}
}

func minimalWatchMessage(message map[string]interface{}) map[string]interface{} {
	if message == nil {
		return nil
	}
	out := make(map[string]interface{}, 6)
	for _, key := range []string{"message_id", "thread_id", "folder_id", "label_ids", "internal_date", "message_state"} {
		if value, ok := message[key]; ok {
			out[key] = value
		}
	}
	return out
}

func watchFetchFailureValue(messageID, fetchFormat string, err error, eventBody map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"ok": false,
		"error": map[string]interface{}{
			"type":       "fetch_message_failed",
			"message_id": messageID,
			"format":     fetchFormat,
			"message":    err.Error(),
		},
	}
	if len(eventBody) > 0 {
		payload["event"] = eventBody
	}
	return payload
}

// messageHasLabel checks if a message metadata map contains any of the given label IDs.
func messageHasLabel(meta map[string]interface{}, labelIDSet map[string]bool) bool {
	labels, _ := meta["label_ids"].([]interface{})
	for _, l := range labels {
		if id, ok := l.(string); ok && labelIDSet[id] {
			return true
		}
	}
	return false
}

func wrapWatchSubscribeError(err error) error {
	if err == nil {
		return nil
	}
	hint := "ensure the app has scope mail:event and the event mail.user_mailbox.event.message_received_v1 is enabled"
	if p, ok := errs.ProblemOf(err); ok {
		p.Message = "subscribe mailbox events failed: " + p.Message
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint = p.Hint + "; " + hint
		} else {
			p.Hint = hint
		}
		return err
	}
	return errs.NewAPIError(errs.SubtypeUnknown, "subscribe mailbox events failed: %v", err).WithHint("%s", hint).WithCause(err)
}

// enhanceProfileError wraps a profile API error with actionable hints.
// Permission errors get a scope-specific hint; other errors (network, 5xx)
// are reported as-is so diagnostics aren't misleading.
func enhanceProfileError(err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		lower := strings.ToLower(p.Message)
		if p.Category == errs.CategoryAuthorization {
			p.Message = "unable to resolve mailbox address: " + p.Message
			p.Hint = "run `lark-cli auth login --scope \"mail:user_mailbox:readonly\"` to grant mailbox profile access"
			return err
		}
		if strings.Contains(lower, "permission") || strings.Contains(lower, "scope") {
			permErr := errs.NewPermissionError(errs.SubtypeMissingScope, "unable to resolve mailbox address: %s", p.Message).
				WithHint("run `lark-cli auth login --scope \"mail:user_mailbox:readonly\"` to grant mailbox profile access").
				WithCause(err)
			if p.Code != 0 {
				permErr = permErr.WithCode(p.Code)
			}
			if p.LogID != "" {
				permErr = permErr.WithLogID(p.LogID)
			}
			return permErr
		}
	}
	// Preserve original error (and its exit code) for non-permission failures.
	return err
}

// decodeBodyFieldsForFile returns a shallow copy of outputData with body_html and
// body_plain_text decoded from base64url, so that files saved via --output-dir contain
// human-readable content instead of raw base64 strings.
// It handles both a top-level message map and a {"message": {...}} wrapper.
func decodeBodyFieldsForFile(data interface{}) interface{} {
	m, ok := data.(map[string]interface{})
	if !ok {
		return data
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	decodeBodyFields(out, out)
	if msg, ok := out["message"].(map[string]interface{}); ok {
		decoded := make(map[string]interface{}, len(msg))
		for k, v := range msg {
			decoded[k] = v
		}
		decodeBodyFields(decoded, decoded)
		out["message"] = decoded
	}
	return out
}
