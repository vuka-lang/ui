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
  `Live.PushPatch`/`PushNavigate`); client state the browser keeps and
  JSX reads (`State`, `Derived`, `Class`, `Show`, `Bind`, `BindAttr`);
  islands — client components hydrated in a server-rendered page
  (`Island`, `MountIsland`), and pages backed by one (`IslandPage`).
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

Documentation: https://vuka-lang.github.io/ui/

## Live patches without render tracking

JSX and `ui.Template` build frames: a component's markup splits into
statics, sent once per connection, and dynamics, of which a live session
sends only those that changed. A component built from nodes by hand
(`ui.El`, `ui.Text`, a `ui.NodeFunc`, templ without vuka's instrumentation)
has no frames, and needs nothing else: the session diffs its whole output —
a token patch of the markup when that is small, else the markup — and the
browser ends with the same page. The node type decides, so the tracked path
pays no check. On the demo pages' random-event tests a v2 connection
carries 1.7–2.2× the bytes without frames; one click in a 100-counter page
is 168 bytes instead of 63 (`go test -bench Fallback ./live`).

## Versions

From v0.50.0, vuka, ui, web and the `vuka` command are released together
under one version; require them at the same version (`vuka upgrade` moves an
app between versions). See
[Versions and compatibility](https://vuka-lang.github.io/project/versions).

The code moved here from github.com/vuka-lang/vuka at commit 1af97d1 (v0.8.0);
see that repository's history for its earlier changes.

MIT licensed.
