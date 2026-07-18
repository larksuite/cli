// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package approval

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/schemas"
)

type recordedCall struct {
	method string
	path   string
	body   interface{}
}

type fakeAPIClient struct {
	calls     []recordedCall
	err       error
	errOnCall int
}

func (f *fakeAPIClient) CallAPI(_ context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	f.calls = append(f.calls, recordedCall{method: method, path: path, body: body})
	if f.err != nil && (f.errOnCall == 0 || f.errOnCall == len(f.calls)) {
		return nil, f.err
	}
	return json.RawMessage(`{}`), nil
}

func TestKeysApprovalMetadata(t *testing.T) {
	keys := Keys()
	if len(keys) != 2 {
		t.Fatalf("len(Keys()) = %d, want 2", len(keys))
	}

	tests := []struct {
		key        string
		scope      string
		schemaType reflect.Type
		subscribe  string
	}{
		{
			key:        eventTypeApprovalInstanceStatusChangedV4,
			scope:      "approval:instance:read",
			schemaType: reflect.TypeOf(ApprovalInstanceStatusChangedV4Output{}),
			subscribe:  pathApprovalInstancesSubscription,
		},
		{
			key:        eventTypeApprovalTaskStatusChangedV4,
			scope:      "approval:task:read",
			schemaType: reflect.TypeOf(ApprovalTaskStatusChangedV4Output{}),
			subscribe:  pathApprovalTasksSubscription,
		},
	}

	byKey := make(map[string]event.KeyDefinition, len(keys))
	for _, def := range keys {
		byKey[def.Key] = def
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			def, ok := byKey[tc.key]
			if !ok {
				t.Fatalf("missing key %s", tc.key)
			}
			if def.EventType != tc.key {
				t.Errorf("EventType = %q, want %q", def.EventType, tc.key)
			}
			if def.Schema.Custom == nil || def.Schema.Custom.Type != tc.schemaType {
				t.Fatalf("Custom schema Type = %v, want %v", def.Schema.Custom, tc.schemaType)
			}
			if def.Schema.Native != nil {
				t.Fatal("approval events must use Custom schema while SDK event types are not exported")
			}
			if def.Process == nil {
				t.Fatal("Process must flatten raw V2 envelopes")
			}
			if def.PreConsume == nil {
				t.Fatal("PreConsume must subscribe approval user-auth events")
			}
			if !reflect.DeepEqual(def.Scopes, []string{tc.scope}) {
				t.Errorf("Scopes = %#v, want %q", def.Scopes, tc.scope)
			}
			if !reflect.DeepEqual(def.AuthTypes, []string{"user"}) {
				t.Errorf("AuthTypes = %#v, want user", def.AuthTypes)
			}
			if !reflect.DeepEqual(def.RequiredConsoleEvents, []string{tc.key}) {
				t.Errorf("RequiredConsoleEvents = %#v, want %q", def.RequiredConsoleEvents, tc.key)
			}
			assertSubscriptionParam(t, def.Params)
		})
	}
}

func assertSubscriptionParam(t *testing.T, params []event.ParamDef) {
	t.Helper()
	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1", len(params))
	}
	p := params[0]
	if p.Name != "subscription_type" || p.Type != event.ParamMulti || p.Required || p.SubscriptionKey {
		t.Fatalf("subscription_type param = %+v, want optional multi non-subscription-key param", p)
	}
	got := map[string]string{}
	for _, v := range p.Values {
		got[v.Value] = v.Desc
	}
	for _, want := range []string{approvalSubscriptionTypeInvolved, approvalSubscriptionTypeManaged} {
		if got[want] == "" {
			t.Errorf("subscription_type value %q missing or empty desc; values=%+v", want, p.Values)
		}
	}
}

type reflectedApprovalSchema struct {
	Properties map[string]reflectedApprovalSchemaProperty `json:"properties"`
}

type reflectedApprovalSchemaProperty struct {
	Format     string                                     `json:"format"`
	Enum       []string                                   `json:"enum"`
	Properties map[string]reflectedApprovalSchemaProperty `json:"properties"`
}

