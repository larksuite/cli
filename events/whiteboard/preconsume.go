// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/events/internal/subscribeprep"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/validate"
)

// whiteboardSubscriptionPreConsume calls the whiteboard event subscribe OAPI
// and returns a cleanup that invokes the matching unsubscribe.
//
// board.whiteboard.updated_v1 is subscribed per-whiteboard (by whiteboard_id),
// so the path contains a :whiteboard_id placeholder that must be supplied via params.
func whiteboardSubscriptionPreConsume(eventType string) func(context.Context, event.APIClient, map[string]string) (func() error, error) {
	return func(ctx context.Context, rt event.APIClient, params map[string]string) (func() error, error) {
		if rt == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"runtime API client is required for pre-consume subscription")
		}
		whiteboardID := params["whiteboard_id"]
		if whiteboardID == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"param whiteboard_id is required for %s", eventType).
				WithParam("--param").
				WithHint("pass it as --param whiteboard_id=<id>; run `lark-cli event schema %s` for details", eventType)
		}
		encoded := validate.EncodePathSegment(whiteboardID)
		subscribePath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/subscribe", encoded)
		unsubscribePath := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/unsubscribe", encoded)

		return subscribeprep.SubscribeWithCleanup(ctx, rt, eventType, subscribePath, unsubscribePath)
	}
}
