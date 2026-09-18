// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// calendar +list-attendees — list attendees of a single calendar event with type filter.

package calendar

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// attendeeType is one of the four upstream attendee types.
type attendeeType string

const (
	attendeeTypeUser       attendeeType = "user"
	attendeeTypeResource   attendeeType = "resource"
	attendeeTypeChat       attendeeType = "chat"
	attendeeTypeThirdParty attendeeType = "third_party"
)

var validAttendeeTypes = map[string]struct{}{
	string(attendeeTypeUser):       {},
	string(attendeeTypeResource):   {},
	string(attendeeTypeChat):       {},
	string(attendeeTypeThirdParty): {},
}

// listAttendeesOutput is the structured output for +list-attendees.
type listAttendeesOutput struct {
	Attendees []map[string]interface{} `json:"attendees"`
	HasMore   bool                     `json:"has_more"`
	PageToken string                   `json:"page_token"`
}

// projectAttendee copies the fields relevant to an attendee's `type`, dropping
// upstream fields that do not apply. It keeps upstream ordering by preserving
// the incoming map keys via explicit copy.
func projectAttendee(a map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}

	// Always-present shared fields.
	if v, ok := a["type"].(string); ok && v != "" {
		out["type"] = v
	}
	if v, ok := a["display_name"].(string); ok && v != "" {
		out["display_name"] = v
	}
	// rsvp_status is meaningful for user / resource / third_party attendees only.
	// Chat attendees don't carry a group-level RSVP — the "rsvp_status" upstream
	// returns for a chat entry is not semantically valid; the per-member RSVP is
	// retrieved separately via event.attendees.chat_members.list.
	attendeeT := attendeeType(strings.TrimSpace(strings.ToLower(fmt.Sprint(a["type"]))))
	if attendeeT != attendeeTypeChat {
		if v, ok := a["rsvp_status"].(string); ok && v != "" {
			out["rsvp_status"] = v
		}
	}
	if v, ok := a["is_optional"].(bool); ok && v {
		out["is_optional"] = v
	}
	if v, ok := a["is_organizer"].(bool); ok && v {
		out["is_organizer"] = v
	}
	if v, ok := a["is_external"].(bool); ok {
		out["is_external"] = v
	}
	if v, ok := a["operate_id"].(string); ok && v != "" {
		out["operate_id"] = v
	}

	// Type-specific fields.
	switch attendeeT {
	case attendeeTypeUser:
		if v, ok := a["user_id"].(string); ok && v != "" {
			out["user_id"] = v
		}
	case attendeeTypeResource:
		if v, ok := a["room_id"].(string); ok && v != "" {
			out["room_id"] = v
		}
		if v, ok := a["resource_customization"]; ok && v != nil {
			out["resource_customization"] = v
		}
	case attendeeTypeChat:
		if v, ok := a["chat_id"].(string); ok && v != "" {
			out["chat_id"] = v
		}
	case attendeeTypeThirdParty:
		if v, ok := a["third_party_email"].(string); ok && v != "" {
			out["third_party_email"] = v
		}
	}
	return out
}

