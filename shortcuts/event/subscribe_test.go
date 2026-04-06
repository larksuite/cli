package event

import (
	"context"
	"reflect"
	"testing"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
)

func TestSubscribedEventTypesForCatchAllReturnsNil(t *testing.T) {
	got := subscribedEventTypesFor("")
	if got != nil {
		t.Fatalf("subscribedEventTypesFor(\"\") = %v, want nil for catch-all", got)
	}
}

func TestSubscribedEventTypesForExplicitListSortsValues(t *testing.T) {
	got := subscribedEventTypesFor("calendar.event.updated_v1,im.message.receive_v1")
	want := []string{"calendar.event.updated_v1", "im.message.receive_v1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subscribedEventTypesFor(explicit) = %v, want %v", got, want)
	}
}

func TestCatchAllUsesSDKDefaultCustomizedHandler(t *testing.T) {
	eventDispatcher := dispatcher.NewEventDispatcher("", "")
	eventDispatcher.OnCustomizedEvent("", func(_ context.Context, _ *larkevent.EventReq) error {
		return nil
	})

	_, err := eventDispatcher.Do(context.Background(), []byte(`{"header":{"event_type":"contact.user.created_v3"}}`))
	if err == nil {
		t.Fatal("dispatcher.Do() error = nil, want not found because parse() still routes by concrete event type")
	}
	if err.Error() != "event type: contact.user.created_v3, not found handler" {
		t.Fatalf("dispatcher.Do() error = %v, want concrete event type not found", err)
	}
}

func TestSubscribePipelineConfigUsesCompactFlag(t *testing.T) {
	config := pipelineConfigFor(false, true)
	if config.Mode != TransformCompact {
		t.Fatalf("Mode = %v, want TransformCompact", config.Mode)
	}
	if config.PrettyJSON {
		t.Fatalf("PrettyJSON = true, want false")
	}
}

func TestSubscribePipelineConfigUsesJSONFlag(t *testing.T) {
	config := pipelineConfigFor(true, false)
	if config.Mode != TransformRaw {
		t.Fatalf("Mode = %v, want TransformRaw", config.Mode)
	}
	if !config.PrettyJSON {
		t.Fatalf("PrettyJSON = false, want true")
	}
}
