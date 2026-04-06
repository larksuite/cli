package event

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/spf13/cobra"
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

func TestCatchAllSubscribedEventTypesIncludeLegacyNonIMDomains(t *testing.T) {
	want := []string{
		"contact.user.created_v3",
		"contact.user.updated_v3",
		"contact.user.deleted_v3",
		"contact.department.created_v3",
		"contact.department.updated_v3",
		"contact.department.deleted_v3",
		"calendar.calendar.acl.created_v4",
		"calendar.calendar.event.changed_v4",
		"approval.approval.updated",
		"application.application.visibility.added_v6",
		"task.task.update_tenant_v1",
		"task.task.comment_updated_v1",
		"drive.notice.comment_add_v1",
	}

	gotSet := make(map[string]struct{}, len(subscribedEventTypes))
	for _, eventType := range subscribedEventTypes {
		gotSet[eventType] = struct{}{}
	}

	for _, eventType := range want {
		if _, ok := gotSet[eventType]; !ok {
			t.Fatalf("catch-all subscribedEventTypes missing legacy event type %q", eventType)
		}
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

type fakeSubscribeClient struct {
	start func(context.Context) error
}

func (c fakeSubscribeClient) Start(ctx context.Context) error {
	return c.start(ctx)
}

func newSubscribeRuntimeForTest(t *testing.T) (*common.RuntimeContext, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	cfg := &core.CliConfig{
		AppID:     "cli_test_app",
		AppSecret: "cli_test_secret",
		Brand:     core.BrandFeishu,
	}
	factory, stdoutBuf, stderrBuf, _ := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "event +subscribe"}
	cmd.Flags().String("output-dir", "", "")
	cmd.Flags().StringArray("route", nil, "")
	cmd.Flags().Bool("compact", false, "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("event-types", "", "")
	cmd.Flags().String("filter", "", "")
	cmd.Flags().Bool("quiet", false, "")
	cmd.Flags().Bool("force", false, "")

	return &common.RuntimeContext{
		Cmd:     cmd,
		Config:  cfg,
		Factory: factory,
	}, stdoutBuf, stderrBuf
}

func TestEventSubscribeExecuteCatchAllProcessesLegacyDomain(t *testing.T) {
	runtime, stdoutBuf, _ := newSubscribeRuntimeForTest(t)
	if err := runtime.Cmd.Flags().Set("force", "true"); err != nil {
		t.Fatalf("Set(force) error = %v", err)
	}

	origFactory := newWSClient
	newWSClient = func(_ *core.CliConfig, eventDispatcher *dispatcher.EventDispatcher, _ larkcore.Logger) wsClient {
		return fakeSubscribeClient{
			start: func(ctx context.Context) error {
				_, err := eventDispatcher.Do(ctx, []byte(`{"schema":"2.0","header":{"event_id":"evt_contact","event_type":"contact.user.created_v3"},"event":{"user":{"open_id":"ou_123"}}}`))
				return err
			},
		}
	}
	t.Cleanup(func() { newWSClient = origFactory })

	if err := EventSubscribe.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("EventSubscribe.Execute() error = %v", err)
	}

	got := stdoutBuf.String()
	if got == "" {
		t.Fatal("stdout is empty, want routed contact event record")
	}
	if !bytes.Contains(stdoutBuf.Bytes(), []byte(`"event_type":"contact.user.created_v3"`)) {
		t.Fatalf("stdout = %q, want contact.user.created_v3 record", got)
	}
	if !bytes.Contains(stdoutBuf.Bytes(), []byte(`"status":"handled"`)) {
		t.Fatalf("stdout = %q, want handled status", got)
	}
}

func TestEventSubscribeExecuteRouteWritesEventFile(t *testing.T) {
	runtime, stdoutBuf, _ := newSubscribeRuntimeForTest(t)
	if err := runtime.Cmd.Flags().Set("force", "true"); err != nil {
		t.Fatalf("Set(force) error = %v", err)
	}

	tmpDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()

	routeDir := filepath.Join(tmpDir, "im")
	routeSpec := `^im\.message=dir:./im`
	if err := runtime.Cmd.Flags().Set("route", routeSpec); err != nil {
		t.Fatalf("Set(route) error = %v", err)
	}

	origFactory := newWSClient
	newWSClient = func(_ *core.CliConfig, eventDispatcher *dispatcher.EventDispatcher, _ larkcore.Logger) wsClient {
		return fakeSubscribeClient{
			start: func(ctx context.Context) error {
				_, err := eventDispatcher.Do(ctx, []byte(`{"schema":"2.0","header":{"event_id":"evt_im","event_type":"im.message.receive_v1"},"event":{"message":{"message_id":"om_123"}}}`))
				return err
			},
		}
	}
	t.Cleanup(func() { newWSClient = origFactory })

	if err := EventSubscribe.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("EventSubscribe.Execute() error = %v", err)
	}

	if stdoutBuf.Len() != 0 {
		t.Fatalf("stdout = %q, want routed file output only", stdoutBuf.String())
	}

	entries, err := os.ReadDir(routeDir)
	if err != nil {
		t.Fatalf("ReadDir(routeDir) error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	body, err := os.ReadFile(filepath.Join(routeDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var record map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(body), &record); err != nil {
		t.Fatalf("unmarshal routed record: %v", err)
	}
	if got, want := record["event_type"], "im.message.receive_v1"; got != want {
		t.Fatalf("event_type = %v, want %v", got, want)
	}
}