func TestApprovalSchemasAnnotations(t *testing.T) {
	tests := []struct {
		name         string
		schemaType   reflect.Type
		eventType    string
		statusValues []string
		userField    string
	}{
		{
			name:         "instance",
			schemaType:   reflect.TypeOf(ApprovalInstanceStatusChangedV4Output{}),
			eventType:    eventTypeApprovalInstanceStatusChangedV4,
			statusValues: []string{"PENDING", "APPROVED", "REJECTED", "CANCELED", "DELETED", "REVERTED", "OVERTIME_CLOSE", "OVERTIME_RECOVER"},
			userField:    "start_user",
		},
		{
			name:         "task",
			schemaType:   reflect.TypeOf(ApprovalTaskStatusChangedV4Output{}),
			eventType:    eventTypeApprovalTaskStatusChangedV4,
			statusValues: []string{"REVERTED", "PENDING", "APPROVED", "REJECTED", "TRANSFERRED", "ROLLBACK", "DONE", "OVERTIME_CLOSE", "OVERTIME_RECOVER"},
			userField:    "assigned_user",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var schema reflectedApprovalSchema
			if err := json.Unmarshal(schemas.FromType(tc.schemaType), &schema); err != nil {
				t.Fatalf("unmarshal schema: %v", err)
			}
			props := schema.Properties
			eventTypeEnum := props["type"].Enum
			if len(eventTypeEnum) != 1 || eventTypeEnum[0] != tc.eventType {
				t.Fatalf("type enum = %v, want %s", eventTypeEnum, tc.eventType)
			}
			if got := props["timestamp"].Format; got != "timestamp_ms" {
				t.Errorf("timestamp format = %v, want timestamp_ms", got)
			}
			assertEnumContains(t, props["status"].Enum, tc.statusValues)
			if got := props["operate_time"].Format; got != "timestamp_ms" {
				t.Errorf("event.operate_time format = %v, want timestamp_ms", got)
			}

			userProps := props[tc.userField].Properties
			if got := userProps["open_id"].Format; got != "open_id" {
				t.Errorf("%s.open_id format = %v, want open_id", tc.userField, got)
			}
			if got := userProps["union_id"].Format; got != "union_id" {
				t.Errorf("%s.union_id format = %v, want union_id", tc.userField, got)
			}
			if got := userProps["user_id"].Format; got != "user_id" {
				t.Errorf("%s.user_id format = %v, want user_id", tc.userField, got)
			}
		})
	}
}

func assertEnumContains(t *testing.T, raw []string, wants []string) {
	t.Helper()
	got := make(map[string]bool, len(raw))
	for _, v := range raw {
		got[v] = true
	}
	for _, want := range wants {
		if !got[want] {
			t.Errorf("enum missing %q; enum=%v", want, raw)
		}
	}
}

