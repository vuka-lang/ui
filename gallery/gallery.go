// Package gallery shows the kit's components the way a component catalogue
// does: an index, a page per component with its examples (a preview and the
// .vuka.html source that renders it), its props and related components, and
// a page of every component in every state it takes, light and dark. It
// backs vuka ui gallery, the component pages of vuka's docs, and the kit's
// gallery test.
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

// Case is one sample of the states page: a component in one variant or
// state. Name is the component, then the state after a slash
// (button/disabled).
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

// Label is the case's name in words: "Button · primary sm icon".
func (c Case) Label() string {
	g := c.Group()
	rest := strings.TrimPrefix(strings.TrimPrefix(c.Name, g), "/")
	head := strings.ReplaceAll(g, "widget/", "")
	head = strings.ToUpper(head[:1]) + head[1:]
	if rest == "" {
		return head
	}
	return head + " · " + strings.ReplaceAll(rest, "-", " ")
}

// Render renders a node as a live page shows it, in a live session of its
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
	body = strings.TrimSuffix(strings.TrimPrefix(body, "<main>"), "</main>")
	return body, nil
}

// Options are what the gallery is drawn with.
type Options struct {
	// Styles replaces the kit's stylesheet (ui.Styles) when set.
	Styles string
	// CSS are stylesheets loaded after the kit's, in order: an app's token
	// overrides, the shadcn preset (ui.ShadcnStyles).
	CSS []string
	// CSSFiles are stylesheet files loaded after CSS, read each time a page
	// is built, so a served gallery shows a file's changes on reload.
	CSSFiles []string
	// Cases are the states page's samples; Cases() when nil.
	Cases []Case
	// Vendored are the components vuka ui add writes a copy of, by slug:
	// their pages show the command.
	Vendored map[string]bool
}

// Themes are the states pages Write writes besides states.html: one per
// theme, the theme fixed, for screenshots.
var Themes = []string{"light", "dark"}

// site is the gallery, rendered.
type site struct {
	comps   []Component
	html    map[string][]string // a component's examples, rendered
	rows    []row
	byGroup map[string]bool
}

type row struct {
	c    Case
	html string
}

func build(opts Options) (*site, error) {
	s := &site{comps: Components(), html: map[string][]string{}}
	for _, c := range s.comps {
		for _, e := range c.Examples {
			h, err := RenderExample(e)
			if err != nil {
				return nil, err
			}
			s.html[c.Slug] = append(s.html[c.Slug], h)
		}
	}
	cases := opts.Cases
	if cases == nil {
		cases = Cases()
	}
	for _, c := range cases {
		h, err := Render(c)
		if err != nil {
			return nil, err
		}
		s.rows = append(s.rows, row{c, h})
	}
	return s, nil
}

// Pages are the gallery's pages by path: index.html, components/<slug>.html,
// states.html, light.html and dark.html.
func Pages(opts Options) (map[string][]byte, error) {
	s, err := build(opts)
	if err != nil {
		return nil, err
	}
	paths := []string{"index.html", "states.html", "light.html", "dark.html"}
	for _, c := range s.comps {
		paths = append(paths, "components/"+c.Slug+".html")
	}
	out := map[string][]byte{}
	for _, p := range paths {
		b, err := s.page(opts, p)
		if err != nil {
			return nil, err
		}
		out[p] = b
	}
	return out, nil
}

