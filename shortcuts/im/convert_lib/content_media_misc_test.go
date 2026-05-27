// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"math"
	"net/url"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error: %v", raw, err)
	}
	return u
}

func assertURLHasQuery(t *testing.T, raw, host, path string, want map[string]string) {
	t.Helper()
	u := mustParseURL(t, raw)
	if u.Scheme != "https" {
		t.Fatalf("url scheme = %q, want https (%q)", u.Scheme, raw)
	}
	if u.Host != host {
		t.Fatalf("url host = %q, want %q (%q)", u.Host, host, raw)
	}
	if u.Path != path {
		t.Fatalf("url path = %q, want %q (%q)", u.Path, path, raw)
	}
	q := u.Query()
	for k, v := range want {
		if got := q.Get(k); got != v {
			t.Fatalf("query[%q] = %q, want %q (%q)", k, got, v, raw)
		}
	}
}

func TestConvertBodyContent(t *testing.T) {
	ctx := &ConvertContext{RawContent: `{"text":"hello"}`}

	if got := ConvertBodyContent("text", ctx); got != "hello" {
		t.Fatalf("ConvertBodyContent(text) = %q, want %q", got, "hello")
	}
	if got := ConvertBodyContent("unknown_type", ctx); got != "[unknown_type]" {
		t.Fatalf("ConvertBodyContent(unknown) = %q, want %q", got, "[unknown_type]")
	}
	if got := ConvertBodyContent("text", &ConvertContext{}); got != "" {
		t.Fatalf("ConvertBodyContent(empty) = %q, want empty", got)
	}
}

func TestFormatMessageItem(t *testing.T) {
	raw := map[string]interface{}{
		"msg_type":    "text",
		"message_id":  "om_123",
		"deleted":     true,
		"updated":     true,
		"thread_id":   "omt_1",
		"create_time": "1710500000",
		"sender": map[string]interface{}{
			"id":          "ou_sender",
			"sender_type": "user",
		},
		"mentions": []interface{}{
			map[string]interface{}{"key": "@_user_1", "id": map[string]interface{}{"open_id": "ou_alice"}, "name": "Alice"},
		},
		"body": map[string]interface{}{
			"content": `{"text":"hi @_user_1"}`,
		},
	}

	got := FormatMessageItem(raw, nil)
	if got["message_id"] != "om_123" {
		t.Fatalf("FormatMessageItem() message_id = %#v", got["message_id"])
	}
	if got["content"] != "hi @Alice" {
		t.Fatalf("FormatMessageItem() content = %#v, want %#v", got["content"], "hi @Alice")
	}
	if got["create_time"] != common.FormatTime("1710500000") {
		t.Fatalf("FormatMessageItem() create_time = %#v, want %#v", got["create_time"], common.FormatTime("1710500000"))
	}
	if got["thread_id"] != "omt_1" {
		t.Fatalf("FormatMessageItem() thread_id = %#v, want %#v", got["thread_id"], "omt_1")
	}
	mentions, _ := got["mentions"].([]map[string]interface{})
	if len(mentions) != 1 || mentions[0]["id"] != "ou_alice" {
		t.Fatalf("FormatMessageItem() mentions = %#v", got["mentions"])
	}
}

func TestFormatMessageItem_UpdateTime_Present(t *testing.T) {
	raw := map[string]interface{}{
		"msg_type":    "text",
		"message_id":  "om_edit",
		"updated":     true,
		"create_time": "1710500000",
		"update_time": "1710600000",
		"sender":      map[string]interface{}{"id": "ou_sender", "sender_type": "user"},
		"body":        map[string]interface{}{"content": `{"text":"edited"}`},
	}

	got := FormatMessageItem(raw, nil)
	want := common.FormatTime("1710600000")
	if got["update_time"] != want {
		t.Fatalf("FormatMessageItem() update_time = %#v, want %#v", got["update_time"], want)
	}
}

