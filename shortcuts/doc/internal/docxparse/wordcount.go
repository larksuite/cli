// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

// This file implements the LarkOpenCLI document text-counting contract.

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"
)

const chinesePunctuation = "，。！？；：、（）《》〈〉“”‘’【】「」『』〔〕…—～·￥"
const englishPunctuation = `!"#$%&'()*+,-./:;<=>?@[\]^_` + "`" + `{|}~`

var (
	urlToken           = regexp.MustCompile(`^https?://[!-~]+`)
	asciiCompoundToken = regexp.MustCompile(`^[A-Za-z0-9]+(?:[._/@:-][A-Za-z0-9]+)+`)
)

type lexemeKind uint8

const (
	lexemeNone lexemeKind = iota
	lexemeEnglish
	lexemeNumber
)

type textCounter struct {
	stats           TextProfile
	lexeme          lexemeKind
	lexemeHasDigit  bool
	symbolRunLength int
	atBoundary      bool
}

func newTextCounter() *textCounter {
	return &textCounter{atBoundary: true}
}

func (c *textCounter) countSegments(segments []textSegment) TextProfile {
	for _, segment := range segments {
		c.endUnit()
		c.atBoundary = true
		switch segment.kind {
		case segmentMarker:
			c.writeMarker(segment.text)
		case segmentCode:
			c.writeCode(segment.text)
		default:
			c.write(segment.text)
		}
		c.endUnit()
		c.atBoundary = true
	}
	c.endUnit()
	return c.stats
}

