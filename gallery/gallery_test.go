package gallery_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/gallery"
)

func TestCasesRender(t *testing.T) {
	cases := gallery.Cases()
	if len(cases) <= len(gallery.Kit()) {
		t.Fatalf("Cases adds nothing to Kit: %d cases", len(cases))
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.Name] {
			t.Errorf("two cases named %s", c.Name)
		}
		seen[c.Name] = true
		if _, err := gallery.Render(c); err != nil {
			t.Error(err)
		}
	}
	for _, name := range []string{"button/disabled", "button/loading", "button/primary-sm-icon", "input/error", "widget/Text/error", "dialog"} {
		if !seen[name] {
			t.Errorf("no case %s", name)
		}
	}
}

func TestGroup(t *testing.T) {
	for name, want := range map[string]string{"button": "button", "button/disabled": "button", "widget/Text/error": "widget/Text", "modal/full": "modal"} {
		if got := (gallery.Case{Name: name}).Group(); got != want {
			t.Errorf("%s: group %q, want %q", name, got, want)
		}
	}
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	css := filepath.Join(dir, "app.css")
	if err := os.WriteFile(css, []byte(":root{--ui-primary:#123456}"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := gallery.Write(out, gallery.Options{CSS: []string{ui.ShadcnStyles}, CSSFiles: []string{css}}); err != nil {
		t.Fatal(err)
	}
	for file, theme := range map[string]string{"index.html": "light", "light.html": "light", "dark.html": "dark"} {
		b, err := os.ReadFile(filepath.Join(out, file))
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, want := range []string{`data-theme="` + theme + `"`, `data-set-theme="dark"`, `href="#c-button"`, `href="#c-widget-Text"`, `data-case="button/disabled"`, "--ui-primary:#123456", "--ui-bg:var(--background)"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s: no %s", file, want)
			}
		}
		// The kit's tokens come first, the app's after, so the app's win.
		if strings.Index(s, "--ui-bg:#ffffff") > strings.Index(s, "--ui-primary:#123456") {
			t.Errorf("%s: the app's stylesheet loads before the kit's", file)
		}
	}
}

func TestHandler(t *testing.T) {
	css := filepath.Join(t.TempDir(), "app.css")
	os.WriteFile(css, []byte(":root{--ui-primary:#000001}"), 0o644)
	cases := gallery.Kit()[:3]
	srv := httptest.NewServer(gallery.Handler(gallery.Options{Cases: cases, CSSFiles: []string{css}}))
	defer srv.Close()
	get := func(path string) (int, string) {
		t.Helper()
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if code, body := get("/"); code != 200 || !strings.Contains(body, `data-case="`+cases[0].Name+`"`) || !strings.Contains(body, "#000001") {
		t.Fatalf("/: %d\n%s", code, body)
	}
	if code, body := get("/dark.html"); code != 200 || !strings.Contains(body, `data-theme="dark"`) {
		t.Fatalf("/dark.html: %d", code)
	}
	// The stylesheet is read on every request.
	os.WriteFile(css, []byte(":root{--ui-primary:#000002}"), 0o644)
	if _, body := get("/light.html"); !strings.Contains(body, "#000002") {
		t.Fatal("the edited stylesheet is not served")
	}
	if code, _ := get("/nope"); code != 404 {
		t.Fatalf("/nope: %d", code)
	}
	os.Remove(css)
	if code, _ := get("/"); code != 500 {
		t.Fatalf("missing stylesheet: %d", code)
	}
}

// An app's stylesheet loads last: no sample brings the kit's again.
func TestNoStylesInSamples(t *testing.T) {
	for _, c := range gallery.Cases() {
		h, err := gallery.Render(c)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(h, "<style") {
			t.Errorf("%s renders a stylesheet", c.Name)
		}
	}
}
