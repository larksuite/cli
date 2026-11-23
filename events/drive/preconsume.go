// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

const commentSubscriptionPath = "/open-apis/drive/v1/user/subscription"

// commentSubscriptionPreConsume registers the current identity (user or bot)
// for comment notices. The server keys the record by (app, identity,
// event_type) rather than by document, so it is a durable relation shared by
// every consumer of this app and identity: unsubscribing on exit would cut
// off other consumers, and re-subscribing on every start is idempotent.
func commentSubscriptionPreConsume(ctx context.Context, rt event.APIClient, _ map[string]string) (func() error, error) {
	if rt == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"runtime API client is required for pre-consume subscription")
	}

	body := map[string]string{"event_type": eventTypeCommentAddV1}
	if _, err := rt.CallAPI(ctx, "POST", commentSubscriptionPath, body); err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewNetworkError(
			errs.SubtypeNetworkTransport,
			"failed to subscribe drive comment notice event",
		).WithCause(err)
	}

	return nil, nil
}
