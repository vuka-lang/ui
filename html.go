package ui

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"

	"github.com/a-h/templ"
	templruntime "github.com/a-h/templ/runtime"
)

// renderHTML is every built-in Node's Render: a walk into an HTMLRenderer
// writing through a pooled buffer, in chunks.
func renderHTML(ctx context.Context, w io.Writer, n Node) (err error) {
	defer Catch(&err)
	sw, ok := w.(io.StringWriter)
	if !ok {
		sw = stringWriter{w}
	}
	h := renderers.Get().(*HTMLRenderer)
	h.ctx, h.w, h.out, h.buf = ctx, sw, w, h.buf[:0]
	err = Walk(ctx, n, h)
	h.flush()
	if err == nil {
		err = h.err
	}
	*h = HTMLRenderer{buf: h.buf[:0]}
	if cap(h.buf) <= maxPooledBuf {
		renderers.Put(h)
	}
	return err
}

// renderers are renderHTML's HTMLRenderers, each with its buffer.
var renderers = sync.Pool{New: func() any { return &HTMLRenderer{buf: make([]byte, 0, renderBuf)} }}

const (
	renderBuf    = 8 << 10 // a buffered HTMLRenderer writes its buffer out when it is full
	maxPooledBuf = 64 << 10
)

// HTMLRenderer is the Renderer writing HTML: what Render does. Escaping and
// sanitizing are templ's (see Attr and Element).
type HTMLRenderer struct {
	ctx  context.Context
	w    io.StringWriter
	buf  []byte    // buffered (renderHTML's): what is yet to be written to out
	out  io.Writer // what w writes to, when buffered
	mode uint8
	err  error
}

const (
	inHTML uint8 = iota
	inScript
	inStyle
)

// NewHTMLRenderer is an HTMLRenderer writing to w, unbuffered; ctx renders
// Node attribute values.
func NewHTMLRenderer(ctx context.Context, w io.Writer) *HTMLRenderer {
	sw, ok := w.(io.StringWriter)
	if !ok {
		sw = stringWriter{w}
	}
	return &HTMLRenderer{ctx: ctx, w: sw}
}

type stringWriter struct{ io.Writer }

func (w stringWriter) WriteString(s string) (int, error) { return io.WriteString(w.Writer, s) }

func (h *HTMLRenderer) write(s string) {
	if len(s) <= cap(h.buf)-len(h.buf) {
		h.buf = append(h.buf, s...)
		return
	}
	h.writeMore(s)
}

func (h *HTMLRenderer) writeByte(c byte) {
	if len(h.buf) < cap(h.buf) {
		h.buf = append(h.buf, c)
		return
	}
	h.writeMore(byteStrings[c : int(c)+1])
}

// byteStrings holds every byte, for writeByte's slow path.
var byteStrings = func() string {
	b := make([]byte, 256)
	for i := range b {
		b[i] = byte(i)
	}
	return string(b)
}()

// writeMore is write's slow path: a full buffer, or none.
func (h *HTMLRenderer) writeMore(s string) {
	if h.buf != nil {
		h.flush()
		if len(s) <= cap(h.buf) {
			h.buf = append(h.buf, s...)
			return
		}
	}
	if h.err == nil {
		_, h.err = h.w.WriteString(s)
	}
}

// writeEscaped writes s escaped as templ.EscapeString (html.EscapeString)
// escapes it.
func (h *HTMLRenderer) writeEscaped(s string) {
	i := 0
	for i < len(s) && !htmlSpecial[s[i]] {
		i++
	}
	if i == len(s) {
		h.write(s)
		return
	}
	last := 0
	for ; i < len(s); i++ {
		var esc string
		switch s[i] {
		case '&':
			esc = "&amp;"
		case '\'':
			esc = "&#39;"
		case '<':
			esc = "&lt;"
		case '>':
			esc = "&gt;"
		case '"':
			esc = "&#34;"
		default:
			continue
		}
		h.write(s[last:i])
		h.write(esc)
		last = i + 1
	}
	h.write(s[last:])
}

var htmlSpecial = [256]bool{'&': true, '\'': true, '<': true, '>': true, '"': true}

// flush writes a buffered renderer's buffer out.
func (h *HTMLRenderer) flush() {
	if len(h.buf) > 0 {
		if h.err == nil {
			_, h.err = h.out.Write(h.buf)
		}
		h.buf = h.buf[:0]
	}
}

func (h *HTMLRenderer) Open(e *Element) error {
	if err := h.openTag(e); err != nil {
		return err
	}
	if err := h.attrs(e); err != nil {
		return err
	}
	return h.endTag(e)
}

// openTag writes an element's start up to its attributes: <tag.
func (h *HTMLRenderer) openTag(e *Element) error {
	if !validName(e.Tag) {
		return fmt.Errorf("ui: invalid tag name %q", e.Tag)
	}
	if e.Tag == "html" {
		h.write("<!DOCTYPE html>")
	}
	h.writeByte('<')
	h.write(e.Tag)
	return h.err
}

