// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package approval registers Approval-domain EventKeys.
package approval

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/larksuite/cli/internal/event"
)

const (
	eventTypeApprovalInstanceStatusChangedV4 = "approval.instance.status_changed_v4"
	eventTypeApprovalTaskStatusChangedV4     = "approval.task.status_changed_v4"

	pathApprovalInstancesSubscription = "/open-apis/approval/v4/instances/subscription"
	pathApprovalTasksSubscription     = "/open-apis/approval/v4/tasks/subscription"

	approvalSubscriptionTypeInvolved = "INVOLVED_APPROVAL"
	approvalSubscriptionTypeManaged  = "MANAGED_APPROVAL"
)

var approvalAllSubscriptionTypes = []string{
	approvalSubscriptionTypeInvolved,
	approvalSubscriptionTypeManaged,
}

// Keys returns all Approval-domain EventKey definitions.
func Keys() []event.KeyDefinition {
	return []event.KeyDefinition{
		{
			Key:         eventTypeApprovalInstanceStatusChangedV4,
			DisplayName: "Approval instance status changed",
			Description: "Triggered after an approval instance status becomes visible to the requester or approval participants",
			EventType:   eventTypeApprovalInstanceStatusChangedV4,
			Params:      approvalSubscriptionParams(),
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(ApprovalInstanceStatusChangedV4Output{})},
			},
			Process: processApprovalInstanceStatusChanged,
			PreConsume: approvalSubscriptionPreConsume(approvalSubscriptionConfig{
				eventType:     eventTypeApprovalInstanceStatusChangedV4,
				subscribePath: pathApprovalInstancesSubscription,
			}),
			Scopes: []string{"approval:instance:read"},
			AuthTypes: []string{
				"user",
			},
			RequiredConsoleEvents: []string{eventTypeApprovalInstanceStatusChangedV4},
		},
		{
			Key:         eventTypeApprovalTaskStatusChangedV4,
			DisplayName: "Approval task status changed",
			Description: "Triggered after an approval task status becomes visible to the requester or task approver",
			EventType:   eventTypeApprovalTaskStatusChangedV4,
			Params:      approvalSubscriptionParams(),
			Schema: event.SchemaDef{
				Custom: &event.SchemaSpec{Type: reflect.TypeOf(ApprovalTaskStatusChangedV4Output{})},
			},
			Process: processApprovalTaskStatusChanged,
			PreConsume: approvalSubscriptionPreConsume(approvalSubscriptionConfig{
				eventType:     eventTypeApprovalTaskStatusChangedV4,
				subscribePath: pathApprovalTasksSubscription,
			}),
			Scopes: []string{"approval:task:read"},
			AuthTypes: []string{
				"user",
			},
			RequiredConsoleEvents: []string{eventTypeApprovalTaskStatusChangedV4},
		},
	}
}

func approvalSubscriptionParams() []event.ParamDef {
	return []event.ParamDef{
		{
			Name:        "subscription_type",
			Type:        event.ParamMulti,
			Description: "Approval subscription relation type(s) to register for the current authorized user. Omit to register both involved and managed approval relations.",
			Values: []event.ParamValue{
				{
					Value: approvalSubscriptionTypeInvolved,
					Desc:  "Receive events where the current user is the approval requester or approver.",
				},
				{
					Value: approvalSubscriptionTypeManaged,
					Desc:  "Receive events under approval definitions managed by the current user.",
				},
			},
		},
	}
}

func processApprovalInstanceStatusChanged(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	if raw == nil {
		return nil, nil
	}
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event struct {
			ApprovalCode string          `json:"approval_code"`
			InstanceCode string          `json:"instance_code"`
			ExternalID   string          `json:"external_id"`
			Status       string          `json:"status"`
			OperateTime  string          `json:"operate_time"`
			StartUser    *ApprovalUserID `json:"start_user"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	out := &ApprovalInstanceStatusChangedV4Output{
		Type:         envelope.Header.EventType,
		EventID:      envelope.Header.EventID,
		Timestamp:    envelope.Header.CreateTime,
		ApprovalCode: envelope.Event.ApprovalCode,
		InstanceCode: envelope.Event.InstanceCode,
		ExternalID:   envelope.Event.ExternalID,
		Status:       envelope.Event.Status,
		OperateTime:  envelope.Event.OperateTime,
		StartUser:    envelope.Event.StartUser,
	}
	if out.Type == "" {
		out.Type = raw.EventType
	}
	return json.Marshal(out)
}

func processApprovalTaskStatusChanged(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	if raw == nil {
		return nil, nil
	}
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event struct {
			ApprovalCode   string          `json:"approval_code"`
			InstanceCode   string          `json:"instance_code"`
			TaskID         string          `json:"task_id"`
			ExternalID     string          `json:"external_id"`
			TaskExternalID string          `json:"task_external_id"`
			AssignedUser   *ApprovalUserID `json:"assigned_user"`
			Status         string          `json:"status"`
			OperateTime    string          `json:"operate_time"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	out := &ApprovalTaskStatusChangedV4Output{
		Type:           envelope.Header.EventType,
		EventID:        envelope.Header.EventID,
		Timestamp:      envelope.Header.CreateTime,
		ApprovalCode:   envelope.Event.ApprovalCode,
		InstanceCode:   envelope.Event.InstanceCode,
		TaskID:         envelope.Event.TaskID,
		ExternalID:     envelope.Event.ExternalID,
		TaskExternalID: envelope.Event.TaskExternalID,
		AssignedUser:   envelope.Event.AssignedUser,
		Status:         envelope.Event.Status,
		OperateTime:    envelope.Event.OperateTime,
	}
	if out.Type == "" {
		out.Type = raw.EventType
	}
	return json.Marshal(out)
}
