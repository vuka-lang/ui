package ui

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/vuka-lang/ui/v2/forms"
	"github.com/vuka-lang/vuka/v2"
)

// Rows are many rows of a T edited in one form (Django's formsets): an
// order's lines, a product's variants. A form type holds them as a field
// of its own, each row a form of T — its fields, rules, Clean and
// Clean<Field> methods and errors its own — and the form's markup lays
// them out:
//
//	type OrderForm struct {
//		ui.FormFor[Order]
//		Lines ui.Rows[OrderLine] @ui.Spec{Min: 1, Max: 20}
//	}
//
//	<form for={Form} onSubmit={save}>
//	  for _, r := range Form.Lines.Each() {
//	    <widget.Select model={r.Product}/>
//	    <widget.Number model={r.Quantity}/>
//	    <widget.Error for={r.Quantity}/>
//	    <button type="button" onClick={Form.Lines.Remove(r)}>Remove</button>
//	  }
//	  <button type="button" onClick={Form.Lines.Add()}>Add line</button>
//	</form>
//
// The form's binding, checks and submit take its rows with it: a row's
// error shows at its field and in the form's summary, a form-level Clean
// sees every row (Form.Lines.Values()), and a submit runs its handler only
// when every row passes too. @ui.Spec{Min, Max} on the field bound how
// many rows the form takes; Add beyond Max is refused. A new row starts
// from T's zero value, its fields' defaults, and T's Init(ctx) error when
// T has one.
//
// Of a model whose T has an orm.FK back to the form's model, the rows are
// the record's related rows: the form's Load loads them, and its Save
// writes the rows added, changed and removed in one transaction with the
// record. Rows of any other struct are the form's to read (Values) and
// set (Load).
type Rows[T any] struct {
	rows []*FormFor[T]
	keys []string
	gone []*FormFor[T] // loaded rows removed: deleted by the form's Save
	next int
	path string // the field's path in the form: "lines"
	min  int
	max  int
}

// Each is the rows, in order, each a form of T: markup names a row's
// field through it, model={r.Quantity}.
func (r *Rows[T]) Each() []*FormFor[T] { return r.rows }

// Len is how many rows there are.
func (r *Rows[T]) Len() int { return len(r.rows) }

// Values are the rows' values, in order.
func (r *Rows[T]) Values() []T {
	out := make([]T, len(r.rows))
	for i, row := range r.rows {
		out[i] = row.Value
	}
	return out
}

// Add is the event adding a row: <button type="button"
// onClick={Form.Lines.Add()}>. Beyond Max it adds none.
func (r *Rows[T]) Add() func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := r.Append(ctx)
		return err
	}
}

// Remove is the event removing row: <button type="button"
// onClick={Form.Lines.Remove(r)}>. A loaded row removed is deleted when
// the form saves.
func (r *Rows[T]) Remove(row *FormFor[T]) func(context.Context) error {
	return func(context.Context) error {
		r.Delete(row)
		return nil
	}
}

// Append adds a new row and returns it: T's zero value, its fields'
// defaults, then T's Init(ctx) error when it has one. It adds none, and
// returns nil, when the rows are at their Max.
func (r *Rows[T]) Append(ctx context.Context) (*FormFor[T], error) {
	if r.max > 0 && len(r.rows) >= r.max {
		return nil, nil
	}
	row := r.newRow()
	st, form := row.attached()
	st.runInit(ctx, form)
	if in, ok := any(&row.Value).(interface{ Init(context.Context) error }); ok {
		if err := in.Init(ctx); err != nil {
			r.Delete(row)
			return nil, err
		}
		st.base = cloneValue(form)
	}
	return row, nil
}

// Delete removes row; a loaded one is deleted when the form saves.
func (r *Rows[T]) Delete(row *FormFor[T]) {
	for i, x := range r.rows {
		if x != row {
			continue
		}
		r.rows = append(r.rows[:i:i], r.rows[i+1:]...)
		r.keys = append(r.keys[:i:i], r.keys[i+1:]...)
		if st, _ := row.attached(); !st.isNew() {
			r.gone = append(r.gone, row)
		}
		return
	}
}

// Load makes vs the rows, each a record a row edits (Dirty and Changed
// compare with it): what the form's Load does with a model's related
// rows, and a page does with any other struct's.
func (r *Rows[T]) Load(ctx context.Context, vs ...T) error {
	r.rows, r.keys, r.gone = nil, nil, nil
	for _, v := range vs {
		if err := r.newRow().Load(ctx, v); err != nil {
			return err
		}
	}
	return nil
}

func (r *Rows[T]) newRow() *FormFor[T] {
	r.next++
	row := &FormFor[T]{}
	if i, b := r.backRef(); b != nil {
		// the back reference is the form's to set, never a row's field
		row.setNames(r.rowNames(reflect.TypeFor[T]().Field(i).Name), true)
	}
	r.rows = append(r.rows, row)
	r.keys = append(r.keys, "r"+strconv.Itoa(r.next))
	r.prepRow(row, r.keys[len(r.keys)-1])
	return row
}

