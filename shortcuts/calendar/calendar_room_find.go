// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	roomFindPath    = "/open-apis/calendar/v4/freebusy/room_find"
	roomFindWorkers = 10
	flagSlot        = "slot"
	flagCity        = "city"
	flagBuilding    = "building"
	flagFloor       = "floor"
	flagRoomName    = "room-name"
	flagMinCapacity = "min-capacity"
	flagMaxCapacity = "max-capacity"
)

type roomFindRequest struct {
	City            string   `json:"city,omitempty"`
	Building        string   `json:"building,omitempty"`
	Floor           string   `json:"floor,omitempty"`
	RoomName        string   `json:"room_name,omitempty"`
	MinCapacity     int      `json:"min_capacity,omitempty"`
	MaxCapacity     int      `json:"max_capacity,omitempty"`
	EventStartTime  string   `json:"event_start_time,omitempty"`
	EventEndTime    string   `json:"event_end_time,omitempty"`
	AttendeeUserIDs []string `json:"attendee_user_ids,omitempty"`
	AttendeeChatIDs []string `json:"attendee_chat_ids,omitempty"`
	EventRrule      string   `json:"event_rrule,omitempty"`
	Timezone        string   `json:"timezone,omitempty"`
}

type roomFindSuggestion struct {
	RoomID           string `json:"room_id,omitempty"`
	RoomName         string `json:"room_name,omitempty"`
	Capacity         int    `json:"capacity,omitempty"`
	ReserveUntilTime string `json:"reserve_until_time,omitempty"`
}

type roomFindData struct {
	AvailableRooms []*roomFindSuggestion `json:"available_rooms,omitempty"`
}

type roomFindSlot struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

type roomFindTimeSlot struct {
	Start        string                `json:"start,omitempty"`
	End          string                `json:"end,omitempty"`
	MeetingRooms []*roomFindSuggestion `json:"meeting_rooms,omitempty"`
}

type roomFindOutput struct {
	TimeSlots []*roomFindTimeSlot `json:"time_slots,omitempty"`
}

func collectRoomFindResults(slots []roomFindSlot, limit int, fetch func(roomFindSlot) ([]*roomFindSuggestion, error)) (*roomFindOutput, error) {
	if limit <= 0 {
		limit = 1
	}

	out := &roomFindOutput{
		TimeSlots: make([]*roomFindTimeSlot, 0, len(slots)),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, limit)

	for _, slot := range slots {
		wg.Add(1)
		sem <- struct{}{}
		go func(slot roomFindSlot) {
			defer wg.Done()
			defer func() { <-sem }()

			suggestions, err := fetch(slot)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			out.TimeSlots = append(out.TimeSlots, &roomFindTimeSlot{
				Start:        slot.Start,
				End:          slot.End,
				MeetingRooms: suggestions,
			})
		}(slot)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	sort.Slice(out.TimeSlots, func(i, j int) bool {
		return out.TimeSlots[i].Start < out.TimeSlots[j].Start
	})

	return out, nil
}

func parseRoomFindSlots(runtime *common.RuntimeContext) ([]roomFindSlot, error) {
	rawSlots := runtime.StrArray(flagSlot)
	if len(rawSlots) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "specify at least one --slot").WithParam("--slot")
	}
	slots := make([]roomFindSlot, 0, len(rawSlots))
	for _, raw := range rawSlots {
		parts := strings.Split(strings.TrimSpace(raw), "~")
		if len(parts) != 2 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --slot format %q, expected start~end", raw).WithParam("--slot")
		}
		startTs, err := common.ParseTime(parts[0])
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid slot start time %q: %v", parts[0], err).WithParam("--slot")
		}
		endTs, err := common.ParseTime(parts[1])
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid slot end time %q: %v", parts[1], err).WithParam("--slot")
		}
		startSec, err := strconv.ParseInt(startTs, 10, 64)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid slot start timestamp %q: %v", startTs, err).WithParam("--slot")
		}
		endSec, err := strconv.ParseInt(endTs, 10, 64)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid slot end timestamp %q: %v", endTs, err).WithParam("--slot")
		}
		if endSec <= startSec {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--slot end time must be after start time: %q", raw).WithParam("--slot")
		}
		startRFC3339, err := unixStringToRFC3339(startTs)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid slot start timestamp %q: %v", startTs, err).WithParam("--slot")
		}
		endRFC3339, err := unixStringToRFC3339(endTs)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid slot end timestamp %q: %v", endTs, err).WithParam("--slot")
		}
		slots = append(slots, roomFindSlot{Start: startRFC3339, End: endRFC3339})
	}
	return slots, nil
}

