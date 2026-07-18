// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

type approvalEventType string
type approvalSubscriptionPath string

type approvalSubscriptionConfig struct {
	eventType     approvalEventType
	subscribePath approvalSubscriptionPath
}

func approvalSubscriptionPreConsume(cfg approvalSubscriptionConfig) func(context.Context, event.APIClient, map[string]string) (func() error, error) {
	return func(ctx context.Context, rt event.APIClient, params map[string]string) (func() error, error) {
		if rt == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"runtime API client is required for pre-consume subscription")
		}

		eventType := string(cfg.eventType)
		subscribePath := string(cfg.subscribePath)
		subscriptionTypes, err := approvalSubscriptionTypes(eventType, params)
		if err != nil {
			return nil, err
		}

		registered := make([]string, 0, len(subscriptionTypes))
		for _, subscriptionType := range subscriptionTypes {
			body := map[string]string{"subscription_type": subscriptionType}
			if _, err := rt.CallAPI(ctx, "POST", subscribePath, body); err != nil {
				return nil, approvalSubscriptionRegistrationError(eventType, registered, subscriptionType, err)
			}
			registered = append(registered, subscriptionType)
		}

		// Approval subscriptions are durable user-auth relations. Consuming events
		// should not cancel that relation when this local process exits.
		return nil, nil
	}
}

func approvalSubscriptionTypes(eventType string, params map[string]string) ([]string, error) {
	raw := strings.TrimSpace(params["subscription_type"])
	if raw == "" {
		return append([]string(nil), approvalAllSubscriptionTypes...), nil
	}

	values, err := parseApprovalSubscriptionTypeValues(raw)
	if err != nil {
		return nil, invalidApprovalSubscriptionTypeError(eventType, raw)
	}

	selected := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		switch value {
		case approvalSubscriptionTypeInvolved, approvalSubscriptionTypeManaged:
			selected[value] = true
		default:
			return nil, invalidApprovalSubscriptionTypeError(eventType, value)
		}
	}

	result := make([]string, 0, len(selected))
	for _, value := range approvalAllSubscriptionTypes {
		if selected[value] {
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return nil, invalidApprovalSubscriptionTypeError(eventType, raw)
	}
	return result, nil
}

func parseApprovalSubscriptionTypeValues(raw string) ([]string, error) {
	if strings.HasPrefix(raw, "[") {
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, err
		}
		return values, nil
	}
	return strings.Split(raw, ","), nil
}

func approvalSubscriptionRegistrationError(eventType string, registered []string, failed string, err error) error {
	if err == nil {
		return nil
	}

	msg := fmt.Sprintf(
		"approval subscription pre-consume failed for EventKey %s: failed subscription_type %s",
		eventType,
		failed,
	)
	hint := fmt.Sprintf(
		"no approval subscription relation was registered for EventKey %s; fix the cause and retry",
		eventType,
	)
	if len(registered) > 0 {
		msg = fmt.Sprintf(
			"approval subscription pre-consume partially completed for EventKey %s: registered subscription_type(s) [%s], failed subscription_type %s",
			eventType,
			strings.Join(registered, ", "),
			failed,
		)
		hint = fmt.Sprintf(
			"server-side approval subscription relation(s) already registered for EventKey %s: %s; after fixing the cause, retry with --param subscription_type=%s to register the failed relation",
			eventType,
			strings.Join(registered, ", "),
			failed,
		)
	}

	if p, ok := errs.ProblemOf(err); ok {
		if upstream := strings.TrimSpace(p.Message); upstream != "" {
			p.Message = msg + ": " + upstream
		} else {
			p.Message = msg
		}
		if upstreamHint := strings.TrimSpace(p.Hint); upstreamHint != "" {
			p.Hint = upstreamHint + "\n" + hint
		} else {
			p.Hint = hint
		}
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "%s: %v", msg, err).
		WithHint("%s", hint).
		WithCause(err)
}

func invalidApprovalSubscriptionTypeError(eventType, value string) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"invalid subscription_type for EventKey %s: %q", eventType, value).
		WithParam("--param").
		WithHint("omit subscription_type to register both approval subscription relations, or pass --param subscription_type=%s, --param subscription_type=%s, or --param subscription_type=%s,%s; run `lark-cli event schema %s` for details",
			approvalSubscriptionTypeInvolved,
			approvalSubscriptionTypeManaged,
			approvalSubscriptionTypeInvolved,
			approvalSubscriptionTypeManaged,
			eventType)
}
