package ui

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/vuka-lang/ui/v2/forms"
)

// Widgets: a form field's control as a component (package
// github.com/vuka-lang/ui/v2/widget holds the built-in ones). A widget is a
// struct with a Model FieldRef field — the field it edits, which
// <x model={Form.Price}/> sets — and any of the props the form fills by
// name and type, those still zero (what the markup sets wins):
//
//	Label, Help, Placeholder, Error string   the field's, in the visitor's language; Error when it shows one
//	Required, Disabled bool
//	Value T                                  the field's value (an Option's when it holds one)
//	Choices []C                              the field's choices: C a struct of Value, Label string,
//	                                         Selected bool and Row R, R the type of the rows (any for any)
//	Search func(context.Context, string) []C the field's rows whose label holds the text, up to the
//	                                         widget's Limit int (20 when 0)
//
// A Value or Choices of a type the field's isn't is the render's error,
// naming both. A widget with Format(v V) map[string]string and Parse(map
// [string]string) (V, error) methods is made of several inputs, each named
// Model.Part(name): Parse turns what they sent into the field's value, its
// error the field's, and Format the value into what each shows
// (Model.PartValue(name)).
//
// A field takes its widget from, first to last: the markup naming it
// (<widget.Textarea model={Form.Notes}/>); @ui.Spec{Widget: widget.Radios{}}
// on the form type or the model; the app's widget for the field's type
// (UseWidget, widget.Use[T]); else the control its type gives.

// rowSource is a query whose rows are a field's choices: an orm query, an
// orm.FK's FormRows. FormChoice finds a key only among the query's rows.
type rowSource interface {
	FormChoices(ctx context.Context, search string, limit int) (keys, labels []string, rows []any, err error)
	FormChoice(ctx context.Context, key string) (label string, row any, ok bool, err error)
}

var rowSourceType = reflect.TypeFor[rowSource]()

// relatedValue is a field type referring to a row of a model (orm.FK):
// its choices are every row of the model, unless a <Field>Choices method
// says which.
type relatedValue interface{ FormRows() any }

var relatedType = reflect.TypeFor[relatedValue]()

// choiceRow is one of a field's choices, the row it is when it has one.
type choiceRow struct {
	Option
	row any
}

// widgetInfo is what a widget type declares of the props the form fills.
type widgetInfo struct {
	t                                       reflect.Type
	model                                   []int
	label, help, placeholder, errText       []int
	required, disabled                      []int
	value                                   []int
	choices, search                         []int
	elem                                    reflect.Type // the Choices' (and Search's) element
	cValue, cLabel, cSelected, cRow, cGroup []int
	limit                                   []int
	format, parse                           reflect.Value // the methods, of the type
	partType                                reflect.Type  // the value Format takes and Parse returns
	layout                                  string        // FieldLayout(): "check", "fieldset", or ""
	err                                     error
}

var (
	fieldRefType = reflect.TypeFor[FieldRef]()
	widgetInfos  sync.Map // reflect.Type → *widgetInfo (nil: not a widget)
	appWidgets   sync.Map // reflect.Type → reflect.Value: UseWidget's
)

// widgetInfoOf is t's widget description; nil when t isn't a widget (no
// Model FieldRef field).
func widgetInfoOf(t reflect.Type) *widgetInfo {
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	if v, ok := widgetInfos.Load(t); ok {
		w, _ := v.(*widgetInfo)
		return w
	}
	w := buildWidgetInfo(t)
	v, _ := widgetInfos.LoadOrStore(t, w)
	w, _ = v.(*widgetInfo)
	return w
}

