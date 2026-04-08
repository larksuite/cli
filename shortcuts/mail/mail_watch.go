// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

type mailWatchLogger struct {
	w io.Writer
}

func (l *mailWatchLogger) Debug(_ context.Context, _ ...interface{}) {}
func (l *mailWatchLogger) Info(_ context.Context, args ...interface{}) {
	fmt.Fprintln(l.w, append([]interface{}{"[SDK Info]"}, args...)...)
}
func (l *mailWatchLogger) Warn(_ context.Context, args ...interface{}) {
	fmt.Fprintln(l.w, append([]interface{}{"[SDK Warn]"}, args...)...)
}
func (l *mailWatchLogger) Error(_ context.Context, args ...interface{}) {
	fmt.Fprintln(l.w, append([]interface{}{"[SDK Error]"}, args...)...)
}

var _ larkcore.Logger = (*mailWatchLogger)(nil)

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
	Description: "Watch for incoming mail events via WebSocket (requires scope mail:event and bot event mail.user_mailbox.event.message_received_v1 added). Run with --print-output-schema to see per-format field reference before parsing output.",
	Risk:        "read",
	Scopes:      []string{"mail:event", "mail:user_mailbox.message:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "format", Default: "data", Desc: "json: NDJSON stream with ok/data envelope; data: bare NDJSON stream"},
		{Name: "msg-format", Default: "metadata", Desc: "message payload mode: metadata(headers + meta, for triage/notification) | minimal(IDs and state only, no headers, for tracking read/folder changes) | plain_text_full(all metadata fields + full plain-text body) | event(raw WebSocket event, no API call, for debug) | full(full message including HTML body and attachments)"},
		{Name: "output-dir", Desc: "Write each message as a JSON file (always full payload, regardless of --msg-format)"},
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "labels", Desc: "filter: label names JSON array, e.g. [\"important\",\"team-label\"]"},
		{Name: "folders", Desc: "filter: folder names JSON array, e.g. [\"inbox\",\"news\"]"},
		{Name: "label-ids", Desc: "filter: label IDs JSON array, e.g. [\"FLAGGED\",\"IMPORTANT\"]"},
		{Name: "folder-ids", Desc: "filter: folder IDs JSON array, e.g. [\"INBOX\",\"SENT\"]"},
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

		dryRunDesc := "Step 1: subscribe mailbox events; Step 2: watch via WebSocket (long-running)"
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
			return output.ErrValidation("invalid --format %q: must be json or data", outFormat)
		}
		msgFormat := runtime.Str("msg-format")
		outputDir := runtime.Str("output-dir")
		if outputDir != "" {
			if outputDir == "~" || strings.HasPrefix(outputDir, "~/") {
				home, err := vfs.UserHomeDir()
				if err != nil {
					return fmt.Errorf("cannot expand ~: %w", err)
				}
				if outputDir == "~" {
					outputDir = home
				} else {
					outputDir = filepath.Join(home, outputDir[2:])
				}
			} else if filepath.IsAbs(outputDir) {
				outputDir = filepath.Clean(outputDir)
			} else {
				safePath, err := validate.SafeOutputPath(outputDir)
				if err != nil {
					return err
				}
				outputDir = safePath
			}
			// Resolve symlinks on the output directory so all writes use the real
			// filesystem path. This prevents a symlink from redirecting writes to
			// an unintended location (TOCTOU mitigation).
			if err := vfs.MkdirAll(outputDir, 0700); err != nil {
				return fmt.Errorf("cannot create output directory %q: %w", outputDir, err)
			}
			resolved, err := filepath.EvalSymlinks(outputDir)
			if err != nil {
				return fmt.Errorf("cannot resolve output directory: %w", err)
			}
			outputDir = resolved
		}
		labelIDsInput := runtime.Str("label-ids")
		folderIDsInput := runtime.Str("folder-ids")
		labelsInput := runtime.Str("labels")
		foldersInput := runtime.Str("folders")

		errOut := runtime.IO().ErrOut
		out := runtime.IO().Out

		info := func(msg string) {
			fmt.Fprintln(errOut, msg)
		}

		// Resolve --labels / --folders strictly as names, and --label-ids / --folder-ids strictly as IDs.
		resolvedLabelIDs, err := resolveWatchFilterIDs(runtime, mailbox, labelIDsInput, labelsInput, resolveLabelID, resolveLabelNames, resolveLabelSystemID, "label-ids", "labels", "label")
		if err != nil {
			return err
		}
		resolvedFolderIDs, err := resolveWatchFilterIDs(runtime, mailbox, folderIDsInput, foldersInput, resolveFolderID, resolveFolderNames, resolveFolderSystemAliasOrID, "folder-ids", "folders", "folder")
		if err != nil {
			return err
		}
		labelIDSet := make(map[string]bool, len(resolvedLabelIDs))
		for _, id := range resolvedLabelIDs {
			if id != "" {
				labelIDSet[id] = true
			}
		}
		folderIDSet := make(map[string]bool, len(resolvedFolderIDs))
		for _, id := range resolvedFolderIDs {
			if id != "" {
				folderIDSet[id] = true
			}
		}

		// Step 1: subscribe mailbox events (required before WebSocket pushes mail events)
		info(fmt.Sprintf("Subscribing mailbox events for: %s", mailbox))
		_, err = runtime.CallAPI("POST", mailboxPath(mailbox, "event", "subscribe"), nil, map[string]interface{}{"event_type": 1})
		if err != nil {
			return wrapWatchSubscribeError(err)
		}
		info("Mailbox subscribed.")

		var unsubOnce sync.Once
		var unsubErr error
		unsubscribe := func() error {
			unsubOnce.Do(func() {
				_, unsubErr = runtime.CallAPI("POST", mailboxPath(mailbox, "event", "unsubscribe"), nil, map[string]interface{}{"event_type": 1})
			})
			return unsubErr
		}

		// Resolve "me" to the actual email address so we can filter events.
		mailboxFilter := mailbox
		if mailbox == "me" {
			resolved, profileErr := fetchMailboxPrimaryEmail(runtime, "me")
			if profileErr != nil {
				unsubscribe() //nolint:errcheck // best-effort cleanup; primary error is profileErr
				return enhanceProfileError(profileErr)
			}
			mailboxFilter = resolved
		}

		eventCount := 0

		handleEvent := func(data map[string]interface{}) {
			// Extract event body
			eventBody := extractMailEventBody(data)

			// Filter by --mailbox
			if mailboxFilter != "" {
				mailAddr, _ := eventBody["mail_address"].(string)
				if !strings.EqualFold(mailAddr, mailboxFilter) {
					return
				}
			}

			messageID, _ := eventBody["message_id"].(string)
			if messageID == "" {
				return
			}

			// Use event's mail_address as the fetch mailbox when available,
			// because "me" only resolves to the current user's mailbox but
			// WebSocket events may arrive for other mailboxes the app can access.
			fetchMailbox := mailbox
			if eventAddr, _ := eventBody["mail_address"].(string); eventAddr != "" {
				fetchMailbox = eventAddr
			}

			// Fetch message payload when needed:
			// 1) msg-format != event (output message/meta)
			// 2) label/folder filtering is enabled
			// 3) output-dir is set (always fetch full for file writing)
			needMessage := msgFormat != "event" || len(labelIDSet) > 0 || len(folderIDSet) > 0 || outputDir != ""
			var message map[string]interface{}
			if needMessage {
				var err error
				fetchFormat := watchFetchFormat(msgFormat, len(labelIDSet) > 0 || len(folderIDSet) > 0)
				if outputDir != "" {
					fetchFormat = "full"
				}
				message, err = fetchMessageForWatch(runtime, fetchMailbox, messageID, fetchFormat)
				if err != nil {
					output.PrintError(errOut, fmt.Sprintf("fetch message %s failed: %v", fetchFormat, err))
					failureData := watchFetchFailureValue(messageID, fetchFormat, err, eventBody)
					if outputDir != "" {
						if _, writeErr := writeMailEventFile(outputDir, failureData, data); writeErr != nil {
							output.PrintError(errOut, fmt.Sprintf("failed to write event file: %v", writeErr))
						}
					}
					output.PrintJson(out, failureData)
					return
				}
			}

			// Filter by --labels/--folders + --label-ids/--folder-ids resolved ID sets (based on message metadata)
			if len(folderIDSet) > 0 {
				folderID, _ := message["folder_id"].(string)
				if !folderIDSet[folderID] {
					return
				}
			}
			if len(labelIDSet) > 0 {
				if !messageHasLabel(message, labelIDSet) {
					return
				}
			}

			eventCount++

			// Prompt injection detection: warn when email body contains known injection patterns.
			// Body fields may be base64url-encoded; decode before scanning.
			if message != nil {
				for _, field := range []string{"body_plain_text", "body_preview", "body_plain"} {
					if body, ok := message[field].(string); ok && body != "" {
						decoded := decodeBase64URL(body)
						if detectPromptInjection(decoded) {
							from, _ := message["from"].(string)
							fmt.Fprintf(errOut, "[SECURITY WARNING] Possible prompt injection detected in message from %s\n", sanitizeForTerminal(from))
						}
						break
					}
				}
			}

			// Save full message for file writing before any stdout trimming.
			fullMessage := message

			var outputData interface{} = data
			if msgFormat != "event" && message != nil {
				if msgFormat == "minimal" {
					message = minimalWatchMessage(message)
				}
				outputData = map[string]interface{}{"message": message}
			}

			if outputDir != "" {
				_, err := writeMailEventFile(outputDir, decodeBodyFieldsForFile(fullMessage), data)
				if err != nil {
					output.PrintError(errOut, fmt.Sprintf("failed to write event file: %v", err))
				}
			}

			switch outFormat {
			case "json", "":
				output.PrintNdjson(out, output.Envelope{OK: true, Identity: string(runtime.As()), Data: outputData})
			case "data":
				output.PrintNdjson(out, outputData)
			}
		}

		rawHandler := func(ctx context.Context, event *larkevent.EventReq) error {
			var eventData map[string]interface{}
			if event.Body != nil {
				dec := json.NewDecoder(bytes.NewReader(event.Body))
				dec.UseNumber()
				if err := dec.Decode(&eventData); err != nil {
					fmt.Fprintf(errOut, "warning: failed to decode event body: %v\n", err)
				}
			}
			if eventData == nil {
				eventData = make(map[string]interface{})
			}
			handleEvent(eventData)
			return nil
		}

		sdkLogger := &mailWatchLogger{w: errOut}

		eventDispatcher := dispatcher.NewEventDispatcher("", "")
		eventDispatcher.InitConfig(larkevent.WithLogger(sdkLogger))
		eventDispatcher.OnCustomizedEvent(mailEventType, rawHandler)

		endpoints := core.ResolveEndpoints(runtime.Config.Brand)
		domain := endpoints.Open

		info("Connecting to Feishu event WebSocket...")
		info(fmt.Sprintf("Listening for: %s", mailEventType))
		info(fmt.Sprintf("Output mode: %s", msgFormat))
		if mailboxFilter != "" {
			info(fmt.Sprintf("Filter: mailbox=%s", mailboxFilter))
		}
		if len(folderIDSet) > 0 {
			info(fmt.Sprintf("Filter: folder-ids=%s", strings.Join(setKeys(folderIDSet), ",")))
		}
		if len(labelIDSet) > 0 {
			info(fmt.Sprintf("Filter: label-ids=%s", strings.Join(setKeys(labelIDSet), ",")))
		}

		cli := larkws.NewClient(runtime.Config.AppID, runtime.Config.AppSecret,
			larkws.WithEventHandler(eventDispatcher),
			larkws.WithDomain(domain),
			larkws.WithLogger(sdkLogger),
		)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(errOut, "panic in signal handler: %v\n", r)
				}
			}()
			<-sigCh
			info(fmt.Sprintf("\nShutting down... (received %d events)", eventCount))
			info("Unsubscribing mailbox events...")
			if unsubErr := unsubscribe(); unsubErr != nil {
				fmt.Fprintf(errOut, "Warning: unsubscribe failed: %v\n", unsubErr)
			} else {
				info("Mailbox unsubscribed.")
			}
			signal.Stop(sigCh)
			os.Exit(0)
		}()

		info("Connected. Waiting for mail events... (Ctrl+C to stop)")
		if err := cli.Start(ctx); err != nil {
			unsubscribe() //nolint:errcheck // best-effort cleanup
			return output.ErrNetwork("WebSocket connection failed: %v", err)
		}
		return nil
	},
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
		return nil, output.ErrValidation("invalid --%s: expected JSON array of strings, e.g. [\"INBOX\",\"SENT\"]", flagName)
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

