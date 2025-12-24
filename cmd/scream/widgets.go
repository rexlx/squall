package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"
)

// SubmitEntry is a MultiLineEntry that submits on Enter, and newlines on Shift+Enter
type SubmitEntry struct {
	widget.Entry
	OnSubmit func(string)
}

func NewSubmitEntry() *SubmitEntry {
	e := &SubmitEntry{}
	e.ExtendBaseWidget(e)
	e.MultiLine = true
	e.Wrapping = fyne.TextWrapWord
	return e
}

// TypedKey overrides the key handler to trap Enter
func (e *SubmitEntry) TypedKey(key *fyne.KeyEvent) {
	// Check for Enter (Main) or Return (Numpad)
	if key.Name == fyne.KeyReturn || key.Name == fyne.KeyEnter {

		// 1. Check if SHIFT is held down
		// We cast the driver to a desktop driver to access modifiers.
		// On mobile, this check fails safely (ok is false), so we just submit.
		shiftHeld := false
		if drv, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
			if drv.CurrentKeyModifiers()&fyne.KeyModifierShift != 0 {
				shiftHeld = true
			}
		}

		// 2. If Shift is held, insert newline (default behavior)
		if shiftHeld {
			e.Entry.TypedKey(key)
			return
		}

		// 3. Otherwise, Submit
		if e.OnSubmit != nil && e.Text != "" {
			e.OnSubmit(e.Text)
			// We do NOT call e.Entry.TypedKey(key) here to prevent the newline
		}
		return
	}

	// Process all other keys normally
	e.Entry.TypedKey(key)
}

func (e *SubmitEntry) Keyboard() mobile.KeyboardType {
	return mobile.DefaultKeyboard
}
