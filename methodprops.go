package ui

// MethodProp says what a stateful component's func prop was set to when its
// tag passes a method value (onCancel={p.Cancel}): the method Method bound to
// the receiver Recv. A Go method value is a new closure each time it's
// evaluated and can't be compared, so the compiler records this beside it; a
// live session counts two renders passing the same method on the same receiver
// as the prop being unchanged.
type MethodProp struct {
	Field  string // the prop's field
	Recv   any    // the receiver the method value is bound to
	Method string
}

// Method records that the prop field is recv's method named method. Compiled
// code calls it: Recv must be the receiver the value is really bound to.
func Method(field string, recv any, method string) MethodProp {
	return MethodProp{field, recv, method}
}

// MethodProps records on a component's tag which of its func props are method
// values, and returns the tag. Compiled code calls it:
//
//	ui.MethodProps(&OrderRow{Order: o, OnCancel: p.Cancel}, ui.Method("OnCancel", p, "Cancel"))
func MethodProps[T Stateful](c T, props ...MethodProp) T {
	if l := c.vukaLive(); l != nil {
		l.methods = props
	}
	return c
}

// MethodPropsOf is what MethodProps recorded on c. Package live calls it.
func MethodPropsOf(c Stateful) []MethodProp {
	if l := c.vukaLive(); l != nil {
		return l.methods
	}
	return nil
}

// SetMethodProps records props on an instance, from the tag that set its
// props. Package live calls it.
func SetMethodProps(c Stateful, props []MethodProp) {
	if l := c.vukaLive(); l != nil {
		l.methods = props
	}
}
