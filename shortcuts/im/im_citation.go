// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Citation builders for the IM domain's read commands.
//
// # What a citation is
//
// A citation is one entry of the `citations` array that read commands attach
// to the top-level JSON success envelope, next to `data`:
//
//	{"ok": true, "data": {...}, "citations": [{...}, {...}]}
//
// The consumer renders each entry as a clickable source footnote under a model
// answer, so every entry must be able to stand alone: a URL that opens the
// thing, a title to show, and enough metadata to rank it. It never replaces
// `data` — the model still reads the payload; the citations array only tells
// it how to point back at the source.
//
// # How the framework invokes these builders
//
// Nothing here is called directly. The wiring is:
//
//  1. A shortcut declares `Citation: &common.CitationDefinition{...}` in its
//     common.Shortcut literal (see the three call sites in this package).
//  2. At registration, Shortcut.mountDeclarative validates the declaration and
//     panics on a bad one — citations require Risk "read", a non-empty
//     SourceTypes set, and a non-nil Build hook. A mistake here fails at
//     startup, not at request time.
//  3. At request time, RuntimeContext.citationProvider wraps Build into a
//     closure and hands it to the emitter via output.EmitOptions.Citations.
//  4. The emitter calls that closure lazily — only after it has committed to
//     emitting an envelope at all. Formats that never produce an envelope
//     (--format table/csv/ndjson, pretty with a renderer) therefore never
//     build citations; a --jq run does build them, because jq filters the
//     full envelope. That laziness is why Build receives the finished
//     payload rather than being called inline in Execute.
//  5. Each returned entry is validated: an entry whose SourceType is unset or
//     outside the declared SourceTypes is dropped with a stderr warning, as is
//     one whose URL is not an absolute https URL. citation.Normalize then
//     drops entries with an empty URL silently — that is the protocol's normal
//     degradation, not an error — and returns nil when nothing survives, so
//     the `citations` key is omitted entirely rather than emitted as [].
//
// The whole capability is behind the LARKSUITE_CLI_CITATION environment
// variable (exact value "1"). With the gate off, citationProvider returns nil
// and output is byte-identical to before citations existed.
//
// # The constraint that matters most
//
// A builder must draw every text field from material the command already has:
// the output payload, the command's own flags, and the brand. It must not call
// an API, read a file, or invent text.
//
// This is a safety property, not a style rule. Content-safety scanning runs
// over `data` before these builders run. A builder that only reshuffles text
// already present in `data` can therefore only ever surface text that has
// already been scanned. Pulling in anything from outside would emit unscanned
// text on the wire and break that guarantee.
//
// A builder also must not fail the command. Citations are strictly additive;
// a shape the builder does not recognize returns nil ("this result carries no
// citations") rather than an error.

package im

