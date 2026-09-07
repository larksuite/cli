// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var driveReactReplyOp = driveCommentOp{
	Label: "reply reaction",
	Types: []string{"doc", "docx", "sheet", "file", "slides", "bitable", "apps"},
}

const (
	driveReactReplyActionAdd    = "add"
	driveReactReplyActionDelete = "delete"
)

// driveReactReplyReactionTypes mirrors the reaction_type enum from the
// platform metadata (file.comment.reply.reactions.update_reaction). The
// server does NOT validate this field — an arbitrary string is accepted and
// persisted as a broken reaction on the reply, so this local check is the
// only guard. Values are case-sensitive.
var driveReactReplyReactionTypes = map[string]struct{}{
	"ANGRY": {}, "APPLAUSE": {}, "ATTENTION": {}, "AWESOME": {}, "BEAR": {}, "BEER": {},
	"BETRAYED": {}, "BIGKISS": {}, "BLACKFACE": {}, "BLUBBER": {}, "BLUSH": {}, "BOMB": {},
	"CAKE": {}, "CHUCKLE": {}, "CLAP": {}, "CLEAVER": {}, "COMFORT": {}, "CRAZY": {}, "CRY": {},
	"CUCUMBER": {}, "DETERGENT": {}, "DIZZY": {}, "DONE": {}, "DONNOTGO": {}, "DROOL": {},
	"DROWSY": {}, "DULL": {}, "DULLSTARE": {}, "EATING": {}, "EMBARRASSED": {}, "ENOUGH": {},
	"ERROR": {}, "EYESCLOSED": {}, "FACEPALM": {}, "FINGERHEART": {}, "FISTBUMP": {},
	"FOLLOWME": {}, "FROWN": {}, "GIFT": {}, "GLANCE": {}, "GOODJOB": {}, "HAMMER": {},
	"HAUGHTY": {}, "HEADSET": {}, "HEART": {}, "HEARTBROKEN": {}, "HIGHFIVE": {}, "HUG": {},
	"HUSKY": {}, "INNOCENTSMILE": {}, "JIAYI": {}, "JOYFUL": {}, "KISS": {}, "LAUGH": {},
	"LIPS": {}, "LOL": {}, "LOOKDOWN": {}, "LOVE": {}, "MONEY": {}, "MUSCLE": {},
	"NOSEPICK": {}, "OBSESSED": {}, "OK": {}, "PARTY": {}, "PETRIFIED": {}, "POOP": {},
	"PRAISE": {}, "PROUD": {}, "PUKE": {}, "RAINBOWPUKE": {}, "ROSE": {}, "SALUTE": {},
	"SCOWL": {}, "SHAKE": {}, "SHHH": {}, "SHOCKED": {}, "SHOWOFF": {}, "SHY": {}, "SICK": {},
	"SILENT": {}, "SKULL": {}, "SLAP": {}, "SLEEP": {}, "SLIGHT": {}, "SMART": {}, "SMILE": {},
	"SMIRK": {}, "SMOOCH": {}, "SMUG": {}, "SOB": {}, "SPEECHLESS": {}, "SPITBLOOD": {},
	"STRIVE": {}, "SWEAT": {}, "TEARS": {}, "TEASE": {}, "TERROR": {}, "THANKS": {},
	"THINKING": {}, "THUMBSUP": {}, "TOASTED": {}, "TONGUE": {}, "TRICK": {}, "UPPERLEFT": {},
	"WAIL": {}, "WAVE": {}, "WELLDONE": {}, "WHAT": {}, "WHIMPER": {}, "WINK": {}, "WITTY": {},
	"WOW": {}, "WRONGED": {}, "XBLUSH": {}, "YAWN": {}, "YEAH": {}, "FIREWORKS": {}, "BULL": {},
	"CALF": {}, "AWESOMEN": {}, "2021": {}, "CANDIEDHAWS": {}, "REDPACKET": {}, "FORTUNE": {},
	"LUCK": {}, "FIRECRACKER": {}, "Yes": {}, "No": {}, "Get": {}, "LGTM": {}, "Lemon": {},
	"EatingFood": {}, "Hundred": {}, "MinusOne": {}, "ThumbsDown": {}, "Fire": {}, "OKR": {},
	"Drumstick": {}, "BubbleTea": {}, "Loudspeaker": {}, "Pin": {}, "Coffee": {}, "Alarm": {},
	"Trophy": {}, "Music": {}, "Typing": {}, "Pepper": {}, "CheckMark": {}, "CrossMark": {},
}

type driveReactReplySpec struct {
	Ref          driveCommentRef
	ReplyID      string
	ReactionType string
	Action       string
}

func (s driveReactReplySpec) RequestBody() map[string]interface{} {
	return map[string]interface{}{
		"action":        s.Action,
		"reaction_type": s.ReactionType,
		"reply_id":      s.ReplyID,
	}
}

