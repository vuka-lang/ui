package ui

import (
	"context"
	"hash/maphash"
	"io"
	"math"
	"reflect"
	"strconv"
)

// TemporaryProps are what Temporary takes.
type TemporaryProps struct {
	// Of is the value the content shows: the field holding the rows. The
	// content is sent again when Of is another value than at the last
	// send — see Temporary.
	Of       any
	Children Node
}

// Temporary is content the browser keeps and the server forgets: a live
// page sends it, then keeps nothing of its rendered form, so a large
// one-shot render — a report's two thousand rows — costs the connection
// nothing once it is on screen.
//
//	<ui.Temporary of={Rows}>
//	  <table>
//	    for _, r := range Rows {
//	      <tr><td>{r.Name}</td><td>{r.Total}</td></tr>
//	    }
//	  </table>
//	</ui.Temporary>
//
// On a live connection the content renders and is sent whole the first
// time, and whenever Of is another value than at the last send; then the
// session drops it from the render it keeps (the instance's tree and the
// shadow it diffs against), as it drops a stream's rows. Each later render
// of the page leaves the content as the browser has it: the region isn't
// compared, and isn't in the patch. Another value is a set of the field
// that changes what it holds: a new slice, map or pointer, a slice of
// another length, a different string, number or bool (a struct or array
// compares its fields). Changing the elements of the same slice in place
// is not one — assign a new slice, or pass a version the page bumps as Of.
// When the browser has to be sent the region's surroundings whole (the
// page joins or remounts, an element around it changes its markup), the
// content is sent with them, as it is.
//
// The field's Go value stays: the page's methods may read it. A page done
// with the data may clear the field itself; the browser keeps what it was
// sent until Of changes again. The content still renders on each render of
// its instance, so its handlers stay registered, and HTML renders (a page's
// first response, a page without a live connection) render it as any
// content. Session.HTML no longer holds it once it is forgotten. The
// content is one region of the tree: place it inside an element of the
// component, not as the component's root.
func Temporary(p TemporaryProps) Node {
	return &tempRegion{of: p.Of, n: p.Children}
}

// tempRegion is Temporary's node.
type tempRegion struct {
	of any
	n  Node
}

func (n *tempRegion) Render(ctx context.Context, w io.Writer) error { return renderHTML(ctx, w, n) }

// View is the content: every renderer but the live tree's sees only it.
func (n *tempRegion) View() Node {
	if n == nil {
		return nil
	}
	return n.n
}

// tempHost is a live host told its render has a Temporary region.
type tempHost interface{ TempRegion() }

// tempTree is the region in a render tree: a list of the content's frames
// marked Temp with what it shows.
func (b *treeBuilder) tempTree(ctx context.Context, n *tempRegion) (*TreeList, error) {
	if h, ok := liveHost(ctx).(tempHost); ok {
		h.TempRegion()
	}
	v, err := b.value(ctx, n.n)
	if err != nil {
		return nil, err
	}
	l := &TreeList{Temp: tempStamp(n.of)}
	switch v := v.(type) {
	case *Tree:
		l.Items = []*Tree{v}
	case *TreeList:
		l.Items = v.Items
	case nil:
	default:
		if s, ok := v.(string); ok && s == "" {
			break
		}
		l.Items = []*Tree{OpaqueTree(v)}
	}
	return l, nil
}

var tempSeed = maphash.MakeSeed()

// tempStamp names the value v is, as Temporary compares it: never "".
func tempStamp(v any) string {
	var h maphash.Hash
	h.SetSeed(tempSeed)
	stamp(&h, reflect.ValueOf(v), 0)
	return "t" + strconv.FormatUint(h.Sum64(), 36)
}

func stamp(h *maphash.Hash, v reflect.Value, depth int) {
	if !v.IsValid() {
		h.WriteByte(0)
		return
	}
	writeUint := func(u uint64) {
		var b [8]byte
		for i := range b {
			b[i] = byte(u >> (8 * i))
		}
		h.Write(b[:])
	}
	h.WriteString(v.Type().String())
	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			h.WriteByte(1)
		} else {
			h.WriteByte(0)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		writeUint(uint64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		writeUint(v.Uint())
	case reflect.Float32, reflect.Float64:
		writeUint(math.Float64bits(v.Float()))
	case reflect.Complex64, reflect.Complex128:
		writeUint(math.Float64bits(real(v.Complex())))
		writeUint(math.Float64bits(imag(v.Complex())))
	case reflect.String:
		h.WriteString(v.String())
	case reflect.Slice:
		writeUint(uint64(uintptr(v.UnsafePointer())))
		writeUint(uint64(v.Len()))
		writeUint(uint64(v.Cap()))
	case reflect.Map, reflect.Pointer, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		writeUint(uint64(v.Pointer()))
	case reflect.Interface:
		stamp(h, v.Elem(), depth+1)
	case reflect.Struct:
		if depth > 8 {
			return
		}
		for i := range v.NumField() {
			stamp(h, v.Field(i), depth+1)
		}
	case reflect.Array:
		if depth > 8 {
			return
		}
		writeUint(uint64(v.Len()))
		for i := range min(v.Len(), 64) {
			stamp(h, v.Index(i), depth+1)
		}
	}
}
