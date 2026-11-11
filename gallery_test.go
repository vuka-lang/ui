package ui_test

import (
	"flag"
	"os"
	"testing"

	"github.com/vuka-lang/ui/v2/gallery"
)

var galleryOutput = flag.String("gallery-output", "", "write the dev gallery HTML to this directory")
var galleryCSS = flag.String("gallery-css", "", "compare with this stylesheet instead of ui.Styles")

// TestGallery writes the dev gallery (package gallery, also vuka ui
// gallery): every component and widget in every state, light and dark.
func TestGallery(t *testing.T) {
	if *galleryOutput == "" {
		t.Skip("dev-only fixture: pass -gallery-output=gallery/rendered")
	}
	var opts gallery.Options
	if *galleryCSS != "" {
		b, err := os.ReadFile(*galleryCSS)
		if err != nil {
			t.Fatal(err)
		}
		opts.Styles = string(b)
	}
	if err := gallery.Write(*galleryOutput, opts); err != nil {
		t.Fatal(err)
	}
}
