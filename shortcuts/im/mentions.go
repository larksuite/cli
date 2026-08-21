// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"regexp"
	"strings"
	"unicode"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

const (
	mentionEmptyMessage = "--mention requires a non-empty user_id or open_id"
	mentionEmptyHint    = "Pass each user_id or open_id with --mention; repeat the flag or use comma-separated values."
	mentionAllMessage   = "Use --mention-all instead of passing all to --mention"
	mentionAllHint      = "Remove the all value from --mention and add --mention-all."
	mentionInvalidMsg   = "--mention contains an invalid user_id or open_id"
	mentionInvalidHint  = "Pass each user_id or open_id without whitespace or tag/attribute delimiter characters."

	mentionTypeMessage = "--mention and --mention-all support only text or post messages"
	mentionTypeHint    = "Use --text, --markdown, or --content with --msg-type text|post; otherwise remove the mention flags."

	manualAtConflictMessage = "Do not combine mention flags with manual at tags"
	manualAtConflictHint    = "Remove the manual at tags and pass each target with --mention or --mention-all."
	manualAtAliasMessage    = "Manual <at id> and <at open_id> tags are not supported"
	manualAtAliasHint       = "Use --mention <user_id-or-open_id> instead."
)

var (
	manualAtTagRE   = regexp.MustCompile(`(?i)<at(?:[[:space:]]|/?>)`)
	manualAtAliasRE = regexp.MustCompile(`(?i)<at[[:space:]]+(?:id|open_id)[[:space:]]*=`)
)

type mentionRequest struct {
	IDs []string
	All bool
}

// mentionSliceValue preserves repeat/comma string-slice behavior without
// letting encoding/csv reject malformed quotes before IM validation. The
// default pflag string-slice parser includes the full raw value in that parse
// error; mention IDs are untrusted and must instead reach the fixed,
// non-echoing validation errors below.
type mentionSliceValue struct {
	values  []string
	changed bool
}

func (v *mentionSliceValue) Set(raw string) error {
	parts := strings.Split(raw, ",")
	if !v.changed {
		v.values = parts
	} else {
		v.values = append(v.values, parts...)
	}
	v.changed = true
	return nil
}

func (v *mentionSliceValue) Type() string {
	return "stringSlice"
}

func (v *mentionSliceValue) String() string {
	if len(v.values) == 0 {
		return "[]"
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(v.values); err != nil {
		return "[]"
	}
	writer.Flush()
	if writer.Error() != nil {
		return "[]"
	}
	return "[" + strings.TrimSuffix(buffer.String(), "\n") + "]"
}

func (v *mentionSliceValue) Append(value string) error {
	v.values = append(v.values, value)
	v.changed = true
	return nil
}

func (v *mentionSliceValue) Replace(values []string) error {
	v.values = append(v.values[:0], values...)
	v.changed = true
	return nil
}

func (v *mentionSliceValue) GetSlice() []string {
	return append([]string(nil), v.values...)
}

func installMentionFlagParser(cmd *cobra.Command) {
	flag := cmd.Flags().Lookup("mention")
	if flag == nil {
		return
	}
	flag.Value = &mentionSliceValue{}
	// pflag does not recognize this custom slice type as a zero value. Keep the
	// semantic default empty so help does not render the synthetic "(default [])".
	flag.DefValue = ""
}

func (r mentionRequest) requested() bool {
	return len(r.IDs) > 0 || r.All
}

func (r mentionRequest) flagParam() string {
	if len(r.IDs) > 0 {
		return "--mention"
	}
	return "--mention-all"
}

func parseMentionValues(values []string, mentionChanged, mentionAll bool) (mentionRequest, error) {
	request := mentionRequest{All: mentionAll}
	if !mentionChanged && len(values) == 0 {
		return request, nil
	}
	if len(values) == 0 {
		return mentionRequest{}, mentionValidationError(
			"--mention", mentionEmptyMessage, mentionEmptyHint,
		)
	}

	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		// RuntimeContext.StrSlice already implements CSV splitting. Splitting
		// here as well keeps this parser correct for direct callers and makes
		// repeat/comma behavior one explicit IM-local contract.
		for _, value := range strings.Split(raw, ",") {
			if value == "" {
				return mentionRequest{}, mentionValidationError(
					"--mention", mentionEmptyMessage, mentionEmptyHint,
				)
			}
			if strings.EqualFold(value, "all") || strings.EqualFold(value, "@_all") {
				return mentionRequest{}, mentionValidationError(
					"--mention", mentionAllMessage, mentionAllHint,
				)
			}
			if !validMentionID(value) {
				return mentionRequest{}, mentionValidationError(
					"--mention", mentionInvalidMsg, mentionInvalidHint,
				)
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			request.IDs = append(request.IDs, value)
		}
	}
	return request, nil
}

func validMentionID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
		switch r {
		case '<', '>', '\'', '"', '=', '/', '\\':
			return false
		}
	}
	return true
}

