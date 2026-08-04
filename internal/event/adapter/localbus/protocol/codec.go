// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package protocol defines the newline-delimited JSON wire format used over IPC.
package protocol

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// MaxEventPayloadBytes is the largest event body this wire format promises to
// relay. The ingress caps what it accepts at the same number; a boundary test
// keeps the two in step, since the ingress deliberately does not import this
// package.
const MaxEventPayloadBytes = 1 << 20

// maxFrameOverheadBytes is the room a frame gets on top of its payload for the
// canonical metadata and JSON punctuation around it. Worst-case realistic
// metadata measures a few hundred bytes, so this is generous on purpose: a
// frame limit that merely equalled the payload limit would make the top of the
// accepted payload range undeliverable, which is how an accepted event turned
// into a dropped one.
const maxFrameOverheadBytes = 4 << 10

// MaxFrameBytes bounds reader buffer growth. It must stay above
// MaxEventPayloadBytes, or the bus can frame an event the consumer then refuses
// to read.
const MaxFrameBytes = MaxEventPayloadBytes + maxFrameOverheadBytes

// ErrFrameTooLarge is returned by ReadFrame when a single frame exceeds MaxFrameBytes.
var ErrFrameTooLarge = errors.New("protocol: frame exceeds MaxFrameBytes")

const WriteTimeout = 5 * time.Second // bound writes against wedged peer kernel buffer

type typeEnvelope struct {
	Type string `json:"type"`
}

func Encode(w io.Writer, msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("protocol encode: %w", err)
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

func EncodeWithDeadline(conn net.Conn, msg interface{}, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	return Encode(conn, msg)
}

// ReadFrame reads one newline-delimited message; caps at MaxFrameBytes to defang slowloris.
func ReadFrame(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		switch err {
		case nil:
			if len(buf) == 0 {
				return chunk, nil
			}
			if len(buf)+len(chunk) > MaxFrameBytes {
				return nil, ErrFrameTooLarge
			}
			return append(buf, chunk...), nil
		case bufio.ErrBufferFull:
			if len(buf)+len(chunk) > MaxFrameBytes {
				return nil, ErrFrameTooLarge
			}
			buf = append(buf, chunk...)
		default:
			return nil, err
		}
	}
}

func Decode(line []byte) (interface{}, error) {
	var env typeEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return nil, fmt.Errorf("protocol decode type: %w", err)
	}

	var msg interface{}
	switch env.Type {
	case MsgTypeHello:
		msg = &Hello{}
	case MsgTypeHelloAck:
		msg = &HelloAck{}
	case MsgTypeEvent:
		msg = &Event{}
	case MsgTypeBye:
		msg = &Bye{}
	case MsgTypePreShutdownCheck:
		msg = &PreShutdownCheck{}
	case MsgTypePreShutdownAck:
		msg = &PreShutdownAck{}
	case MsgTypeStatusQuery:
		msg = &StatusQuery{}
	case MsgTypeStatusResponse:
		msg = &StatusResponse{}
	case MsgTypeShutdown:
		msg = &Shutdown{}
	case MsgTypeSourceStatus:
		msg = &SourceStatus{}
	default:
		return nil, fmt.Errorf("protocol: unknown message type %q", env.Type)
	}

	if err := json.Unmarshal(line, msg); err != nil {
		return nil, fmt.Errorf("protocol decode %s: %w", env.Type, err)
	}
	return msg, nil
}