func buildWidgetInfo(t reflect.Type) *widgetInfo {
	m, ok := t.FieldByName("Model")
	if !ok || m.Type != fieldRefType || !m.IsExported() {
		return nil
	}
	w := &widgetInfo{t: t, model: m.Index}
	field := func(name string, typ reflect.Type) []int {
		sf, ok := t.FieldByName(name)
		if !ok || !sf.IsExported() || typ != nil && sf.Type != typ {
			return nil
		}
		return sf.Index
	}
	str, boolean := reflect.TypeFor[string](), reflect.TypeFor[bool]()
	w.label, w.help, w.placeholder, w.errText = field("Label", str), field("Help", str), field("Placeholder", str), field("Error", str)
	w.required, w.disabled = field("Required", boolean), field("Disabled", boolean)
	w.value = field("Value", nil)
	w.limit = field("Limit", reflect.TypeFor[int]())
	if idx := field("Choices", nil); idx != nil {
		ct := t.FieldByIndex(idx).Type
		if ct.Kind() != reflect.Slice || !w.choiceElem(ct.Elem()) {
			w.err = fmt.Errorf("ui: %s.Choices is a %s: a widget's Choices are a []widget.Choice[T] (a struct of Value, Label, Selected and Row)", t.Name(), ct)
			return w
		}
		w.choices = idx
	}
	if idx := field("Search", nil); idx != nil {
		ft := t.FieldByIndex(idx).Type
		ok := ft.Kind() == reflect.Func && ft.NumIn() == 2 && ft.In(0) == ctxType && ft.In(1) == str && ft.NumOut() == 1 && ft.Out(0).Kind() == reflect.Slice
		if ok && w.elem != nil && ft.Out(0).Elem() != w.elem {
			ok = false
		}
		if !ok || !w.choiceElem(ft.Out(0).Elem()) {
			w.err = fmt.Errorf("ui: %s.Search is a %s: a widget's Search is a func(context.Context, string) []widget.Choice[T]", t.Name(), ft)
			return w
		}
		w.search = idx
	}
	if fm, ok := t.MethodByName("Format"); ok {
		pm, ok2 := t.MethodByName("Parse")
		mt := reflect.TypeFor[map[string]string]()
		switch {
		case !ok2:
			w.err = fmt.Errorf("ui: %s has Format but no Parse: a widget of several inputs has both", t.Name())
		case fm.Type.NumIn() != 2 || fm.Type.NumOut() != 1 || fm.Type.Out(0) != mt:
			w.err = fmt.Errorf("ui: %s.Format is %s: want func (%[1]s) Format(v V) map[string]string", t.Name(), fm.Type)
		case pm.Type.NumIn() != 2 || pm.Type.In(1) != mt || pm.Type.NumOut() != 2 || pm.Type.Out(0) != fm.Type.In(1) || pm.Type.Out(1) != errType:
			w.err = fmt.Errorf("ui: %s.Parse is %s: want func (%[1]s) Parse(in map[string]string) (%s, error)", t.Name(), pm.Type, fm.Type.In(1))
		default:
			w.format, w.parse, w.partType = fm.Func, pm.Func, fm.Type.In(1)
		}
	}
	if lm, ok := t.MethodByName("FieldLayout"); ok && lm.Type.NumIn() == 1 && lm.Type.NumOut() == 1 && lm.Type.Out(0).Kind() == reflect.String {
		w.layout = lm.Func.Call([]reflect.Value{reflect.Zero(t)})[0].String()
	}
	return w
}

// choiceElem records a Choices element type: a struct of Value and Label
// (strings), and maybe Selected (bool), Row and Group.
func (w *widgetInfo) choiceElem(e reflect.Type) bool {
	if w.elem != nil {
		return e == w.elem
	}
	if e.Kind() != reflect.Struct {
		return false
	}
	get := func(name string, typ reflect.Type) []int {
		sf, ok := e.FieldByName(name)
		if !ok || typ != nil && sf.Type != typ {
			return nil
		}
		return sf.Index
	}
	str := reflect.TypeFor[string]()
	w.cValue, w.cLabel = get("Value", str), get("Label", str)
	if w.cValue == nil || w.cLabel == nil {
		return false
	}
	w.cSelected, w.cRow, w.cGroup = get("Selected", reflect.TypeFor[bool]()), get("Row", nil), get("Group", str)
	w.elem = e
	return true
}

// IsWidget reports whether v is a widget: a struct with a Model FieldRef
// field (see UseWidget).
func IsWidget(v any) bool {
	w := widgetInfoOf(reflect.TypeOf(v))
	return w != nil && w.err == nil
}

// UseWidget makes w the widget of every form field of type t that names no
// control of its own (a tag, @ui.Widget or @ui.Spec): what widget.Use[T]
// does. w is a widget value, its props its options (widget.Textarea{Rows:
// 6}); a field of an Option or a pointer of t takes it too. A nil w
// gives the type its own control back.
func UseWidget(t reflect.Type, w any) {
	if w == nil {
		appWidgets.Delete(t) // the type's own control again
		return
	}
	info := widgetInfoOf(reflect.TypeOf(w))
	switch {
	case info == nil:
		panic(fmt.Sprintf("ui: UseWidget(%s, %T): a widget is a struct with a Model ui.FieldRef field", t, w))
	case info.err != nil:
		panic(info.err.Error())
	}
	appWidgets.Store(t, reflect.ValueOf(w))
}

