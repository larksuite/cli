// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

type minuteSpeaker struct {
	SpeakerID string
	Name      string
}

func minuteTranscriptSpeakerlistPath(minuteToken string) string {
	return fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/transcript/speakerlist", validate.EncodePathSegment(minuteToken))
}

func fetchMinuteSpeakers(runtime *common.RuntimeContext, minuteToken string) ([]minuteSpeaker, error) {
	data, err := runtime.CallAPITyped(http.MethodGet, minuteTranscriptSpeakerlistPath(minuteToken), nil, nil)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	items := common.GetSlice(data, "speakers")
	speakers := make([]minuteSpeaker, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		if item == nil {
			continue
		}
		id := strings.TrimSpace(common.GetString(item, "speaker_id"))
		name := strings.TrimSpace(common.GetString(item, "name"))
		if id == "" {
			continue
		}
		speakers = append(speakers, minuteSpeaker{SpeakerID: id, Name: name})
	}
	return speakers, nil
}

func resolveSpeakerIDByName(speakers []minuteSpeaker, name string) (string, error) {
	name = strings.TrimSpace(name)
	var matches []minuteSpeaker
	for _, s := range speakers {
		if s.Name == name {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		return "", errs.NewValidationError(errs.SubtypeNotFound,
			"no speaker named %q in minute transcript", name).
			WithParam("--from-speaker-id").
			WithHint("Check the speaker name spelling or open the minute to see transcript speaker labels")
	case 1:
		return matches[0].SpeakerID, nil
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.SpeakerID
		}
		return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"multiple speakers named %q (%d matches); pass the exact --from-speaker-id", name, len(matches)).
			WithParam("--from-speaker-id").
			WithHint(fmt.Sprintf("Matching speaker_ids: %s. Review each speaker's utterances in the minute, then retry with the exact speaker_id", strings.Join(ids, ", ")))
	}
}

// resolveFromSpeakerID resolves --from-speaker-id to an API speaker_id.
// The input may already be an opaque speaker_id, or a display name that requires
// an internal speaker-list fetch.
func resolveFromSpeakerID(runtime *common.RuntimeContext, minuteToken, input string) (string, error) {
	input = strings.TrimSpace(input)
	speakers, err := fetchMinuteSpeakers(runtime, minuteToken)
	if err != nil {
		return "", err
	}
	for _, s := range speakers {
		if s.SpeakerID == input {
			return input, nil
		}
	}
	return resolveSpeakerIDByName(speakers, input)
}

func resolveSpeakerReplaceFrom(runtime *common.RuntimeContext, minuteToken string) (fromSpeakerID, fromUserID string, err error) {
	fromUserID = strings.TrimSpace(runtime.Str("from-user-id"))
	if fromUserID != "" {
		return "", fromUserID, nil
	}

	fromSpeakerID, err = resolveFromSpeakerID(runtime, minuteToken, runtime.Str("from-speaker-id"))
	return fromSpeakerID, "", err
}
