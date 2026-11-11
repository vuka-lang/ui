// Package gallery renders every component of the kit in every state it
// takes, light and dark, on one page: a place to see the components side by
// side and compare them before and after a change to tokens or markup. It
// backs vuka ui gallery and the kit's gallery test.
package gallery

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// Case is one sample: a component in one variant or state. Name is the
// component, then the state after a slash (button/disabled).
type Case struct {
	Name string
	Node func() ui.Node
}

// Group is the component a case shows: the name up to its first slash, or,
// for a widget, up to its second (widget/Text).
func (c Case) Group() string {
	parts := strings.SplitN(c.Name, "/", 3)
	if parts[0] == "widget" && len(parts) > 1 {
		return parts[0] + "/" + parts[1]
	}
	return parts[0]
}

// Render renders a case as a live page shows it, in a live session of its
// own, so it has the form and event context an app gives it.
func Render(c Case) (string, error) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.El("main", nil, c.Node()) })
	cl, err := livetest.Join(s, 1)
	if err != nil {
		return "", fmt.Errorf("%s: %w", c.Name, err)
	}
	body := cl.HTML()
	if strings.Contains(body, "data-vk-error") && strings.Contains(body, "E0") {
		return "", fmt.Errorf("%s failed to render: %s", c.Name, body)
	}
	return body, nil
}

// Options are what a gallery page is drawn with.
type Options struct {
	// Styles replaces the kit's stylesheet (ui.Styles) when set.
	Styles string
	// CSS are stylesheets loaded after the kit's, in order: an app's token
	// overrides, the shadcn preset (ui.ShadcnStyles).
	CSS []string
	// CSSFiles are stylesheet files loaded after CSS, read each time a page
	// is built, so a served gallery shows a file's changes on reload.
	CSSFiles []string
	// Cases are the samples; Cases() when nil.
	Cases []Case
}

// Themes are the pages Write writes besides index.html: one per theme, the
// theme fixed, for screenshots.
var Themes = []string{"light", "dark"}

// Page builds a gallery page. theme is "light" or "dark" for a page fixed in
// that theme, or "" for the index, which remembers the theme last chosen.
func Page(opts Options, theme string) ([]byte, error) {
	rows, err := renderRows(opts.cases())
	if err != nil {
		return nil, err
	}
	return page(opts, theme, rows)
}