func TestFormatMessageItem_UpdateTime_Absent(t *testing.T) {
	raw := map[string]interface{}{
		"msg_type":    "text",
		"message_id":  "om_no_edit",
		"updated":     false,
		"create_time": "1710500000",
		"sender":      map[string]interface{}{"id": "ou_sender", "sender_type": "user"},
		"body":        map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, nil)
	if _, ok := got["update_time"]; ok {
		t.Fatalf("FormatMessageItem() should not include update_time when absent, got = %#v", got["update_time"])
	}
}

// TestFormatMessageItem_UpdateTime_UnchangedMessage: real API behavior — even
// for unedited messages, server returns update_time == create_time. We must
// NOT echo it through, otherwise every message looks "edited" to consumers.
// Gate the output on updated==true.
func TestFormatMessageItem_UpdateTime_UnchangedMessage(t *testing.T) {
	raw := map[string]interface{}{
		"msg_type":    "text",
		"message_id":  "om_unchanged",
		"updated":     false,
		"create_time": "1710500000",
		"update_time": "1710500000", // server echoes create_time
		"sender":      map[string]interface{}{"id": "ou_sender", "sender_type": "user"},
		"body":        map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, nil)
	if v, ok := got["update_time"]; ok {
		t.Fatalf("FormatMessageItem() must skip update_time for unedited message, got = %#v", v)
	}
}

func TestResolveAppLinkDomain(t *testing.T) {
	if got := resolveAppLinkDomain(core.BrandFeishu); got != "applink.feishu.cn" {
		t.Fatalf("resolveAppLinkDomain(feishu) = %q", got)
	}
	if got := resolveAppLinkDomain(core.BrandLark); got != "applink.larksuite.com" {
		t.Fatalf("resolveAppLinkDomain(lark) = %q", got)
	}
	if got := resolveAppLinkDomain(core.LarkBrand("other")); got != "applink.feishu.cn" {
		t.Fatalf("resolveAppLinkDomain(other) = %q, want feishu", got)
	}
}

func TestFormatMessageItem_MessageAppLink_PassThrough(t *testing.T) {
	runtime := &common.RuntimeContext{Config: &core.CliConfig{Brand: core.BrandFeishu}}
	raw := map[string]interface{}{
		"msg_type":         "text",
		"message_id":       "om_123",
		"create_time":      "1710500000",
		"chat_id":          "oc_1",
		"message_position": 12,
		"message_app_link": "https://applink.feishu.cn/client/chat/open?openChatId=oc_1&position=12",
		"body":             map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, runtime)
	if got["message_app_link"] != raw["message_app_link"] {
		t.Fatalf("FormatMessageItem() message_app_link = %#v, want pass-through", got["message_app_link"])
	}
}

func TestFormatMessageItem_MessageAppLink_AssembleChat(t *testing.T) {
	runtime := &common.RuntimeContext{Config: &core.CliConfig{Brand: core.BrandFeishu}}
	raw := map[string]interface{}{
		"msg_type":         "text",
		"message_id":       "om_123",
		"create_time":      "1710500000",
		"chat_id":          "oc_1",
		"message_position": float64(12),
		"body":             map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, runtime)
	assertURLHasQuery(t, got["message_app_link"].(string), "applink.feishu.cn", "/client/chat/open", map[string]string{
		"openChatId": "oc_1",
		"position":   "12",
	})
}

func TestFormatMessageItem_MessageAppLink_AssembleThread(t *testing.T) {
	runtime := &common.RuntimeContext{Config: &core.CliConfig{Brand: core.BrandLark}}
	raw := map[string]interface{}{
		"msg_type":                "text",
		"message_id":              "om_123",
		"create_time":             "1710500000",
		"chat_id":                 "oc_1",
		"thread_id":               "omt_1",
		"thread_message_position": "9",
		"message_position":        12,
		"body":                    map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, runtime)
	assertURLHasQuery(t, got["message_app_link"].(string), "applink.larksuite.com", "/client/thread/open", map[string]string{
		"openthreadid":    "omt_1",
		"openchatid":      "oc_1",
		"open_thread_id":  "omt_1",
		"open_chat_id":    "oc_1",
		"thread_position": "9",
	})
}

