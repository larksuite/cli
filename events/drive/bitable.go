// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/validate"
)

const (
	eventTypeBitableFieldChangedV1  = "drive.file.bitable_field_changed_v1"
	eventTypeBitableRecordChangedV1 = "drive.file.bitable_record_changed_v1"

	// bitableFileType is the only file_type these two EventKeys apply to, so
	// it is fixed here instead of being a param: a Base is always addressed
	// as file_type=bitable by the drive file-subscription APIs.
	bitableFileType = "bitable"
)

// fileTokenParam declares the Base whose changes this consumer wants.
//
// The drive file-subscription API is keyed per file, so the token is part of
// the subscription identity: two consumers watching different Bases get their
// own setup and cleanup instead of sharing one record.
func fileTokenParam(what string) event.ParamDef {
	return event.ParamDef{
		Name:            "file_token",
		Type:            event.ParamString,
		Required:        true,
		SubscriptionKey: true,
		Description:     fmt.Sprintf("Base (bitable) app token to subscribe; subscription is per file. %s", what),
	}
}

// BitableOperatorID identifies who produced a change, in the three Lark
// identity formats.
type BitableOperatorID struct {
	// OpenID is the operator's open_id within the current app.
	OpenID string `json:"open_id"`
	// UnionID is the operator's union_id across apps under the same developer.
	UnionID string `json:"union_id"`
	// UserID is the operator's user_id within the tenant.
	UserID string `json:"user_id"`
}

// DriveFileBitableFieldChangedV1Data is the bitable field changed source payload.
type DriveFileBitableFieldChangedV1Data struct {
	// FileType is always bitable for this event.
	FileType string `json:"file_type"`
	// FileToken is the Base app token the changed table belongs to.
	FileToken string `json:"file_token"`
	// TableID is the table whose fields changed.
	TableID string `json:"table_id"`
	// Revision is the Base revision this change produced.
	Revision int64 `json:"revision"`
	// UpdateTime is when the change was applied, in seconds.
	UpdateTime int64 `json:"update_time"`
	// OperatorID is who changed the fields.
	OperatorID BitableOperatorID `json:"operator_id"`
	// ActionList is the batch of field changes carried by this event. One
	// edit in the UI can produce several entries: adding a field is observed
	// as field_added followed by field_edited for the same field_id.
	ActionList []BitableFieldAction `json:"action_list"`
	// SubscriberIDList lists the subscribers this delivery was produced for.
	SubscriberIDList []BitableOperatorID `json:"subscriber_id_list"`
}

// BitableFieldAction is one field-level change inside a field changed event.
type BitableFieldAction struct {
	// Action is the change kind, for example field_added, field_deleted or field_edited.
	Action string `json:"action"`
	// FieldID is the field the change applies to.
	FieldID string `json:"field_id"`
	// BeforeValue is the field definition before the change; absent for an added field.
	BeforeValue *BitableFieldDefinition `json:"before_value,omitempty"`
	// AfterValue is the field definition after the change; absent for a deleted field.
	AfterValue *BitableFieldDefinition `json:"after_value,omitempty"`
}

// BitableFieldDefinition is a field as the event describes it.
type BitableFieldDefinition struct {
	// ID is the field id.
	ID string `json:"id"`
	// Name is the field name.
	Name string `json:"name"`
	// Type is the numeric Base field type, as used by the Base field APIs.
	Type int `json:"type"`
}

// DriveFileBitableRecordChangedV1Data is the bitable record changed source payload.
type DriveFileBitableRecordChangedV1Data struct {
	// FileType is always bitable for this event.
	FileType string `json:"file_type"`
	// FileToken is the Base app token the changed table belongs to.
	FileToken string `json:"file_token"`
	// TableID is the table whose records changed.
	TableID string `json:"table_id"`
	// Revision is the Base revision this change produced.
	Revision int64 `json:"revision"`
	// UpdateTime is when the change was applied, in seconds.
	UpdateTime int64 `json:"update_time"`
	// OperatorID is who changed the records.
	OperatorID BitableOperatorID `json:"operator_id"`
	// ActionList is the batch of record changes carried by this event.
	ActionList []BitableRecordAction `json:"action_list"`
	// SubscriberIDList lists the subscribers this delivery was produced for.
	SubscriberIDList []BitableOperatorID `json:"subscriber_id_list"`
}

