// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"fmt"
	"strings"
)

const calendarMediaType = "text/calendar"

// applyCalendarSet installs or replaces the text/calendar MIME part in the
// snapshot. The caller is expected to have pre-built icsData using the
// snapshot's From/To/Cc addresses.
func applyCalendarSet(snapshot *DraftSnapshot, icsData []byte) error {
	if len(icsData) == 0 {
		return fmt.Errorf("set_calendar: ICS data is empty (shortcut layer must pre-build it)")
	}
	setCalendarPart(snapshot, icsData)
	return nil
}

// applyCalendarRemove strips the text/calendar part from the snapshot.
// No-op if no calendar part exists.
func applyCalendarRemove(snapshot *DraftSnapshot) error {
	removeCalendarPart(snapshot)
	return nil
}

// setCalendarPart places exactly one text/calendar part inside
// multipart/alternative, matching the Feishu client behavior. Any existing
// text/calendar parts elsewhere in the tree are removed first.
func setCalendarPart(snapshot *DraftSnapshot, icsData []byte) {
	newPart := &Part{
		MediaType:   calendarMediaType,
		MediaParams: map[string]string{"charset": "UTF-8", "method": "REQUEST"},
		Body:        icsData,
		Dirty:       true,
	}

	if snapshot.Body == nil {
		snapshot.Body = newPart
		return
	}

	// Remove all existing text/calendar parts from everywhere in the tree.
	if strings.EqualFold(snapshot.Body.MediaType, calendarMediaType) {
		snapshot.Body = newPart
		return
	}
	removeAllPartsByMediaType(snapshot.Body, calendarMediaType)

	// Place inside the existing multipart/alternative.
	if alt := FindPartByMediaType(snapshot.Body, "multipart/alternative"); alt != nil {
		alt.Children = append(alt.Children, newPart)
		alt.Dirty = true
		return
	}

	// No multipart/alternative exists. If the body is a single leaf,
	// wrap it in multipart/alternative together with the calendar.
	if !snapshot.Body.IsMultipart() {
		original := *snapshot.Body
		// Reset all header-carrying fields so the serializer constructs a fresh
		// Content-Type from MediaType instead of reusing the stale leaf headers.
		snapshot.Body.Headers = nil
		snapshot.Body.MediaType = "multipart/alternative"
		snapshot.Body.MediaParams = nil
		snapshot.Body.ContentDisposition = ""
		snapshot.Body.ContentDispositionArg = nil
		snapshot.Body.ContentID = ""
		snapshot.Body.PartID = ""
		snapshot.Body.Body = nil
		snapshot.Body.TransferEncoding = ""
		snapshot.Body.RawEntity = nil
		snapshot.Body.Preamble = nil
		snapshot.Body.Epilogue = nil
		snapshot.Body.EncodingProblem = false
		snapshot.Body.Children = []*Part{&original, newPart}
		snapshot.Body.Dirty = true
		return
	}

	// Multipart body without an alternative sub-part (e.g. multipart/mixed
	// with a text/html child). Find the first text/* child and wrap it in
	// a new multipart/alternative that also contains the calendar.
	for i, child := range snapshot.Body.Children {
		if child != nil && strings.HasPrefix(strings.ToLower(child.MediaType), "text/") {
			alt := &Part{
				MediaType: "multipart/alternative",
				Children:  []*Part{child, newPart},
				Dirty:     true,
			}
			snapshot.Body.Children[i] = alt
			snapshot.Body.Dirty = true
			return
		}
	}

	// Fallback: append to the root multipart container.
	snapshot.Body.Children = append(snapshot.Body.Children, newPart)
	snapshot.Body.Dirty = true
}

func removeCalendarPart(snapshot *DraftSnapshot) {
	if snapshot.Body == nil {
		return
	}
	if strings.EqualFold(snapshot.Body.MediaType, calendarMediaType) {
		snapshot.Body = nil
		return
	}
	removeAllPartsByMediaType(snapshot.Body, calendarMediaType)
}

// FindPartByMediaType walks the MIME tree and returns the first part with
// the given media type, or nil when not found.
func FindPartByMediaType(root *Part, mediaType string) *Part {
	if root == nil {
		return nil
	}
	if strings.EqualFold(root.MediaType, mediaType) {
		return root
	}
	for _, child := range root.Children {
		if found := FindPartByMediaType(child, mediaType); found != nil {
			return found
		}
	}
	return nil
}

// findAllPartsByMediaType walks the MIME tree and returns every part with
// the given media type. Used in tests to assert tree contents.
func findAllPartsByMediaType(root *Part, mediaType string) []*Part {
	if root == nil {
		return nil
	}
	var result []*Part
	if strings.EqualFold(root.MediaType, mediaType) {
		result = append(result, root)
	}
	for _, child := range root.Children {
		result = append(result, findAllPartsByMediaType(child, mediaType)...)
	}
	return result
}

// removePartByMediaType removes the first part with the given media type from
// the MIME tree. The parent is marked dirty when a removal happens.
func removePartByMediaType(root *Part, mediaType string) {
	if root == nil {
		return
	}
	for i, child := range root.Children {
		if child != nil && strings.EqualFold(child.MediaType, mediaType) {
			root.Children = append(root.Children[:i], root.Children[i+1:]...)
			root.Dirty = true
			return
		}
		removePartByMediaType(child, mediaType)
	}
}

// removeAllPartsByMediaType removes every part with the given media type from
// the MIME tree, at all nesting levels.
func removeAllPartsByMediaType(root *Part, mediaType string) {
	if root == nil {
		return
	}
	var kept []*Part
	removed := false
	for _, child := range root.Children {
		if child != nil && strings.EqualFold(child.MediaType, mediaType) {
			removed = true
			continue
		}
		kept = append(kept, child)
	}
	if removed {
		root.Children = kept
		root.Dirty = true
	}
	for _, child := range root.Children {
		removeAllPartsByMediaType(child, mediaType)
	}
}
