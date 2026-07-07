// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	vcMeetingEventsAPIPath     = "/open-apis/vc/v1/bots/events"
	defaultVCMeetingEventsSize = 20
	minVCMeetingEventsPageSize = 20
	maxVCMeetingEventsPageSize = 100
	maxVCMeetingEventsPages    = 200
	leaveReasonUserLeft        = 1
	leaveReasonMeetingEnded    = 2
	leaveReasonKicked          = 3
)

var meetingDisplayLocation = time.FixedZone("UTC+8", 8*60*60)

// toUnixSeconds converts a supported CLI time input into a Unix seconds string.
func toUnixSeconds(input string, hint ...string) (string, error) {
	ts, err := common.ParseTime(input, hint...)
	if err != nil {
		return "", err
	}
	if _, err := strconv.ParseInt(ts, 10, 64); err != nil {
		return "", fmt.Errorf("invalid timestamp %q: %w", ts, err) //nolint:forbidigo // intermediate parse error; callers wrap it into a typed ValidationError
	}
	return ts, nil
}

// VCMeetingEvents lists meeting events for a meeting.
var VCMeetingEvents = common.Shortcut{
	Service:     "vc",
	Command:     "+meeting-events",
	Description: "List meeting events by meeting ID",
	Risk:        "read",
	Scopes:      []string{"vc:meeting.meetingevent:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-id", Required: true, Desc: "meeting ID to query"},
		{Name: "start", Desc: "time lower bound (ISO 8601, YYYY-MM-DD, or Unix seconds)"},
		{Name: "end", Desc: "time upper bound (ISO 8601, YYYY-MM-DD, or Unix seconds)"},
		{Name: "page-token", Desc: "page token for the next page"},
		{Name: "page-size", Default: "20", Desc: "page size, 20-100 (default 20)"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all available pages"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateMeetingEventsMeetingID(runtime.Str("meeting-id")); err != nil {
			return err
		}
		if _, err := meetingEventsPageSize(runtime); err != nil {
			return err
		}
		if _, _, err := parseMeetingEventsTimeRange(runtime); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		startTime, endTime, err := parseMeetingEventsTimeRange(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		params, err := buildMeetingEventsParams(runtime, startTime, endTime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		dryRun := common.NewDryRunAPI()
		if runtime.Bool("page-all") {
			dryRun = dryRun.Desc("Auto-paginates through all available pages")
		}
		dryRun = dryRun.GET(vcMeetingEventsAPIPath)
		if flat := flattenQueryParams(params); len(flat) > 0 {
			dryRun.Params(flat)
		}
		return dryRun
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		startTime, endTime, err := parseMeetingEventsTimeRange(runtime)
		if err != nil {
			return err
		}
		data, events, hasMore, pageToken, err := fetchMeetingEvents(ctx, runtime, startTime, endTime)
		if err != nil {
			return err
		}
		events = compactMeetingEvents(events)
		identity, identityWarning := meetingEventsCurrentIdentity(runtime)
		outData := buildMeetingEventsOutput(data, events, identity, identityWarning)
		metadata := map[string]interface{}{
			"row_type":   "metadata",
			"meeting":    outData.Meeting,
			"identity":   outData.Identity,
			"has_more":   outData.HasMore,
			"page_token": outData.PageToken,
		}
		if len(outData.Warnings) > 0 {
			metadata["warnings"] = outData.Warnings
		}
		ndjsonData := meetingEventsEventRows(outData.Events, metadata)

		timeline := buildMeetingEventTimeline(events)
		if runtime.Format == "ndjson" {
			runtime.OutFormat(ndjsonData, &output.Meta{Count: len(events)}, func(w io.Writer) {})
		} else {
			runtime.OutFormat(outData, &output.Meta{Count: len(events)}, func(w io.Writer) {
				renderMeetingEventsCompactPretty(w, outData, timeline)
			})
		}
		if runtime.Format == "pretty" && pageToken != "" {
			fmt.Fprintf(runtime.IO().Out, "\npage_token: %s\n", pageToken)
			if hasMore {
				fmt.Fprintln(runtime.IO().Out, "more available")
			}
		}
		return nil
	},
}

type meetingEventsOutput struct {
	Meeting   meetingEventsMeeting  `json:"meeting"`
	Identity  meetingEventsIdentity `json:"identity"`
	Events    []meetingEventsEvent  `json:"events"`
	Warnings  []string              `json:"warnings,omitempty"`
	HasMore   bool                  `json:"has_more"`
	PageToken string                `json:"page_token,omitempty"`
}

type meetingEventsMeeting struct {
	ID        string `json:"id,omitempty"`
	Topic     string `json:"topic,omitempty"`
	MeetingNo string `json:"meeting_no,omitempty"`
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
	Status    string `json:"status"`
}

type meetingEventsIdentity struct {
	ID              string `json:"id,omitempty"`
	Name            string `json:"name,omitempty"`
	ParticipantType string `json:"participant_type,omitempty"`
	Role            string `json:"role,omitempty"`
	Label           string `json:"label,omitempty"`
}

type meetingEventsEvent struct {
	EventID   string                  `json:"event_id,omitempty"`
	EventType string                  `json:"event_type,omitempty"`
	EventTime string                  `json:"event_time,omitempty"`
	Actors    []meetingEventsIdentity `json:"actors,omitempty"`
	Payload   map[string]interface{}  `json:"payload,omitempty"`
}

type meetingEventsEndSignal struct {
	Ended      bool
	EndTime    time.Time
	HasEndTime bool
}

func buildMeetingEventsOutput(data map[string]interface{}, events []interface{}, identity meetingEventsIdentity, warnings ...string) meetingEventsOutput {
	output := meetingEventsOutput{
		Meeting:   meetingEventsMeetingFromPayload(nil),
		Identity:  identity,
		HasMore:   common.GetBool(data, "has_more"),
		PageToken: common.GetString(data, "page_token"),
	}
	for _, warning := range warnings {
		if warning = strings.TrimSpace(warning); warning != "" {
			output.Warnings = append(output.Warnings, warning)
		}
	}
	for _, raw := range events {
		event, _ := raw.(map[string]interface{})
		if event == nil {
			continue
		}
		payload := common.GetMap(event, "payload")
		if meeting := common.GetMap(payload, "meeting"); meeting != nil {
			output.Meeting = meetingEventsMeetingFromPayload(meeting)
		}
		output.Events = append(output.Events, meetingEventsEventFromPayload(event, output.Identity))
	}
	applyMeetingEventsEndSignal(&output.Meeting, meetingEventsEndSignalFromEvents(events))
	return output
}

func meetingEventsCurrentIdentity(runtime *common.RuntimeContext) (meetingEventsIdentity, string) {
	if runtime.As() == core.AsBot {
		botInfo, err := runtime.BotInfo()
		if err != nil {
			return meetingEventsBotIdentity(nil), fmt.Sprintf("identity unavailable: %v", err)
		}
		return meetingEventsBotIdentity(botInfo), ""
	}
	userOpenID := strings.TrimSpace(runtime.UserOpenId())
	identity := meetingEventsIdentity{
		ID:              userOpenID,
		Name:            strings.TrimSpace(runtime.Config.UserName),
		ParticipantType: "human",
	}
	identity.Label = identityLabel(identity)
	if userOpenID == "" {
		return identity, "identity unavailable: current user open_id is unavailable"
	}
	return identity, ""
}

func meetingEventsBotIdentity(botInfo *common.BotInfo) meetingEventsIdentity {
	if botInfo == nil {
		return meetingEventsIdentity{ParticipantType: "bot", Label: "bot"}
	}
	identity := meetingEventsIdentity{
		ID:              botInfo.OpenID,
		Name:            botInfo.AppName,
		ParticipantType: "bot",
	}
	identity.Label = identityLabel(identity)
	return identity
}

func meetingEventsMeetingFromPayload(meeting map[string]interface{}) meetingEventsMeeting {
	out := meetingEventsMeeting{
		ID:        common.GetString(meeting, "id"),
		Topic:     common.GetString(meeting, "topic"),
		MeetingNo: common.GetString(meeting, "meeting_no"),
		StartTime: meetingEventsTimeString(common.GetString(meeting, "start_time")),
		EndTime:   meetingEventsTimeString(common.GetString(meeting, "end_time")),
		Status:    "unknown",
	}
	start, hasStart := parseFlexibleTime(out.StartTime)
	end, hasEnd := parseFlexibleTime(out.EndTime)
	if hasStart && !hasEnd {
		out.Status = "ongoing"
	}
	if hasStart && hasEnd {
		if end.After(start) {
			out.Status = "ended"
		} else {
			out.Status = "ongoing"
			out.EndTime = ""
		}
	}
	return out
}

func applyMeetingEventsEndSignal(meeting *meetingEventsMeeting, signal meetingEventsEndSignal) {
	if meeting == nil || !signal.Ended {
		return
	}
	meeting.Status = "ended"
	if signal.HasEndTime {
		meeting.EndTime = signal.EndTime.UTC().Format(time.RFC3339)
	}
}

func meetingEventsEndSignalFromEvents(events []interface{}) meetingEventsEndSignal {
	var signal meetingEventsEndSignal
	for _, raw := range events {
		event, _ := raw.(map[string]interface{})
		if event == nil || meetingEventType(event) != "participant_left" {
			continue
		}
		payload := common.GetMap(event, "payload")
		if payload == nil {
			continue
		}
		fallbackTime, fallbackOK := parseFlexibleTime(common.GetString(event, "event_time"))
		for _, rawItem := range common.GetSlice(payload, "participant_left_items") {
			item, _ := rawItem.(map[string]interface{})
			if item == nil || int(common.GetFloat(item, "leave_reason")) != leaveReasonMeetingEnded {
				continue
			}
			signal.Ended = true
			endTime, ok := parseFlexibleTime(common.GetString(item, "leave_time"))
			if !ok {
				endTime, ok = fallbackTime, fallbackOK
			}
			if ok && (!signal.HasEndTime || endTime.After(signal.EndTime)) {
				signal.EndTime = endTime
				signal.HasEndTime = true
			}
		}
	}
	return signal
}

func meetingEventsEventFromPayload(event map[string]interface{}, selfIdentity meetingEventsIdentity) meetingEventsEvent {
	payload := common.GetMap(event, "payload")
	out := meetingEventsEvent{
		EventID:   common.GetString(event, "event_id"),
		EventType: meetingEventType(event),
		EventTime: meetingEventsTimeString(common.GetString(event, "event_time")),
		Payload:   payload,
	}
	out.Actors = eventActors(out.EventType, payload, selfIdentity)
	return out
}

func eventActors(eventType string, payload map[string]interface{}, selfIdentity meetingEventsIdentity) []meetingEventsIdentity {
	var actors []meetingEventsIdentity
	addFromItems := func(key, participantKey string) {
		for _, raw := range common.GetSlice(payload, key) {
			item, _ := raw.(map[string]interface{})
			if item == nil {
				continue
			}
			if participant := common.GetMap(item, participantKey); participant != nil {
				actors = append(actors, meetingEventsIdentityFromParticipant(participant, selfIdentity))
			}
		}
	}
	switch eventType {
	case "participant_joined":
		addFromItems("participant_joined_items", "participant")
	case "participant_left":
		addFromItems("participant_left_items", "participant")
	case "transcript_received":
		addFromItems("transcript_received_items", "speaker")
	case "chat_received":
		addFromItems("chat_received_items", "operator")
	case "magic_share_started":
		addFromItems("magic_share_started_items", "operator")
	case "magic_share_ended":
		addFromItems("magic_share_ended_items", "operator")
	}
	return actors
}

func meetingEventsIdentityFromParticipant(participant map[string]interface{}, selfIdentity meetingEventsIdentity) meetingEventsIdentity {
	identity := meetingEventsIdentity{
		ID:              common.GetString(participant, "id"),
		Name:            common.GetString(participant, "user_name"),
		ParticipantType: meetingEventsParticipantType(participant),
		Role:            meetingEventsParticipantRole(participant),
	}
	if identity.ID != "" && selfIdentity.ID != "" && identity.ID == selfIdentity.ID {
		if selfIdentity.ParticipantType == "bot" && (identity.ParticipantType == "" || identity.ParticipantType == "human") {
			identity.ParticipantType = "bot"
		}
		if selfIdentity.ParticipantType == "bot" && (identity.Role == "" || identity.Role == "participant") {
			identity.Role = "bot"
		}
	}
	if identity.ParticipantType == "" {
		identity.ParticipantType = "human"
	}
	if identity.Role == "" {
		identity.Role = "participant"
	}
	identity.Label = identityLabel(identity)
	return identity
}

func meetingEventsParticipantType(participant map[string]interface{}) string {
	if raw := meetingEventsParticipantTypeFromParticipantType(fieldValueString(participant, "participant_type")); raw != "" {
		return raw
	}
	return meetingEventsParticipantTypeFromUserType(fieldValueString(participant, "user_type"))
}

func meetingEventsParticipantTypeFromParticipantType(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "1", "user", "human":
		return "human"
	case "2", "bot", "app":
		return "bot"
	case "":
		return ""
	default:
		return "unknown"
	}
}