import (
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// citationTextMsgType is the only msg_type this round emits citations for.
//
// IM messages come in many types (image, file, audio, media, post, sticker,
// share_chat, share_user, interactive, merge_forward, ...). The protocol
// defines a message's title as the message itself, and only "text" has such a
// plain-text body; the others would either cite an empty string or a rendered
// placeholder like "[image]", which is worse than not citing them at all.
// Scope for this round is text only, by product decision.
//
// Widening this is a one-line change plus a decision about what title each new
// type should carry — "post" (rich text) is the obvious next candidate, since
// convert_lib already flattens it to text.
const citationTextMsgType = "text"

// citationBrand resolves the brand whose applink host is used to build URLs.
//
// Brand is Feishu (applink.feishu.cn) or Lark (applink.larksuite.com) and is
// fixed per CLI config. The nil checks are not defensive noise: unit tests call
// the builders with a nil RuntimeContext, and Feishu is the right default for
// that path.
func citationBrand(rt *common.RuntimeContext) core.LarkBrand {
	if rt != nil && rt.Config != nil {
		return rt.Config.Brand
	}
	return core.BrandFeishu
}

// chatOpenURL builds the applink deep link that opens a chat in the Lark
// client. It is the citation URL for a chat, and the fallback for a message
// whose own message_app_link is missing.
//
// Returning "" for an empty chatID is load-bearing: a URL-less entry is
// dropped by citation.Normalize, which is the correct outcome for something we
// cannot link to. Never synthesize a URL just to keep an entry alive — sending
// the reader somewhere wrong is worse than showing no footnote.
func chatOpenURL(brand core.LarkBrand, chatID string) string {
	if chatID == "" {
		return ""
	}
	return core.ResolveEndpoints(brand).AppLink + "/client/chat/open?openChatId=" + url.QueryEscape(chatID)
}

// messageCitation builds one message's citation.
//
// The bool result distinguishes "this message is not citable at all" (a
// non-text message, skipped entirely) from "citable, though some optional
// field may be empty". Callers append only when it is true, so skipped
// messages leave no gap in the citations array.
//
// The msg argument is one element of the command's formatted `messages` list,
// not a raw API response. convert_lib.formatMessageItem has already flattened
// the message: `content` is display text with mentions resolved to names (the
// raw wire form is a JSON string under body.content), `create_time` is
// normalized, and `message_app_link` is either the API's own link or one
// assembled from chat_id and message_position. That is why this function can
// be a pure field projection with no parsing.
//
// fallbackChatID exists because the two message commands shape their payloads
// differently: +chat-messages-list resolves one chat up front and its items may
// omit chat_id, while +messages-search spans chats and every item carries its
// own. Passing "" opts out.
//
// URL prefers message_app_link because it lands on the exact message (it
// carries a position anchor); the chat-level link is a degradation that only
// gets the reader to the right conversation.
func messageCitation(brand core.LarkBrand, msg map[string]interface{}, fallbackChatID string) (citation.Citation, bool) {
	if msgType, _ := msg["msg_type"].(string); msgType != citationTextMsgType {
		return citation.Citation{}, false
	}

	// Comma-ok on every read: these come from a map[string]interface{} built
	// from a JSON response, so a missing or differently-typed key must yield
	// the zero value rather than panic.
	text, _ := msg["content"].(string)
	createTime, _ := msg["create_time"].(string)
	chatID, _ := msg["chat_id"].(string)
	if chatID == "" {
		chatID = fallbackChatID
	}

	entry := citation.Citation{
		SourceType: citation.SourceMessage,
		// Title is the message itself, verbatim — the protocol defines it that
		// way, so no truncation and no ellipsis. Snippet stays empty: the
		// protocol calls it an excerpt of the body, and the whole body is
		// already in Title, so a copy adds nothing. The product's
		// atomic-command list maps this command as carrying no snippet.
		Title: text,
		// citation.Time normalizes to RFC3339 with an explicit offset. It
		// accepts unix seconds/millis and several formatted layouts, and
		// returns "" for anything it cannot parse — publish_time is optional,
		// so an unparseable timestamp degrades instead of failing.
		PublishTime: citation.Time(createTime),
	}

	appLink, _ := msg["message_app_link"].(string)
	entry.URL = strings.TrimSpace(appLink)
	if entry.URL == "" {
		entry.URL = chatOpenURL(brand, chatID)
	}
	return entry, true
}

// chatCitation builds one chat's citation.
//
// The cited object is the conversation, not anything said in it, so the
// text-only rule does not apply — a chat has no msg_type. Title is the group
// name, verbatim, as the protocol prescribes. Snippet is the group
// description: unlike a message's body it is genuinely distinct from the
// title, and it is the closest a chat has to the protocol's "excerpt". Both
// may legitimately be empty.
//
// The chat argument is one `meta_data` record from +chat-search, which is the
// endpoint's actual business record (the response nests it one level deeper
// than the item).
//
// SourceType is SourceMessage rather than a chat-specific scene because the
// consumer's protocol has no chat scene — the product's command list files
// +chat-search under the same host as the message commands.
func chatCitation(brand core.LarkBrand, chat map[string]interface{}) citation.Citation {
	chatID, _ := chat["chat_id"].(string)
	name, _ := chat["name"].(string)
	description, _ := chat["description"].(string)
	createTime, _ := chat["create_time"].(string)

	return citation.Citation{
		SourceType:  citation.SourceMessage,
		URL:         chatOpenURL(brand, chatID),
		Title:       name,
		Snippet:     description,
		PublishTime: citation.Time(createTime),
	}
}

// citationItems projects a list field of the output payload to a concrete
// slice of maps.
//
// The two cases are both real. Legacy shortcuts hand runtime.OutFormat a
// map[string]interface{} they assembled themselves, so a list they built stays
// []map[string]interface{}; a list that round-tripped through JSON or was
// copied from a decoded response arrives as []interface{} with map elements.
// The builder sees whichever the command happened to produce.
//
// Anything else — a missing key, a scalar, a list of non-maps — yields nil, and
// the caller degrades to "no citations" rather than failing the command.
func citationItems(data any, key string) []map[string]interface{} {
	out, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	switch v := out[key].(type) {
	case []map[string]interface{}:
		return v
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(v))
		for _, entry := range v {
			if m, ok := entry.(map[string]interface{}); ok {
				items = append(items, m)
			}
		}
		return items
	default:
		return nil
	}
}
