package bus

import (
	"bufio"
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

func TestConn_SenderWritesEvents(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	bc := NewConn(server, nil, "im.msg", []string{"im.message.receive_v1"}, 12345)
	go bc.SenderLoop()

	bc.SendCh() <- &protocol.Event{
		Type:      protocol.MsgTypeEvent,
		EventType: "im.message.receive_v1",
	}

	scanner := bufio.NewScanner(client)
	client.SetReadDeadline(time.Now().Add(time.Second))
	if !scanner.Scan() {
		t.Fatalf("expected to read a line: %v", scanner.Err())
	}
	line := scanner.Bytes()
	if !bytes.Contains(line, []byte(`"event"`)) {
		t.Errorf("unexpected line: %s", line)
	}
}

func TestConn_ReaderDetectsEOF(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	bc := NewConn(server, nil, "im.msg", []string{"im.msg"}, 12345)

	done := make(chan struct{})
	go func() {
		bc.ReaderLoop()
		close(done)
	}()

	client.Close()

	select {
	case <-done:
		// ReaderLoop exited
	case <-time.After(time.Second):
		t.Fatal("ReaderLoop did not exit on EOF")
	}
}