func meetingEventsParticipantRole(participant map[string]interface{}) string {
	if raw := meetingEventsRoleFromParticipantRole(fieldValueString(participant, "role")); raw != "" {
		return raw
	}
	return meetingEventsRoleFromEventUserRole(fieldValueString(participant, "user_role"))
}

func meetingEventsParticipantTypeFromUserType(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "1", "user", "human":
		return "human"
	case "2", "10", "bot", "app":
		return "bot"
	case "":
		return ""
	default:
		return "unknown"
	}
}

func meetingEventsRoleFromParticipantRole(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "1", "host":
		return "host"
	case "2", "co_host", "cohost":
		return "co_host"
	case "3", "participant", "attendee":
		return "participant"
	case "4", "bot", "app":
		return "bot"
	case "":
		return ""
	default:
		return raw
	}
}

func meetingEventsRoleFromEventUserRole(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "1", "participant", "attendee":
		return "participant"
	case "2", "host":
		return "host"
	case "4", "bot", "app":
		return "bot"
	case "", "0":
		return ""
	default:
		return raw
	}
}

func fieldValueString(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	switch value := values[key].(type) {
	case string:
		return value
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.FormatInt(int64(value), 10)
	case json.Number:
		return value.String()
	default:
		return ""
	}
}

func identityLabel(identity meetingEventsIdentity) string {
	name := identity.Name
	if name == "" {
		name = identity.ID
	}
	if name == "" {
		name = "unknown"
	}
	var tags []string
	if identity.ParticipantType != "" {
		tags = append(tags, identity.ParticipantType)
	}
	if identity.Role != "" && identity.Role != identity.ParticipantType {
		tags = append(tags, identity.Role)
	}
	if len(tags) == 0 {
		return name
	}
	return fmt.Sprintf("%s [%s]", name, strings.Join(tags, ","))
}