func (c *textCounter) write(value string) {
	for offset := 0; offset < len(value); {
		if c.lexeme == lexemeNone && isASCIIAlphaNumericByte(value[offset]) {
			if token := matchASCIICompound(value[offset:]); token != "" {
				c.writeASCIICompound(token)
				offset += len(token)
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(value[offset:])
		if r == '/' && isVisibleHanSeparator(value, offset, size) {
			c.endUnit()
			c.stats.Breakdown.EnglishPunctuations++
			c.stats.Breakdown.SymbolWords++
			c.stats.WordCount++
			c.stats.CharCount++
			c.atBoundary = false
			offset += size
			continue
		}
		c.writeRune(r)
		offset += size
	}
}

func isASCIIAlphaNumericByte(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}

func (c *textCounter) writeMarker(value string) {
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		c.endUnit()
		c.stats.WordCount++
		c.stats.CharCount++
		c.atBoundary = false
	}
}

func (c *textCounter) writeCode(value string) {
	for _, r := range value {
		c.writeCodeRune(r)
	}
}

func (c *textCounter) writeCodeRune(r rune) {
	if unicode.IsSpace(r) {
		c.endUnit()
		c.atBoundary = true
		return
	}
	if unicode.Is(unicode.Han, r) {
		c.endLexeme()
		c.endSymbolRun(false)
		c.stats.Breakdown.HanChars++
		c.stats.WordCount++
		c.stats.CharCount++
		c.atBoundary = false
		return
	}
	if isASCIILetterRune(r) {
		c.endSymbolRun(false)
		c.stats.Breakdown.EnglishLetters++
		c.stats.CharCount++
		if c.lexeme == lexemeNone || c.lexeme == lexemeNumber {
			c.lexeme = lexemeEnglish
		}
		c.atBoundary = false
		return
	}
	if isASCIIDigitRune(r) {
		c.endSymbolRun(false)
		c.stats.Breakdown.Digits++
		c.stats.CharCount++
		c.lexemeHasDigit = true
		if c.lexeme == lexemeNone {
			c.lexeme = lexemeNumber
		}
		c.atBoundary = false
		return
	}
	if isChinesePunctuation(r) {
		c.endLexeme()
		c.endSymbolRun(false)
		c.stats.Breakdown.ChinesePunctuations++
		c.stats.WordCount++
		c.stats.CharCount++
		c.atBoundary = false
		return
	}
	if isEnglishPunctuation(r) {
		keepsLexeme := c.lexeme == lexemeEnglish && (r == '\'' || r == '-')
		if !keepsLexeme {
			hadLexeme := c.lexeme != lexemeNone
			c.endLexeme()
			if !hadLexeme && (c.symbolRunLength > 0 || c.atBoundary) {
				c.symbolRunLength++
			}
		}
		c.stats.Breakdown.EnglishPunctuations++
		c.stats.CharCount++
		if keepsLexeme {
			c.atBoundary = false
		}
		return
	}
	if unicode.Is(unicode.Symbol, r) {
		c.writeSymbol(r)
		return
	}
	c.endLexeme()
	c.endSymbolRun(false)
	c.atBoundary = false
}

func (c *textCounter) writeRune(r rune) {
	if unicode.IsSpace(r) {
		c.endUnit()
		c.atBoundary = true
		return
	}
	if unicode.Is(unicode.Han, r) {
		c.endLexeme()
		c.endSymbolRun(false)
		c.stats.Breakdown.HanChars++
		c.stats.WordCount++
		c.stats.CharCount++
		c.atBoundary = false
		return
	}
	if isASCIILetterRune(r) {
		c.endSymbolRun(false)
		c.stats.Breakdown.EnglishLetters++
		c.stats.CharCount++
		if c.lexeme == lexemeNone || c.lexeme == lexemeNumber {
			c.lexeme = lexemeEnglish
		}
		c.atBoundary = false
		return
	}
	if isASCIIDigitRune(r) {
		c.endSymbolRun(false)
		c.stats.Breakdown.Digits++
		c.stats.CharCount++
		c.lexemeHasDigit = true
		if c.lexeme == lexemeNone {
			c.lexeme = lexemeNumber
		}
		c.atBoundary = false
		return
	}
	if isChinesePunctuation(r) {
		c.endLexeme()
		c.endSymbolRun(false)
		c.stats.Breakdown.ChinesePunctuations++
		c.stats.WordCount++
		c.stats.CharCount++
		c.atBoundary = false
		return
	}
	if isEnglishPunctuation(r) {
		keepsLexeme := c.lexeme == lexemeEnglish && (r == '\'' || r == '-' || c.lexemeHasDigit && r == '.') ||
			c.lexeme == lexemeNumber && (r == '.' || r == ',' || r == '-')
		if !keepsLexeme {
			hadLexeme := c.lexeme != lexemeNone
			c.endLexeme()
			if !hadLexeme && (c.symbolRunLength > 0 || c.atBoundary) {
				c.symbolRunLength++
			}
		}
		c.stats.Breakdown.EnglishPunctuations++
		c.stats.CharCount++
		if keepsLexeme {
			c.atBoundary = false
		}
		return
	}
	if unicode.Is(unicode.Symbol, r) {
		c.writeSymbol(r)
		return
	}
	c.endLexeme()
	c.endSymbolRun(false)
	c.atBoundary = false
}

func matchASCIICompound(value string) string {
	if match := urlToken.FindString(value); match != "" {
		return match
	}
	match := asciiCompoundToken.FindString(value)
	if match == "" || !strings.ContainsAny(match, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return ""
	}
	return match
}

func (c *textCounter) writeASCIICompound(token string) {
	c.endUnit()
	c.stats.Breakdown.EnglishWords++
	c.stats.WordCount++
	for _, r := range token {
		switch {
		case isASCIILetterRune(r):
			c.stats.Breakdown.EnglishLetters++
			c.stats.CharCount++
		case isASCIIDigitRune(r):
			c.stats.Breakdown.Digits++
			c.stats.CharCount++
		case isEnglishPunctuation(r):
			c.stats.Breakdown.EnglishPunctuations++
			c.stats.CharCount++
		}
	}
	c.atBoundary = false
}

func (c *textCounter) writeSymbol(r rune) {
	c.endLexeme()
	c.endSymbolRun(false)
	units := utf16Units(r)
	c.stats.Breakdown.SymbolWords++
	c.stats.Breakdown.SymbolChars += units
	c.stats.WordCount++
	c.stats.CharCount += units
	c.atBoundary = false
}

func (c *textCounter) endUnit() {
	c.endLexeme()
	c.endSymbolRun(true)
}

func (c *textCounter) endLexeme() {
	switch c.lexeme {
	case lexemeEnglish:
		c.stats.Breakdown.EnglishWords++
		c.stats.WordCount++
	case lexemeNumber:
		c.stats.Breakdown.NumberWords++
		c.stats.WordCount++
	}
	c.lexeme = lexemeNone
	c.lexemeHasDigit = false
}

func (c *textCounter) endSymbolRun(countWord bool) {
	if c.symbolRunLength > 0 && countWord {
		c.stats.Breakdown.SymbolWords++
		c.stats.WordCount++
	}
	if c.symbolRunLength > 0 {
		c.atBoundary = false
	}
	c.symbolRunLength = 0
}

func isVisibleHanSeparator(value string, offset, size int) bool {
	if offset == 0 || offset+size >= len(value) {
		return false
	}
	previous, _ := utf8.DecodeLastRuneInString(value[:offset])
	next, _ := utf8.DecodeRuneInString(value[offset+size:])
	return unicode.Is(unicode.Han, previous) && unicode.Is(unicode.Han, next)
}

func isASCIILetterRune(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }
func isASCIIDigitRune(r rune) bool  { return r >= '0' && r <= '9' }

func isChinesePunctuation(r rune) bool {
	if strings.ContainsRune(chinesePunctuation, r) {
		return true
	}
	kind := width.LookupRune(r).Kind()
	return unicode.Is(unicode.Punct, r) && (kind == width.EastAsianWide || kind == width.EastAsianFullwidth)
}

func isEnglishPunctuation(r rune) bool {
	return r < utf8.RuneSelf && strings.ContainsRune(englishPunctuation, r)
}

func utf16Units(r rune) int {
	if r > 0xffff {
		return 2
	}
	return 1
}