// fetchMessageForWatch fetches message payload used by watch output/filtering.
func fetchMessageForWatch(runtime *common.RuntimeContext, mailbox, messageID, format string) (map[string]interface{}, error) {
	queryParams := make(larkcore.QueryParams)
	queryParams.Set("format", format)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     fmt.Sprintf("/open-apis/mail/v1/user_mailboxes/%s/messages/%s", validate.EncodePathSegment(mailbox), validate.EncodePathSegment(messageID)),
		QueryParams: queryParams,
	})
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return nil, err
	}
	if code, _ := result["code"].(float64); code != 0 {
		msg, _ := result["msg"].(string)
		return nil, fmt.Errorf("[%.0f] %s", code, msg)
	}
	data, _ := result["data"].(map[string]interface{})
	msg, _ := data["message"].(map[string]interface{})
	if msg == nil {
		return data, nil
	}
	return msg, nil
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
	if exitErr, ok := err.(*output.ExitError); ok && exitErr.Detail != nil {
		msg := "subscribe mailbox events failed: " + exitErr.Detail.Message
		if exitErr.Detail.Hint != "" {
			hint = exitErr.Detail.Hint + "; " + hint
		}
		return output.ErrWithHint(exitErr.Code, exitErr.Detail.Type, msg, hint)
	}
	return output.ErrWithHint(output.ExitAPI, "api_error", fmt.Sprintf("subscribe mailbox events failed: %v", err), hint)
}