func meetingEventsTimeString(raw string) string {
	if parsed, ok := parseFlexibleTime(raw); ok {
		return parsed.UTC().Format(time.RFC3339)
	}
	return strings.TrimSpace(raw)
}

func meetingEventsEventRows(events []meetingEventsEvent, metadata map[string]interface{}) []interface{} {
	rows := make([]interface{}, 0, len(events)+1)
	for _, event := range events {
		row := meetingEventsEventRow(event)
		rows = append(rows, row)
	}
	if metadata != nil {
		rows = append(rows, metadata)
	}
	return rows
}

func meetingEventsEventRow(event meetingEventsEvent) map[string]interface{} {
	raw, err := json.Marshal(event)
	if err != nil {
		return map[string]interface{}{"row_type": "event"}
	}
	var row map[string]interface{}
	if err := json.Unmarshal(raw, &row); err != nil {
		return map[string]interface{}{"row_type": "event"}
	}
	row["row_type"] = "event"
	return row
}

func renderMeetingEventsCompactPretty(w io.Writer, data meetingEventsOutput, timeline meetingTimeline) {
	if data.Identity.Label != "" {
		fmt.Fprintf(w, "当前身份：%s\n", escapePrettyText(data.Identity.Label))
	}
	if len(timeline.entries) == 0 {
		fmt.Fprintln(w, "No meeting events.")
		return
	}
	io.WriteString(w, renderMeetingEventsPretty(timeline))
}

