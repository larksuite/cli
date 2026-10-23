// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/processing"
)

// CommentAddOutput is the flattened shape for drive.notice.comment_add_v1.
type CommentAddOutput struct {
	Type         string `json:"type"                      desc:"Event type; always drive.notice.comment_add_v1"`
	EventID      string `json:"event_id,omitempty"        desc:"Globally unique event ID; safe for deduplication"`
	Timestamp    string `json:"timestamp,omitempty"       desc:"Event delivery time (ms timestamp string) from header.create_time"                                             kind:"timestamp_ms"`
	AppID        string `json:"app_id,omitempty"          desc:"Application ID from the event header"`
	TenantKey    string `json:"tenant_key,omitempty"      desc:"Tenant key from the event header"`
	FileToken    string `json:"file_token"                desc:"Token of the commented document; pass it with file_type to the drive comment APIs"`
	FileType     string `json:"file_type,omitempty"       desc:"Type of the commented document, for example docx, doc, sheet, or file"`
	CommentID    string `json:"comment_id"                desc:"Comment ID; for a reply this is the parent comment"`
	ReplyID      string `json:"reply_id,omitempty"        desc:"ID of the reply that triggered the notice; for add_comment it is the first reply, which holds the comment body"`
	NoticeType   string `json:"notice_type,omitempty"     desc:"Comment operation that produced the notice"                                                                 enum:"add_comment,add_reply"`
	IsMentioned  bool   `json:"is_mentioned"              desc:"Whether the notice receiver (to_*) was @mentioned in the comment or reply"`
	Visibility   string `json:"visibility,omitempty"      desc:"Comment visibility reported by the server"`
	FromOpenID   string `json:"from_open_id,omitempty"    desc:"Comment author open_id"                                                                                     kind:"open_id"`
	FromUnionID  string `json:"from_union_id,omitempty"   desc:"Comment author union_id"                                                                                    kind:"union_id"`
	FromUserID   string `json:"from_user_id,omitempty"    desc:"Comment author user_id"                                                                                     kind:"user_id"`
	FromUserType string `json:"from_user_type,omitempty"  desc:"Comment author type, for example user or bot"`
	ToOpenID     string `json:"to_open_id,omitempty"      desc:"Notice receiver open_id: the subscribed user, or the bot when consuming as bot"                            kind:"open_id"`
	ToUnionID    string `json:"to_union_id,omitempty"     desc:"Notice receiver union_id"                                                                                   kind:"union_id"`
	ToUserID     string `json:"to_user_id,omitempty"      desc:"Notice receiver user_id"                                                                                    kind:"user_id"`
}

type commentUserID struct {
	OpenID  string `json:"open_id"`
	UnionID string `json:"union_id"`
	UserID  string `json:"user_id"`
}

func processCommentAdd(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	if raw == nil {
		return nil, nil
	}
	var envelope struct {
		Event struct {
			CommentID   string `json:"comment_id"`
			ReplyID     string `json:"reply_id"`
			IsMentioned *bool  `json:"is_mentioned"`
			Visibility  string `json:"visibility"`
			// Live payloads have been observed carrying the file fields at
			// the top level and is_mentioned inside notice_meta, instead of
			// where the published event schema puts them; read both places.
			FileToken  string `json:"file_token"`
			FileType   string `json:"file_type"`
			NoticeMeta struct {
				FileToken    string        `json:"file_token"`
				FileType     string        `json:"file_type"`
				FromUserID   commentUserID `json:"from_user_id"`
				ToUserID     commentUserID `json:"to_user_id"`
				FromUserType string        `json:"from_user_type"`
				NoticeType   string        `json:"notice_type"`
				IsMentioned  *bool         `json:"is_mentioned"`
			} `json:"notice_meta"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return nil, processing.DropMalformed(raw.EventType)
	}
	body := envelope.Event
	meta := body.NoticeMeta

	fileToken := firstNonEmpty(meta.FileToken, body.FileToken)
	// Without both identifiers the notice cannot be resolved to a comment
	// through the drive comment APIs, which is the reason to consume it.
	if fileToken == "" || body.CommentID == "" {
		return nil, processing.DropMalformed(raw.EventType)
	}

	isMentioned := body.IsMentioned
	if isMentioned == nil {
		isMentioned = meta.IsMentioned
	}

	out := &CommentAddOutput{
		Type:         eventTypeCommentAddV1,
		EventID:      raw.EventID,
		Timestamp:    raw.SourceTime,
		AppID:        raw.AppID,
		TenantKey:    raw.TenantKey,
		FileToken:    fileToken,
		FileType:     firstNonEmpty(meta.FileType, body.FileType),
		CommentID:    body.CommentID,
		ReplyID:      body.ReplyID,
		NoticeType:   meta.NoticeType,
		IsMentioned:  isMentioned != nil && *isMentioned,
		Visibility:   body.Visibility,
		FromOpenID:   meta.FromUserID.OpenID,
		FromUnionID:  meta.FromUserID.UnionID,
		FromUserID:   meta.FromUserID.UserID,
		FromUserType: meta.FromUserType,
		ToOpenID:     meta.ToUserID.OpenID,
		ToUnionID:    meta.ToUserID.UnionID,
		ToUserID:     meta.ToUserID.UserID,
	}
	return json.Marshal(out)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