// BitableRecordAction is one record-level change inside a record changed event.
type BitableRecordAction struct {
	// RecordID is the record the change applies to.
	RecordID string `json:"record_id"`
	// Action is the change kind: record_added, record_deleted or record_edited.
	Action string `json:"action"`
	// BeforeValue holds the record's cells before the change; absent for an added record.
	BeforeValue []BitableRecordCell `json:"before_value,omitempty"`
	// AfterValue holds the record's cells after the change; absent for a deleted record.
	AfterValue []BitableRecordCell `json:"after_value,omitempty"`
}

// BitableRecordCell is one cell of a changed record. Every field of the table
// is listed, not only the ones that changed.
type BitableRecordCell struct {
	// FieldID is the field this cell belongs to.
	FieldID string `json:"field_id"`
	// FieldValue is the cell value as the server serialises it: a string,
	// empty when the cell has no value, and itself JSON for structured field
	// types such as text runs.
	FieldValue string `json:"field_value"`
}

// bitableFileTokenMatch keeps a consumer's stream to the Base it asked for.
//
// The subscription is registered per file, but the app's event stream carries
// every Base anyone has subscribed under this app, and the local bus fans out
// by event type. Filtering here is what makes "subscription is per file" true
// for the consumer's stdout as well. An event whose file token cannot be read
// is dropped rather than attributed to the wrong Base; Match has no diagnostic
// channel, so these drops are silent.
func bitableFileTokenMatch(raw *event.RawEvent, params map[string]string) bool {
	want := params["file_token"]
	if want == "" {
		return false
	}
	var envelope struct {
		Event struct {
			FileToken string `json:"file_token"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return false
	}
	return envelope.Event.FileToken == want
}

// bitableSubscriptionPreConsume subscribes the Base named by --param
// file_token and returns a cleanup that unsubscribes it.
//
// Unlike the per-event_type drive subscription used for comment notices, the
// file subscription is a single per-(app, identity, file) record shared by
// every cloud-document file event: subscribing for field changes also starts
// record changes, and one unsubscribe stops both. A record that already
// existed before this consumer started is therefore left alone on exit —
// tearing it down would silently cut off whoever created it.
func bitableSubscriptionPreConsume(eventType string) func(context.Context, event.APIClient, map[string]string) (func() error, error) {
	return func(ctx context.Context, rt event.APIClient, params map[string]string) (func() error, error) {
		if rt == nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown,
				"runtime API client is required for pre-consume subscription")
		}
		fileToken := params["file_token"]
		if fileToken == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"param file_token is required for %s", eventType).
				WithParam("--param").
				WithHint("pass it as --param file_token=<base_token>; run `lark-cli event schema %s` for details", eventType)
		}
		encoded := validate.EncodePathSegment(fileToken)
		base := fmt.Sprintf("/open-apis/drive/v1/files/%s", encoded)
		query := "?file_type=" + bitableFileType

		alreadySubscribed, err := bitableIsSubscribed(ctx, rt, base+"/get_subscribe"+query)
		if err != nil {
			return nil, err
		}
		// Subscribe even when the file already carries a record: the call is
		// idempotent, and doing it unconditionally keeps a stale read of
		// get_subscribe from leaving the consumer listening to nothing. What
		// the read decides is only whether to tear the record down on exit.
		if _, err := rt.CallAPI(ctx, "POST", base+"/subscribe"+query, nil); err != nil {
			return nil, err
		}
		if alreadySubscribed {
			return nil, nil
		}
		return func() error {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), bitableCleanupTimeout)
			defer cancel()
			_, err := rt.CallAPI(cleanupCtx, "DELETE", base+"/delete_subscribe"+query, nil)
			return err
		}, nil
	}
}

// bitableIsSubscribed reports whether the file already carries a subscription
// for this identity, so cleanup only removes what this consumer created.
func bitableIsSubscribed(ctx context.Context, rt event.APIClient, path string) (bool, error) {
	raw, err := rt.CallAPI(ctx, "GET", path, nil)
	if err != nil {
		return false, err
	}
	var envelope struct {
		Data struct {
			IsSubscribe bool `json:"is_subscribe"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return false, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"decode drive file subscription status").WithCause(err)
	}
	return envelope.Data.IsSubscribe, nil
}