func TestApprovalPreConsumeRegistersSubscriptionTypesWithoutCleanup(t *testing.T) {
	tests := []struct {
		name          string
		eventType     string
		subscribePath string
		params        map[string]string
		wantTypes     []string
	}{
		{
			name:          "instance omitted subscription_type registers both",
			eventType:     eventTypeApprovalInstanceStatusChangedV4,
			subscribePath: pathApprovalInstancesSubscription,
			wantTypes: []string{
				approvalSubscriptionTypeInvolved,
				approvalSubscriptionTypeManaged,
			},
		},
		{
			name:          "task explicit single managed",
			eventType:     eventTypeApprovalTaskStatusChangedV4,
			subscribePath: pathApprovalTasksSubscription,
			params:        map[string]string{"subscription_type": approvalSubscriptionTypeManaged},
			wantTypes:     []string{approvalSubscriptionTypeManaged},
		},
		{
			name:          "task comma separated multi canonicalizes and deduplicates",
			eventType:     eventTypeApprovalTaskStatusChangedV4,
			subscribePath: pathApprovalTasksSubscription,
			params: map[string]string{
				"subscription_type": approvalSubscriptionTypeManaged + "," + approvalSubscriptionTypeInvolved + "," + approvalSubscriptionTypeManaged,
			},
			wantTypes: []string{
				approvalSubscriptionTypeInvolved,
				approvalSubscriptionTypeManaged,
			},
		},
		{
			name:          "instance json array multi",
			eventType:     eventTypeApprovalInstanceStatusChangedV4,
			subscribePath: pathApprovalInstancesSubscription,
			params: map[string]string{
				"subscription_type": `["MANAGED_APPROVAL","INVOLVED_APPROVAL"]`,
			},
			wantTypes: []string{
				approvalSubscriptionTypeInvolved,
				approvalSubscriptionTypeManaged,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pc := approvalSubscriptionPreConsume(approvalSubscriptionConfig{
				eventType:     approvalEventType(tc.eventType),
				subscribePath: approvalSubscriptionPath(tc.subscribePath),
			})
			rt := &fakeAPIClient{}
			cleanup, err := pc(context.Background(), rt, tc.params)
			if err != nil {
				t.Fatalf("PreConsume returned error: %v", err)
			}
			if cleanup != nil {
				t.Fatal("cleanup must be nil; approval consume must not unsubscribe on exit")
			}
			assertSubscriptionCalls(t, rt.calls, tc.subscribePath, tc.wantTypes)
		})
	}
}

func assertSubscriptionCalls(t *testing.T, got []recordedCall, wantPath string, wantTypes []string) {
	t.Helper()
	if len(got) != len(wantTypes) {
		t.Fatalf("calls after pre-consume = %d, want %d; calls=%+v", len(got), len(wantTypes), got)
	}
	for i, wantType := range wantTypes {
		assertCall(t, got[i], "POST", wantPath, map[string]string{"subscription_type": wantType})
	}
}

func assertCall(t *testing.T, got recordedCall, wantMethod, wantPath string, wantBody interface{}) {
	t.Helper()
	if got.method != wantMethod {
		t.Errorf("method = %q, want %q", got.method, wantMethod)
	}
	if got.path != wantPath {
		t.Errorf("path = %q, want %q", got.path, wantPath)
	}
	if !reflect.DeepEqual(got.body, wantBody) {
		t.Errorf("body = %#v, want %#v", got.body, wantBody)
	}
}

