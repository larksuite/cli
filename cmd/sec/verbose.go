// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// verboseOut returns the trace destination for a sec subcommand: the given
// stderr writer when the inherited --verbose / -v flag is set, otherwise nil.
// Pair with tracef — a nil destination silently drops traces, so callers can
// emit unconditionally.
func verboseOut(cmd *cobra.Command, errOut io.Writer) io.Writer {
	if v, _ := cmd.Flags().GetBool("verbose"); v {
		return errOut
	}
	return nil
}

// tracef writes one trace line to w when w is non-nil. The prefix names the
// emitting subcommand (e.g. "sec run") so layered output from the install
// pipeline + the command itself stays distinguishable.
func tracef(w io.Writer, prefix, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "[%s] "+format+"\n", append([]any{prefix}, args...)...)
}