func TestFormatMessageItem_MessageAppLink_FallbackToChatWhenThreadPositionInvalid(t *testing.T) {
	runtime := &common.RuntimeContext{Config: &core.CliConfig{Brand: core.BrandFeishu}}
	raw := map[string]interface{}{
		"msg_type":                "text",
		"message_id":              "om_123",
		"create_time":             "1710500000",
		"chat_id":                 "oc_1",
		"thread_id":               "omt_1",
		"thread_message_position": "bad",
		"message_position":        "12",
		"body":                    map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, runtime)
	assertURLHasQuery(t, got["message_app_link"].(string), "applink.feishu.cn", "/client/chat/open", map[string]string{
		"openChatId": "oc_1",
		"position":   "12",
	})
}

func TestFormatMessageItem_MessageAppLink_BrandUnknownDefaultsToFeishu(t *testing.T) {
	runtime := &common.RuntimeContext{Config: &core.CliConfig{Brand: core.LarkBrand("other")}}
	raw := map[string]interface{}{
		"msg_type":         "text",
		"message_id":       "om_123",
		"create_time":      "1710500000",
		"chat_id":          "oc_1",
		"message_position": 12,
		"body":             map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, runtime)
	assertURLHasQuery(t, got["message_app_link"].(string), "applink.feishu.cn", "/client/chat/open", map[string]string{
		"openChatId": "oc_1",
		"position":   "12",
	})
}