// widgetFor is the widget a field takes when its markup names none: its
// @ui.Spec's, else the app's for its type, unless a tag or attribute
// named its control.
func (f *formField) widgetFor() (reflect.Value, bool) {
	if f.widget.IsValid() {
		return f.widget, true
	}
	if f.inputTag != "" {
		return reflect.Value{}, false
	}
	for _, t := range []reflect.Type{f.typ, valueType(f.typ)} {
		if v, ok := appWidgets.Load(t); ok {
			return v.(reflect.Value), true
		}
	}
	return reflect.Value{}, false
}

// FillWidget fills the props of the widget w points to from the form field
// its Model names, those still zero: what the form does for a widget it
// renders. A widget written in Go calls it from its Render.
func FillWidget(ctx context.Context, w any) error {
	v := reflect.ValueOf(w)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("ui: FillWidget takes a pointer to a widget, not %T", w)
	}
	info := widgetInfoOf(v.Elem().Type())
	if info == nil {
		return fmt.Errorf("ui: FillWidget: %T has no Model ui.FieldRef field", w)
	}
	if info.err != nil {
		return info.err
	}
	ref := v.Elem().FieldByIndex(info.model).Interface().(FieldRef)
	if !ref.field.IsValid() && ref.bound == nil {
		return fmt.Errorf("ui: <%s> has no model: give it model={Form.Field}", info.t.Name())
	}
	b, err := ref.resolve()
	if err != nil {
		return err
	}
	if b.f == nil || b.f.group {
		return fmt.Errorf("ui: <%s model={…}> takes a field of a form, not the form or a group of fields", info.t.Name())
	}
	return b.fill(ctx, v.Elem(), info)
}

// fill sets w's zero props from the field.
func (b boundField) fill(ctx context.Context, w reflect.Value, info *widgetInfo) error {
	f, st, form := b.f, b.st, b.value()
	set := func(idx []int, v any) {
		if idx == nil {
			return
		}
		if d := w.FieldByIndex(idx); d.IsZero() {
			d.Set(reflect.ValueOf(v))
		}
	}
	if m := w.FieldByIndex(info.model); m.IsZero() {
		bb := b
		m.Set(reflect.ValueOf(FieldRef{bound: &bb}))
	}
	set(info.label, tr(ctx, f.label))
	set(info.help, tr(ctx, f.help))
	set(info.placeholder, tr(ctx, f.placeholder))
	if e, shows := st.visible(f.path); shows {
		set(info.errText, st.message(ctx, e))
	}
	set(info.required, f.required())
	set(info.disabled, f.disabled)
	if info.value != nil {
		if d := w.FieldByIndex(info.value); d.IsZero() {
			fv := form.FieldByIndex(f.index)
			switch {
			case fv.Type().AssignableTo(d.Type()):
				d.Set(fv)
			case isOption(fv.Type()) && fv.Type().Field(0).Type.AssignableTo(d.Type()):
				if val, ok := optionParts(fv); ok.Bool() {
					d.Set(val)
				}
			default:
				return fmt.Errorf("ui: <%s model={…}>: its Value is a %s, and %s is a %s", info.t.Name(), d.Type(), f.goPath, fv.Type())
			}
		}
	}
	if info.choices != nil {
		if d := w.FieldByIndex(info.choices); d.Len() == 0 && f.hasChoices() {
			rows, err := st.choiceRows(ctx, form, f)
			if err != nil {
				return err
			}
			out, err := info.choiceSlice(rows, func(o Option) bool { return selected(form, f, o.Value) }, f)
			if err != nil {
				return err
			}
			d.Set(out)
		}
	}
	if info.search != nil {
		if d := w.FieldByIndex(info.search); d.IsNil() {
			src, err := st.sourceOf(ctx, form, f)
			if err != nil {
				return err
			}
			if src != nil {
				limit := 20
				if info.limit != nil {
					if n := int(w.FieldByIndex(info.limit).Int()); n > 0 {
						limit = n
					}
				}
				d.Set(reflect.MakeFunc(d.Type(), func(args []reflect.Value) []reflect.Value {
					sctx := args[0].Interface().(context.Context)
					keys, labels, rows, err := src.FormChoices(sctx, args[1].String(), limit)
					out := reflect.MakeSlice(d.Type().Out(0), 0, len(keys))
					if err == nil {
						cr := make([]choiceRow, len(keys))
						for i := range keys {
							cr[i] = choiceRow{Option{Value: keys[i], Label: labels[i]}, rows[i]}
						}
						if o, cerr := info.choiceSlice(cr, func(o Option) bool { return selected(form, f, o.Value) }, f); cerr == nil {
							out = o
						}
					}
					return []reflect.Value{out}
				}))
			}
		}
	}
	if info.parse.IsValid() {
		st.ft.parts.Store(f.path, info)
	}
	return nil
}