func unixStringToRFC3339(ts string) (string, error) {
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return "", err
	}
	return time.Unix(sec, 0).Format(time.RFC3339), nil
}

func parseRoomFindAttendees(attendeesStr string, currentUserID string) ([]string, []string, error) {
	var userIDs []string
	var chatIDs []string
	seenUsers := map[string]bool{}
	seenChats := map[string]bool{}
	for _, id := range strings.Split(attendeesStr, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		switch {
		case strings.HasPrefix(id, "ou_"):
			if !seenUsers[id] {
				userIDs = append(userIDs, id)
				seenUsers[id] = true
			}
		case strings.HasPrefix(id, "oc_"):
			if !seenChats[id] {
				chatIDs = append(chatIDs, id)
				seenChats[id] = true
			}
		default:
			return nil, nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid attendee id format %q: should start with 'ou_' or 'oc_'", id).WithParam("--" + flagAttendees)
		}
	}
	if currentUserID != "" && !seenUsers[currentUserID] {
		userIDs = append(userIDs, currentUserID)
	}
	return userIDs, chatIDs, nil
}

func normalizeCommaSeparatedNames(raw string) string {
	parts := strings.Split(raw, ",")
	var cleaned []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			cleaned = append(cleaned, p)
		}
	}
	return strings.Join(cleaned, ",")
}

func buildRoomFindBaseRequest(runtime *common.RuntimeContext) (*roomFindRequest, error) {
	req := &roomFindRequest{
		City:        strings.TrimSpace(runtime.Str(flagCity)),
		Building:    strings.TrimSpace(runtime.Str(flagBuilding)),
		Floor:       strings.TrimSpace(runtime.Str(flagFloor)),
		RoomName:    normalizeCommaSeparatedNames(runtime.Str(flagRoomName)),
		MinCapacity: runtime.Int(flagMinCapacity),
		MaxCapacity: runtime.Int(flagMaxCapacity),
		Timezone:    strings.TrimSpace(runtime.Str(flagTimezone)),
		EventRrule:  strings.TrimSpace(runtime.Str(flagEventRrule)),
	}

	currentUserID := ""
	if !runtime.IsBot() {
		currentUserID = runtime.UserOpenId()
	}
	attendeeUserIDs, attendeeChatIDs, err := parseRoomFindAttendees(runtime.Str(flagAttendees), currentUserID)
	if err != nil {
		return nil, err
	}
	req.AttendeeUserIDs = attendeeUserIDs
	req.AttendeeChatIDs = attendeeChatIDs
	return req, nil
}

func callRoomFind(runtime *common.RuntimeContext, req *roomFindRequest) ([]*roomFindSuggestion, error) {
	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: "POST",
		ApiPath:    roomFindPath,
		Body:       req,
	})
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.WrapInternal(err)
	}

	if _, err := runtime.ClassifyAPIResponse(apiResp); err != nil {
		return nil, err
	}

	var resp = &OpenAPIResponse[*roomFindData]{}
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "unmarshal response fail").WithCause(err)
	}

	if resp.Data != nil {
		return resp.Data.AvailableRooms, nil
	}
	return nil, nil
}