func TestNormalizeMessagePosition_TypedIntsAndUints(t *testing.T) {
	if got, ok := normalizeMessagePosition(int32(-3)); !ok || got != "-3" {
		t.Fatalf("normalizeMessagePosition(int32(-3)) = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition(uint64(9)); !ok || got != "9" {
		t.Fatalf("normalizeMessagePosition(uint64(9)) = (%q,%v)", got, ok)
	}
}

func TestNormalizeMessagePosition_CoversMoreNumericTypesAndInvalidInputs(t *testing.T) {
	// ints
	if got, ok := normalizeMessagePosition(int8(-1)); !ok || got != "-1" {
		t.Fatalf("normalizeMessagePosition(int8(-1)) = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition(int16(2)); !ok || got != "2" {
		t.Fatalf("normalizeMessagePosition(int16(2)) = (%q,%v)", got, ok)
	}

	// uints
	if got, ok := normalizeMessagePosition(uint(3)); !ok || got != "3" {
		t.Fatalf("normalizeMessagePosition(uint(3)) = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition(uintptr(4)); !ok || got != "4" {
		t.Fatalf("normalizeMessagePosition(uintptr(4)) = (%q,%v)", got, ok)
	}

	// float32
	if got, ok := normalizeMessagePosition(float32(1)); !ok || got != "1" {
		t.Fatalf("normalizeMessagePosition(float32(1)) = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition(float64(1.5)); !ok || got != "1.5" {
		t.Fatalf("normalizeMessagePosition(float64(1.5)) = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition(float64(-1.5)); !ok || got != "-1.5" {
		t.Fatalf("normalizeMessagePosition(float64(-1.5)) = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition(float32(math.NaN())); ok || got != "" {
		t.Fatalf("normalizeMessagePosition(float32(NaN)) = (%q,%v), want ('',false)", got, ok)
	}
	if got, ok := normalizeMessagePosition(float32(math.Inf(1))); ok || got != "" {
		t.Fatalf("normalizeMessagePosition(float32(+Inf)) = (%q,%v), want ('',false)", got, ok)
	}

	// json.Number invalid
	if got, ok := normalizeMessagePosition(json.Number("1.5")); !ok || got != "1.5" {
		t.Fatalf("normalizeMessagePosition(json.Number(1.5)) = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition(json.Number("bad")); ok || got != "" {
		t.Fatalf("normalizeMessagePosition(json.Number(bad)) = (%q,%v), want ('',false)", got, ok)
	}
	if got, ok := normalizeMessagePosition(json.Number("1e309")); ok || got != "" {
		t.Fatalf("normalizeMessagePosition(json.Number(1e309)) = (%q,%v), want ('',false)", got, ok)
	}

	// string invalid
	if got, ok := normalizeMessagePosition(" 1.5 "); !ok || got != "1.5" {
		t.Fatalf("normalizeMessagePosition(\" 1.5 \") = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition("   "); ok || got != "" {
		t.Fatalf("normalizeMessagePosition(blank) = (%q,%v), want ('',false)", got, ok)
	}
	if got, ok := normalizeMessagePosition("not-a-number"); ok || got != "" {
		t.Fatalf("normalizeMessagePosition(not-a-number) = (%q,%v), want ('',false)", got, ok)
	}

	// reflect fallback: pointers
	i := int32(7)
	if got, ok := normalizeMessagePosition(&i); !ok || got != "7" {
		t.Fatalf("normalizeMessagePosition(*int32(7)) = (%q,%v)", got, ok)
	}
	u := uint64(8)
	if got, ok := normalizeMessagePosition(&u); !ok || got != "8" {
		t.Fatalf("normalizeMessagePosition(*uint64(8)) = (%q,%v)", got, ok)
	}
	f := float64(2.25)
	if got, ok := normalizeMessagePosition(&f); !ok || got != "2.25" {
		t.Fatalf("normalizeMessagePosition(*float64(2.25)) = (%q,%v)", got, ok)
	}
	fNaN := float64(math.NaN())
	if got, ok := normalizeMessagePosition(&fNaN); ok || got != "" {
		t.Fatalf("normalizeMessagePosition(*float64(NaN)) = (%q,%v), want ('',false)", got, ok)
	}
	var nilPtr *int
	if got, ok := normalizeMessagePosition(nilPtr); ok || got != "" {
		t.Fatalf("normalizeMessagePosition(nil ptr) = (%q,%v), want ('',false)", got, ok)
	}
	if got, ok := normalizeMessagePosition(struct{}{}); ok || got != "" {
		t.Fatalf("normalizeMessagePosition(struct{}) = (%q,%v), want ('',false)", got, ok)
	}
}

func TestAssembleMessageAppLink_EncodesQueryValues(t *testing.T) {
	// chat link encoding
	chat := map[string]interface{}{
		"chat_id":          "oc_1+2/3",
		"message_position": 12,
	}
	gotChat := assembleMessageAppLink(chat, core.BrandFeishu)
	assertURLHasQuery(t, gotChat, "applink.feishu.cn", "/client/chat/open", map[string]string{
		"openChatId": "oc_1+2/3",
		"position":   "12",
	})

	// thread link encoding
	thread := map[string]interface{}{
		"chat_id":                 "oc_1+2/3",
		"thread_id":               "omt_1+2/3",
		"thread_message_position": -1,
	}
	gotThread := assembleMessageAppLink(thread, core.BrandFeishu)
	assertURLHasQuery(t, gotThread, "applink.feishu.cn", "/client/thread/open", map[string]string{
		"open_thread_id":  "omt_1+2/3",
		"open_chat_id":    "oc_1+2/3",
		"openthreadid":    "omt_1+2/3",
		"openchatid":      "oc_1+2/3",
		"thread_position": "-1",
	})
}

func TestFormatMessageItem_MessageAppLink_NonStringDoesNotLeakNull(t *testing.T) {
	runtime := &common.RuntimeContext{Config: &core.CliConfig{Brand: core.BrandFeishu}}
	raw := map[string]interface{}{
		"msg_type":         "text",
		"message_id":       "om_123",
		"create_time":      "1710500000",
		"chat_id":          "oc_1",
		"message_position": 12,
		"message_app_link": nil,
		"body":             map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, runtime)
	// Should assemble instead of emitting JSON null.
	assertURLHasQuery(t, got["message_app_link"].(string), "applink.feishu.cn", "/client/chat/open", map[string]string{
		"openChatId": "oc_1",
		"position":   "12",
	})
}

func TestFormatMessageItem_MessageAppLink_RuntimeNilNoAssemble(t *testing.T) {
	raw := map[string]interface{}{
		"msg_type":         "text",
		"message_id":       "om_123",
		"create_time":      "1710500000",
		"chat_id":          "oc_1",
		"message_position": 12,
		"body":             map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, nil)
	if _, ok := got["message_app_link"]; ok {
		t.Fatalf("FormatMessageItem() should not assemble without runtime, got %#v", got["message_app_link"])
	}
}

func TestFormatMessageItem_MessageAppLink_MissingFieldsNoPanic(t *testing.T) {
	runtime := &common.RuntimeContext{Config: &core.CliConfig{Brand: core.BrandFeishu}}
	raw := map[string]interface{}{
		"msg_type":    "text",
		"message_id":  "om_123",
		"create_time": "1710500000",
		"body":        map[string]interface{}{"content": `{"text":"hi"}`},
	}

	got := FormatMessageItem(raw, runtime)
	if _, ok := got["message_app_link"]; ok {
		t.Fatalf("FormatMessageItem() message_app_link should be absent when fields are missing, got %#v", got["message_app_link"])
	}
}

func TestNormalizeMessagePosition_AllowsZeroAndNegative(t *testing.T) {
	if got, ok := normalizeMessagePosition("0"); !ok || got != "0" {
		t.Fatalf("normalizeMessagePosition(\"0\") = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition("-3"); !ok || got != "-3" {
		t.Fatalf("normalizeMessagePosition(\"-3\") = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition(float64(0)); !ok || got != "0" {
		t.Fatalf("normalizeMessagePosition(0.0) = (%q,%v)", got, ok)
	}
	if got, ok := normalizeMessagePosition(float64(-1)); !ok || got != "-1" {
		t.Fatalf("normalizeMessagePosition(-1.0) = (%q,%v)", got, ok)
	}
}

func TestExtractMentionOpenIdAndTruncateContent(t *testing.T) {
	if got := extractMentionOpenId("ou_1"); got != "ou_1" {
		t.Fatalf("extractMentionOpenId(string) = %q", got)
	}
	if got := extractMentionOpenId(map[string]interface{}{"open_id": "ou_2"}); got != "ou_2" {
		t.Fatalf("extractMentionOpenId(map) = %q", got)
	}
	if got := extractMentionOpenId(123); got != "" {
		t.Fatalf("extractMentionOpenId(other) = %q, want empty", got)
	}

	if got := TruncateContent("hello\nworld", 20); got != "hello world" {
		t.Fatalf("TruncateContent(no truncate) = %q", got)
	}
	if got := TruncateContent("你好世界和平", 4); got != "你好世界…" {
		t.Fatalf("TruncateContent(truncate) = %q", got)
	}
}

func TestMediaConverters(t *testing.T) {
	if got := (imageConverter{}).Convert(&ConvertContext{RawContent: `{"image_key":"img_1"}`}); got != "[Image: img_1]" {
		t.Fatalf("imageConverter.Convert() = %q", got)
	}
	if got := (imageConverter{}).Convert(&ConvertContext{RawContent: `{invalid`}); got != "[Invalid image JSON]" {
		t.Fatalf("imageConverter.Convert(invalid) = %q", got)
	}
	if got := (fileConverter{}).Convert(&ConvertContext{RawContent: `{"file_key":"file_1","file_name":"demo.pdf"}`}); got != `<file key="file_1" name="demo.pdf"/>` {
		t.Fatalf("fileConverter.Convert() = %q", got)
	}
	if got := (fileConverter{}).Convert(&ConvertContext{RawContent: `{"file_key":"file_\"1","file_name":"demo\\\".pdf"}`}); got != `<file key="file_\"1" name="demo\\\".pdf"/>` {
		t.Fatalf("fileConverter.Convert(escaped) = %q", got)
	}
	if got := (audioMsgConverter{}).Convert(&ConvertContext{RawContent: `{"duration":3500}`}); got != "[Voice: 4s]" {
		t.Fatalf("audioMsgConverter.Convert() = %q", got)
	}
	if got := (videoMsgConverter{}).Convert(&ConvertContext{RawContent: `{"file_key":"file_2","file_name":"clip.mp4","duration":5000,"image_key":"img_cover"}`}); got != `<video key="file_2" name="clip.mp4" duration="5s" cover_image_key="img_cover"/>` {
		t.Fatalf("videoMsgConverter.Convert() = %q", got)
	}
	if got := (videoMsgConverter{}).Convert(&ConvertContext{RawContent: `{"file_key":"file_\"2","file_name":"clip\\\".mp4","duration":5000,"image_key":"img_\"cover"}`}); got != `<video key="file_\"2" name="clip\\\".mp4" duration="5s" cover_image_key="img_\"cover"/>` {
		t.Fatalf("videoMsgConverter.Convert(escaped) = %q", got)
	}
}

func TestMiscConverters(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "sticker", got: (stickerConverter{}).Convert(nil), want: "[Sticker]"},
		{name: "video chat", got: (videoChatConverter{}).Convert(nil), want: "[Video call]"},
		{name: "share chat", got: (shareChatConverter{}).Convert(&ConvertContext{RawContent: `{"chat_id":"oc_1"}`}), want: "[Chat card: oc_1]"},
		{name: "share user", got: (shareUserConverter{}).Convert(&ConvertContext{RawContent: `{"user_id":"ou_1"}`}), want: "[User card: ou_1]"},
		{name: "location", got: (locationConverter{}).Convert(&ConvertContext{RawContent: `{"name":"Shanghai"}`}), want: "[Location: Shanghai]"},
		{name: "folder", got: (folderConverter{}).Convert(&ConvertContext{RawContent: `{"file_key":"fld_1","file_name":"Docs"}`}), want: `<folder key="fld_1" name="Docs"/>`},
		{name: "calendar share", got: (calendarEventConverter{}).Convert(&ConvertContext{RawContent: `{"summary":"Review","start_time":"1710500000","end_time":"1710503600","open_calendar_id":"cal_1","open_event_id":"evt_1"}`}), want: "<calendar_share open_calendar_id=\"cal_1\" open_event_id=\"evt_1\">\nReview\n" + formatTimestamp("1710500000") + " ~ " + formatTimestamp("1710503600") + "\n</calendar_share>"},
		{name: "calendar invite", got: (calendarInviteConverter{}).Convert(&ConvertContext{RawContent: `{"summary":"Invite","start_time":"1710500000"}`}), want: "<calendar_invite>\nInvite\n" + formatTimestamp("1710500000") + "\n</calendar_invite>"},
		{name: "general calendar", got: (generalCalendarConverter{}).Convert(&ConvertContext{RawContent: `{"summary":"All Hands"}`}), want: "<calendar>\nAll Hands\n</calendar>"},
		{name: "vote", got: (voteConverter{}).Convert(&ConvertContext{RawContent: `{"topic":"Lunch","options":["A","B"],"status":1}`}), want: "<vote>\nLunch\n• A\n• B\n(Closed)\n</vote>"},
		{name: "hongbao", got: (hongbaoConverter{}).Convert(&ConvertContext{RawContent: `{"text":"恭喜发财"}`}), want: `<hongbao text="恭喜发财"/>`},
		{name: "system", got: (systemConverter{}).Convert(&ConvertContext{RawContent: `{"template":"{from_user} invited {to_chatters} to {name}","from_user":["Alice"],"to_chatters":["Bob","Carol"],"name":"Room A"}`}), want: "Alice invited Bob, Carol to Room A"},
		{name: "invalid user card", got: (shareUserConverter{}).Convert(&ConvertContext{RawContent: `{invalid`}), want: "[Invalid user card JSON]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestTodoConverter(t *testing.T) {
	got := (todoConverter{}).Convert(&ConvertContext{RawContent: `{"task_id":"task_1","summary":{"title":"Finish report","content":[[{"tag":"text","text":"prepare slides"}]]},"due_time":"1710500000"}`})
	want := "<todo task_id=\"task_1\">\nFinish report\nprepare slides\nDue: " + formatTimestamp("1710500000") + "\n</todo>"
	if got != want {
		t.Fatalf("todoConverter.Convert() = %q, want %q", got, want)
	}
}
