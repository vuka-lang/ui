package ui

import "unsafe"

// El and F allocate an element (a frame) with its attributes and children in
// one object, copied from the caller's slices, which then stay on the
// caller's stack. Larger ones take the slices as they come.

type elBox[A, K any] struct {
	e Element
	a A
	k K
}

func boxEl[A, K any](tag string, attrs []Attr, kids []Node) *Element {
	b := new(elBox[A, K])
	b.e.Tag = tag
	if len(attrs) > 0 {
		b.e.Attrs = unsafe.Slice((*Attr)(unsafe.Pointer(&b.a)), len(attrs))
		copy(b.e.Attrs, attrs)
	}
	if len(kids) > 0 {
		b.e.Children = unsafe.Slice((*Node)(unsafe.Pointer(&b.k)), len(kids))
		copy(b.e.Children, kids)
	}
	return &b.e
}

type frBox[K any] struct {
	f Frame
	k K
}

func boxFrame[K any](fp uint64, shape string, kids []Node) *Frame {
	b := new(frBox[K])
	b.f.FP, b.f.Shape = fp, shape
	if len(kids) > 0 {
		b.f.Kids = unsafe.Slice((*Node)(unsafe.Pointer(&b.k)), len(kids))
		copy(b.f.Kids, kids)
	}
	return &b.f
}