// normalizeTypeFilter deduplicates and validates the --type flag values.
func normalizeTypeFilter(rawTypes []string) (map[string]struct{}, error) {
	if len(rawTypes) == 0 {
		return nil, nil
	}
	out := make(map[string]struct{}, len(rawTypes))
	for _, raw := range rawTypes {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := validAttendeeTypes[v]; !ok {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"invalid --type %q, allowed: user, resource, chat, third_party", v).
				WithParam("--type")
		}
		out[v] = struct{}{}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

const (
	// listAttendeesDefaultPageSize is used when --page-size is not provided.
	listAttendeesDefaultPageSize = 20
	// listAttendeesMinPageSize is the floor for --page-size. Values below this
	// are silently raised to the floor and reported on stderr as a hint.
	listAttendeesMinPageSize = 10
	// listAttendeesMaxPageSize is the ceiling for --page-size. Values above this
	// are silently clamped to the ceiling and reported on stderr as a hint.
	listAttendeesMaxPageSize = 100
)

// resolveListAttendeesPageSize decides the effective page_size sent upstream
// and returns an optional stderr hint when the caller's value was clamped to
// the [min, max] range. Not providing --page-size falls back to the default; a
// negative value is rejected by Validate before Execute reaches this helper.
func resolveListAttendeesPageSize(runtime *common.RuntimeContext) (int, string) {
	if !runtime.Changed("page-size") {
		return listAttendeesDefaultPageSize, ""
	}
	requested := runtime.Int("page-size")
	if requested < listAttendeesMinPageSize {
		return listAttendeesMinPageSize, fmt.Sprintf(
			"[calendar +list-attendees] --page-size %d is below the minimum %d; using %d instead",
			requested, listAttendeesMinPageSize, listAttendeesMinPageSize,
		)
	}
	if requested > listAttendeesMaxPageSize {
		return listAttendeesMaxPageSize, fmt.Sprintf(
			"[calendar +list-attendees] --page-size %d exceeds the maximum %d; using %d instead",
			requested, listAttendeesMaxPageSize, listAttendeesMaxPageSize,
		)
	}
	return requested, ""
}

// CalendarListAttendees lists attendees of a calendar event.
var CalendarListAttendees = common.Shortcut{
	Service:     "calendar",
	Command:     "+list-attendees",
	Description: "List attendees of a calendar event; supports --type filter and page-token pagination",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.event:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
		{Name: "event-id", Desc: "event ID", Required: true},
		{Name: "type", Type: "string_slice", Desc: "filter by attendee type; repeatable or comma-separated (user|resource|chat|third_party); empty means all"},
		{Name: "page-size", Type: "int", Desc: fmt.Sprintf("upstream page size; range [%d, %d] (values outside are clamped), default %d", listAttendeesMinPageSize, listAttendeesMaxPageSize, listAttendeesDefaultPageSize)},
		{Name: "page-token", Desc: "upstream page token for the next page"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		for _, flag := range []string{"calendar-id", "event-id", "page-token"} {
			if val := strings.TrimSpace(runtime.Str(flag)); val != "" {
				if err := common.RejectDangerousCharsTyped("--"+flag, val); err != nil {
					return err
				}
			}
		}
		eventId := strings.TrimSpace(runtime.Str("event-id"))
		if eventId == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "event-id cannot be empty").WithParam("--event-id")
		}
		if _, err := normalizeTypeFilter(runtime.StrSlice("type")); err != nil {
			return err
		}
		if pageSize := runtime.Int("page-size"); pageSize < 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-size must be a non-negative integer").WithParam("--page-size")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		calendarId := strings.TrimSpace(runtime.Str("calendar-id"))
		d := common.NewDryRunAPI()
		switch calendarId {
		case "":
			d.Desc("(calendar-id omitted) Will use primary calendar")
			calendarId = "<primary>"
		case "primary":
			calendarId = "<primary>"
		}
		eventId := strings.TrimSpace(runtime.Str("event-id"))
		pageSize, _ := resolveListAttendeesPageSize(runtime)
		params := map[string]interface{}{
			"page_size": pageSize,
		}
		if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
			params["page_token"] = pageToken
		}
		return d.
			GET("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id/attendees").
			Params(params).
			Set("calendar_id", calendarId).
			Set("event_id", eventId)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		calendarId := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarId == "" {
			calendarId = PrimaryCalendarIDStr
		}
		eventId := strings.TrimSpace(runtime.Str("event-id"))

		allowed, err := normalizeTypeFilter(runtime.StrSlice("type"))
		if err != nil {
			return err
		}

		pageSize, hint := resolveListAttendeesPageSize(runtime)
		if hint != "" {
			fmt.Fprintln(runtime.IO().ErrOut, hint)
		}
		params := map[string]interface{}{
			"page_size": pageSize,
		}
		if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
			params["page_token"] = pageToken
		}

		data, err := runtime.CallAPITyped("GET",
			fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s/attendees",
				validate.EncodePathSegment(calendarId),
				validate.EncodePathSegment(eventId)),
			params, nil)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}

		items := common.GetSlice(data, "items")
		hasMore, _ := data["has_more"].(bool)
		pageToken, _ := data["page_token"].(string)

		attendees := make([]map[string]interface{}, 0, len(items))
		for _, raw := range items {
			item, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if len(allowed) > 0 {
				t, _ := item["type"].(string)
				if _, keep := allowed[t]; !keep {
					continue
				}
			}
			attendees = append(attendees, projectAttendee(item))
		}

		out := listAttendeesOutput{
			Attendees: attendees,
			HasMore:   hasMore,
			PageToken: pageToken,
		}

		runtime.OutFormat(out, &output.Meta{Count: len(attendees)}, func(w io.Writer) {
			if len(attendees) == 0 {
				fmt.Fprintln(w, "No attendees found.")
				return
			}

			// Group by type for pretty rendering while JSON stays flat.
			groups := map[string][]map[string]interface{}{}
			order := []string{string(attendeeTypeResource), string(attendeeTypeUser), string(attendeeTypeChat), string(attendeeTypeThirdParty)}
			for _, a := range attendees {
				t, _ := a["type"].(string)
				groups[t] = append(groups[t], a)
			}
			titles := map[string]string{
				string(attendeeTypeResource):   "rooms",
				string(attendeeTypeUser):       "users",
				string(attendeeTypeChat):       "chats",
				string(attendeeTypeThirdParty): "third_parties",
			}
			for _, t := range order {
				group := groups[t]
				if len(group) == 0 {
					continue
				}
				fmt.Fprintf(w, "%s (%d)\n", titles[t], len(group))
				var rows []map[string]interface{}
				for _, a := range group {
					row := map[string]interface{}{
						"display_name": a["display_name"],
					}
					if attendeeType(t) != attendeeTypeChat {
						row["rsvp_status"] = a["rsvp_status"]
					}
					switch attendeeType(t) {
					case attendeeTypeUser:
						row["user_id"] = a["user_id"]
					case attendeeTypeResource:
						row["room_id"] = a["room_id"]
					case attendeeTypeChat:
						row["chat_id"] = a["chat_id"]
					case attendeeTypeThirdParty:
						row["third_party_email"] = a["third_party_email"]
					}
					rows = append(rows, row)
				}
				output.PrintTable(w, rows)
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "%d attendee(s) total", len(attendees))
			if hasMore {
				fmt.Fprintf(w, "; more available, page_token: %s", pageToken)
			}
			fmt.Fprintln(w)
		})
		return nil
	},
}