// choiceSlice is rows as a widget's Choices.
func (info *widgetInfo) choiceSlice(rows []choiceRow, sel func(Option) bool, f *formField) (reflect.Value, error) {
	out := reflect.MakeSlice(reflect.SliceOf(info.elem), len(rows), len(rows))
	for i, r := range rows {
		c := out.Index(i)
		c.FieldByIndex(info.cValue).SetString(r.Value)
		c.FieldByIndex(info.cLabel).SetString(r.Label)
		if info.cSelected != nil {
			c.FieldByIndex(info.cSelected).SetBool(sel(r.Option))
		}
		if info.cGroup != nil {
			c.FieldByIndex(info.cGroup).SetString(r.Group)
		}
		if info.cRow != nil && r.row != nil {
			d := c.FieldByIndex(info.cRow)
			rv := reflect.ValueOf(r.row)
			if !rv.Type().AssignableTo(d.Type()) {
				return reflect.Value{}, fmt.Errorf("ui: <%s model={…}>: its Choices' Row is a %s, and the rows of %s's choices are %s", info.t.Name(), d.Type(), f.goPath, rv.Type())
			}
			d.Set(rv)
		}
	}
	return out, nil
}

// widgetNode renders a widget its markup gave a model: filled from the
// field, then rendered.
type widgetNode struct {
	v    reflect.Value // the widget, as written
	info *widgetInfo
}

func (n widgetNode) Render(ctx context.Context, w io.Writer) error {
	cp := reflect.New(n.info.t).Elem()
	cp.Set(n.v)
	if err := FillWidget(ctx, cp.Addr().Interface()); err != nil {
		return Try(nil, err).Render(ctx, w)
	}
	return cp.Interface().(Node).Render(ctx, w)
}

// asWidget is v as a widget filled as it renders, when v is a widget its
// markup gave a model; else v.
func asWidget(v any) (Node, bool) {
	rv := reflect.ValueOf(v)
	info := widgetInfoOf(rv.Type())
	if info == nil {
		return nil, false
	}
	if ref := rv.FieldByIndex(info.model).Interface().(FieldRef); !ref.field.IsValid() && ref.bound == nil {
		return nil, false
	}
	return widgetNode{rv, info}, true
}

// widgetControl is the control of the field b, its widget w filled from
// it.
func (b boundField) widgetControl(ctx context.Context, w reflect.Value) Node {
	info := widgetInfoOf(w.Type())
	cp := reflect.New(info.t).Elem()
	cp.Set(w)
	bb := b
	cp.FieldByIndex(info.model).Set(reflect.ValueOf(FieldRef{bound: &bb}))
	if err := b.fill(ctx, cp, info); err != nil {
		return Try(nil, err)
	}
	n, ok := cp.Interface().(Node)
	if !ok {
		return Try(nil, fmt.Errorf("ui: the widget %s isn't a component: it has no Render method", info.t))
	}
	return n
}

// widgetGroup is a field's group around its widget: label, hint, error
// and the widget, laid out as the widget says (FieldLayout).
func (b boundField) widgetGroup(ctx context.Context, opts Field, w reflect.Value) Node {
	f, st := b.f, b.st
	info := widgetInfoOf(w.Type())
	label := tr(ctx, cmpOr(opts.Label, f.label))
	hint := b.hint(tr(ctx, cmpOr(opts.Help, f.help)))
	e, shows := st.visible(f.path)
	errNode := b.errorNode(ctx, e, shows)
	ctl := b.widgetControl(ctx, w)
	classes := "vk-field"
	if shows {
		classes += " vk-field--invalid"
	}
	id := st.id(f.path)
	switch info.layout {
	case "fieldset":
		return El("fieldset", []Attr{{"class", withClass("vk-fieldset "+classes, opts.Class)}, {"data-invalid", shows},
			{"aria-describedby", lazyValue{func(context.Context) any { return st.describedBy(f) }}}},
			El("legend", []Attr{{"class", "vk-fieldset__legend"}}, Text(label), b.marker(ctx)), hint, errNode, ctl, b.update(ctx))
	case "check":
		return El("div", []Attr{{"class", withClass(classes+" vk-field--check", opts.Class)}, {"data-invalid", shows}}, hint, errNode,
			El("div", []Attr{{"class", "vk-check"}}, ctl, El("label", []Attr{{"class", "vk-check__label"}, {"for", id}}, Text(label), b.marker(ctx))), b.update(ctx))
	}
	return El("div", []Attr{{"class", withClass(classes, opts.Class)}, {"data-invalid", shows}},
		El("label", []Attr{{"class", "vk-field__label"}, {"for", id}}, Text(label), b.marker(ctx)), hint, errNode, ctl, b.update(ctx))
}