func meetingEventsPageSize(runtime *common.RuntimeContext) (int, error) {
	if runtime.Bool("page-all") {
		return maxVCMeetingEventsPageSize, nil
	}
	pageSizeStr := strings.TrimSpace(runtime.Str("page-size"))
	if pageSizeStr == "" {
		return defaultVCMeetingEventsSize, nil
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil {
		return 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --page-size %q: must be an integer", pageSizeStr).WithParam("--page-size")
	}
	if pageSize < minVCMeetingEventsPageSize {
		return minVCMeetingEventsPageSize, nil
	}
	if pageSize > maxVCMeetingEventsPageSize {
		return maxVCMeetingEventsPageSize, nil
	}
	return pageSize, nil
}

func meetingEventsPaginationConfig(runtime *common.RuntimeContext) (bool, int) {
	if !runtime.Bool("page-all") {
		return false, 0
	}
	return true, maxVCMeetingEventsPages
}

func validateMeetingEventsMeetingID(meetingID string) error {
	meetingID = strings.TrimSpace(meetingID)
	if meetingID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--meeting-id is required").WithParam("--meeting-id")
	}
	if validMeetingNumber(meetingID) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--meeting-id must be a long meeting_id, not a 9-digit meeting number; use +meeting-join or +meeting-list-active to get meeting_id").WithParam("--meeting-id")
	}
	value, err := strconv.ParseInt(meetingID, 10, 64)
	if err != nil || value <= 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--meeting-id must be a positive integer, got %q", meetingID).WithParam("--meeting-id")
	}
	return nil
}

// parseMeetingEventsTimeRange validates --start/--end and returns Unix seconds strings.
func parseMeetingEventsTimeRange(runtime *common.RuntimeContext) (string, string, error) {
	start := strings.TrimSpace(runtime.Str("start"))
	end := strings.TrimSpace(runtime.Str("end"))
	if start == "" && end == "" {
		return "", "", nil
	}

	var startTime, endTime string
	if start != "" {
		parsed, err := toUnixSeconds(start)
		if err != nil {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--start: %v", err).WithParam("--start")
		}
		startTime = parsed
	}
	if end != "" {
		parsed, err := toUnixSeconds(end, "end")
		if err != nil {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--end: %v", err).WithParam("--end")
		}
		endTime = parsed
	}
	if startTime != "" && endTime != "" {
		startValue, _ := strconv.ParseInt(startTime, 10, 64)
		endValue, _ := strconv.ParseInt(endTime, 10, 64)
		if startValue > endValue {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--start (%s) is after --end (%s)", start, end).WithParam("--start")
		}
	}
	return startTime, endTime, nil
}

func buildMeetingEventsParams(runtime *common.RuntimeContext, startTime, endTime string) (map[string]interface{}, error) {
	pageSize, err := meetingEventsPageSize(runtime)
	if err != nil {
		return nil, err
	}

	params := map[string]interface{}{
		"meeting_id": strings.TrimSpace(runtime.Str("meeting-id")),
		"page_size":  strconv.Itoa(pageSize),
	}
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	if startTime != "" {
		params["start_time"] = startTime
	}
	if endTime != "" {
		params["end_time"] = endTime
	}
	return params, nil
}

func fetchMeetingEvents(ctx context.Context, runtime *common.RuntimeContext, startTime, endTime string) (map[string]interface{}, []interface{}, bool, string, error) {
	params, err := buildMeetingEventsParams(runtime, startTime, endTime)
	if err != nil {
		return nil, nil, false, "", err
	}
	autoPaginate, pageLimit := meetingEventsPaginationConfig(runtime)
	if !autoPaginate {
		data, err := runtime.CallAPITyped(http.MethodGet, vcMeetingEventsAPIPath, params, nil)
		if err != nil {
			return nil, nil, false, "", err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		events := common.GetSlice(data, "events")
		hasMore, _ := data["has_more"].(bool)
		pageToken, _ := data["page_token"].(string)
		return data, events, hasMore, pageToken, nil
	}

	var (
		allEvents     []interface{}
		lastData      map[string]interface{}
		lastPageToken string
		lastHasMore   bool
	)
	for page := 0; page < pageLimit; page++ {
		data, err := runtime.CallAPITyped(http.MethodGet, vcMeetingEventsAPIPath, params, nil)
		if err != nil {
			return nil, nil, false, "", err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		lastData = data
		events := common.GetSlice(data, "events")
		allEvents = append(allEvents, events...)
		lastHasMore, _ = data["has_more"].(bool)
		lastPageToken, _ = data["page_token"].(string)
		if !lastHasMore || lastPageToken == "" {
			break
		}
		params["page_token"] = lastPageToken
	}
	if lastData == nil {
		lastData = map[string]interface{}{}
	}
	lastData["events"] = allEvents
	lastData["has_more"] = lastHasMore
	lastData["page_token"] = lastPageToken
	return lastData, allEvents, lastHasMore, lastPageToken, nil
}

func flattenQueryParams(params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}
	return params
}

func compactMeetingEvents(events []interface{}) []interface{} {
	compacted := make([]interface{}, 0, len(events))
	for _, raw := range events {
		event, _ := raw.(map[string]interface{})
		if event == nil {
			continue
		}
		if payload := common.GetMap(event, "payload"); payload != nil {
			event["payload"] = compactMeetingPayload(payload)
		}
		compacted = append(compacted, event)
	}
	return compacted
}

func compactMeetingPayload(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return nil
	}
	compacted := make(map[string]interface{}, len(payload))
	for key, value := range payload {
		if items, ok := value.([]interface{}); ok && len(items) == 0 {
			continue
		}
		compacted[key] = value
	}
	return compacted
}

