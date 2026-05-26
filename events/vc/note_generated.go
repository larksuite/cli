// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
)

const (
	vcNoteArtifactTypeNote     = 1
	vcNoteArtifactTypeVerbatim = 2

	vcNoteDetailRetryDelay   = 500 * time.Millisecond
	vcNoteDetailMaxRetries   = 2
	vcNoteDetailNotFoundCode = 121004
)

// VCNoteSourceOutput is the flattened note source payload.
type VCNoteSourceOutput struct {
	SourceType     string `json:"source_type,omitempty"      desc:"Note source type"`
	SourceEntityID string `json:"source_entity_id,omitempty" desc:"Source entity ID"`
}

// VCNoteGeneratedOutput is the flattened shape for vc.note.generated_v1.
type VCNoteGeneratedOutput struct {
	Type          string              `json:"type"                        desc:"Event type; always vc.note.generated_v1"`
	EventID       string              `json:"event_id,omitempty"          desc:"Globally unique event ID; safe for deduplication"`
	Timestamp     string              `json:"timestamp,omitempty"         desc:"Event delivery time (ms timestamp string); taken from header.create_time when present" kind:"timestamp_ms"`
	NoteID        string              `json:"note_id,omitempty"           desc:"Note ID"`
	NoteToken     string              `json:"note_token,omitempty"        desc:"Generated note document token"`
	VerbatimToken string              `json:"verbatim_token,omitempty"    desc:"Generated verbatim document token"`
	NoteSource    *VCNoteSourceOutput `json:"note_source,omitempty"       desc:"Note source metadata"`
}

func processVCNoteGenerated(ctx context.Context, rt event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
	var envelope struct {
		Header struct {
			EventID    string `json:"event_id"`
			EventType  string `json:"event_type"`
			CreateTime string `json:"create_time"`
		} `json:"header"`
		Event struct {
			NoteID string `json:"note_id"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return raw.Payload, nil //nolint:nilerr // passthrough on malformed payload so consumers still see the event
	}

	out := &VCNoteGeneratedOutput{
		Type:      envelope.Header.EventType,
		EventID:   envelope.Header.EventID,
		Timestamp: envelope.Header.CreateTime,
		NoteID:    envelope.Event.NoteID,
	}
	if out.Type == "" {
		out.Type = raw.EventType
	}

	if rt != nil && out.NoteID != "" {
		fillVCNoteGeneratedDetails(ctx, rt, out)
	}

	return json.Marshal(out)
}

func fillVCNoteGeneratedDetails(ctx context.Context, rt event.APIClient, out *VCNoteGeneratedOutput) {
	if rt == nil || out == nil || out.NoteID == "" {
		return
	}

	path := fmt.Sprintf(pathNoteDetailFmt, validate.EncodePathSegment(out.NoteID))

	type noteDetailResp struct {
		Data struct {
			Note struct {
				Artifacts []struct {
					ArtifactType int    `json:"artifact_type"`
					DocToken     string `json:"doc_token"`
				} `json:"artifacts"`
				NoteSource struct {
					SourceEntityID string `json:"source_entity_id"`
					SourceType     string `json:"source_type"`
				} `json:"note_source"`
			} `json:"note"`
		} `json:"data"`
	}

	for attempt := 0; attempt <= vcNoteDetailMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(vcNoteDetailRetryDelay)
		}

		raw, err := rt.CallAPI(ctx, "GET", path, nil)
		if err != nil {
			if isLarkCode(err, vcNoteDetailNotFoundCode) {
				continue
			}
			return
		}

		var resp noteDetailResp
		if err := json.Unmarshal(raw, &resp); err != nil {
			continue
		}

		var noteToken, verbatimToken string
		for _, artifact := range resp.Data.Note.Artifacts {
			switch artifact.ArtifactType {
			case vcNoteArtifactTypeNote:
				if noteToken == "" {
					noteToken = artifact.DocToken
				}
			case vcNoteArtifactTypeVerbatim:
				if verbatimToken == "" {
					verbatimToken = artifact.DocToken
				}
			}
		}

		if noteToken == "" && verbatimToken == "" {
			continue
		}

		if noteToken != "" {
			out.NoteToken = noteToken
		}
		if verbatimToken != "" {
			out.VerbatimToken = verbatimToken
		}
		if src := resp.Data.Note.NoteSource; src.SourceType != "" || src.SourceEntityID != "" {
			out.NoteSource = &VCNoteSourceOutput{
				SourceType:     src.SourceType,
				SourceEntityID: src.SourceEntityID,
			}
		}
		return
	}
}

func isLarkCode(err error, code int) bool {
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) && exitErr.Detail != nil {
		return exitErr.Detail.Code == code
	}
	return false
}