// Part is the name of one input of a widget of several (one with Format
// and Parse): <input name={Model.Part("amount")}/>. What the inputs send
// is the widget's Parse's to turn into the field's value.
func (r FieldRef) Part(name string) string {
	b, err := r.resolve()
	if err != nil || b.f == nil {
		return name
	}
	b.st.renderField(b.f.path, "")
	return b.st.prefix + b.f.path + partSep + name
}

// partSep joins a field's path and one of its parts in an input's name.
const partSep = ":"

// PartValue is what the input Part(name) shows: what it last sent, else
// the widget's Format of the field's value.
func (r FieldRef) PartValue(name string) string {
	b, err := r.resolve()
	if err != nil || b.f == nil {
		return ""
	}
	if vs, ok := b.st.parts[b.f.path]; ok {
		if s, ok := vs[name]; ok {
			return s
		}
	}
	v, ok := b.st.ft.parts.Load(b.f.path)
	if !ok {
		return ""
	}
	info := v.(*widgetInfo)
	fv := b.value().FieldByIndex(b.f.index)
	in, ok := partInput(fv, info.partType)
	if !ok {
		return ""
	}
	out := info.format.Call([]reflect.Value{reflect.Zero(info.t), in})[0].Interface().(map[string]string)
	return out[name]
}

// ID is the id of the field's control: a label's for.
func (r FieldRef) ID() string {
	b, err := r.resolve()
	if err != nil || b.f == nil {
		return ""
	}
	return b.st.id(b.f.path)
}

// Path is the field's path, its control's name: "price", "address.city".
// A widget naming its control with it renders the field (a submit sets
// it), as model={Model} does.
func (r FieldRef) Path() string {
	b, err := r.resolve()
	if err != nil || b.f == nil {
		return ""
	}
	b.st.renderField(b.f.path, "")
	return b.st.prefix + b.f.path
}

// Chosen is the field's current choice: its value as text and its label,
// read from the field's choices (an orm query's row); both "" when the
// field has no value, the label "" when its choices don't hold it.
func (r FieldRef) Chosen(ctx context.Context) (value, label string) {
	b, err := r.resolve()
	if err != nil || b.f == nil {
		return "", ""
	}
	value = b.st.fieldText(b.value(), b.f)
	if value == "" {
		return "", ""
	}
	if src, err := b.st.sourceOf(ctx, b.value(), b.f); err == nil && src != nil {
		if l, _, ok, err := src.FormChoice(ctx, value); err == nil && ok {
			return value, l
		}
		return value, ""
	}
	rows, _ := b.st.choiceRows(ctx, b.value(), b.f)
	for _, o := range rows {
		if o.Value == value {
			return value, o.Label
		}
	}
	return value, ""
}

// partInput is a field's value as Format takes it: the field itself, or
// an Option's or a pointer's value; ok false when it holds none.
func partInput(fv reflect.Value, t reflect.Type) (reflect.Value, bool) {
	switch {
	case fv.Type() == t:
		return fv, true
	case isOption(fv.Type()) && fv.Type().Field(0).Type == t:
		val, ok := optionParts(fv)
		return val, ok.Bool()
	case fv.Kind() == reflect.Pointer && fv.Type().Elem() == t:
		if fv.IsNil() {
			return reflect.Value{}, false
		}
		return fv.Elem(), true
	}
	return reflect.Value{}, false
}