func TestApprovalPreConsumeValidationErrors(t *testing.T) {
	t.Run("nil runtime", func(t *testing.T) {
		pc := approvalSubscriptionPreConsume(approvalSubscriptionConfig{
			eventType: eventTypeApprovalInstanceStatusChangedV4,
		})
		_, err := pc(context.Background(), nil, map[string]string{"subscription_type": approvalSubscriptionTypeInvolved})
		if err == nil {
			t.Fatal("expected nil runtime error")
		}
		p, ok := errs.ProblemOf(err)
		if !ok || p.Category != errs.CategoryInternal {
			t.Fatalf("err = %T/%v, want typed internal error", err, err)
		}
	})

	for _, raw := range []string{"BAD", "[]", `["INVOLVED_APPROVAL",3]`} {
		t.Run("invalid subscription type "+raw, func(t *testing.T) {
			pc := approvalSubscriptionPreConsume(approvalSubscriptionConfig{
				eventType: eventTypeApprovalInstanceStatusChangedV4,
			})
			cleanup, err := pc(context.Background(), &fakeAPIClient{}, map[string]string{"subscription_type": raw})
			if err == nil {
				t.Fatal("expected invalid subscription_type error")
			}
			if cleanup != nil {
				t.Fatal("cleanup must be nil on validation error")
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %T/%v, want *errs.ValidationError", err, err)
			}
			if ve.Subtype != errs.SubtypeInvalidArgument || ve.Param != "--param" {
				t.Errorf("subtype/param = %s/%q, want invalid_argument/--param", ve.Subtype, ve.Param)
			}
			if ve.Hint == "" {
				t.Error("invalid subscription_type should carry a hint")
			}
		})
	}

	t.Run("partial registration failure reports registered and failed relation types", func(t *testing.T) {
		upstream := errs.NewAPIError(errs.SubtypeServerError, "approval subscription API failed")
		rt := &fakeAPIClient{err: upstream, errOnCall: 2}
		pc := approvalSubscriptionPreConsume(approvalSubscriptionConfig{
			eventType:     eventTypeApprovalTaskStatusChangedV4,
			subscribePath: pathApprovalTasksSubscription,
		})

		cleanup, err := pc(context.Background(), rt, map[string]string{})
		if err == nil {
			t.Fatal("expected partial registration error")
		}
		if cleanup != nil {
			t.Fatal("cleanup must be nil on registration error")
		}
		assertSubscriptionCalls(t, rt.calls, pathApprovalTasksSubscription, []string{
			approvalSubscriptionTypeInvolved,
			approvalSubscriptionTypeManaged,
		})
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("err = %T/%v, want typed error", err, err)
		}
		if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeServerError {
			t.Fatalf("category/subtype = %s/%s, want api/server_error", p.Category, p.Subtype)
		}
		for _, want := range []string{
			"registered subscription_type(s) [INVOLVED_APPROVAL]",
			"failed subscription_type MANAGED_APPROVAL",
		} {
			if !strings.Contains(p.Message, want) {
				t.Errorf("partial error message missing %q: %q", want, p.Message)
			}
		}
		for _, want := range []string{
			"already registered",
			"--param subscription_type=MANAGED_APPROVAL",
		} {
			if !strings.Contains(p.Hint, want) {
				t.Errorf("partial error hint missing %q: %q", want, p.Hint)
			}
		}
	})
}

func TestApprovalSubscriptionRegistrationErrorVariants(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if err := approvalSubscriptionRegistrationError(eventTypeApprovalTaskStatusChangedV4, nil, approvalSubscriptionTypeInvolved, nil); err != nil {
			t.Fatalf("nil cause returned error: %v", err)
		}
	})

	t.Run("typed error with existing hint and empty message", func(t *testing.T) {
		upstream := errs.NewAPIError(errs.SubtypeServerError, "").WithHint("retry later")
		err := approvalSubscriptionRegistrationError(
			eventTypeApprovalTaskStatusChangedV4,
			nil,
			approvalSubscriptionTypeInvolved,
			upstream,
		)
		if err != upstream {
			t.Fatalf("typed error should be annotated in place; got %T/%v", err, err)
		}
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("err = %T/%v, want typed error", err, err)
		}
		if !strings.Contains(p.Message, "failed subscription_type INVOLVED_APPROVAL") {
			t.Errorf("message missing failed relation: %q", p.Message)
		}
		for _, want := range []string{"retry later", "no approval subscription relation was registered"} {
			if !strings.Contains(p.Hint, want) {
				t.Errorf("hint missing %q: %q", want, p.Hint)
			}
		}
	})

	t.Run("untyped error is wrapped with retry context", func(t *testing.T) {
		cause := errors.New("transport closed")
		err := approvalSubscriptionRegistrationError(
			eventTypeApprovalTaskStatusChangedV4,
			nil,
			approvalSubscriptionTypeInvolved,
			cause,
		)
		if !errors.Is(err, cause) {
			t.Fatalf("wrapped error should preserve cause; got %T/%v", err, err)
		}
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("err = %T/%v, want typed error", err, err)
		}
		if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeSDKError {
			t.Fatalf("category/subtype = %s/%s, want internal/sdk_error", p.Category, p.Subtype)
		}
		if !strings.Contains(p.Hint, "no approval subscription relation was registered") {
			t.Errorf("hint missing no-registration context: %q", p.Hint)
		}
	})
}

