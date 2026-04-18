package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/larksuite/cli/internal/core"
)

// EventSubscriber manages Lark event subscription for the bot
type EventSubscriber struct {
	botHandler   *BotHandler
	sender       *MessageSender
	appID        string
	appSecret    core.SecretInput
	domain       string
	eventCount   int
	quiet        bool
	larkClient   *lark.Client
}

// EventSubscriberConfig configures a new EventSubscriber
type EventSubscriberConfig struct {
	BotHandler    *BotHandler
	MessageSender *MessageSender
	AppID         string
	AppSecret     core.SecretInput
	Brand         string // "feishu" or "lark"
	Quiet         bool
	LarkClient    *lark.Client // Optional; created from app credentials if nil
}

// NewEventSubscriber creates a new event subscriber
func NewEventSubscriber(config EventSubscriberConfig) *EventSubscriber {
	domain := lark.FeishuBaseUrl
	if config.Brand == "lark" {
		domain = lark.LarkBaseUrl
	}

	// Create Lark client from app credentials if not provided
	larkClient := config.LarkClient
	if larkClient == nil && config.AppID != "" && config.AppSecret.Plain != "" {
		larkClient = lark.NewClient(config.AppID, config.AppSecret.Plain)
	}

	// Create sender with real Lark client
	sender := config.MessageSender
	if sender == nil {
		if larkClient != nil {
			sender = NewMessageSenderWithClient(larkClient)
		} else {
			sender = &MessageSender{}
		}
	}

	return &EventSubscriber{
		botHandler: config.BotHandler,
		sender:     sender,
		appID:      config.AppID,
		appSecret:  config.AppSecret,
		domain:     domain,
		quiet:      config.Quiet,
		larkClient: larkClient,
	}
}

// Subscribe starts listening for Lark events
func (s *EventSubscriber) Subscribe(ctx context.Context) error {
	// Create event dispatcher
	eventDispatcher := dispatcher.NewEventDispatcher("", "")
	eventDispatcher.InitConfig()

	// Register message event handler
	rawHandler := s.createEventHandler()
	eventDispatcher.OnCustomizedEvent("im.message.receive_v1", rawHandler)

	// Create WebSocket client
	cli := larkws.NewClient(s.appID, s.appSecret.Plain,
		larkws.WithEventHandler(eventDispatcher),
		larkws.WithDomain(s.domain),
	)

	s.info("Connecting to Lark event WebSocket...")
	s.info("Listening for: im.message.receive_v1")

	// Setup graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- cli.Start(ctx)
	}()

	s.info("Connected. Waiting for events... (Ctrl+C to stop)")

	// Wait for shutdown or error
	select {
	case sig := <-sigCh:
		s.info(fmt.Sprintf("\nReceived %s, shutting down... (received %d events)", sig, s.eventCount))
		return nil
	case err := <-startErrCh:
		if err != nil {
			return fmt.Errorf("WebSocket connection failed: %w", err)
		}
		return nil
	}
}

// createEventHandler creates the Lark event handler
func (s *EventSubscriber) createEventHandler() func(ctx context.Context, event *larkevent.EventReq) error {
	return func(ctx context.Context, event *larkevent.EventReq) error {
		if event == nil || event.Body == nil {
			return nil
		}

			atomic.AddInt64(&s.eventCount, 1)

		// Parse event
		var rawData map[string]interface{}
		if err := json.Unmarshal(event.Body, &rawData); err != nil {
			s.error(fmt.Sprintf("Failed to parse event: %v", err))
			return nil
		}

		// Extract event type
		header, ok := rawData["header"].(map[string]interface{})
		if !ok {
			s.error("Event header missing")
			return nil
		}

		eventType, _ := header["event_type"].(string)
		s.debug(fmt.Sprintf("Received event: %s", eventType))

		// Handle message events
		if eventType == "im.message.receive_v1" {
			return s.handleMessageEvent(ctx, event)
		}

		return nil
	}
}

// handleMessageEvent processes an incoming message event
func (s *EventSubscriber) handleMessageEvent(ctx context.Context, event *larkevent.EventReq) error {
	// Process message through bot handler
	response, err := s.botHandler.HandleMessage(ctx, event)
	if err != nil {
		s.error(fmt.Sprintf("Failed to handle message: %v", err))
		return nil
	}

	// Send response back to Lark
	if response != "" {
		if err := s.sendReply(ctx, event, response); err != nil {
			s.error(fmt.Sprintf("Failed to send reply: %v", err))
			return nil
		}
		s.debug("Reply sent successfully")
	}

	return nil
}

// sendReply sends a reply message back to Lark
func (s *EventSubscriber) sendReply(ctx context.Context, event *larkevent.EventReq, message string) error {
	if event == nil || event.Body == nil {
		return fmt.Errorf("nil event or event body")
	}

	// Extract chat_id and message_id from event
	var rawData map[string]interface{}
	if err := json.Unmarshal(event.Body, &rawData); err != nil {
		return err
	}

	eventData, ok := rawData["event"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("event data not found")
	}

	chatID, _ := eventData["chat_id"].(string)
	messageID, _ := eventData["message_id"].(string)

	if chatID == "" {
		return fmt.Errorf("chat_id not found in event")
	}

	// Send reply using MessageSender
	return s.sender.SendMessage(ctx, chatID, message, messageID)
}

// GetStats returns subscriber statistics
func (s *EventSubscriber) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"events_received": s.eventCount,
		"app_id":          s.appID,
		"domain":          s.domain,
	}
}

// info prints an info message if not quiet
func (s *EventSubscriber) info(msg string) {
	if !s.quiet {
		fmt.Println(msg)
	}
}

// error prints an error message
func (s *EventSubscriber) error(msg string) {
	fmt.Fprintf(os.Stderr, "[Error] %s\n", msg)
}

// debug prints a debug message
func (s *EventSubscriber) debug(msg string) {
	// Only print in debug mode (TODO: add debug flag)
	// fmt.Printf("[Debug] %s\n", msg)
}