func newElement(tag string, attrs []Attr, kids []Node) *Element {
	if len(attrs) > 6 || len(kids) > 8 {
		return &Element{Tag: tag, Attrs: append([]Attr(nil), attrs...), Children: append([]Node(nil), kids...)}
	}
	switch len(attrs)<<4 | len(kids) {
	case 0<<4 | 0:
		return boxEl[[0]Attr, [0]Node](tag, attrs, kids)
	case 0<<4 | 1:
		return boxEl[[0]Attr, [1]Node](tag, attrs, kids)
	case 0<<4 | 2:
		return boxEl[[0]Attr, [2]Node](tag, attrs, kids)
	case 0<<4 | 3:
		return boxEl[[0]Attr, [3]Node](tag, attrs, kids)
	case 0<<4 | 4:
		return boxEl[[0]Attr, [4]Node](tag, attrs, kids)
	case 0<<4 | 5:
		return boxEl[[0]Attr, [5]Node](tag, attrs, kids)
	case 0<<4 | 6:
		return boxEl[[0]Attr, [6]Node](tag, attrs, kids)
	case 0<<4 | 7:
		return boxEl[[0]Attr, [7]Node](tag, attrs, kids)
	case 0<<4 | 8:
		return boxEl[[0]Attr, [8]Node](tag, attrs, kids)
	case 1<<4 | 0:
		return boxEl[[1]Attr, [0]Node](tag, attrs, kids)
	case 1<<4 | 1:
		return boxEl[[1]Attr, [1]Node](tag, attrs, kids)
	case 1<<4 | 2:
		return boxEl[[1]Attr, [2]Node](tag, attrs, kids)
	case 1<<4 | 3:
		return boxEl[[1]Attr, [3]Node](tag, attrs, kids)
	case 1<<4 | 4:
		return boxEl[[1]Attr, [4]Node](tag, attrs, kids)
	case 1<<4 | 5:
		return boxEl[[1]Attr, [5]Node](tag, attrs, kids)
	case 1<<4 | 6:
		return boxEl[[1]Attr, [6]Node](tag, attrs, kids)
	case 1<<4 | 7:
		return boxEl[[1]Attr, [7]Node](tag, attrs, kids)
	case 1<<4 | 8:
		return boxEl[[1]Attr, [8]Node](tag, attrs, kids)
	case 2<<4 | 0:
		return boxEl[[2]Attr, [0]Node](tag, attrs, kids)
	case 2<<4 | 1:
		return boxEl[[2]Attr, [1]Node](tag, attrs, kids)
	case 2<<4 | 2:
		return boxEl[[2]Attr, [2]Node](tag, attrs, kids)
	case 2<<4 | 3:
		return boxEl[[2]Attr, [3]Node](tag, attrs, kids)
	case 2<<4 | 4:
		return boxEl[[2]Attr, [4]Node](tag, attrs, kids)
	case 2<<4 | 5:
		return boxEl[[2]Attr, [5]Node](tag, attrs, kids)
	case 2<<4 | 6:
		return boxEl[[2]Attr, [6]Node](tag, attrs, kids)
	case 2<<4 | 7:
		return boxEl[[2]Attr, [7]Node](tag, attrs, kids)
	case 2<<4 | 8:
		return boxEl[[2]Attr, [8]Node](tag, attrs, kids)
	case 3<<4 | 0:
		return boxEl[[3]Attr, [0]Node](tag, attrs, kids)
	case 3<<4 | 1:
		return boxEl[[3]Attr, [1]Node](tag, attrs, kids)
	case 3<<4 | 2:
		return boxEl[[3]Attr, [2]Node](tag, attrs, kids)
	case 3<<4 | 3:
		return boxEl[[3]Attr, [3]Node](tag, attrs, kids)
	case 3<<4 | 4:
		return boxEl[[3]Attr, [4]Node](tag, attrs, kids)
	case 3<<4 | 5:
		return boxEl[[3]Attr, [5]Node](tag, attrs, kids)
	case 3<<4 | 6:
		return boxEl[[3]Attr, [6]Node](tag, attrs, kids)
	case 3<<4 | 7:
		return boxEl[[3]Attr, [7]Node](tag, attrs, kids)
	case 3<<4 | 8:
		return boxEl[[3]Attr, [8]Node](tag, attrs, kids)
	case 4<<4 | 0:
		return boxEl[[4]Attr, [0]Node](tag, attrs, kids)
	case 4<<4 | 1:
		return boxEl[[4]Attr, [1]Node](tag, attrs, kids)
	case 4<<4 | 2:
		return boxEl[[4]Attr, [2]Node](tag, attrs, kids)
	case 4<<4 | 3:
		return boxEl[[4]Attr, [3]Node](tag, attrs, kids)
	case 4<<4 | 4:
		return boxEl[[4]Attr, [4]Node](tag, attrs, kids)
	case 4<<4 | 5:
		return boxEl[[4]Attr, [5]Node](tag, attrs, kids)
	case 4<<4 | 6:
		return boxEl[[4]Attr, [6]Node](tag, attrs, kids)
	case 4<<4 | 7:
		return boxEl[[4]Attr, [7]Node](tag, attrs, kids)
	case 4<<4 | 8:
		return boxEl[[4]Attr, [8]Node](tag, attrs, kids)
	case 5<<4 | 0:
		return boxEl[[5]Attr, [0]Node](tag, attrs, kids)
	case 5<<4 | 1:
		return boxEl[[5]Attr, [1]Node](tag, attrs, kids)
	case 5<<4 | 2:
		return boxEl[[5]Attr, [2]Node](tag, attrs, kids)
	case 5<<4 | 3:
		return boxEl[[5]Attr, [3]Node](tag, attrs, kids)
	case 5<<4 | 4:
		return boxEl[[5]Attr, [4]Node](tag, attrs, kids)
	case 5<<4 | 5:
		return boxEl[[5]Attr, [5]Node](tag, attrs, kids)
	case 5<<4 | 6:
		return boxEl[[5]Attr, [6]Node](tag, attrs, kids)
	case 5<<4 | 7:
		return boxEl[[5]Attr, [7]Node](tag, attrs, kids)
	case 5<<4 | 8:
		return boxEl[[5]Attr, [8]Node](tag, attrs, kids)
	case 6<<4 | 0:
		return boxEl[[6]Attr, [0]Node](tag, attrs, kids)
	case 6<<4 | 1:
		return boxEl[[6]Attr, [1]Node](tag, attrs, kids)
	case 6<<4 | 2:
		return boxEl[[6]Attr, [2]Node](tag, attrs, kids)
	case 6<<4 | 3:
		return boxEl[[6]Attr, [3]Node](tag, attrs, kids)
	case 6<<4 | 4:
		return boxEl[[6]Attr, [4]Node](tag, attrs, kids)
	case 6<<4 | 5:
		return boxEl[[6]Attr, [5]Node](tag, attrs, kids)
	case 6<<4 | 6:
		return boxEl[[6]Attr, [6]Node](tag, attrs, kids)
	case 6<<4 | 7:
		return boxEl[[6]Attr, [7]Node](tag, attrs, kids)
	case 6<<4 | 8:
		return boxEl[[6]Attr, [8]Node](tag, attrs, kids)
	}
	return &Element{Tag: tag, Attrs: append([]Attr(nil), attrs...), Children: append([]Node(nil), kids...)}
}

func newFrame(fp uint64, shape string, kids []Node) *Frame {
	if len(kids) > 8 {
		return &Frame{FP: fp, Shape: shape, Kids: append([]Node(nil), kids...)}
	}
	switch len(kids) {
	case 0:
		return boxFrame[[0]Node](fp, shape, kids)
	case 1:
		return boxFrame[[1]Node](fp, shape, kids)
	case 2:
		return boxFrame[[2]Node](fp, shape, kids)
	case 3:
		return boxFrame[[3]Node](fp, shape, kids)
	case 4:
		return boxFrame[[4]Node](fp, shape, kids)
	case 5:
		return boxFrame[[5]Node](fp, shape, kids)
	case 6:
		return boxFrame[[6]Node](fp, shape, kids)
	case 7:
		return boxFrame[[7]Node](fp, shape, kids)
	case 8:
		return boxFrame[[8]Node](fp, shape, kids)
	}
	return &Frame{FP: fp, Shape: shape, Kids: append([]Node(nil), kids...)}
}
