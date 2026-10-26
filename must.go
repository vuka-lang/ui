package ui

// Must is a call in markup returning (T, error), as markup takes it: its
// T, or — when err isn't nil — a failed render. The compiler writes it
// around such a call where markup takes one value:
//
//	<span>{s.Users()}</span>   →   ui.HTML(`<span>{ui.Must(s.Users())}</span>`, ui.Must(s.Users()))
//
// The render fails with err as a Load or Mount error fails it: rendering
// a Node returns err (a page's status from it, its message shown only
// in development), and nothing the render wrote is sent. Must panics to
// leave the markup; every render this package and web run, and a live
// session's, stops the panic and returns err (Catch).
func Must[T any](v T, err error) T {
	if err != nil {
		panic(mustFailure{err})
	}
	return v
}

// mustFailure is Must's panic.
type mustFailure struct{ err error }

// Catch, deferred, stops a panic of Must and sets *err to Must's error.
// Any other panic goes on.
//
//	func render(ctx context.Context, n ui.Node, w io.Writer) (err error) {
//		defer ui.Catch(&err)
//		return n.Render(ctx, w)
//	}
func Catch(err *error) {
	if r := recover(); r != nil {
		f, ok := r.(mustFailure)
		if !ok {
			panic(r)
		}
		*err = f.err
	}
}

// Failed reports whether p, a value recover returned, is a panic of Must,
// and its error: for a recover that handles other panics too.
func Failed(p any) (error, bool) {
	f, ok := p.(mustFailure)
	return f.err, ok
}