func TestProcessApprovalInstanceStatusChanged(t *testing.T) {
	out := runApprovalInstanceStatusChanged(t, `{
		"schema": "2.0",
		"header": {
			"event_id": "evt_approval_instance_001",
			"event_type": "approval.instance.status_changed_v4",
			"create_time": "1710000000000"
		},
		"event": {
			"approval_code": "approval_code_001",
			"instance_code": "instance_code_001",
			"external_id": "external_001",
			"status": "PENDING",
			"operate_time": "1666079207003",
			"start_user": {
				"open_id": "ou_start",
				"union_id": "on_start",
				"user_id": "user_start"
			}
		}
	}`)

	if out.Type != eventTypeApprovalInstanceStatusChangedV4 {
		t.Errorf("Type = %q, want %q", out.Type, eventTypeApprovalInstanceStatusChangedV4)
	}
	if out.EventID != "evt_approval_instance_001" || out.Timestamp != "1710000000000" {
		t.Errorf("EventID/Timestamp = %q/%q", out.EventID, out.Timestamp)
	}
	if out.ApprovalCode != "approval_code_001" || out.InstanceCode != "instance_code_001" {
		t.Errorf("approval/instance code = %q/%q", out.ApprovalCode, out.InstanceCode)
	}
	if out.ExternalID != "external_001" || out.Status != "PENDING" || out.OperateTime != "1666079207003" {
		t.Errorf("external/status/operate_time = %q/%q/%q", out.ExternalID, out.Status, out.OperateTime)
	}
	if out.StartUser == nil || out.StartUser.OpenID != "ou_start" || out.StartUser.UnionID != "on_start" || out.StartUser.UserID != "user_start" {
		t.Fatalf("StartUser = %+v, want full user ids", out.StartUser)
	}
}

func TestProcessApprovalTaskStatusChanged(t *testing.T) {
	out := runApprovalTaskStatusChanged(t, `{
		"schema": "2.0",
		"header": {
			"event_id": "evt_approval_task_001",
			"event_type": "approval.task.status_changed_v4",
			"create_time": "1710000000001"
		},
		"event": {
			"approval_code": "approval_code_002",
			"instance_code": "instance_code_002",
			"task_id": "task_001",
			"external_id": "external_002",
			"task_external_id": "task_external_001",
			"status": "APPROVED",
			"operate_time": "1666079207004",
			"assigned_user": {
				"open_id": "ou_assignee",
				"union_id": "on_assignee",
				"user_id": "user_assignee"
			}
		}
	}`)

	if out.Type != eventTypeApprovalTaskStatusChangedV4 {
		t.Errorf("Type = %q, want %q", out.Type, eventTypeApprovalTaskStatusChangedV4)
	}
	if out.EventID != "evt_approval_task_001" || out.Timestamp != "1710000000001" {
		t.Errorf("EventID/Timestamp = %q/%q", out.EventID, out.Timestamp)
	}
	if out.ApprovalCode != "approval_code_002" || out.InstanceCode != "instance_code_002" || out.TaskID != "task_001" {
		t.Errorf("approval/instance/task = %q/%q/%q", out.ApprovalCode, out.InstanceCode, out.TaskID)
	}
	if out.ExternalID != "external_002" || out.TaskExternalID != "task_external_001" || out.Status != "APPROVED" || out.OperateTime != "1666079207004" {
		t.Errorf("external/task_external/status/operate_time = %q/%q/%q/%q", out.ExternalID, out.TaskExternalID, out.Status, out.OperateTime)
	}
	if out.AssignedUser == nil || out.AssignedUser.OpenID != "ou_assignee" || out.AssignedUser.UnionID != "on_assignee" || out.AssignedUser.UserID != "user_assignee" {
		t.Fatalf("AssignedUser = %+v, want full user ids", out.AssignedUser)
	}
}

