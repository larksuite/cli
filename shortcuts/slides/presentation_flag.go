// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import "github.com/larksuite/cli/shortcuts/common"

const presentationRefDescription = "xml_presentation_id, slides URL, or wiki URL that resolves to slides"

var presentationFlagAliases = []string{
	"presentation-id",
	"presentation-token",
	"token",
	"presentation_id",
	"xml-presentation-id",
	"url",
}

// basePresentationRefFlag declares the shared Slides presentation-locator
// contract. Every alias accepts the same token / Slides URL / Wiki URL grammar
// as the canonical --presentation flag.
func basePresentationRefFlag() common.Flag {
	return common.Flag{
		Name:    "presentation",
		Aliases: append([]string(nil), presentationFlagAliases...),
		Desc:    presentationRefDescription,
	}
}

func requiredPresentationRefFlag() common.Flag {
	flag := basePresentationRefFlag()
	flag.Required = true
	return flag
}

func listModePresentationRefFlag() common.Flag {
	flag := basePresentationRefFlag()
	flag.Desc += "; list mode only"
	return flag
}