// DriveReactReply adds or removes an emoji reaction on a comment reply
// through the Drive comment reaction API (POST /drive/v2/files/:file_token/
// comments/reaction), while accepting Wiki URLs/tokens and resolving them to
// the underlying object.
var DriveReactReply = common.Shortcut{
	Service:           "drive",
	Command:           "+react-reply",
	Description:       "Add or remove an emoji reaction on a comment reply for doc/docx/sheet/file/slides/base(bitable)/apps, with URL parsing and Wiki token unwrapping",
	Risk:              "write",
	Scopes:            []string{"docs:document.comment:write_only"},
	ConditionalScopes: []string{"wiki:node:read"},
	AuthTypes:         []string{"user", "bot"},
	Flags: append(driveCommentTargetFlags(driveReactReplyOp),
		common.Flag{Name: "reply-id", Desc: "reply ID to react to (from drive +list-replies); the root reply carries the comment body", Required: true},
		common.Flag{Name: "emoji", Desc: "reaction_type value, case-sensitive, e.g. THUMBSUP, HEART, DONE, OK", Required: true},
		common.Flag{Name: "action", Desc: "add attaches the reaction; delete removes the current identity's reaction", Required: true, Enum: []string{driveReactReplyActionAdd, driveReactReplyActionDelete}},
	),
	Tips: []string{
		"Reply IDs come from `drive +list-replies` (items[].reply_id); reacting to the root reply reacts to the comment itself.",
		"--emoji is case-sensitive and validated locally against the platform reaction_type list (the server accepts and persists arbitrary strings as broken reactions); the full list is in the lark-drive reactions guide.",
		"Read reactions back via --need-reaction on `drive +list-replies` / `drive +batch-query-comments`; entries with count=0 are leftovers of removed reactions — filter by count>0.",
		"add and delete are idempotent: re-adding an existing reaction or deleting an absent one succeeds without change. delete only cancels the current identity's reaction.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readDriveReactReplySpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readDriveReactReplySpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return buildDriveReactReplyDryRun(spec)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readDriveReactReplySpec(runtime)
		if err != nil {
			return err
		}

		target, err := resolveDriveCommentTarget(ctx, runtime, driveReactReplyOp, spec.Ref)
		if err != nil {
			return err
		}

		path := fmt.Sprintf("/open-apis/drive/v2/files/%s/comments/reaction", validate.EncodePathSegment(target.FileToken))
		if _, err := runtime.CallAPITyped(
			"POST",
			path,
			map[string]interface{}{"file_type": target.FileType},
			spec.RequestBody(),
		); err != nil {
			return err
		}

		runtime.Out(driveCommentTargetOutput(target, map[string]interface{}{
			"reply_id":      spec.ReplyID,
			"reaction_type": spec.ReactionType,
			"action":        spec.Action,
			"updated":       true,
		}), nil)
		return nil
	},
}

func readDriveReactReplySpec(runtime *common.RuntimeContext) (driveReactReplySpec, error) {
	ref, err := resolveDriveCommentInput(driveReactReplyOp, runtime.Str("url"), runtime.Str("token"), runtime.Str("type"))
	if err != nil {
		return driveReactReplySpec{}, err
	}
	replyID := strings.TrimSpace(runtime.Str("reply-id"))
	if replyID == "" {
		return driveReactReplySpec{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--reply-id must not be empty").WithParam("--reply-id")
	}
	reactionType, err := parseDriveReactReplyEmoji(runtime.Str("emoji"))
	if err != nil {
		return driveReactReplySpec{}, err
	}
	action, err := parseDriveReactReplyAction(runtime.Str("action"))
	if err != nil {
		return driveReactReplySpec{}, err
	}
	return driveReactReplySpec{
		Ref:          ref,
		ReplyID:      replyID,
		ReactionType: reactionType,
		Action:       action,
	}, nil
}

// parseDriveReactReplyEmoji validates the reaction_type against the platform
// enum. Case matters: the wire values mix all-caps and CamelCase (THUMBSUP vs
// ThumbsDown), and the server persists any unknown string as a broken
// reaction instead of rejecting it.
func parseDriveReactReplyEmoji(raw string) (string, error) {
	emoji := strings.TrimSpace(raw)
	if emoji == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--emoji must not be empty").WithParam("--emoji")
	}
	if _, ok := driveReactReplyReactionTypes[emoji]; !ok {
		return "", errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"unknown --emoji %q; reaction_type values are case-sensitive (e.g. THUMBSUP, HEART, DONE, OK) — see the lark-drive reactions guide for the full list",
			emoji,
		).WithParam("--emoji")
	}
	return emoji, nil
}

// parseDriveReactReplyAction normalizes and validates the --action value.
// The flag's Enum already rejects unknown values from the CLI, so the error
// branch only guards direct callers.
func parseDriveReactReplyAction(raw string) (string, error) {
	action := strings.ToLower(strings.TrimSpace(raw))
	if action != driveReactReplyActionAdd && action != driveReactReplyActionDelete {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --action %q; allowed: %s, %s", raw, driveReactReplyActionAdd, driveReactReplyActionDelete).WithParam("--action")
	}
	return action, nil
}

func buildDriveReactReplyDryRun(spec driveReactReplySpec) *common.DryRunAPI {
	if spec.Ref.Type == "wiki" {
		return common.NewDryRunAPI().
			Desc("2-step orchestration: resolve wiki -> update reply reaction").
			GET("/open-apis/wiki/v2/spaces/get_node").
			Desc("[1] Resolve wiki node to underlying document").
			Params(map[string]interface{}{"token": spec.Ref.Token}).
			POST("/open-apis/drive/v2/files/<obj_token from step 1>/comments/reaction").
			Desc("[2] Add or remove the reaction on the resolved document").
			Params(map[string]interface{}{"file_type": "<obj_type from step 1>"}).
			Body(spec.RequestBody()).
			Set("reply_id", spec.ReplyID)
	}

	return common.NewDryRunAPI().
		Desc("1-step request: update reply reaction").
		POST("/open-apis/drive/v2/files/:file_token/comments/reaction").
		Params(map[string]interface{}{"file_type": spec.Ref.Type}).
		Body(spec.RequestBody()).
		Set("file_token", spec.Ref.Token).
		Set("reply_id", spec.ReplyID)
}
