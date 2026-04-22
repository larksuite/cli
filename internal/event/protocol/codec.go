// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package protocol defines the newline-delimited JSON wire format the
// bus and consume client use to exchange events and lifecycle messages
// over an IPC socket.
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

// MaxFrameBytes caps the size of a single wire frame on read. Reached
// values come from legitimate consumer Hello / bus StatusResponse with
// many consumers / large event payloads; 1 MB is comfortable upper
// bound. Oversized frames (whether from a hostile local process or a
// runaway upstream event) are rejected rather than allowed to grow the
// reader's buffer unbounded and OOM the process.
const MaxFrameBytes = 1 << 20

// WriteTimeout is the deadline applied to every write of a control
// message (Hello, HelloAck, StatusQuery/Response, Shutdown,
// PreShutdownCheck/Ack). Prevents a wedged peer kernel buffer from
// stalling a writer indefinitely.
const WriteTimeout = 5 * time.Second

// typeEnvelope is used to peek at the "type" field before full decode.
type typeEnvelope struct {
	Type string `json:"type"`
}

// Encode writes a message as a single JSON line followed by \n.
func Encode(w io.Writer, msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("protocol encode: %w", err)
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

// EncodeWithDeadline wraps Encode with a WriteDeadline on conn so the
// write can't block forever on a wedged peer.
func EncodeWithDeadline(conn net.Conn, msg interface{}, timeout time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	return Encode(conn, msg)
}

// ReadFrame reads one newline-delimited message from br, rejecting
// frames larger than MaxFrameBytes with a size-cap error. Replaces the
// raw `br.ReadBytes('\n')` pattern which grows the internal buffer
// unbounded — a DoS vector any local peer could trigger by writing
// without a newline.
func ReadFrame(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		switch err {
		case nil:
			if len(buf) == 0 {
				// Fast path: whole frame fit in br's internal buffer.
				return chunk, nil
			}
			if len(buf)+len(chunk) > MaxFrameBytes {
				return nil, fmt.Errorf("protocol: frame exceeds %d bytes", MaxFrameBytes)
			}
			return append(buf, chunk...), nil
		case bufio.ErrBufferFull:
			if len(buf)+len(chunk) > MaxFrameBytes {
				return nil, fmt.Errorf("protocol: frame exceeds %d bytes", MaxFrameBytes)
			}
			buf = append(buf, chunk...)
		default:
			return nil, err
		}
	}
}

// Decode parses a single JSON line into the appropriate message type.
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