// Write writes index.html and a page per theme (light.html, dark.html) to dir.
func Write(dir string, opts Options) error {
	rows, err := renderRows(opts.cases())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, theme := range append([]string{""}, Themes...) {
		b, err := page(opts, theme, rows)
		if err != nil {
			return err
		}
		name := theme + ".html"
		if theme == "" {
			name = "index.html"
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Handler serves the gallery: / (the index), /light.html and /dark.html. The
// samples render once; the stylesheets are read on every request.
func Handler(opts Options) http.Handler {
	var once sync.Once
	var rows []row
	var rerr error
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		theme := ""
		switch r.URL.Path {
		case "/", "/index.html":
		case "/light.html", "/dark.html":
			theme = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".html")
		default:
			http.NotFound(w, r)
			return
		}
		once.Do(func() { rows, rerr = renderRows(opts.cases()) })
		if rerr != nil {
			http.Error(w, rerr.Error(), http.StatusInternalServerError)
			return
		}
		b, err := page(opts, theme, rows)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
}

func (o Options) cases() []Case {
	if o.Cases != nil {
		return o.Cases
	}
	return Cases()
}

type row struct {
	c    Case
	html string
}

func renderRows(cases []Case) ([]row, error) {
	rows := make([]row, len(cases))
	for i, c := range cases {
		h, err := Render(c)
		if err != nil {
			return nil, err
		}
		rows[i] = row{c, h}
	}
	return rows, nil
}

func page(opts Options, theme string, rows []row) ([]byte, error) {
	styles := opts.Styles
	if styles == "" {
		styles = ui.Styles
	}
	sheets := append([]string{styles}, opts.CSS...)
	for _, f := range opts.CSSFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		sheets = append(sheets, string(b))
	}

	var b strings.Builder
	initial := theme
	if initial == "" {
		initial = "light"
	}
	title := "Vuka kit gallery"
	if theme != "" {
		title += " — " + theme
	}
	fmt.Fprintf(&b, `<!doctype html><html data-theme="%s" data-gallery-theme="%s"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title>`, initial, theme, html.EscapeString(title))
	for _, s := range sheets {
		b.WriteString("<style>" + strings.ReplaceAll(s, "</", `<\/`) + "</style>")
	}
	b.WriteString("<style>" + chrome + "</style></head><body>")

	// The index: one link per component, with how many samples it has.
	var groups []string
	count := map[string]int{}
	for _, r := range rows {
		g := r.c.Group()
		if count[g] == 0 {
			groups = append(groups, g)
		}
		count[g]++
	}
	b.WriteString(`<header class="gallery-bar"><strong>Vuka kit gallery</strong><input type="search" id="gallery-filter" placeholder="Filter components" aria-label="Filter components">` +
		`<div class="gallery-theme" role="group" aria-label="Theme"><button type="button" data-set-theme="light">Light</button><button type="button" data-set-theme="dark">Dark</button></div></header>`)
	b.WriteString(`<div class="gallery-body"><nav class="gallery-index" aria-label="Components"><ul>`)
	for _, g := range groups {
		fmt.Fprintf(&b, `<li data-group="%s"><a href="#%s">%s</a> <span>%d</span></li>`, html.EscapeString(g), anchor(g), html.EscapeString(g), count[g])
	}
	b.WriteString(`</ul></nav><main class="gallery-main"><p class="gallery-note">Every component in the variants and states it takes: default, disabled, error, loading, with and without an icon; every built-in widget. Unsupported states are omitted.</p><table class="gallery"><tbody>`)
	last := ""
	for i, r := range rows {
		g := r.c.Group()
		if g != last {
			fmt.Fprintf(&b, `<tr class="gallery-group" data-group="%s"><th colspan="2" id="%s">%s</th></tr>`, html.EscapeString(g), anchor(g), html.EscapeString(g))
			last = g
		}
		name := html.EscapeString(r.c.Name)
		fmt.Fprintf(&b, `<tr data-case="%s" data-group="%s"><th scope="row">%s</th><td><div class="sample" id="sample-%d">%s</div></td></tr>`, name, html.EscapeString(g), name, i, r.html)
	}
	b.WriteString(`</tbody></table></main></div><script>` + script + `</script></body></html>`)
	return []byte(b.String()), nil
}

func anchor(group string) string { return "c-" + strings.ReplaceAll(group, "/", "-") }

const chrome = `body{margin:0;background:var(--ui-bg);color:var(--ui-fg);font:16px system-ui,Arial,sans-serif}
.gallery-bar{position:sticky;top:0;z-index:1000;display:flex;gap:12px;align-items:center;padding:10px 16px;background:var(--ui-bg);border-bottom:1px solid var(--ui-border)}
.gallery-bar input{flex:1;max-width:320px;font:inherit;padding:4px 8px;border:1px solid var(--ui-border);border-radius:6px;background:var(--ui-bg);color:var(--ui-fg)}
.gallery-theme{margin-left:auto;display:flex}.gallery-theme button{font:inherit;padding:4px 12px;border:1px solid var(--ui-border);background:var(--ui-bg);color:var(--ui-fg);cursor:pointer}
.gallery-theme button:first-child{border-radius:6px 0 0 6px}.gallery-theme button:last-child{border-radius:0 6px 6px 0;border-left:0}.gallery-theme button[aria-pressed=true]{background:var(--ui-subtle);font-weight:600}
.gallery-body{display:flex;align-items:flex-start}.gallery-index{position:sticky;top:53px;width:200px;flex:none;max-height:calc(100vh - 53px);overflow:auto;padding:12px 0;border-right:1px solid var(--ui-border)}
.gallery-index ul{list-style:none;margin:0;padding:0}.gallery-index li{display:flex;justify-content:space-between;padding:2px 16px;font:13px monospace}.gallery-index a{color:inherit}.gallery-index span{color:var(--ui-muted)}
.gallery-main{flex:1;min-width:0;padding:0 16px 16px}.gallery-note{color:var(--ui-muted);font-size:14px}
table.gallery{border-collapse:collapse;width:100%;table-layout:fixed}table.gallery>tbody>tr>th{width:230px;text-align:left;font:12px monospace;padding:12px;vertical-align:top;border-bottom:1px solid var(--ui-border);overflow-wrap:anywhere}
table.gallery>tbody>tr.gallery-group>th{width:auto;font:600 15px system-ui,Arial,sans-serif;padding:20px 12px 8px;scroll-margin-top:60px}table.gallery>tbody>tr>td{padding:12px;border-bottom:1px solid var(--ui-border)}
.sample{position:relative;transform:translateZ(0);isolation:isolate;min-height:40px;overflow:hidden;padding:8px}.sample:has(dialog),.sample:has(.vk-modal-root){height:280px}.sample:has(.vk-toasts){height:150px}
.sample dialog{position:relative!important;inset:auto!important;max-height:260px}.sample img{max-width:100%;max-height:120px}
.sample *,.sample *::before,.sample *::after{animation-play-state:paused!important;caret-color:transparent!important}
@media (max-width:700px){.gallery-body{display:block}.gallery-index{position:static;width:auto;max-height:30vh;border-right:0;border-bottom:1px solid var(--ui-border)}table.gallery>tbody>tr>th{width:110px}}`

const script = `(function(){
var root=document.documentElement,fixed=root.dataset.galleryTheme,key='vk-gallery-theme';
function set(t,save){root.setAttribute('data-theme',t);document.querySelectorAll('[data-set-theme]').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.setTheme===t)});if(save&&!fixed){try{localStorage.setItem(key,t)}catch(e){}}}
var t=root.getAttribute('data-theme');if(!fixed){try{t=localStorage.getItem(key)||t}catch(e){}}set(t,false);
document.querySelectorAll('[data-set-theme]').forEach(function(b){b.addEventListener('click',function(){set(b.dataset.setTheme,true)})});
var q=document.getElementById('gallery-filter');q.addEventListener('input',function(){var v=q.value.toLowerCase();
document.querySelectorAll('[data-group]').forEach(function(el){var n=(el.dataset.case||el.dataset.group).toLowerCase();el.hidden=v!==''&&n.indexOf(v)<0&&el.dataset.group.toLowerCase().indexOf(v)<0})});
document.querySelectorAll('.sample dialog').forEach(function(d){d.setAttribute('open','')});
document.querySelectorAll('.sample .vk-modal-root').forEach(function(d){d.style.display='flex'});
document.querySelectorAll('.sample img').forEach(function(im){im.src='data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="160" height="90"%3E%3C/svg%3E'});
})();`