var CalendarRoomFind = common.Shortcut{
	Service:     "calendar",
	Command:     "+room-find",
	Description: "Find available meeting room candidates for one or more event time slots",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.free_busy:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: flagSlot, Type: "string_array", Desc: "event time slot in start~end format; repeatable"},
		{Name: flagCity, Type: "string", Desc: "meeting room city constraint"},
		{Name: flagBuilding, Type: "string", Desc: "meeting room building constraint"},
		{Name: flagFloor, Type: "string", Desc: "meeting room floor constraint (e.g., F2)"},
		{Name: flagRoomName, Type: "string", Desc: "meeting room name constraint; comma-separated for multiple names (e.g., 01,02,03)"},
		{Name: flagMinCapacity, Type: "int", Desc: "minimum meeting room capacity"},
		{Name: flagMaxCapacity, Type: "int", Desc: "maximum meeting room capacity"},
		{Name: flagAttendees, Type: "string", Desc: "attendee IDs, comma-separated (supports user ou_, chat oc_)"},
		{Name: flagEventRrule, Type: "string", Desc: "event recurrence rule"},
		{Name: flagTimezone, Type: "string", Desc: "current time zone"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		baseReq, err := buildRoomFindBaseRequest(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		slots, err := parseRoomFindSlots(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		d := common.NewDryRunAPI()
		for _, slot := range slots {
			req := *baseReq
			req.EventStartTime = slot.Start
			req.EventEndTime = slot.End
			d.POST(roomFindPath).
				Desc(fmt.Sprintf("Lookup meeting room suggestions for %s - %s", slot.Start, slot.End)).
				Body(req)
		}
		return d
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		for _, flag := range []string{flagCity, flagBuilding, flagFloor, flagEventRrule, flagTimezone} {
			if val := strings.TrimSpace(runtime.Str(flag)); val != "" {
				if err := common.RejectDangerousCharsTyped("--"+flag, val); err != nil {
					return err
				}
			}
		}
		for _, name := range strings.Split(runtime.Str(flagRoomName), ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if err := common.RejectDangerousCharsTyped("--"+flagRoomName, name); err != nil {
				return err
			}
		}
		if _, err := parseRoomFindSlots(runtime); err != nil {
			return err
		}
		if _, _, err := parseRoomFindAttendees(runtime.Str(flagAttendees), ""); err != nil {
			return err
		}
		if minCapacity := runtime.Int(flagMinCapacity); minCapacity < 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--min-capacity must be >= 0").WithParam("--min-capacity")
		}
		if maxCapacity := runtime.Int(flagMaxCapacity); maxCapacity < 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--max-capacity must be >= 0").WithParam("--max-capacity")
		}
		if minCapacity, maxCapacity := runtime.Int(flagMinCapacity), runtime.Int(flagMaxCapacity); minCapacity > 0 && maxCapacity > 0 && minCapacity > maxCapacity {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--min-capacity must be <= --max-capacity").WithParam("--min-capacity")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseReq, err := buildRoomFindBaseRequest(runtime)
		if err != nil {
			return err
		}
		slots, err := parseRoomFindSlots(runtime)
		if err != nil {
			return err
		}

		out, err := collectRoomFindResults(slots, roomFindWorkers, func(slot roomFindSlot) ([]*roomFindSuggestion, error) {
			req := *baseReq
			req.EventStartTime = slot.Start
			req.EventEndTime = slot.End
			return callRoomFind(runtime, &req)
		})
		if err != nil {
			return err
		}

		runtime.OutFormat(out, &output.Meta{Count: len(out.TimeSlots)}, func(w io.Writer) {
			if len(out.TimeSlots) == 0 {
				fmt.Fprintln(w, "No meeting room suggestions available.")
				return
			}
			for _, slot := range out.TimeSlots {
				fmt.Fprintf(w, "%s - %s\n", slot.Start, slot.End)
				var rows []map[string]interface{}
				for _, room := range slot.MeetingRooms {
					rows = append(rows, map[string]interface{}{
						"room_id":            room.RoomID,
						"room_name":          room.RoomName,
						"capacity":           room.Capacity,
						"reserve_until_time": room.ReserveUntilTime,
					})
				}
				output.PrintTable(w, rows)
				fmt.Fprintln(w)
			}
		})
		return nil
	},
}