// sentParts are the values sent for the parts of the form's fields, by
// field path and part name.
func sentParts(vals url.Values, prefix string) map[string]map[string]string {
	var out map[string]map[string]string
	for k, vs := range vals {
		k, ok := strings.CutPrefix(k, prefix)
		if !ok {
			continue
		}
		path, name, ok := strings.Cut(k, partSep)
		if !ok || len(vs) == 0 {
			continue
		}
		if out == nil {
			out = map[string]map[string]string{}
		}
		if out[path] == nil {
			out[path] = map[string]string{}
		}
		out[path][name] = vs[len(vs)-1]
	}
	return out
}

// bindParts sets a field of a widget of several inputs from what they
// sent: its Parse's value, or its error, the field's.
func (st *formState) bindParts(fv reflect.Value, f *formField, info *widgetInfo, in map[string]string) {
	out := info.parse.Call([]reflect.Value{reflect.Zero(info.t), reflect.ValueOf(in)})
	if err, _ := out[1].Interface().(error); err != nil {
		fv.SetZero()
		if st.parsed == nil {
			st.parsed = map[string]forms.Error{}
		}
		e := forms.Errorf("%s", err.Error())
		e.Path, e.Rule = f.path, "format"
		st.parsed[f.path] = e
		return
	}
	v := out[0]
	switch {
	case v.Type().AssignableTo(fv.Type()):
		fv.Set(v)
	case isOption(fv.Type()):
		fv.SetZero()
		if !v.IsZero() {
			val, ok := optionParts(fv)
			val.Set(v)
			ok.SetBool(true)
		}
	case fv.Kind() == reflect.Pointer:
		p := reflect.New(v.Type())
		p.Elem().Set(v)
		fv.Set(p)
	}
}

// sourceOf is the query a field's choices are: what its <Field>Choices
// returned, when that is one, or an orm.FK's every row; nil for a field
// whose choices are a list.
func (st *formState) sourceOf(ctx context.Context, form reflect.Value, f *formField) (rowSource, error) {
	if f.choices != nil {
		_, src, err := st.choiceResult(ctx, form, f)
		return src, err
	}
	if f.related {
		v := reflect.Zero(valueType(f.typ))
		if src, ok := v.Interface().(relatedValue).FormRows().(rowSource); ok {
			return src, nil
		}
	}
	return nil, nil
}

// choiceResult is what a field's <Field>Choices returned: a list, or a
// query; called once per event or render.
func (st *formState) choiceResult(ctx context.Context, form reflect.Value, f *formField) ([]Option, rowSource, error) {
	if r, ok := st.results[f.path]; ok {
		return r.opts, r.src, nil
	}
	opts, src, err := choices(ctx, f.choices, st.ft.recvOf(f.choices, form))
	if err != nil {
		return nil, nil, fmt.Errorf("ui: %s.%s: %w", st.ft.t.Name(), f.choices.Name, err)
	}
	if st.results == nil {
		st.results = map[string]choicesResult{}
	}
	st.results[f.path] = choicesResult{opts, src}
	return opts, src, nil
}

// choicesResult is a <Field>Choices method's result.
type choicesResult struct {
	opts []Option
	src  rowSource
}

// choiceRows are a field's choices with their rows: its <Field>Choices'
// (a list, or a query's rows), its attributes', or an orm.FK's model's
// every row; computed once per event or render.
func (st *formState) choiceRows(ctx context.Context, form reflect.Value, f *formField) ([]choiceRow, error) {
	if f.choices == nil && !f.related {
		out := make([]choiceRow, len(f.options))
		for i, o := range f.options {
			out[i] = choiceRow{Option: o}
		}
		return out, nil
	}
	if rows, ok := st.memo[f.path]; ok {
		return rows, nil
	}
	var rows []choiceRow
	var src rowSource
	if f.choices != nil {
		opts, s, err := st.choiceResult(ctx, form, f)
		if err != nil {
			return nil, err
		}
		for _, o := range opts {
			rows = append(rows, choiceRow{Option: o})
		}
		src = s
	} else {
		s, err := st.sourceOf(ctx, form, f)
		if err != nil {
			return nil, err
		}
		src = s
	}
	if src != nil {
		keys, labels, rs, err := src.FormChoices(ctx, "", 0)
		if err != nil {
			return nil, fmt.Errorf("ui: the choices of %s: %w", f.goPath, err)
		}
		for i := range keys {
			rows = append(rows, choiceRow{Option{Value: keys[i], Label: labels[i]}, rs[i]})
		}
	}
	if st.memo == nil {
		st.memo = map[string][]choiceRow{}
	}
	st.memo[f.path] = rows
	return rows, nil
}