// Write writes the pages to dir.
func Write(dir string, opts Options) error {
	pages, err := Pages(opts)
	if err != nil {
		return err
	}
	for p, b := range pages {
		path := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Handler serves the pages. The samples render once; the stylesheets are
// read on every request.
func Handler(opts Options) http.Handler {
	var once sync.Once
	var s *site
	var serr error
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if !strings.HasSuffix(p, ".html") {
			p += ".html"
		}
		once.Do(func() { s, serr = build(opts) })
		if serr != nil {
			http.Error(w, serr.Error(), http.StatusInternalServerError)
			return
		}
		if !s.has(p) {
			http.NotFound(w, r)
			return
		}
		b, err := s.page(opts, p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
}

func (s *site) has(p string) bool {
	switch p {
	case "index.html", "states.html", "light.html", "dark.html":
		return true
	}
	return s.comp(strings.TrimSuffix(strings.TrimPrefix(p, "components/"), ".html")) != nil && strings.HasPrefix(p, "components/")
}

func (s *site) comp(slug string) *Component {
	for i := range s.comps {
		if s.comps[i].Slug == slug {
			return &s.comps[i]
		}
	}
	return nil
}

func stylesheets(opts Options) ([]string, error) {
	styles := opts.Styles
	if styles == "" {
		styles = ui.Styles
	}
	sheets := append([]string{styles, PreviewUtilities}, opts.CSS...)
	for _, f := range opts.CSSFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		sheets = append(sheets, string(b))
	}
	return sheets, nil
}

func (s *site) page(opts Options, p string) ([]byte, error) {
	sheets, err := stylesheets(opts)
	if err != nil {
		return nil, err
	}
	prefix := ""
	if strings.HasPrefix(p, "components/") {
		prefix = "../"
	}
	theme, title, current := "", "Components", ""
	var main strings.Builder
	switch p {
	case "index.html":
		s.index(&main, prefix)
	case "states.html", "light.html", "dark.html":
		title = "All states"
		if p != "states.html" {
			theme = strings.TrimSuffix(p, ".html")
			title += " — " + theme
		}
		current = "states"
		s.states(&main)
	default:
		c := s.comp(strings.TrimSuffix(strings.TrimPrefix(p, "components/"), ".html"))
		title, current = c.Title, c.Slug
		s.component(&main, c, opts, prefix)
	}

	var b strings.Builder
	initial := theme
	if initial == "" {
		initial = "light"
	}
	fmt.Fprintf(&b, `<!doctype html><html lang="en" data-theme="%s" data-gallery-theme="%s"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s · Vuka UI</title>`, initial, theme, html.EscapeString(title))
	for _, sh := range sheets {
		b.WriteString("<style>" + strings.ReplaceAll(sh, "</", `<\/`) + "</style>")
	}
	b.WriteString("<style>" + chrome + "</style></head><body>")
	fmt.Fprintf(&b, `<header class="gx-bar"><a class="gx-brand" href="%sindex.html">Vuka UI</a><a href="%sindex.html">Components</a><a href="%sstates.html">All states</a>`+
		`<div class="gx-theme" role="group" aria-label="Theme"><button type="button" data-set-theme="light">Light</button><button type="button" data-set-theme="dark">Dark</button></div></header>`, prefix, prefix, prefix)
	b.WriteString(`<div class="gx-body"><nav class="gx-side" aria-label="Components"><p>Components</p><ul>`)
	for _, c := range s.comps {
		cur := ""
		if c.Slug == current {
			cur = ` aria-current="page"`
		}
		fmt.Fprintf(&b, `<li><a href="%scomponents/%s.html"%s>%s</a></li>`, prefix, c.Slug, cur, html.EscapeString(c.Title))
	}
	b.WriteString(`</ul></nav><main class="gx-main">` + main.String() + `</main></div><script>` + script + `</script></body></html>`)
	return []byte(b.String()), nil
}

func (s *site) index(b *strings.Builder, prefix string) {
	b.WriteString(`<h1>Components</h1><p class="gx-lead">The kit's components, each with its examples, the source that writes them and its props.</p><div class="gx-grid">`)
	for _, c := range s.comps {
		fmt.Fprintf(b, `<a class="gx-card" href="%scomponents/%s.html"><div class="gx-thumb" aria-hidden="true" inert><div class="gx-preview">%s</div></div><span>%s</span></a>`, prefix, c.Slug, s.html[c.Slug][0], html.EscapeString(c.Title))
	}
	b.WriteString(`</div>`)
}

func (s *site) component(b *strings.Builder, c *Component, opts Options, prefix string) {
	fmt.Fprintf(b, `<h1>%s</h1><p class="gx-lead">%s</p>`, html.EscapeString(c.Title), html.EscapeString(c.Description))
	for i, e := range c.Examples {
		if e.Section != "" {
			fmt.Fprintf(b, `<h2>%s</h2>`, html.EscapeString(e.Section))
			if e.Heading != "" {
				fmt.Fprintf(b, `<p>%s</p>`, html.EscapeString(e.Heading))
			}
		}
		block(b, e, s.html[c.Slug][i])
	}
	if opts.Vendored[c.Slug] {
		fmt.Fprintf(b, `<h2>Vendored copy</h2><p>A copy written into the app as its own source, drawn with Tailwind utilities:</p><pre class="gx-code"><code>vuka ui add %s</code></pre>`, c.Slug)
	}
	if len(c.Props) > 0 {
		b.WriteString(`<h2>Props</h2><table class="gx-props"><thead><tr><th>Prop</th><th>Type</th><th>Default</th><th>Description</th></tr></thead><tbody>`)
		for _, p := range c.Props {
			def := p.Default
			if def == "" {
				def = "—"
			}
			fmt.Fprintf(b, `<tr><td><code>%s</code></td><td><code>%s</code></td><td><code>%s</code></td><td>%s</td></tr>`, html.EscapeString(p.Attr), html.EscapeString(p.Type), html.EscapeString(def), html.EscapeString(FirstSentence(p.Doc)))
		}
		b.WriteString(`</tbody></table>`)
	}
	if len(c.Related) > 0 {
		b.WriteString(`<h2>Related</h2><p class="gx-related">`)
		for i, r := range c.Related {
			if i > 0 {
				b.WriteString(" · ")
			}
			fmt.Fprintf(b, `<a href="%scomponents/%s.html">%s</a>`, prefix, r, html.EscapeString(s.comp(r).Title))
		}
		b.WriteString(`</p>`)
	}
}

// block is an example's Preview | Code block.
func block(b *strings.Builder, e Example, rendered string) {
	style := ""
	if e.Height > 0 {
		style = fmt.Sprintf(` style="min-height:%dpx"`, e.Height)
	}
	fmt.Fprintf(b, `<div class="gx-block"><div class="gx-tabs" role="tablist"><button type="button" role="tab" data-tab="preview" aria-selected="true">Preview</button><button type="button" role="tab" data-tab="code" aria-selected="false">Code</button><button type="button" class="gx-copy">Copy</button></div>`+
		`<div class="gx-preview gx-stage"%s>%s</div><pre class="gx-code" hidden><code>%s</code></pre></div>`, style, rendered, html.EscapeString(e.Source))
}

func (s *site) states(b *strings.Builder) {
	b.WriteString(`<h1>All states</h1><p class="gx-lead">Every component in the variants and states it takes, and every built-in widget, for comparing before and after a change. Unsupported states are omitted.</p><input type="search" id="gx-filter" placeholder="Filter" aria-label="Filter"><table class="gx-states"><tbody>`)
	for i, r := range s.rows {
		fmt.Fprintf(b, `<tr data-case="%s"><th scope="row">%s</th><td><div class="gx-preview sample" id="sample-%d">%s</div></td></tr>`, html.EscapeString(r.c.Name), html.EscapeString(r.c.Label()), i, r.html)
	}
	b.WriteString(`</tbody></table>`)
}

// FirstSentence is a doc comment's first sentence.
func FirstSentence(doc string) string {
	s := strings.Join(strings.Fields(doc), " ")
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

// PreviewUtilities are the few layout classes the examples use besides the
// kit's own (a test keeps the examples to them), so a preview needs only
// the kit's stylesheet and these.
const PreviewUtilities = `.flex{display:flex}.flex-wrap{flex-wrap:wrap}.items-center{align-items:center}.grid{display:grid}.gap-2{gap:.5rem}.gap-4{gap:1rem}.h-12{height:3rem}.w-12{width:3rem}.rounded-full{border-radius:9999px}`

const chrome = `body{margin:0;background:var(--ui-bg);color:var(--ui-fg);font:15px/1.6 system-ui,-apple-system,"Segoe UI",sans-serif}
a{color:inherit}
.gx-bar{position:sticky;top:0;z-index:1000;display:flex;gap:20px;align-items:center;height:56px;padding:0 24px;background:var(--ui-bg);border-bottom:1px solid var(--ui-border);font-size:14px}
.gx-bar a{text-decoration:none;color:var(--ui-muted)}.gx-bar a:hover{color:var(--ui-fg)}.gx-bar .gx-brand{font-weight:700;color:var(--ui-fg)}
.gx-theme{margin-left:auto;display:flex}.gx-theme button{font:inherit;font-size:13px;padding:4px 12px;border:1px solid var(--ui-border);background:var(--ui-bg);color:var(--ui-fg);cursor:pointer}
.gx-theme button:first-child{border-radius:6px 0 0 6px}.gx-theme button:last-child{border-radius:0 6px 6px 0;border-left:0}.gx-theme button[aria-pressed=true]{background:var(--ui-subtle);font-weight:600}
.gx-body{display:flex;align-items:flex-start;max-width:1200px;margin:0 auto}
.gx-side{position:sticky;top:56px;width:200px;flex:none;max-height:calc(100vh - 56px);overflow:auto;padding:24px 16px}
.gx-side p{margin:0 0 6px;font-size:13px;font-weight:600}.gx-side ul{list-style:none;margin:0;padding:0}
.gx-side a{display:block;padding:3px 8px;border-radius:6px;font-size:14px;text-decoration:none;color:var(--ui-muted)}.gx-side a:hover,.gx-side a[aria-current]{color:var(--ui-fg);background:var(--ui-subtle)}
.gx-main{flex:1;min-width:0;max-width:760px;padding:32px 32px 96px}
.gx-main h1{font-size:30px;line-height:1.2;margin:0 0 8px;letter-spacing:-.02em}.gx-lead{margin:0 0 32px;font-size:17px;color:var(--ui-muted)}
.gx-main h2{font-size:20px;margin:48px 0 4px;padding-bottom:6px;border-bottom:1px solid var(--ui-border)}.gx-main h2+p{color:var(--ui-muted);margin:8px 0 16px}
.gx-block{margin:16px 0}.gx-tabs{display:flex;gap:4px;align-items:center;margin-bottom:8px}
.gx-tabs button{font:inherit;font-size:14px;padding:4px 10px;border:0;border-bottom:2px solid transparent;background:none;color:var(--ui-muted);cursor:pointer}
.gx-tabs button[aria-selected=true]{color:var(--ui-fg);border-bottom-color:var(--ui-fg);font-weight:600}.gx-tabs .gx-copy{margin-left:auto;border:1px solid var(--ui-border);border-radius:6px;padding:2px 10px;font-size:12px}
.gx-stage{display:flex;align-items:center;justify-content:center;min-height:200px;padding:40px 24px;border:1px solid var(--ui-border);border-radius:10px}
.gx-stage>*{max-width:100%}
.gx-code{margin:0;padding:16px;overflow:auto;border-radius:10px;background:var(--ui-subtle);font:13px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace}
.gx-props{width:100%;border-collapse:collapse;font-size:14px}.gx-props th{text-align:left;font-weight:600;color:var(--ui-muted)}.gx-props th,.gx-props td{padding:8px 10px 8px 0;border-bottom:1px solid var(--ui-border);vertical-align:top}.gx-props code{font-size:12.5px}
.gx-related a{color:var(--ui-fg)}
.gx-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:16px}
.gx-card{display:block;border:1px solid var(--ui-border);border-radius:10px;overflow:hidden;text-decoration:none}.gx-card:hover{border-color:var(--ui-ring)}
.gx-card span{display:block;padding:10px 12px;font-size:14px;font-weight:500;border-top:1px solid var(--ui-border)}
.gx-thumb{height:130px;overflow:hidden;display:flex;align-items:center;justify-content:center;pointer-events:none}.gx-thumb>.gx-preview{transform:scale(.6);transform-origin:center}
.gx-preview{position:relative;transform:translateZ(0);isolation:isolate}
.gx-preview dialog{position:relative!important;inset:auto!important;margin:0}
#gx-filter{font:inherit;margin:0 0 16px;padding:6px 10px;width:260px;border:1px solid var(--ui-border);border-radius:6px;background:var(--ui-bg);color:var(--ui-fg)}
.gx-states{border-collapse:collapse;width:100%;table-layout:fixed}.gx-states th{width:200px;text-align:left;font-weight:500;font-size:13px;padding:12px 12px 12px 0;vertical-align:top;border-bottom:1px solid var(--ui-border)}.gx-states td{padding:12px 0;border-bottom:1px solid var(--ui-border)}
.sample{min-height:40px;overflow:hidden;padding:8px}.sample:has(dialog),.sample:has(.vk-modal-root){height:280px}.sample:has(.vk-toasts){height:150px}.sample img{max-width:100%;max-height:120px}
.gx-preview *,.gx-preview *::before,.gx-preview *::after{animation-play-state:paused!important;caret-color:transparent!important}
@media (max-width:760px){.gx-side{display:none}.gx-main{padding:24px 16px 64px}.gx-states th{width:110px}.gx-bar{gap:12px;padding:0 16px}}`

const script = `(function(){
var root=document.documentElement,fixed=root.dataset.galleryTheme,key='vk-gallery-theme';
function set(t,save){root.setAttribute('data-theme',t);document.querySelectorAll('[data-set-theme]').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.setTheme===t)});if(save&&!fixed){try{localStorage.setItem(key,t)}catch(e){}}}
var t=root.getAttribute('data-theme');if(!fixed){try{t=localStorage.getItem(key)||t}catch(e){}}set(t,false);
document.querySelectorAll('[data-set-theme]').forEach(function(b){b.addEventListener('click',function(){set(b.dataset.setTheme,true)})});
document.querySelectorAll('.gx-block').forEach(function(blk){var pv=blk.querySelector('.gx-stage'),code=blk.querySelector('.gx-code');
blk.querySelectorAll('[data-tab]').forEach(function(tb){tb.addEventListener('click',function(){var c=tb.dataset.tab==='code';pv.hidden=c;code.hidden=!c;
blk.querySelectorAll('[data-tab]').forEach(function(o){o.setAttribute('aria-selected',o===tb)})})});
blk.querySelector('.gx-copy').addEventListener('click',function(e){var b=e.target;try{navigator.clipboard.writeText(code.textContent).then(function(){b.textContent='Copied';setTimeout(function(){b.textContent='Copy'},1500)})}catch(x){}})});
var q=document.getElementById('gx-filter');if(q)q.addEventListener('input',function(){var v=q.value.toLowerCase();
document.querySelectorAll('.gx-states tr').forEach(function(tr){tr.hidden=v!==''&&tr.textContent.toLowerCase().indexOf(v)<0&&tr.dataset.case.indexOf(v)<0})});
document.querySelectorAll('.gx-preview dialog').forEach(function(d){d.setAttribute('open','')});
document.querySelectorAll('.gx-preview .vk-modal-root').forEach(function(d){d.style.display='flex'});
document.querySelectorAll('.gx-preview img').forEach(function(im){im.src='data:image/svg+xml,%3Csvg xmlns="http://www.w3.org/2000/svg" width="320" height="180"%3E%3Crect width="320" height="180" fill="%2394a3b8" fill-opacity=".25"/%3E%3C/svg%3E'});
})();`