func TestProcessApprovalStatusChangedUsesRawEventTypeFallback(t *testing.T) {
	instance := runApprovalInstanceStatusChanged(t, `{
		"schema": "2.0",
		"header": {
			"event_id": "evt_approval_instance_fallback",
			"create_time": "1710000000002"
		},
		"event": {
			"approval_code": "approval_code_fallback",
			"instance_code": "instance_code_fallback",
			"status": "APPROVED",
			"operate_time": "1666079207005"
		}
	}`)
	if instance.Type != eventTypeApprovalInstanceStatusChangedV4 {
		t.Errorf("instance Type fallback = %q, want %q", instance.Type, eventTypeApprovalInstanceStatusChangedV4)
	}

	task := runApprovalTaskStatusChanged(t, `{
		"schema": "2.0",
		"header": {
			"event_id": "evt_approval_task_fallback",
			"create_time": "1710000000003"
		},
		"event": {
			"approval_code": "approval_code_fallback",
			"instance_code": "instance_code_fallback",
			"task_id": "task_fallback",
			"status": "DONE",
			"operate_time": "1666079207006"
		}
	}`)
	if task.Type != eventTypeApprovalTaskStatusChangedV4 {
		t.Errorf("task Type fallback = %q, want %q", task.Type, eventTypeApprovalTaskStatusChangedV4)
	}
}

func TestProcessApprovalStatusChangedMalformedPayloadPassthrough(t *testing.T) {
	for _, tc := range []struct {
		name      string
		eventType string
		process   event.ProcessFunc
	}{
		{"instance", eventTypeApprovalInstanceStatusChangedV4, processApprovalInstanceStatusChanged},
		{"task", eventTypeApprovalTaskStatusChangedV4, processApprovalTaskStatusChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := &event.RawEvent{
				EventType: tc.eventType,
				Payload:   json.RawMessage(`not json`),
				Timestamp: time.Now(),
			}
			got, err := tc.process(context.Background(), nil, raw, nil)
			if err != nil {
				t.Fatalf("Process should swallow parse errors, got %v", err)
			}
			if string(got) != "not json" {
				t.Errorf("malformed fallback output = %q, want original bytes", string(got))
			}
		})
	}
}

func TestProcessApprovalStatusChangedNilRaw(t *testing.T) {
	for _, tc := range []struct {
		name    string
		process event.ProcessFunc
	}{
		{"instance", processApprovalInstanceStatusChanged},
		{"task", processApprovalTaskStatusChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.process(context.Background(), nil, nil, nil)
			if err != nil {
				t.Fatalf("Process nil raw returned error: %v", err)
			}
			if got != nil {
				t.Fatalf("Process nil raw output = %s, want nil", string(got))
			}
		})
	}
}

func runApprovalInstanceStatusChanged(t *testing.T, payload string) ApprovalInstanceStatusChangedV4Output {
	t.Helper()
	raw := &event.RawEvent{
		EventType: eventTypeApprovalInstanceStatusChangedV4,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := processApprovalInstanceStatusChanged(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	var out ApprovalInstanceStatusChangedV4Output
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid instance JSON: %v\nraw=%s", err, string(got))
	}
	return out
}

func runApprovalTaskStatusChanged(t *testing.T, payload string) ApprovalTaskStatusChangedV4Output {
	t.Helper()
	raw := &event.RawEvent{
		EventType: eventTypeApprovalTaskStatusChangedV4,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := processApprovalTaskStatusChanged(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	var out ApprovalTaskStatusChangedV4Output
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid task JSON: %v\nraw=%s", err, string(got))
	}
	return out
}

func TestApprovalKeysRegisterCleanly(t *testing.T) {
	for _, key := range []string{eventTypeApprovalInstanceStatusChangedV4, eventTypeApprovalTaskStatusChangedV4} {
		event.UnregisterKeyForTest(key)
		t.Cleanup(func() { event.UnregisterKeyForTest(key) })
	}

	for _, def := range Keys() {
		event.RegisterKey(def)
	}
	for _, key := range []string{eventTypeApprovalInstanceStatusChangedV4, eventTypeApprovalTaskStatusChangedV4} {
		if _, ok := event.Lookup(key); !ok {
			t.Fatalf("event.Lookup(%q) not registered", key)
		}
	}
}

var _ event.APIClient = (*fakeAPIClient)(nil)
