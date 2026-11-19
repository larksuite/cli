package common

import (
	"bytes"
	"testing"
)

func TestReadStdinBytesOnceBounded(t *testing.T) {
	r := newTestRuntimeWithStdin(nil, "%PDF-1.4\x00\xff\n")
	got, err := r.ReadStdinBytesOnce("--attach-stdin-name", 32)
	if err != nil || !bytes.Equal(got, []byte("%PDF-1.4\x00\xff\n")) {
		t.Fatalf("bytes=%q err=%v", got, err)
	}
	if _, err := r.ReadStdinBytesOnce("--other", 32); err == nil {
		t.Fatal("stdin must be single use")
	}
}
func TestReadStdinBytesRejectsOverflow(t *testing.T) {
	r := newTestRuntimeWithStdin(nil, "12345")
	if _, err := r.ReadStdinBytesOnce("--attach-stdin-name", 4); err == nil {
		t.Fatal("expected bounded read rejection")
	}
}
func TestReadStdinBytesRejectsPriorTextConsumer(t *testing.T) {
	r := newTestRuntimeWithStdin(map[string]string{"body": "-"}, "body")
	if err := resolveInputFlags(r, []Flag{{Name: "body", Input: []string{Stdin}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadStdinBytesOnce("--attach-stdin-name", 32); err == nil {
		t.Fatal("must reject stdin already consumed by body")
	}
}

func TestReadStdinBytesAcceptsExactLimit(t *testing.T) {
	r := newTestRuntimeWithStdin(nil, "1234")
	got, err := r.ReadStdinBytesOnce("--attach-stdin-name", 4)
	if err != nil || string(got) != "1234" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}
func TestReadStdinBytesBlocksSubsequentTextConsumer(t *testing.T) {
	r := newTestRuntimeWithStdin(map[string]string{"body": "-"}, "1234")
	if _, err := r.ReadStdinBytesOnce("--attach-stdin-name", 4); err != nil {
		t.Fatal(err)
	}
	if err := resolveInputFlags(r, []Flag{{Name: "body", Input: []string{Stdin}}}); err == nil {
		t.Fatal("text flag reused binary stdin")
	}
}
