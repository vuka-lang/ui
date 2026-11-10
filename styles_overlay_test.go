package ui

import (
	"strings"
	"testing"
)

func TestStylesOverlayHostDialogsTakeClicks(t *testing.T) {
	if !strings.Contains(Styles, "#vk-overlays{") || !strings.Contains(Styles, "pointer-events:none}") {
		t.Fatal("the overlay host rule moved: update this test")
	}
	if !strings.Contains(Styles, "#vk-overlays dialog,") {
		t.Fatal("a dialog in the pointer-events:none overlay host inherits it and takes no clicks")
	}
	if !strings.Contains(Styles, "dialog.vk-dialog,dialog.vk-call{box-sizing:border-box;margin:auto;") {
		t.Fatal("a dialog needs margin:auto: a reset stylesheet zeroes the browser's centering")
	}
}
