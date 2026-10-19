# github.com/vuka-lang/ui

The UI library for [Vuka](https://github.com/vuka-lang/vuka): what JSX in a
`.vuka` file renders with. A file that imports `github.com/vuka-lang/ui` is a
JSX file whose elements become `ui.El`, `ui.Text`, `ui.Child`, … (the package
declares `const VukaJSX = 3`, Vuka's JSX target contract).

- `ui` — `Node` (= `templ.Component`), markup (`HTML`, `Each`, `If`), elements and text, `Safe`, `Style`,
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

## Markup in plain Go: `ui.HTML`

`ui.HTML` is markup for `.go` files: HTML in backticks whose holes are
labelled with the Go of their values, the values after it in the order the
labels first appear. It is parsed once and renders like JSX — the static HTML
is written straight through, and under a live session it is a frame of
statics and dynamics that diffs as JSX does.

```go
func (p *Orders) row(o Order) ui.Node {
	cancel := func() { p.Cancel(o.ID) }
	return ui.HTML(`
<tr key={o.ID} class="row" class:late={o.Late}>
  <td>{o.Ref}</td>
  <td class="tabular-nums">{money(o.Total)}</td>
  <td><button onClick={cancel}>Cancel</button></td>
</tr>`, o.ID, o.Late, o.Ref, money(o.Total), cancel)
}

func (p *Orders) Render() ui.Node {
	rows := ui.Each(p.orders, p.row)
	return ui.HTML(`<table><tbody>{rows}</tbody></table>`, rows)
}
```

A label appearing twice is one value; vukavet checks each value is written as its label. Each hole is escaped for its place,
and a value of the wrong kind fails the render with the file, line and
hole:

| Place | Takes |
|---|---|
| a child, `<td>{x}</td>` | a `ui.Node`, `[]ui.Node`, string or number (escaped), error, nil, a `ui.State` (kept current), a stateful component (`&Counter{…}`, `ui.Key(k, &Counter{…})`), an island (`Chart{…}`, `ui.Mount(Chart{…}, ui.Idle())`) |
| text, `<title>{x}</title>` (title, textarea) | a string or number |
| a value, `class={x}` | what a JSX attribute takes: string, number, bool, nil, `ui.Class` / `ui.Classes` / `ui.Style` on class / style, a `ui.State`; an `on…` attribute takes a handler — a func, `ui.On(…)`, JS commands |
| part of a value, `href="/orders/{id}"` | a string or number |
| a toggled class, `class:late={on}` | a bool or a `ui.State[bool]` |
| an attribute set, `<input {attrs...}>` | `ui.Attr`, `[]ui.Attr`, `templ.Attributes` (`ui.BindAttr` gives one) |

- Loops and conditionals are Go: `ui.Each(items, fn)`, `ui.If(cond, node)`,
  or a `[]ui.Node` built in a loop, given to a hole. Their items diff by key:
  a `key` attribute on an item's first element, or `ui.Key` on a component.
- Client state: `open := ui.UseState(false)` in a component,
  `var theme = ui.UsePageState("light")` for the page.
- `{{` is a literal brace and `{/* … */}` a comment; in script and style,
  braces are text.
- The markup is strict — every element closed (void elements and `<x/>`
  aside), holes only where the table says — and a mistake fails the render
  with its line. Text follows JSX's whitespace rule, so indentation adds
  nothing; pre and textarea keep theirs.

With the `vuka` command, markup goes in `.vuka.html` files beside the Go —
templates with Go statements, compiled to the same calls.

## Versions

From v0.50.0, vuka, ui, web and the `vuka` command are released together
under one version; require them at the same version (`vuka upgrade` moves an
app between versions). See
[Versions and compatibility](https://vuka-lang.github.io/project/versions).

The code moved here from github.com/vuka-lang/vuka at commit 1af97d1 (v0.8.0);
see that repository's history for its earlier changes.

MIT licensed.