func mentionRequestFromRuntime(runtime *common.RuntimeContext) (mentionRequest, error) {
	return parseMentionValues(
		runtime.StrSlice("mention"),
		runtime.Changed("mention"),
		runtime.Bool("mention-all"),
	)
}

func addMessageMentionResult(runtime *common.RuntimeContext, response, result map[string]interface{}) error {
	request, err := mentionRequestFromRuntime(runtime)
	if err != nil {
		return err
	}
	if !request.requested() {
		return nil
	}
	result["mention_result"] = imcontract.BuildMessageMentionResult(
		imcontract.MessageMentionRequest{IDs: request.IDs, All: request.All},
		response["mentions"],
	)
	return nil
}

// buildMessageRequestBody is the single request-body builder used by
// validation, dry-run, and execution. Callers supply command-specific fields
// such as receive_id, reply_in_thread, or uuid in extra.
func buildMessageRequestBody(
	runtime *common.RuntimeContext,
	msgType string,
	content string,
	extra map[string]interface{},
) (map[string]interface{}, error) {
	body := make(map[string]interface{}, len(extra)+2)
	for key, value := range extra {
		body[key] = value
	}
	body["msg_type"] = msgType
	body["content"] = content

	request, err := mentionRequestFromRuntime(runtime)
	if err != nil {
		return body, err
	}
	content, err = applyMentionRequest(msgType, content, messageContentParam(runtime), request)
	if err != nil {
		return body, err
	}

	body["content"] = content
	return body, nil
}

func messageContentParam(runtime *common.RuntimeContext) string {
	for _, flag := range []string{"text", "markdown", "content", "image", "file", "video", "audio"} {
		if runtime.Changed(flag) {
			return "--" + flag
		}
	}
	return "--content"
}

func applyMentionRequest(msgType, content, contentParam string, request mentionRequest) (string, error) {
	if msgType != "text" && msgType != "post" {
		if !request.requested() {
			return content, nil
		}
		return "", mentionValidationError(request.flagParam(), mentionTypeMessage, mentionTypeHint)
	}

	switch msgType {
	case "text":
		return applyTextMentionRequest(content, contentParam, request)
	case "post":
		return applyPostMentionRequest(content, contentParam, request)
	default:
		return content, nil
	}
}

func applyTextMentionRequest(content, contentParam string, request mentionRequest) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		if !request.requested() {
			if manualAtAliasRE.MatchString(content) {
				return "", mentionValidationError(contentParam, manualAtAliasMessage, manualAtAliasHint)
			}
			return content, nil
		}
		return "", invalidMentionContent(contentParam, "text")
	}
	rawText, ok := payload["text"]
	if !ok {
		if request.requested() {
			return "", invalidMentionContent(contentParam, "text")
		}
		return content, nil
	}
	var text string
	if err := json.Unmarshal(rawText, &text); err != nil {
		if request.requested() {
			return "", invalidMentionContent(contentParam, "text")
		}
		if manualAtAliasRE.MatchString(content) {
			return "", mentionValidationError(contentParam, manualAtAliasMessage, manualAtAliasHint)
		}
		return content, nil
	}
	if err := validateInlineManualAt(text, contentParam, request.requested()); err != nil {
		return "", err
	}
	if !request.requested() {
		return content, nil
	}

	prefix := textMentionPrefix(request)
	if text != "" {
		prefix += " "
	}
	encodedText, err := json.Marshal(prefix + text)
	if err != nil {
		return "", invalidMentionContent(contentParam, "text")
	}
	payload["text"] = encodedText
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", invalidMentionContent(contentParam, "text")
	}
	return string(encoded), nil
}

func textMentionPrefix(request mentionRequest) string {
	targets := make([]string, 0, len(request.IDs)+1)
	for _, id := range request.IDs {
		targets = append(targets, `<at user_id="`+id+`"></at>`)
	}
	if request.All {
		targets = append(targets, `<at user_id="all"></at>`)
	}
	return strings.Join(targets, " ")
}