type meetingTimeline struct {
	topic     string
	startTime time.Time
	hasStart  bool
	endTime   time.Time
	hasEnd    bool
	entries   []meetingTimelineEntry
}

type meetingTimelineEntry struct {
	when        time.Time
	hasWhen     bool
	sequence    int
	subject     string
	description string
	details     []string
}

func buildMeetingEventTimeline(events []interface{}) meetingTimeline {
	timeline := meetingTimeline{}
	var sequence int
	for _, raw := range events {
		event, _ := raw.(map[string]interface{})
		if event == nil {
			continue
		}
		payload := common.GetMap(event, "payload")
		if payload == nil {
			continue
		}
		if timeline.topic == "" || !timeline.hasStart || !timeline.hasEnd {
			populateMeetingHeader(&timeline, common.GetMap(payload, "meeting"))
		}
		for _, entry := range buildTimelineEntriesForEvent(event, &sequence) {
			timeline.entries = append(timeline.entries, entry)
		}
	}
	applyMeetingTimelineEndSignal(&timeline, meetingEventsEndSignalFromEvents(events))
	sort.SliceStable(timeline.entries, func(i, j int) bool {
		left := timeline.entries[i]
		right := timeline.entries[j]
		switch {
		case left.hasWhen && right.hasWhen:
			if left.when.Equal(right.when) {
				return left.sequence < right.sequence
			}
			return left.when.Before(right.when)
		case left.hasWhen:
			return true
		case right.hasWhen:
			return false
		default:
			return left.sequence < right.sequence
		}
	})
	return timeline
}

func applyMeetingTimelineEndSignal(timeline *meetingTimeline, signal meetingEventsEndSignal) {
	if timeline == nil || !signal.Ended {
		return
	}
	if signal.HasEndTime {
		if !timeline.hasStart || signal.EndTime.After(timeline.startTime) {
			timeline.endTime = signal.EndTime
			timeline.hasEnd = true
			return
		}
		timeline.hasEnd = false
		return
	}
	if timeline.hasStart && timeline.hasEnd && !timeline.endTime.After(timeline.startTime) {
		timeline.hasEnd = false
	}
}

func populateMeetingHeader(timeline *meetingTimeline, meeting map[string]interface{}) {
	if timeline == nil || meeting == nil {
		return
	}
	if timeline.topic == "" {
		timeline.topic = common.GetString(meeting, "topic")
	}
	if !timeline.hasStart {
		if parsed, ok := parseFlexibleTime(common.GetString(meeting, "start_time")); ok {
			timeline.startTime = parsed
			timeline.hasStart = true
		}
	}
	if !timeline.hasEnd {
		if parsed, ok := parseFlexibleTime(common.GetString(meeting, "end_time")); ok {
			timeline.endTime = parsed
			timeline.hasEnd = true
		}
	}
}

func buildTimelineEntriesForEvent(event map[string]interface{}, sequence *int) []meetingTimelineEntry {
	payload := common.GetMap(event, "payload")
	if payload == nil {
		return nil
	}
	eventType := meetingEventType(event)
	eventTime, eventTimeOK := parseFlexibleTime(common.GetString(event, "event_time"))
	switch eventType {
	case "participant_joined":
		return participantJoinedEntries(payload, eventTime, eventTimeOK, sequence)
	case "participant_left":
		return participantLeftEntries(payload, eventTime, eventTimeOK, sequence)
	case "transcript_received":
		return transcriptEntries(payload, eventTime, eventTimeOK, sequence)
	case "chat_received":
		return chatEntries(payload, eventTime, eventTimeOK, sequence)
	case "magic_share_started":
		return magicShareStartedEntries(payload, eventTime, eventTimeOK, sequence)
	case "magic_share_ended":
		return magicShareEndedEntries(payload, eventTime, eventTimeOK, sequence)
	default:
		return []meetingTimelineEntry{newTimelineEntry(eventTime, eventTimeOK, sequence, meetingEventUserDisplayName(nil), meetingEventSummary(event), nil)}
	}
}

func participantJoinedEntries(payload map[string]interface{}, fallbackTime time.Time, fallbackOK bool, sequence *int) []meetingTimelineEntry {
	items := common.GetSlice(payload, "participant_joined_items")
	if len(items) == 0 {
		return []meetingTimelineEntry{newTimelineEntry(fallbackTime, fallbackOK, sequence, "", "加入了会议", nil)}
	}
	entries := make([]meetingTimelineEntry, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		when, ok := parseFlexibleTime(common.GetString(item, "join_time"))
		if !ok {
			when, ok = fallbackTime, fallbackOK
		}
		subject := meetingEventUserWithID(common.GetMap(item, "participant"))
		if subject == "" {
			subject = "未知参会人"
		}
		entries = append(entries, newTimelineEntry(when, ok, sequence, subject, "加入了会议", nil))
	}
	return entries
}

