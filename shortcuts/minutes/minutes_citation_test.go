// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/httpmock"
)

const (
	citationTestMinuteToken = "tok001"
	citationTestMinuteURL   = "https://example.feishu.cn/minutes/tok001"
	citationTestMinuteTitle = "fixture minute"
)

type minutesCitationEnvelope struct {
	Data      map[string]interface{} `json:"data"`
	Citations []string               `json:"citations"`
}

// minutesCitationDoc decodes one XML <document> citation string so per-field
// assertions stay readable.
type minutesCitationDoc struct {
	ReferenceID string              `xml:"reference_id,attr"`
	Title       string              `xml:"title"`
	SourceType  citation.SourceType `xml:"source_type"`
	URL         string              `xml:"url"`
}

func decodeMinutesCitationDoc(t *testing.T, s string) minutesCitationDoc {
	t.Helper()
	var d minutesCitationDoc
	if err := xml.Unmarshal([]byte(s), &d); err != nil {
		t.Fatalf("xml.Unmarshal(%q) error = %v", s, err)
	}
	return d
}

func decodeMinutesCitationEnvelope(t *testing.T, stdout *bytes.Buffer) minutesCitationEnvelope {
	t.Helper()
	var envelope minutesCitationEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v\nstdout=%s", err, stdout.String())
	}
	return envelope
}

// detailMinuteGetStubWithURL mirrors the real minutes API, which returns the
// minute's own url alongside title and token.
func detailMinuteGetStubWithURL(token, title, url string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"minute": map[string]interface{}{"title": title, "url": url, "token": token},
			},
		},
	}
}

func TestMinutesDetailCitationEnvelopePassesThroughAPIURL(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(detailMinuteGetStubWithURL(citationTestMinuteToken, citationTestMinuteTitle, citationTestMinuteURL))

	if err := detailMountAndRun(t, MinutesDetail,
		[]string{"+detail", "--minute-tokens", citationTestMinuteToken, "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}

	envelope := decodeMinutesCitationEnvelope(t, stdout)
	if len(envelope.Citations) != 1 {
		t.Fatalf("citations = %#v, want exactly 1", envelope.Citations)
	}
	doc := decodeMinutesCitationDoc(t, envelope.Citations[0])
	if doc.SourceType != citation.SourceMinute {
		t.Errorf("source_type = %d, want %d", doc.SourceType, citation.SourceMinute)
	}
	if doc.URL != citationTestMinuteURL {
		t.Errorf("url = %q, want the url the API returned", doc.URL)
	}
	if doc.Title != citationTestMinuteTitle {
		t.Errorf("title = %q, want %q", doc.Title, citationTestMinuteTitle)
	}
	if doc.ReferenceID != doc.URL {
		t.Errorf("reference_id = %q, want it to mirror the url", doc.ReferenceID)
	}

	// The url reaches the builder through the unserialized struct field, so it
	// must never surface in data even when citations are on.
	minutes, _ := envelope.Data["minutes"].([]interface{})
	if len(minutes) != 1 {
		t.Fatalf("data.minutes = %#v, want 1 entry", envelope.Data["minutes"])
	}
	if first, _ := minutes[0].(map[string]interface{}); first["url"] != nil {
		t.Errorf("data.minutes[0].url = %v, want the url to stay off the wire", first["url"])
	}
}

func TestMinutesDetailCitationsAreOmittedWhenGateIsOff(t *testing.T) {
	t.Setenv(envvars.CliCitation, "0")
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(detailMinuteGetStubWithURL(citationTestMinuteToken, citationTestMinuteTitle, citationTestMinuteURL))

	if err := detailMountAndRun(t, MinutesDetail,
		[]string{"+detail", "--minute-tokens", citationTestMinuteToken, "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	envelope := decodeMinutesCitationEnvelope(t, stdout)
	if envelope.Citations != nil {
		t.Errorf("citations = %#v, want none with the gate off", envelope.Citations)
	}
	// url only exists to feed the builder, so with the gate off the payload
	// must look exactly as it did before this command emitted citations.
	minutes, _ := envelope.Data["minutes"].([]interface{})
	if len(minutes) != 1 {
		t.Fatalf("data.minutes = %#v, want 1 entry", envelope.Data["minutes"])
	}
	if first, _ := minutes[0].(map[string]interface{}); first["url"] != nil {
		t.Errorf("data.minutes[0].url = %v, want absent with the gate off", first["url"])
	}
}

func TestMinutesDetailCitationsSkipMinutesWithoutURL(t *testing.T) {
	t.Setenv(envvars.CliCitation, "1")
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// An older tenant may not return url; the entry must be dropped, not faked.
	reg.Register(detailMinuteGetStub(citationTestMinuteToken, "note_1", citationTestMinuteTitle))

	if err := detailMountAndRun(t, MinutesDetail,
		[]string{"+detail", "--minute-tokens", citationTestMinuteToken, "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if got := decodeMinutesCitationEnvelope(t, stdout); got.Citations != nil {
		t.Errorf("citations = %#v, want none when the API returns no url", got.Citations)
	}
}
