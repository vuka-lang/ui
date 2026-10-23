# github.com/vuka-lang/ui/v2

The UI library for [Vuka](https://github.com/vuka-lang/vuka): what the
markup of a `.vuka.html` file renders with. A `.vuka.html` file is one
component: its `<script lang="go">` declares the struct, the markup below it
is the struct's view, and the compiler lowers it to calls of this package (which
declares `const VukaJSX = 3`, Vuka's markup target contract).

- `ui` — `Node` (= `templ.Component`), markup (`HTML`, `Each`, `If`), elements and text, `Safe`, `Style`,
  `ErrorBoundary`, HTML rendering (`String`, `Write`, `Handler`), `Walk` and
  `Renderer`; stateful components (`Live`, `On`, `Event`); JS commands the
  browser runs on an event (`JS`, `Ref`, `Live.PushJS`), and a payload for a
  component's scripts (`Live.PushEvent`); links a live page
  follows over its connection (`Link`, `JS.Patch`/`Navigate`,
  `Live.PushPatch`/`PushNavigate`); client state the browser keeps and
  markup reads (`State`, `Derived`, `Class`, `Show`, `Bind`, `BindAttr`);
  islands — client components hydrated in a server-rendered page
  (`Island`, `MountIsland`), and pages backed by one (`IslandPage`).
- `ui/live` — sessions that keep stateful components alive for a connected
  browser, and the wire protocol transports speak.
- `ui/term` — renders a node tree as terminal text.
- `ui/templx` — templ interop: children for templ components, the
  components of `.templ` files a `vuka.File` names.

```html
<!-- hello.vuka.html -->
package greet

type Hello struct {
	Name string
}
<p>Hello, {Name}!</p>
```

Documentation: https://vuka-lang.github.io/ui/

## What markup compiles to: `ui.HTML`

The compiler writes a `.vuka.html` file's markup as `ui.HTML` calls: HTML in
backticks whose holes are labelled with the Go of their values, the values
after it in the order the labels first appear. It is parsed once — the
static HTML is written straight through, and under a live session it is a
frame of statics and dynamics that diffs by them. `ui.HTML` is compiler
output; vukavet reports a hand-written call.

Each hole is escaped for its place, and a value of the wrong kind fails the
render with the file, line and hole:

| Place | Takes |
|---|---|
| a child, `<td>{x}</td>` | a `ui.Node`, `[]ui.Node`, string or number (escaped), error, nil, a `ui.State` (kept current), a stateful component (`&Counter{…}`, `ui.Key(k, &Counter{…})`), an island (`Chart{…}`, `ui.Mount(Chart{…}, ui.Idle())`) |
| text, `<title>{x}</title>` (title, textarea) | a string or number |
| a value, `class={x}` | string, number, bool, nil, `ui.Class` / `ui.Classes` / `ui.Style` on class / style, a `ui.State`; an `on…` attribute takes a handler — a func, `ui.On(…)`, JS commands |
| part of a value, `href="/orders/{id}"` | a string or number |
| a toggled class, `class:late={on}` | a bool or a `ui.State[bool]` |
| an attribute set, `<input {attrs...}>` | `ui.Attr`, `[]ui.Attr`, `templ.Attributes` (`ui.BindAttr` gives one) |

- Loops and conditionals are Go statements in the markup (`for`, `if`,
  `switch`). Their items diff by key: a `key` attribute on an item's first
  element, or on a component's tag.
- Client state: a `ui.State[T]` field of a `.vuka.html` file's struct, or a
  package-level `var theme ui.State[string]` for the page; markup reads it
  as a plain variable and an `on…` attribute writes it (`open = !open`).
  The compiler lowers both to package `uigen`, which is its output, not an
  API to write by hand.
- `{{` is a literal brace and `{/* … */}` a comment; in script and style,
  braces are text.
- The markup is strict — every element closed (void elements and `<x/>`
  aside), holes only where the table says — and a mistake fails the render
  with its line. Text is trimmed where it meets a line break, so
  indentation adds nothing; pre and textarea keep theirs.

## Components

The kit — `Button`, `Submit`, `Badge`, `Card`, `Alert`, `Modal`, `Tabs`,
`Table`, `DataTable`, `Form` and its fields (`Field`, `Input`, `Password`,
`Textarea`, `Select`, `Checkbox`), `Image`, `Tile`, `Toast` and
`ToastContainer`, with the partials `Actions`, `Cancel`, `Icon`, `Spinner` and
`TileGrid` — is styled by the `--ui-*` tokens `StyleTag` puts in a page. Each
component is a struct whose fields are its props, and itself a `ui.Node`
(a `templ.Component`). In markup it is a tag, its
attributes camelCase, its children the `Children` field and
`<slot:x>` the `X` field:

```html
<ui.Card title={ui.Text("Members")} noPad>
  <slot:header><ui.Button variant={ui.Primary} on={Invite}>Invite</ui.Button></slot:header>
  <ui.Table columns={cols} rows={Members} rowKey={func(m Member) any { return m.ID }}/>
</ui.Card>
```

In Go it is the struct literal, given to a hole, returned or passed as it
is:

```go
ui.Card{Title: ui.Text("Members"), NoPad: true, Children: table}
```

The kit is written the way an app's components are: `card.vuka.html` declares
the `Card` struct in its `<script lang="go">`, above its markup, which the compiler makes its `View`
method, adding the templ method, `Render(ctx, w)`, that renders it. HTML
rendering and live sessions read the `View` itself, so a component renders,
diffs and keys as its markup does.

```html
<!-- card.vuka.html -->
<script lang="go">
package ui

type Card struct {
	Title    Node
	Header   Node
	NoPad    bool
	Class    string
	Children Node
}
</script>

<div class={withClass("vk-card", Class)}>
  if Title != nil || Header != nil {
    <div class="vk-card__header">
      <div class="vk-card__title">{Title}</div>
      if Header != nil {
        <div class="vk-card__header-actions">{Header}</div>
      }
    </div>
  }
  <div class="vk-card__body" class:vk-card__body--flush={NoPad}>{Children}</div>
</div>
```

Markup reads the struct's fields and methods by bare name (`{Title}`), or
through the receiver its methods name (`{c.Title}`). A field shadows a
package name it shares — `Icon`, `Value`, `Class`, `Ref`, `On`, `Page` in
package ui — so a component with an `Icon` field calls the `Icon` func from
a method in its script.

The former props types are aliases of the components, deprecated:
`ui.CardProps` is `ui.Card`, and so on for `AlertProps`, `BadgeProps`,
`ButtonProps`, `DataTableProps[T]`, `FieldProps`, `FormProps`, `ImageProps`,
`ModalProps`, `TableProps[T]`, `TabsProps`, `TileProps`,
`ToastContainerProps` and `ToastProps`. So a call of the former function,
`ui.Card(ui.CardProps{…})`, still compiles — a conversion of the literal to
the component — except for the generic ones, `ui.Table[Row]{…}`; `vuka fix
ui-structs` writes them all as literals. Tags are unchanged, with a `vuka`
command that lowers a tag naming a struct component to the literal.

## Working on ui

The `.vuka.html` files are compiled to the `x_html.go` beside them by the
`vuka` command, and that Go is committed, so the module builds with plain
`go build` and `go test`. After changing a template, regenerate:

```sh
go generate ./...
```

The command is pinned in `tools.mod`, a development-only module file (with
`vukavet`): `go tool -modfile=tools.mod vuka gen -inplace ./...`. CI runs
`go generate` and fails when the committed Go differs. A change needing a
transpiler change vuka hasn't merged pins that vuka commit instead, and
moves back to vuka's main once it lands:

```sh
c=<commit>
go get -modfile=tools.mod github.com/vuka-lang/vuka/v2@$c \
	github.com/vuka-lang/vuka/cmd/vuka/v2@$c github.com/vuka-lang/vuka/cmd/vukavet/v2@$c
```

## Versions

From v0.50.0, vuka, ui, web and the `vuka` command are released together
under one version; require them at the same version (`vuka upgrade` moves an
app between versions). See
[Versions and compatibility](https://vuka-lang.github.io/project/versions).

The code moved here from github.com/vuka-lang/vuka/v2 at commit 1af97d1 (v0.8.0);
see that repository's history for its earlier changes.

MIT licensed.
