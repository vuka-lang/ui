# github.com/vuka-lang/ui

The UI library for [Vuka](https://github.com/vuka-lang/vuka): what JSX in a
`.vuka` file renders with. A file that imports `github.com/vuka-lang/ui` is a
JSX file whose elements become `ui.El`, `ui.Text`, `ui.Child`, … (the package
declares `const VukaJSX = 1`, Vuka's JSX target contract).

- `ui` — `Node` (= `templ.Component`), elements and text, `Safe`, `Style`,
  `ErrorBoundary`, HTML rendering (`String`, `Write`, `Handler`), `Walk` and
  `Renderer`; stateful components (`Live`, `On`, `Event`); JS commands the
  browser runs on an event (`JS`, `Ref`, `Live.PushJS`), and a payload for a
  component's scripts (`Live.PushEvent`); links a live page
  follows over its connection (`Link`, `JS.Patch`/`Navigate`,
  `Live.PushPatch`/`PushNavigate`).
- `ui/live` — sessions that keep stateful components alive for a connected
  browser, and the wire protocol transports speak.
- `ui/term` — renders a node tree as terminal text.
- `ui/templx` — templ interop: children for templ components, the
  components of `.templ` files a `vuka.File` names.

```vuka
import "github.com/vuka-lang/ui"

func Hello(name string) ui.Node {
	return <p>Hello, {name}!</p>
}
```

Documentation: https://vuka-lang.github.io/vuka/ui/

The code moved here from github.com/vuka-lang/vuka at commit 1af97d1 (v0.8.0);
see that repository's history for its earlier changes.

MIT licensed.
