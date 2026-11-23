// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package flagalias

import (
	"errors"

	"github.com/spf13/pflag"
)

// InvalidValueAttribution identifies the canonical flag and the long-form
// spelling that supplied a value which pflag could not convert. Names do not
// include leading dashes.
type InvalidValueAttribution struct {
	Canonical string
	Source    string
}

// InvalidValueAttributionOf resolves a typed pflag conversion error for a flag
// managed by Bind. Ordinary pflags return ok=false so installing aliases does
// not broaden the root error contract for unrelated commands.
//
// pflag's InvalidValueError does not report whether a canonical flag with a
// shorthand was supplied as -x or --long. That ambiguous case also returns
// ok=false; an alias spelling remains unambiguous because Bind records it
// before value conversion.
func InvalidValueAttributionOf(err error) (InvalidValueAttribution, bool) {
	var invalidValue *pflag.InvalidValueError
	if !errors.As(err, &invalidValue) || invalidValue == nil {
		return InvalidValueAttribution{}, false
	}
	flag := invalidValue.GetFlag()
	if flag == nil || len(Aliases(flag)) == 0 {
		return InvalidValueAttribution{}, false
	}

	canonical := flag.Name
	source := Source(flag)
	if source == "" || (source == canonical && flag.Shorthand != "") {
		return InvalidValueAttribution{}, false
	}
	return InvalidValueAttribution{Canonical: canonical, Source: source}, true
}