// endTag ends an element's start tag, entering script or style content.
func (h *HTMLRenderer) endTag(e *Element) error {
	h.writeByte('>')
	switch e.Tag {
	case "script":
		h.mode = inScript
	case "style":
		h.mode = inStyle
	}
	return h.err
}

func (h *HTMLRenderer) Close(e *Element) error {
	h.mode = inHTML
	h.writeByte('<')
	h.writeByte('/')
	h.write(e.Tag)
	h.writeByte('>')
	return h.err
}

func (h *HTMLRenderer) Text(s string) error {
	switch h.mode {
	case inScript:
		js, err := templruntime.ScriptContentOutsideStringLiteral(s)
		if err != nil {
			return err
		}
		h.write(js)
	case inStyle:
		h.write(strings.ReplaceAll(s, "</", `<\/`))
	default:
		h.writeEscaped(s)
	}
	return h.err
}

func (h *HTMLRenderer) Raw(html string) error {
	h.noteRaw(html)
	h.write(html)
	return h.err
}

func (h *HTMLRenderer) Opaque(ctx context.Context, n Node) error {
	if h.err != nil {
		return h.err
	}
	if p := h.probe(); p != nil {
		p.info.Opaque = true
	}
	h.flush()
	w, ok := h.w.(io.Writer)
	if !ok {
		w = writerOf{h.w}
	}
	return n.Render(ctx, w)
}

type writerOf struct{ io.StringWriter }

func (w writerOf) Write(p []byte) (int, error) { return w.WriteString(string(p)) }

// attrs writes the attributes as templ's generated code does: each value
// resolved the way templ resolves an attribute expression of that name, then
// escaped by templ.EscapeString.
func (h *HTMLRenderer) attrs(e *Element) error {
	for _, a := range e.Attrs {
		if err := h.attr(e, a); err != nil {
			return err
		}
	}
	return nil
}

// attr writes one of e's attributes, with its leading space.
func (h *HTMLRenderer) attr(e *Element, a Attr) error { return h.attrNamed(e.Tag, e, a) }

// isEventName reports whether an attribute is an event's: on and a letter.
func isEventName(name string) bool {
	return len(name) > 2 && (name[0] == 'o' || name[0] == 'O') && (name[1] == 'n' || name[1] == 'N')
}

// attrNamed writes attribute a on an element of the given tag, with its
// leading space, exactly as attr does. e, when non-nil, is the element; only
// its data-vk-key is read, for the key attribute. The lazy-frame fast path
// (lazyFrame.fastHTML) passes a nil e, and a key attribute only where no
// live render writes it.
func (h *HTMLRenderer) attrNamed(tag string, e *Element, a Attr) error {
	name := a.Name
	if l, ok := a.Value.(lazyValue); ok {
		a.Value = l.fn(h.ctx)
	}
	if set, ok := a.Value.(attrSet); ok {
		for _, a := range set {
			if err := h.attrNamed(tag, e, a); err != nil {
				return err
			}
		}
		return nil
	}
	if c, ok := a.Value.(Classes); ok {
		a.Value = c.binding()
	}
	if b, ok := a.Value.(Binding); ok {
		return h.bindAttr(tag, e, name, b)
	}
	if fn := a.Value; fn != nil && isEventName(name) {
		if _, ok := fn.(JSChain); ok || reflect.TypeOf(fn).Kind() == reflect.Func {
			a.Value = EventHandler{fn} // a template's on… hole holds the handler as given
		}
	}
	if eh, ok := a.Value.(EventHandler); ok {
		if ch, ok := eh.Fn.(JSChain); ok {
			return h.jsAttr(name, ch)
		}
		if _, script := eh.Fn.(templ.ComponentScript); !script {
			return h.handlerAttr(name, eh)
		}
		a.Value = eh.Fn
	}
	switch name {
	case "key":
		if liveHost(h.ctx) != nil && a.Value != nil && !(e != nil && hasAttr(e, "data-vk-key")) {
			h.write(` data-vk-key="` + templ.EscapeString(fmt.Sprint(a.Value)) + `"`)
		}
		return nil
	case "ref":
		name = "data-vk-ref"
	case "className":
		name = "class"
	case "htmlFor":
		name = "for"
	}
	if !validName(name) {
		return fmt.Errorf("ui: invalid attribute name %q", a.Name)
	}
	v, kind, err := attrValue(h.ctx, tag, name, a.Value)
	if err != nil {
		return err
	}
	switch kind {
	case attrOmit:
	case attrBare:
		h.writeByte(' ')
		h.write(name)
	default:
		h.writeByte(' ')
		h.write(name)
		h.writeByte('=')
		h.writeByte('"')
		if kind == attrEscaped {
			h.writeEscaped(v)
		} else {
			h.write(v)
		}
		h.writeByte('"')
	}
	return nil
}

func hasAttr(e *Element, name string) bool {
	for _, a := range e.Attrs {
		if a.Name == name {
			return true
		}
	}
	return false
}

