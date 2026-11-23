package ui

import (
	"context"
	"fmt"
	"hash/fnv"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// AutoID is a component's id prop, for the ids its markup gives its elements
// and the attributes naming them (aria-labelledby, for, popovertarget):
//
//	type Dialog struct {
//		ID ui.AutoID
//	}
//
//	<dialog id={ID} aria-labelledby={ID.Suffix("title")}>
//	  <h2 id={ID.Suffix("title")}>Confirm</h2>
//	</dialog>
//
// A tag that leaves it out gets one the compiler asks NewAutoID for: an id
// unique on the page and the same in every render of the page, so two
// dialogs never share one and a live patch never renames it. A tag may
// still set it, id="confirm", and that value is the id. An AutoID nothing
// set (the component built in Go, not by its tag) is empty, and an
// attribute set to it is left out.
type AutoID string

// Suffix is the id of a part: the id, a dash and part, ID.Suffix("title")
// for "dlg-title". An empty id has no parts: its suffix is empty too.
func (id AutoID) Suffix(part string) AutoID {
	if id == "" {
		return ""
	}
	return id + "-" + AutoID(part)
}

// String is the id.
func (id AutoID) String() string { return string(id) }

// FillAutoIDs sets each exported AutoID field of the live component c that
// its tag left empty to the instance's own id: "vk-" and id for a field ID,
// with the field's name after it for another. Package live calls it, with
// the instance's id, once it takes its tag's props: a live component's id
// is its instance's, the same for its whole life.
func FillAutoIDs(c Stateful, id string) {
	v := reflect.ValueOf(c)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return
	}
	v = v.Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || f.Type != autoIDType || v.Field(i).String() != "" {
			continue
		}
		name := "vk-" + strings.ReplaceAll(id, ".", "_")
		if f.Name != "ID" {
			name += "-" + strings.ToLower(f.Name)
		}
		v.Field(i).SetString(name)
	}
}

// IsAutoID reports whether t is AutoID: a field a live component's tag
// leaving it empty doesn't set (FillAutoIDs).
func IsAutoID(t reflect.Type) bool { return t == autoIDType }

var autoIDType = reflect.TypeFor[AutoID]()

// IDHost names the AutoIDs rendered under a live host, as StateHost names
// states: package live's scope by the instance rendering them, the keys
// around them, their tag's site and how many of the site's ids that
// instance rendered before under those keys; RenderPage's by the site and
// its count in the page's render.
type IDHost interface {
	AutoID(ctx context.Context, site string) string
}

var staticIDs atomic.Uint64

// NewAutoID is the id of the component tag that calls it, rendered under ctx:
// what the compiler sets an AutoID prop to when the tag leaves it out. It is
// named by the place of the tag in the source (the caller's), the instance
// rendering it and the keys around it, never by a count of the page's ids,
// so it is the same in each render of the same page — the HTTP render, a
// live session's join and its patches, a lazy region's — while the markup
// around it keeps its shape. With no host (a render of a Node outside a page,
// ui.String) it is unique in the process.
func NewAutoID(ctx context.Context) AutoID {
	site := autoIDSite()
	if h, ok := liveHost(ctx).(IDHost); ok {
		return AutoID(h.AutoID(ctx, site))
	}
	return AutoID("vk-" + site + "-x" + strconv.FormatUint(staticIDs.Add(1), 36))
}

var autoIDSites sync.Map // pc → site

// autoIDSite is NewAutoID's caller's place: a short name, the same in every
// run of the binary, valid in an id and a CSS selector.
func autoIDSite() string {
	var pc [1]uintptr
	runtime.Callers(3, pc[:])
	if s, ok := autoIDSites.Load(pc[0]); ok {
		return s.(string)
	}
	f, _ := runtime.CallersFrames(pc[:]).Next()
	h := fnv.New64a()
	fmt.Fprintf(h, "%s:%d:%d", f.Function, f.Line, f.PC-f.Entry)
	s, _ := autoIDSites.LoadOrStore(pc[0], "i"+strconv.FormatUint(h.Sum64()&0xffffffffff, 36))
	return s.(string)
}

// AutoID names an id as a live session names those of its page (c0), by
// site and how many the render met before.
func (p *pageProbe) AutoID(_ context.Context, site string) string {
	if p.ids == nil {
		p.ids = map[string]int{}
	}
	p.ids[site]++
	return "vk-c0-" + site + "-" + strconv.Itoa(p.ids[site])
}

// OmitZero is v, or nil when v is its type's zero value: an attribute
// written name?={v} is left out when v is zero, as one set to nil is.
func OmitZero[T any](v T) any {
	if isZero(v) {
		return nil
	}
	return v
}

func isZero[T any](v T) bool {
	switch x := any(v).(type) {
	case nil:
		return true
	case string:
		return x == ""
	case int:
		return x == 0
	}
	return reflect.ValueOf(&v).Elem().IsZero()
}

// staticInstance names a live component rendered without a session, for
// its AutoIDs: unique in the process.
func staticInstance() string { return "s" + strconv.FormatUint(staticIDs.Add(1), 36) }
