package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/vuka/docs"
)

// TestDiagnosticCodes: every code ui's checks report has a page in vuka's
// catalogue, which `vuka explain` prints.
func TestDiagnosticCodes(t *testing.T) {
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
			if _, ok := docs.Lookup(string(m[1])); !ok {
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
  = help: add the method
          func (d *deferRendered) LoadProfile(ctx context.Context) error
  = note: vuka explain E0324 (https://vuka-lang.github.io/vuka/errors/E0324)`
	if err == nil || err.Error() != want {
		t.Fatalf("got:\n%v\nwant:\n%s", err, want)
	}
}