func applyPostMentionRequest(content, contentParam string, request mentionRequest) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &payload); err != nil || len(payload) == 0 {
		if !request.requested() {
			if manualAtAliasRE.MatchString(content) {
				return "", mentionValidationError(contentParam, manualAtAliasMessage, manualAtAliasHint)
			}
			return content, nil
		}
		return "", invalidMentionContent(contentParam, "post")
	}

	type parsedLocale struct {
		fields  map[string]json.RawMessage
		content [][]map[string]interface{}
	}
	locales := make(map[string]parsedLocale, len(payload))
	for locale, rawLocale := range payload {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rawLocale, &fields); err != nil {
			if !request.requested() {
				continue
			}
			return "", invalidMentionContent(contentParam, "post")
		}
		rawParagraphs, ok := fields["content"]
		if !ok {
			if request.requested() {
				return "", invalidMentionContent(contentParam, "post")
			}
			continue
		}
		var paragraphs [][]map[string]interface{}
		if err := json.Unmarshal(rawParagraphs, &paragraphs); err != nil {
			if !request.requested() {
				continue
			}
			return "", invalidMentionContent(contentParam, "post")
		}
		if err := validatePostManualAt(paragraphs, contentParam, request.requested()); err != nil {
			return "", err
		}
		locales[locale] = parsedLocale{fields: fields, content: paragraphs}
	}
	if !request.requested() {
		return content, nil
	}
	if len(locales) != len(payload) {
		return "", invalidMentionContent(contentParam, "post")
	}

	mentionParagraph := postMentionParagraph(request)
	for locale, parsed := range locales {
		paragraphs := make([][]map[string]interface{}, 0, len(parsed.content)+1)
		paragraphs = append(paragraphs, mentionParagraph)
		paragraphs = append(paragraphs, parsed.content...)
		rawParagraphs, err := json.Marshal(paragraphs)
		if err != nil {
			return "", invalidMentionContent(contentParam, "post")
		}
		parsed.fields["content"] = rawParagraphs
		rawLocale, err := json.Marshal(parsed.fields)
		if err != nil {
			return "", invalidMentionContent(contentParam, "post")
		}
		payload[locale] = rawLocale
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", invalidMentionContent(contentParam, "post")
	}
	return string(encoded), nil
}

func postMentionParagraph(request mentionRequest) []map[string]interface{} {
	targets := append([]string(nil), request.IDs...)
	if request.All {
		targets = append(targets, "all")
	}
	paragraph := make([]map[string]interface{}, 0, len(targets)*2-1)
	for i, target := range targets {
		if i > 0 {
			paragraph = append(paragraph, map[string]interface{}{"tag": "text", "text": " "})
		}
		paragraph = append(paragraph, map[string]interface{}{"tag": "at", "user_id": target})
	}
	return paragraph
}

func validateInlineManualAt(text, contentParam string, withMentionFlags bool) error {
	if manualAtAliasRE.MatchString(text) {
		return mentionValidationError(contentParam, manualAtAliasMessage, manualAtAliasHint)
	}
	if withMentionFlags && manualAtTagRE.MatchString(text) {
		return mentionValidationError(contentParam, manualAtConflictMessage, manualAtConflictHint)
	}
	return nil
}

func validatePostManualAt(paragraphs [][]map[string]interface{}, contentParam string, withMentionFlags bool) error {
	hasManualAt := false
	for _, paragraph := range paragraphs {
		for _, node := range paragraph {
			tag, _ := node["tag"].(string)
			if strings.EqualFold(tag, "at") {
				if _, ok := node["id"]; ok {
					return mentionValidationError(contentParam, manualAtAliasMessage, manualAtAliasHint)
				}
				if _, ok := node["open_id"]; ok {
					return mentionValidationError(contentParam, manualAtAliasMessage, manualAtAliasHint)
				}
				hasManualAt = true
			}
			if text, _ := node["text"].(string); text != "" {
				if err := validateInlineManualAt(text, contentParam, withMentionFlags); err != nil {
					return err
				}
			}
		}
	}
	if withMentionFlags && hasManualAt {
		return mentionValidationError(contentParam, manualAtConflictMessage, manualAtConflictHint)
	}
	return nil
}

func mentionValidationError(param, message, hint string) error {
	err := errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", message).WithParam(param)
	if hint != "" {
		err = err.WithHint("%s", hint)
	}
	return err
}

func invalidMentionContent(param, msgType string) error {
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"mention flags require valid %s message content",
		msgType,
	).WithParam(param)
}
