package gallery_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/gallery"
	"github.com/vuka-lang/ui/v2/gallery/examples"
)

func TestCasesRender(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range gallery.Cases() {
		if seen[c.Name] {
			t.Errorf("two cases named %s", c.Name)
		}
		seen[c.Name] = true
		h, err := gallery.Render(c)
		if err != nil {
			t.Error(err)
		}
		// An app's stylesheet loads last: no sample brings the kit's again.
		if strings.Contains(h, "<style") {
			t.Errorf("%s renders a stylesheet", c.Name)
		}
	}
	for _, name := range []string{"button/disabled", "button/loading", "button/primary-sm-icon", "input/error", "widget/Text/error", "dialog"} {
		if !seen[name] {
			t.Errorf("no case %s", name)
		}
	}
}

// Every component of the kit has an example, and every example renders.
func TestEveryComponentHasExamples(t *testing.T) {
	have := map[string]bool{}
	for _, c := range gallery.Components() {
		have[c.Name] = true
		if c.Description == "" {
			t.Errorf("%s: no description", c.Name)
		}
		for _, e := range c.Examples {
			if _, err := gallery.RenderExample(e); err != nil {
				t.Error(err)
			}
		}
	}
	for _, name := range gallery.KitTypes() {
		if !have[name] && name != "ToastContainer" {
			t.Errorf("%s has no example: add one to gallery/examples", name)
		}
	}
}

// Each example file is listed once, and each listed file exists: the
// snippet shown is the file compiled into what renders.
func TestExampleFiles(t *testing.T) {
	listed := map[string]bool{}
	for _, e := range examples.All {
		if listed[e.File] {
			t.Errorf("%s listed twice", e.File)
		}
		listed[e.File] = true
	}
	for _, f := range examples.Files() {
		if !listed[f] {
			t.Errorf("%s is in no example", f)
		}
	}
}

var classAttr = regexp.MustCompile(`class="([^"]*)"`)

// The examples use only the layout utilities the previews define.
func TestExampleClasses(t *testing.T) {
	for _, f := range examples.Files() {
		for _, m := range classAttr.FindAllStringSubmatch(examples.Source(f), -1) {
			for _, c := range strings.Fields(m[1]) {
				if !strings.Contains(gallery.PreviewUtilities, "."+c+"{") {
					t.Errorf("%s: class %s is not in gallery.PreviewUtilities", f, c)
				}
			}
		}
	}
}

func TestLabel(t *testing.T) {
	for name, want := range map[string]string{"button": "Button", "button/primary-sm-icon": "Button · primary sm icon", "widget/Text/error": "Text · error"} {
		if got := (gallery.Case{Name: name}).Label(); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}

func TestPages(t *testing.T) {
	dir := t.TempDir()
	css := filepath.Join(dir, "app.css")
	os.WriteFile(css, []byte(":root{--ui-primary:#123456}"), 0o644)
	out := filepath.Join(dir, "out")
	if err := gallery.Write(out, gallery.Options{CSS: []string{ui.ShadcnStyles}, CSSFiles: []string{css}, Vendored: map[string]bool{"button": true}}); err != nil {
		t.Fatal(err)
	}
	read := func(p string) string {
		b, err := os.ReadFile(filepath.Join(out, p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, p := range []string{"index.html", "states.html", "light.html", "dark.html", "components/button.html"} {
		s := read(p)
		for _, want := range []string{`data-set-theme="dark"`, "--ui-primary:#123456", "--ui-bg:var(--background)", `components/data-table.html"`} {
			if !strings.Contains(s, want) {
				t.Errorf("%s: no %s", p, want)
			}
		}
		if strings.Index(s, "--ui-bg:#ffffff") > strings.Index(s, "--ui-primary:#123456") {
			t.Errorf("%s: the app's stylesheet loads before the kit's", p)
		}
	}
	if s := read("dark.html"); !strings.Contains(s, `<html lang="en" data-theme="dark"`) {
		t.Error("dark.html is not dark")
	}
	button := read("components/button.html")
	src := html(examples.Source("button_variants.vuka.html"))
	for _, want := range []string{"<h1>Button</h1>", "<h2>Variants</h2>", src, "vuka ui add button", "<h2>Props</h2>", "<code>iconOnly</code>", "<code>SizeMd</code>", "<h2>Related</h2>", `class="vk-btn vk-btn--primary"`} {
		if !strings.Contains(button, want) {
			t.Errorf("button page: no %s", want)
		}
	}
	if strings.Contains(read("components/badge.html"), "vuka ui add") {
		t.Error("badge page names vuka ui add without its being vendored")
	}
	if s := read("states.html"); !strings.Contains(s, "Button · disabled") {
		t.Error("states page: no Button · disabled")
	}
	comps := gallery.Components()
	if !slices.IsSortedFunc(comps, func(a, b gallery.Component) int { return strings.Compare(a.Title, b.Title) }) {
		t.Error("components are not in order")
	}
}

func html(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;").Replace(s)
}

func TestHandler(t *testing.T) {
	css := filepath.Join(t.TempDir(), "app.css")
	os.WriteFile(css, []byte(":root{--ui-primary:#000001}"), 0o644)
	srv := httptest.NewServer(gallery.Handler(gallery.Options{Cases: gallery.Kit()[:3], CSSFiles: []string{css}}))
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
	if code, body := get("/"); code != 200 || !strings.Contains(body, "#000001") || !strings.Contains(body, "gx-grid") {
		t.Fatalf("/: %d", code)
	}
	if code, body := get("/components/button"); code != 200 || !strings.Contains(body, "<h1>Button</h1>") {
		t.Fatalf("/components/button: %d", code)
	}
	if code, body := get("/dark.html"); code != 200 || !strings.Contains(body, `data-theme="dark"`) {
		t.Fatalf("/dark.html: %d", code)
	}
	os.WriteFile(css, []byte(":root{--ui-primary:#000002}"), 0o644)
	if _, body := get("/states.html"); !strings.Contains(body, "#000002") {
		t.Fatal("the edited stylesheet is not served")
	}
	for _, p := range []string{"/nope", "/components/nope", "/components/"} {
		if code, _ := get(p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}
	os.Remove(css)
	if code, _ := get("/"); code != 500 {
		t.Fatalf("missing stylesheet: %d", code)
	}
}