// enhanceProfileError wraps a profile API error with actionable hints.
// Permission errors get a scope-specific hint; other errors (network, 5xx)
// are reported as-is so diagnostics aren't misleading.
func enhanceProfileError(err error) error {
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) && exitErr.Detail != nil {
		errType := exitErr.Detail.Type
		lower := strings.ToLower(exitErr.Detail.Message)
		if errType == "permission" || errType == "missing_scope" ||
			strings.Contains(lower, "permission") || strings.Contains(lower, "scope") {
			return output.ErrWithHint(output.ExitAuth, "missing_scope",
				"unable to resolve mailbox address: "+exitErr.Detail.Message,
				"run `lark-cli auth login --scope \"mail:user_mailbox:readonly\"` to grant mailbox profile access")
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

// writeMailEventFile writes a mail event to a JSON file in outputDir.
// outputDir must be an already-resolved absolute path (symlinks resolved by Execute).
func writeMailEventFile(outputDir string, data interface{}, raw map[string]interface{}) (string, error) {
	sanitizeFilename := func(s string) string {
		safe := regexp.MustCompile(`[^a-zA-Z0-9._\-]+`).ReplaceAllString(s, "_")
		return strings.Trim(safe, "_")
	}

	createTime := ""
	if header, ok := raw["header"].(map[string]interface{}); ok {
		createTime, _ = header["create_time"].(string)
	}
	if createTime == "" {
		createTime = fmt.Sprintf("%d", os.Getpid())
	}
	// Sanitize createTime to prevent path traversal via e.g. "2026/03/24" → subdirectory creation.
	createTime = sanitizeFilename(createTime)
	if createTime == "" {
		createTime = "unknown"
	}

	// Extract sender name and subject from message payload; fall back to event_id.
	subject := ""
	senderName := ""
	if msg, ok := data.(map[string]interface{}); ok {
		subject, _ = msg["subject"].(string)
		senderName, _ = msg["from"].(string)
	}

	var stem string
	if subject != "" || senderName != "" {
		parts := []string{}
		if senderName != "" {
			parts = append(parts, senderName)
		}
		if subject != "" {
			parts = append(parts, subject)
		}
		raw := strings.Join(parts, "_")
		safe := sanitizeFilename(raw)
		if len(safe) > 80 {
			safe = safe[:80]
		}
		stem = safe
	} else {
		eventID := "unknown"
		if header, ok := raw["header"].(map[string]interface{}); ok {
			if id, _ := header["event_id"].(string); id != "" {
				eventID = sanitizeFilename(id)
			}
		}
		if eventID == "" {
			eventID = "unknown"
		}
		stem = eventID
	}
	filename := fmt.Sprintf("%s_%s.json", stem, createTime)
	fp := filepath.Join(outputDir, filename)

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	if err := validate.AtomicWrite(fp, append(jsonData, '\n'), 0600); err != nil {
		return "", err
	}

	return fp, nil
}