func participantLeftEntries(payload map[string]interface{}, fallbackTime time.Time, fallbackOK bool, sequence *int) []meetingTimelineEntry {
	items := common.GetSlice(payload, "participant_left_items")
	if len(items) == 0 {
		return []meetingTimelineEntry{newTimelineEntry(fallbackTime, fallbackOK, sequence, "", "离开了会议", nil)}
	}
	entries := make([]meetingTimelineEntry, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		when, ok := parseFlexibleTime(common.GetString(item, "leave_time"))
		if !ok {
			when, ok = fallbackTime, fallbackOK
		}
		subject := meetingEventUserWithID(common.GetMap(item, "participant"))
		if subject == "" {
			subject = "未知参会人"
		}
		entries = append(entries, newTimelineEntry(when, ok, sequence, subject, leaveAction(item), nil))
	}
	return entries
}

func transcriptEntries(payload map[string]interface{}, fallbackTime time.Time, fallbackOK bool, sequence *int) []meetingTimelineEntry {
	items := common.GetSlice(payload, "transcript_received_items")
	if len(items) == 0 {
		return []meetingTimelineEntry{newTimelineEntry(fallbackTime, fallbackOK, sequence, "", "产生了转写", nil)}
	}
	entries := make([]meetingTimelineEntry, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		when, ok := parseFlexibleTime(common.GetString(item, "start_time_ms"))
		if !ok {
			when, ok = fallbackTime, fallbackOK
		}
		subject := meetingEventUserWithID(common.GetMap(item, "speaker"))
		if subject == "" {
			subject = "未知发言人"
		}
		text := strings.TrimSpace(common.GetString(item, "text"))
		description := "产生了转写"
		if text != "" {
			description = text
		}
		entries = append(entries, newTimelineEntry(when, ok, sequence, subject, description, nil))
	}
	return entries
}

func chatEntries(payload map[string]interface{}, fallbackTime time.Time, fallbackOK bool, sequence *int) []meetingTimelineEntry {
	items := common.GetSlice(payload, "chat_received_items")
	if len(items) == 0 {
		return []meetingTimelineEntry{newTimelineEntry(fallbackTime, fallbackOK, sequence, "", "发送了消息", nil)}
	}
	entries := make([]meetingTimelineEntry, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		when, ok := parseFlexibleTime(common.GetString(item, "send_time"))
		if !ok {
			when, ok = fallbackTime, fallbackOK
		}
		subject := meetingEventUserWithID(common.GetMap(item, "operator"))
		if subject == "" {
			subject = "未知发送者"
		}
		typeLabel := chatMessageTypeLabel(item)
		description := strings.TrimSpace(common.GetString(item, "content"))
		if description == "" {
			description = fmt.Sprintf("[%s] 发送了消息", typeLabel)
		} else {
			description = fmt.Sprintf("[%s] %s", typeLabel, description)
		}
		entries = append(entries, newTimelineEntry(when, ok, sequence, subject, description, nil))
	}
	return entries
}

func magicShareStartedEntries(payload map[string]interface{}, fallbackTime time.Time, fallbackOK bool, sequence *int) []meetingTimelineEntry {
	items := common.GetSlice(payload, "magic_share_started_items")
	if len(items) == 0 {
		return []meetingTimelineEntry{newTimelineEntry(fallbackTime, fallbackOK, sequence, "", "开始共享内容", nil)}
	}
	entries := make([]meetingTimelineEntry, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		when, ok := parseFlexibleTime(common.GetString(item, "time"))
		if !ok {
			when, ok = fallbackTime, fallbackOK
		}
		subject := meetingEventUserWithID(common.GetMap(item, "operator"))
		if subject == "" {
			subject = "未知用户"
		}
		title := strings.TrimSpace(common.GetString(common.GetMap(item, "share_doc"), "title"))
		url := strings.TrimSpace(common.GetString(common.GetMap(item, "share_doc"), "url"))
		description := "开始共享内容"
		if title != "" {
			description = fmt.Sprintf("开始共享「%s」", title)
		}
		var details []string
		if url != "" {
			details = append(details, "URL: "+url)
		}
		entries = append(entries, newTimelineEntry(when, ok, sequence, subject, description, details))
	}
	return entries
}

func magicShareEndedEntries(payload map[string]interface{}, fallbackTime time.Time, fallbackOK bool, sequence *int) []meetingTimelineEntry {
	items := common.GetSlice(payload, "magic_share_ended_items")
	if len(items) == 0 {
		return []meetingTimelineEntry{newTimelineEntry(fallbackTime, fallbackOK, sequence, "", "结束共享", nil)}
	}
	entries := make([]meetingTimelineEntry, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		when, ok := parseFlexibleTime(common.GetString(item, "time"))
		if !ok {
			when, ok = fallbackTime, fallbackOK
		}
		subject := meetingEventUserWithID(common.GetMap(item, "operator"))
		if subject == "" {
			subject = "未知用户"
		}
		entries = append(entries, newTimelineEntry(when, ok, sequence, subject, "结束共享", nil))
	}
	return entries
}

func newTimelineEntry(when time.Time, hasWhen bool, sequence *int, subject, description string, details []string) meetingTimelineEntry {
	entry := meetingTimelineEntry{
		when:        when,
		hasWhen:     hasWhen,
		sequence:    *sequence,
		subject:     subject,
		description: description,
		details:     details,
	}
	*sequence = *sequence + 1
	return entry
}

func parseFlexibleTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if ts, err := strconv.ParseInt(raw, 10, 64); err == nil {
		switch {
		case ts > 1_000_000_000_000:
			return time.UnixMilli(ts), true
		case ts > 0:
			return time.Unix(ts, 0), true
		}
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func renderMeetingEventsPretty(timeline meetingTimeline) string {
	var b strings.Builder
	if timeline.topic != "" {
		fmt.Fprintf(&b, "会议主题：%s\n", escapePrettyText(timeline.topic))
	}
	if timeline.hasStart || timeline.hasEnd {
		fmt.Fprintf(&b, "会议时间：%s\n", escapePrettyText(formatMeetingWindow(timeline.startTime, timeline.hasStart, timeline.endTime, timeline.hasEnd)))
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	for _, entry := range timeline.entries {
		fmt.Fprintf(&b, "[%s] ", formatTimelineOffset(entry.when, entry.hasWhen, timeline.startTime, timeline.hasStart))
		if entry.subject != "" {
			if entry.description == "" {
				fmt.Fprintln(&b, escapePrettyText(entry.subject))
				for _, detail := range entry.details {
					fmt.Fprintf(&b, "           %s\n", escapePrettyText(detail))
				}
				continue
			}
			if needsColon(entry.description) {
				fmt.Fprintf(&b, "%s: %s\n", escapePrettyText(entry.subject), escapePrettyText(entry.description))
			} else {
				fmt.Fprintf(&b, "%s %s\n", escapePrettyText(entry.subject), escapePrettyText(entry.description))
			}
			for _, detail := range entry.details {
				fmt.Fprintf(&b, "           %s\n", escapePrettyText(detail))
			}
			continue
		}
		fmt.Fprintln(&b, escapePrettyText(entry.description))
		for _, detail := range entry.details {
			fmt.Fprintf(&b, "           %s\n", escapePrettyText(detail))
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String()
}

func escapePrettyText(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if unicode.IsControl(r) {
				fmt.Fprintf(&b, "\\u%04X", r)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatMeetingWindow(start time.Time, hasStart bool, end time.Time, hasEnd bool) string {
	switch {
	case hasStart && hasEnd:
		if !end.After(start) {
			return fmt.Sprintf("%s（进行中）", start.In(meetingDisplayLocation).Format("2006-01-02 15:04:05"))
		}
		return fmt.Sprintf("%s - %s", start.In(meetingDisplayLocation).Format("2006-01-02 15:04:05"), end.In(meetingDisplayLocation).Format("2006-01-02 15:04:05"))
	case hasStart:
		return start.In(meetingDisplayLocation).Format("2006-01-02 15:04:05")
	case hasEnd:
		return end.In(meetingDisplayLocation).Format("2006-01-02 15:04:05")
	default:
		return ""
	}
}

func formatTimelineOffset(when time.Time, hasWhen bool, meetingStart time.Time, hasMeetingStart bool) string {
	if hasWhen && hasMeetingStart {
		diff := when.Sub(meetingStart)
		if diff < 0 {
			diff = 0
		}
		totalSeconds := int(diff.Seconds())
		hours := totalSeconds / 3600
		minutes := (totalSeconds % 3600) / 60
		seconds := totalSeconds % 60
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	if hasWhen {
		return when.In(meetingDisplayLocation).Format("15:04:05")
	}
	return "??:??:??"
}

func needsColon(description string) bool {
	switch description {
	case "发送了消息", "产生了转写":
		return false
	default:
		return !strings.HasPrefix(description, "加入了") &&
			!strings.HasPrefix(description, "离开了") &&
			!strings.HasPrefix(description, "被移出") &&
			!strings.HasPrefix(description, "会议结束") &&
			!strings.HasPrefix(description, "开始共享") &&
			!strings.HasPrefix(description, "结束共享")
	}
}

func leaveAction(item map[string]interface{}) string {
	switch int(common.GetFloat(item, "leave_reason")) {
	case leaveReasonMeetingEnded:
		return "因会议结束离开了会议"
	case leaveReasonKicked:
		return "被移出了会议"
	default:
		return "离开了会议"
	}
}

func meetingEventUserWithID(user map[string]interface{}) string {
	if user == nil {
		return ""
	}
	userID := common.GetString(user, "id")
	userName := common.GetString(user, "user_name")
	switch {
	case userName != "" && userID != "":
		return fmt.Sprintf("%s(%s)", userName, userID)
	case userName != "":
		return userName
	case userID != "":
		return userID
	default:
		return ""
	}
}

func meetingEventType(event map[string]interface{}) string {
	if eventType := common.GetString(event, "event_type"); eventType != "" {
		return eventType
	}
	return common.GetString(common.GetMap(event, "payload"), "activity_event_type")
}

func meetingEventSummary(event map[string]interface{}) string {
	payload := common.GetMap(event, "payload")
	eventType := meetingEventType(event)
	switch eventType {
	case "participant_joined":
		return participantJoinedSummary(payload)
	case "participant_left":
		return participantLeftSummary(payload)
	case "transcript_received":
		return transcriptReceivedSummary(payload)
	case "chat_received":
		return chatReceivedSummary(payload)
	case "magic_share_started":
		return magicShareStartedSummary(payload)
	case "magic_share_ended":
		return magicShareEndedSummary(payload)
	default:
		return fallbackMeetingEventSummary(payload, eventType)
	}
}

func participantJoinedSummary(payload map[string]interface{}) string {
	items := common.GetSlice(payload, "participant_joined_items")
	switch len(items) {
	case 0:
		return "participant joined"
	case 1:
		user := common.GetMap(firstSliceMap(payload, "participant_joined_items"), "participant")
		if label := meetingEventUserLabel(user); label != "" {
			return fmt.Sprintf("participant %s joined", label)
		}
		return "participant joined"
	default:
		return fmt.Sprintf("%d participants joined", len(items))
	}
}

func participantLeftSummary(payload map[string]interface{}) string {
	items := common.GetSlice(payload, "participant_left_items")
	switch len(items) {
	case 0:
		return "participant left"
	case 1:
		user := common.GetMap(firstSliceMap(payload, "participant_left_items"), "participant")
		if label := meetingEventUserLabel(user); label != "" {
			return fmt.Sprintf("participant %s left", label)
		}
		return "participant left"
	default:
		return fmt.Sprintf("%d participants left", len(items))
	}
}

func transcriptReceivedSummary(payload map[string]interface{}) string {
	items := common.GetSlice(payload, "transcript_received_items")
	if len(items) > 1 {
		return fmt.Sprintf("%d transcript items", len(items))
	}
	item := firstSliceMap(payload, "transcript_received_items")
	text := common.GetString(item, "text")
	speaker := meetingEventUserLabel(common.GetMap(item, "speaker"))
	switch {
	case speaker != "" && text != "":
		return fmt.Sprintf("speaker %s: %s", speaker, text)
	case speaker != "":
		return fmt.Sprintf("speaker %s transcript received", speaker)
	case text != "":
		return fmt.Sprintf("transcript: %s", text)
	default:
		return "transcript received"
	}
}

func chatReceivedSummary(payload map[string]interface{}) string {
	items := common.GetSlice(payload, "chat_received_items")
	switch len(items) {
	case 0:
		return "chat received"
	case 1:
		item := firstSliceMap(payload, "chat_received_items")
		content := common.GetString(item, "content")
		operator := meetingEventUserDisplayName(common.GetMap(item, "operator"))
		switch {
		case operator != "" && content != "":
			return fmt.Sprintf("%s: %s", operator, content)
		case operator != "":
			return fmt.Sprintf("message by %s", operator)
		case content != "":
			return fmt.Sprintf("message: %s", content)
		default:
			return "chat received"
		}
	default:
		count, operator := summarizeChatOperators(items)
		switch {
		case count == 1 && operator != "":
			return fmt.Sprintf("%d messages by %s", len(items), operator)
		case count > 1:
			return fmt.Sprintf("%d messages by %d users", len(items), count)
		default:
			return fmt.Sprintf("%d messages", len(items))
		}
	}
}

func magicShareStartedSummary(payload map[string]interface{}) string {
	items := common.GetSlice(payload, "magic_share_started_items")
	if len(items) > 1 {
		return fmt.Sprintf("%d share start events", len(items))
	}
	item := firstSliceMap(payload, "magic_share_started_items")
	shareID := common.GetString(item, "share_id")
	title := common.GetString(common.GetMap(item, "share_doc"), "title")
	switch {
	case shareID != "" && title != "":
		return fmt.Sprintf("share %s started: %s", shareID, title)
	case shareID != "":
		return fmt.Sprintf("share %s started", shareID)
	case title != "":
		return fmt.Sprintf("share started: %s", title)
	default:
		return "share started"
	}
}

func magicShareEndedSummary(payload map[string]interface{}) string {
	items := common.GetSlice(payload, "magic_share_ended_items")
	if len(items) > 1 {
		return fmt.Sprintf("%d share end events", len(items))
	}
	item := firstSliceMap(payload, "magic_share_ended_items")
	if shareID := common.GetString(item, "share_id"); shareID != "" {
		return fmt.Sprintf("share %s ended", shareID)
	}
	return "share ended"
}

func fallbackMeetingEventSummary(payload map[string]interface{}, eventType string) string {
	meeting := common.GetMap(payload, "meeting")
	if topic := common.GetString(meeting, "topic"); topic != "" {
		if eventType != "" {
			return fmt.Sprintf("%s: %s", eventType, topic)
		}
		return topic
	}
	if eventType != "" {
		return eventType
	}
	return "meeting event"
}

func firstSliceMap(payload map[string]interface{}, key string) map[string]interface{} {
	items := common.GetSlice(payload, key)
	if len(items) == 0 {
		return nil
	}
	first, _ := items[0].(map[string]interface{})
	return first
}

func meetingEventUserLabel(user map[string]interface{}) string {
	if user == nil {
		return ""
	}
	userID := common.GetString(user, "id")
	userName := common.GetString(user, "user_name")
	switch {
	case userID != "" && userName != "":
		return fmt.Sprintf("%s (%s)", userID, userName)
	case userID != "":
		return userID
	case userName != "":
		return userName
	default:
		return ""
	}
}

func meetingEventUserDisplayName(user map[string]interface{}) string {
	if user == nil {
		return ""
	}
	if userName := common.GetString(user, "user_name"); userName != "" {
		return userName
	}
	return common.GetString(user, "id")
}

func chatMessageTypeLabel(item map[string]interface{}) string {
	code := int(common.GetFloat(item, "message_type"))
	switch code {
	case 1:
		return "text"
	case 2:
		return "system"
	case 3:
		return "reaction"
	case 4:
		return "encrypted"
	default:
		return "unknown"
	}
}

func summarizeChatOperators(items []interface{}) (int, string) {
	seen := make(map[string]struct{}, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		if item == nil {
			continue
		}
		operator := meetingEventUserDisplayName(common.GetMap(item, "operator"))
		if operator == "" {
			continue
		}
		seen[operator] = struct{}{}
	}
	if len(seen) != 1 {
		return len(seen), ""
	}
	for operator := range seen {
		return 1, operator
	}
	return 0, ""
}
