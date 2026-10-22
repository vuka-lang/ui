package ui

import (
	"github.com/vuka-lang/vuka/v2/diag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/vuka/v2/docs"
)

// TestDiagnosticCodes: every code ui's checks report has a page in vuka's
// catalogue, which `vuka explain` prints: the catalogue of $VUKA_DIR (CI's
// is the vuka module tools.mod pins) or of a vuka checked out beside this
// module, where a new code's page lands before a release carries it, else
// the required version's.
func TestDiagnosticCodes(t *testing.T) {
	dir := os.Getenv("VUKA_DIR")
	if dir == "" {
		dir = filepath.Join("..", "vuka")
	}
	page := func(code string) bool {
		if _, err := os.Stat(filepath.Join(dir, "docs", "errors", code+".md")); err == nil {
			return true
		}
		_, ok := docs.Lookup(code)
		return ok
	}
	files, _ := filepath.Glob("*.go")
	more, _ := filepath.Glob("live/*.go")
	codes := regexp.MustCompile(`"([EW]\d{4})"`)
	n := 0
	for _, f := range append(files, more...) {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range codes.FindAllSubmatch(b, -1) {
			n++
			if !page(string(m[1])) {
				t.Errorf("%s: %s has no page in vuka's docs/errors", f, m[1])
			}
		}
	}
	if n == 0 {
		t.Fatal("no codes found")
	}
}

type deferRendered struct {
	Name string
	CV   string `defer:"profile"`
}

func TestBootErrorRenders(t *testing.T) {
	err := CheckIsland(reflect.TypeFor[deferRendered]())
	want := `error[E0324]: deferRendered.CV is deferred, and the group Profile has no loader
  = help: mark its loader @ui.Loads("profile")
  = note: vuka explain E0324 (` + diag.DocsURL + `E0324)`
	if err == nil || err.Error() != want {
		t.Fatalf("got:\n%v\nwant:\n%s", err, want)
	}
}
