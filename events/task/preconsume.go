// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

const taskSubscriptionPath = "/open-apis/task/v2/task_v2/task_subscription?user_id_type=open_id"

func taskSubscriptionPreConsume(ctx context.Context, rt event.APIClient, _ map[string]string) (func() error, error) {
	if rt == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"runtime API client is required for pre-consume subscription")
	}

	if _, err := rt.CallAPI(ctx, "POST", taskSubscriptionPath, nil); err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewNetworkError(
			errs.SubtypeNetworkTransport,
			"failed to subscribe task event",
		).WithCause(err)
	}

	return nil, nil
}
