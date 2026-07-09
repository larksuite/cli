// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// spinnerFrames are braille spinner glyphs cycled to animate progress.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const (
	spinnerInterval   = 80 * time.Millisecond
	spinnerHideCursor = "\x1b[?25l"
	spinnerShowCursor = "\x1b[?25h"
	spinnerClearLine  = "\r\x1b[K" // CR + clear-to-end-of-line
)

// StartSpinner renders a braille spinner with an elapsed-seconds counter to w
// until the returned stop() is called, e.g.:
//
//	⠹ Publishing dev → main... 3s
//
// It is meant for slow operations (long polls, first-time provisioning) so the
// user sees the CLI is alive. Always write to STDERR (w = IO().ErrOut) so the
// animation never pollutes stdout — the JSON/pretty result stays clean.
//
// When enabled is false (stderr is not a TTY: pipes, CI, captured output) it is
// a no-op returning a no-op stop, so non-interactive runs emit nothing. Gate on
// the stderr-TTY check (IOStreams.StderrIsTerminal), not the output format: the
// spinner is stderr-only and self-clears, so it is shown in JSON mode too.
//
// stop() clears the spinner line, restores the cursor, and blocks until the
// render goroutine has finished — so callers can safely write the result to
// stdout/stderr immediately after. Call stop() BEFORE printing the result, and
// it is safe to call more than once (e.g. an explicit call plus a defer).
func StartSpinner(w io.Writer, enabled bool, label string) func() {
	if !enabled || w == nil {
		return func() {}
	}

	done := make(chan struct{})
	finished := make(chan struct{})
	start := time.Now()

	go func() {
		defer close(finished)
		frame := 0
		fmt.Fprint(w, spinnerHideCursor)
		render := func() {
			elapsed := int(time.Since(start).Seconds())
			fmt.Fprintf(w, "%s%s %s... %ds", spinnerClearLine, spinnerFrames[frame], label, elapsed)
			frame = (frame + 1) % len(spinnerFrames)
		}
		render()
		ticker := time.NewTicker(spinnerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				fmt.Fprint(w, spinnerClearLine+spinnerShowCursor)
				return
			case <-ticker.C:
				render()
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-finished // wait for the line to be cleared before returning
		})
	}
}