// prepRow gives a row its state as a row of the form: its controls'
// names under the field's path and its key, its ids its key's.
func (r *Rows[T]) prepRow(row *FormFor[T], key string) {
	st, _ := row.attached()
	if r.path != "" {
		st.prefix = r.path + "." + key + "."
		st.key = r.path + "-" + key
	}
}

// rowSet is a Rows field of a form type, as the form takes it.
type rowSet interface {
	rowsPrep(path string, min, max int)
	rowsBegin()
	rowsFinish()
	rowsBind(ctx context.Context, vals url.Values)
	rowsCheck(ctx context.Context, live, submitted bool) (bad bool, err error)
	rowsSummary(ctx context.Context) []SummaryItem
	rowsCount() int
	rowsLoad(ctx context.Context, parentKey string) error
	rowsSave(ctx context.Context, parentKey string) error
	rowsTx() func(context.Context, func(context.Context) error) error
	rowsClear()
}

var rowSetType = reflect.TypeFor[rowSet]()

func (r *Rows[T]) rowsPrep(path string, min, max int) {
	r.path, r.min, r.max = path, min, max
	for i, row := range r.rows {
		r.prepRow(row, r.keys[i])
	}
}

func (r *Rows[T]) rowsBegin() {
	for _, row := range r.rows {
		st, form := row.attached()
		st.applyDefaults(form)
		st.beginRender()
	}
}

func (r *Rows[T]) rowsFinish() {
	for _, row := range r.rows {
		st, _ := row.attached()
		st.rendered, st.inRender, st.memo, st.results = st.rendering, false, nil, nil
	}
}

func (r *Rows[T]) rowsBind(ctx context.Context, vals url.Values) {
	for _, row := range r.rows {
		st, form := row.attached()
		st.usedFrom(vals)
		st.bind(ctx, form, vals, st.rendered)
	}
}

func (r *Rows[T]) rowsCheck(ctx context.Context, live, submitted bool) (bool, error) {
	bad := false
	for _, row := range r.rows {
		st, form := row.attached()
		st.submitted = st.submitted || submitted
		if err := st.check(ctx, form, live); err != nil {
			return false, err
		}
		bad = bad || len(st.all()) > 0
	}
	return bad, nil
}

func (r *Rows[T]) rowsSummary(ctx context.Context) []SummaryItem {
	var out []SummaryItem
	for _, row := range r.rows {
		st, _ := row.attached()
		if !st.submitted {
			continue
		}
		for _, e := range st.shownErrors() {
			it := SummaryItem{Text: st.message(ctx, e)}
			if e.Path != "" && st.ft.byPath[e.Path] != nil {
				it.Href = "#" + st.id(e.Path)
			}
			out = append(out, it)
		}
	}
	return out
}

func (r *Rows[T]) rowsCount() int { return len(r.rows) }

func (r *Rows[T]) rowsClear() { r.rows, r.keys, r.gone = nil, nil, nil }

// backRef is the field of T referring to the form's model, an orm.FK to
// it, and that FK type's form hooks; nil when T has none.
func (r *Rows[T]) backRef() (int, rowsBackRef) {
	t := reflect.TypeFor[T]()
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		if b, ok := reflect.Zero(valueType(sf.Type)).Interface().(rowsBackRef); ok {
			return i, b
		}
	}
	return -1, nil
}

// rowsBackRef is an orm.FK's form hooks: the rows of a model referring to
// a record, and a transaction.
type rowsBackRef interface {
	FormChildren(ctx context.Context, child reflect.Type, field string, key string) ([]any, error)
	FormAtomic(ctx context.Context, fn func(context.Context) error) error
}

func (r *Rows[T]) rowsLoad(ctx context.Context, parentKey string) error {
	i, b := r.backRef()
	if b == nil {
		return nil // not a model's: the page loads them
	}
	rows, err := b.FormChildren(ctx, reflect.TypeFor[T](), reflect.TypeFor[T]().Field(i).Name, parentKey)
	if err != nil {
		return err
	}
	vs := make([]T, len(rows))
	for j, x := range rows {
		vs[j] = x.(T)
	}
	return r.Load(ctx, vs...)
}

func (r *Rows[T]) rowsTx() func(context.Context, func(context.Context) error) error {
	if _, b := r.backRef(); b != nil {
		return b.FormAtomic
	}
	return nil
}