// What attrValue resolved an attribute to.
const (
	attrOmit    uint8 = iota
	attrBare          // true: the name alone
	attrEscaped       // a value to escape
	attrRaw           // a templ.ComponentScript's Call, which templ writes as is
)

// attrValue resolves an attribute's value the way templ resolves an attribute
// expression of that name.
func attrValue(ctx context.Context, tag, name string, v any) (string, uint8, error) {
	switch v := v.(type) {
	case nil:
		return "", attrOmit, nil
	case bool:
		if v {
			return "", attrBare, nil
		}
		return "", attrOmit, nil
	case string:
		switch {
		case name == "style":
		case isURLAttr(tag, name):
			return sanitizeURL(tag, name, v), attrEscaped, nil
		default:
			return v, attrEscaped, nil
		}
	case templ.ComponentScript: // a Node too
		return v.Call, attrRaw, nil
	case Node:
		s, err := String(ctx, v)
		return s, attrEscaped, err
	}
	switch {
	case name == "class":
		if c, ok := v.(Class); ok {
			v = map[string]bool(c)
		}
		return templ.Classes(v).String(), attrEscaped, nil
	case name == "style":
		if s, ok := v.(Style); ok {
			v = map[string]string(s)
		}
		// Sanitized and escaped, then escaped once more when written, as templ does.
		s, err := templruntime.SanitizeStyleAttributeValues(v)
		return s, attrEscaped, err
	case isURLAttr(tag, name):
		if u, ok := v.(templ.SafeURL); ok {
			return string(u), attrEscaped, nil
		}
		return sanitizeURL(tag, name, toString(v)), attrEscaped, nil
	}
	return toString(v), attrEscaped, nil // templ.ResolveAttributeValue: fmt.Sprint, escaped
}

// validName reports whether s is a tag or attribute name the renderer accepts:
// letters, digits, - _ : and ., starting with a letter.
func validName(s string) bool {
	if s == "" || nameByte[s[0]] != nameLetter {
		return false
	}
	for i := 1; i < len(s); i++ {
		if nameByte[s[i]] == 0 {
			return false
		}
	}
	return true
}

// nameByte classes a name's bytes: letters, then the other bytes a name
// may hold after its first.
var nameByte = func() (t [256]uint8) {
	for c := 'a'; c <= 'z'; c++ {
		t[c], t[c-'a'+'A'] = nameLetter, nameLetter
	}
	for _, c := range "0123456789-_:." {
		t[c] = nameOther
	}
	return t
}()

const (
	nameLetter = 1 + iota
	nameOther
)

// sanitizeURL is the value of a URL attribute (isURLAttr): templ.URL's,
// which keeps relative URLs and http, https, mailto, tel, ftp and ftps, and
// turns anything else into about:invalid#TemplFailedSanitizationURL. An
// image, audio or video source (img, source, video, audio and input src,
// video poster) and a track's text also keep a data: URL of that kind
// (data:image/…, data:audio/…, data:video/…, data:text/vtt), which the
// browser only decodes as media.
func sanitizeURL(tag, name, v string) string {
	if mediaAttr(tag, name) {
		d := strings.ToLower(strings.TrimLeftFunc(v, func(r rune) bool { return r <= ' ' }))
		for _, p := range [...]string{"data:image/", "data:audio/", "data:video/", "data:text/vtt"} {
			if strings.HasPrefix(d, p) {
				return v
			}
		}
	}
	return string(templ.URL(v))
}

func mediaAttr(tag, name string) bool {
	if len(name) != 3 && len(name) != 6 {
		return false
	}
	tag, name = strings.ToLower(tag), strings.ToLower(name)
	switch name {
	case "src":
		switch tag {
		case "img", "source", "video", "audio", "input", "track":
			return true
		}
	case "poster":
		return tag == "video"
	}
	return false
}

// isURLAttr reports whether tag's attribute name holds a URL, which a
// string value sanitizes with templ.URL: templ's set (href on a and link,
// action on form, data on object) and HTML's and SVG's other attributes
// a browser loads or follows — src on iframe, frame, embed, source,
// track, video, audio, img, input and script, poster on video,
// formaction on button and input, href on area, base and every SVG
// element (use, image, …), and xlink:href anywhere. Names and tags match
// ignoring case, as HTML's do.
func isURLAttr(tag, name string) bool {
	switch len(name) {
	case 3, 4, 6, 10: // src, href, data, poster, action, formaction, xlink:href
	default:
		return false
	}
	tag, name = strings.ToLower(tag), strings.ToLower(name)
	switch name {
	case "href":
		// every element with an href takes a URL: a, link, area, base, and
		// SVG's (a, use, image, feImage, textPath, …)
		return true
	case "xlink:href":
		return true
	case "src":
		switch tag {
		case "iframe", "frame", "embed", "source", "track", "video", "audio", "img", "input", "script":
			return true
		}
	case "poster":
		return tag == "video"
	case "formaction":
		return tag == "button" || tag == "input"
	case "action":
		return tag == "form"
	case "data":
		return tag == "object"
	}
	return false
}