func (r *Rows[T]) rowsSave(ctx context.Context, parentKey string) error {
	i, b := r.backRef()
	if b == nil {
		return nil
	}
	for _, row := range r.gone {
		st, _ := row.attached()
		if d, ok := st.record.Interface().(interface{ Delete(context.Context) error }); ok {
			if err := d.Delete(ctx); err != nil {
				return err
			}
		}
	}
	r.gone = nil
	for _, row := range r.rows {
		st, form := row.attached()
		ref := reflect.ValueOf(&row.Value).Elem().Field(i)
		if u, ok := ref.Addr().Interface().(interface{ UnmarshalText([]byte) error }); ok && st.isNew() {
			if err := u.UnmarshalText([]byte(parentKey)); err != nil {
				return err
			}
		}
		st.submitting = true // checked with the form's own
		err := row.Save(ctx)
		st.submitting = false
		if err != nil {
			return err
		}
		_ = form
	}
	return nil
}

// rowsFailed is the path of the error a form holds while a row of it
// fails: no field's, so it never shows; the rows' errors do.
const rowsFailed = "\x00rows"

// rowField is a form type's Rows field.
type rowField struct {
	index    []int
	path     string
	label    string
	min, max int
}

// rowFields records a form type's Rows fields, with their @ui.Spec's Min
// and Max.
func (ft *formType) rowFields(t reflect.Type) {
	attrs := vuka.FieldAttrsOf(t)
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() || !reflect.PointerTo(sf.Type).Implements(rowSetType) {
			continue
		}
		rf := rowField{index: sf.Index, path: lowerCamel(sf.Name), label: words(sf.Name)}
		for _, a := range attrs[sf.Name] {
			switch x := a.(type) {
			case Spec:
				rf.min, rf.max = max(rf.min, x.Min), x.Max
				if x.Label != "" {
					rf.label = x.Label
				}
			case Label:
				rf.label = string(x)
			}
		}
		ft.rows = append(ft.rows, rf)
	}
}

// eachRows calls fn with each of the form's Rows fields, prepared.
func (st *formState) eachRows(form reflect.Value, fn func(rowField, rowSet)) {
	if st.ft == nil || len(st.ft.rows) == 0 {
		return
	}
	for _, rf := range st.ft.rows {
		v, err := form.FieldByIndexErr(rf.index)
		if err != nil || !v.CanAddr() {
			continue
		}
		rs := v.Addr().Interface().(rowSet)
		rs.rowsPrep(rf.path, rf.min, rf.max)
		fn(rf, rs)
	}
}

// checkRows checks the form's rows: each row's fields, then on a submit
// how many there are against Min and Max. It reports whether any failed.
func (st *formState) checkRows(ctx context.Context, form reflect.Value, live bool) (bool, error) {
	bad := false
	var failed error
	st.eachRows(form, func(rf rowField, rs rowSet) {
		if failed != nil {
			return
		}
		b, err := rs.rowsCheck(ctx, live, st.submitted)
		if err != nil {
			failed = err
			return
		}
		bad = bad || b
		if live {
			return
		}
		n := rs.rowsCount()
		for _, lim := range []struct {
			rule string
			n    int
			over bool
		}{{"min", rf.min, rf.min > 0 && n < rf.min}, {"max", rf.max, rf.max > 0 && n > rf.max}} {
			if !lim.over {
				continue
			}
			e := forms.Error{Rule: lim.rule, Params: map[string]string{"n": strconv.Itoa(lim.n), "kind": "rows"}}
			st.failed = append(st.failed, forms.Error{Key: forms.Text(messageCtx(ctx), e, rf.label)})
			bad = true
		}
	})
	return bad, failed
}

// rowsKey is the form's record's key as text, for its rows' back
// reference.
func (st *formState) rowsKey() string {
	if !st.record.IsValid() || st.ft.key == nil {
		return ""
	}
	k, err := st.record.Elem().FieldByIndexErr(st.ft.key)
	if err != nil {
		return ""
	}
	return valueText(k)
}

// saveRows saves the form's rows of a model, after its record.
func (st *formState) saveRows(ctx context.Context, form reflect.Value) error {
	var err error
	st.eachRows(form, func(_ rowField, rs rowSet) {
		if err == nil {
			err = rs.rowsSave(ctx, st.rowsKey())
		}
	})
	return err
}

// rowsTx is how the form's Save runs in one transaction with its rows: an
// orm.FK's FormAtomic; nil without rows of a model.
func (st *formState) rowsTx(form reflect.Value) func(context.Context, func(context.Context) error) error {
	var tx func(context.Context, func(context.Context) error) error
	st.eachRows(form, func(_ rowField, rs rowSet) {
		if tx == nil {
			tx = rs.rowsTx()
		}
	})
	return tx
}

func (rf rowField) String() string { return fmt.Sprintf("%s (%s)", rf.path, rf.label) }

// rowNames are the fields of a row a form edits: every field of T a form
// may edit but the back reference to the form's record (named skip).
func (r *Rows[T]) rowNames(skip string) []string {
	var out []string
	for _, f := range formTypeOf(reflect.TypeFor[FormFor[T]]()).fields {
		if f.fixed == "" && f.goPath != skip && !strings.Contains(f.goPath, ".") {
			out = append(out, f.goPath)
		}
	}
	return out
}
