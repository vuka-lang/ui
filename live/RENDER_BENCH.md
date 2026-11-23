# Render benchmarks: where the time goes

A measurement pass over the live render path. It changes no engine behaviour:
it adds benchmarks (`render_bench_test.go`), the pages they render
(`internal/benchpages`, `.vuka.html` compiled by the compiler `tools.mod`
pins), and a test-only render counter (`live.Renders`, `export_test.go`).
What follows is what they measured and what the profiles say about it.

## The pages

| bench | page | what one op is |
|---|---|---|
| `Table/rows=N/http` | `UsersPage`: N `UserRow` components, keyed, five props (two of them method props), a `ui.Badge` and an actions menu of three `ui.ActionMenu` items | `NewSession` + `Render`: the page's first HTTP response |
| `Table/rows=N/join` | the same | `NewSession` + a v2 join (`RenderTrees`): what the socket's join sends |
| `Table/rows=N/event` | the same, joined | one row's Promote: the page's state changes, the page renders and places every row, one row's role changes |
| `Table/rows=N/event-plain` | rows are `PlainUserRow`: the same markup, its menu items plain `<button>`s | the same event |
| `Wide/tiles=M/own` | `GridPage`: M `OwnTile`s, each keeping its count | one tile's click: its own state, the tile renders alone |
| `Wide/tiles=M/props` | M `Tile`s, the count the page's, passed as a prop | one tile's click: the page renders and places all M |
| `Wide/tiles=M/slice` | `props`, plus a `[]string` prop every tile is passed | the same click |
| `Modal/http`, `Modal/join` | `EditPage`: 20 users, each with an Edit button opening a server-owned `ui.Modal` holding a five-field `ui.FormState` form (`<ui.Form for>`) | the first render |
| `Modal/open-close` | the same, joined | one event, alternately open a user's dialog and close it (each open another user) |
| `Broadcast/conns=K` | K joined sessions of `FeedPage` (100 `PlainUserRow`s, subscribed to `"users"`) | one `RoleChanged` delivered to all K: each session's `InfoBatch` and the patch's JSON (`Marshal(PatchMessage)`) — what each connection's goroutine does in vuka-web's `conn.broadcasts`, run one after another here, so ns/op is the CPU one Broadcast costs the process, socket writes aside |

Every event bench changes something real on every op (a role cycles, a count
increments, a dialog opens with a different user), so no op is a no-op patch.
The pages are compiled once by `go generate` (the compiler is never timed);
`live.Verify` is off in the benchmarks, as in production.

`TestRenderBenchPages` runs in the normal `go test` run: it renders each
fixture and sends one event of each kind, with verification on, and checks the
reply carries the change — so the harness can't rot silently. The benchmarks
themselves only run under `-bench`.

## Running them steadily

```sh
cd live
go test -c -o /tmp/live.test .
GOMAXPROCS=4 /tmp/live.test -test.run '^$' -test.bench BenchmarkRender \
    -test.benchtime 1s -test.count 5 | tee new.txt
```

- Pin `GOMAXPROCS` (4 here): GC runs on the spare Ps, so the wall time
  depends on how many there are. Use `GOMAXPROCS=1` to see the GC's full cost
  in ns/op (below).
- `-count 5`, compare medians (`benchstat`). The small benches (under
  ~50 µs) need `-benchtime 2s` to settle on a shared machine.
- The 1000-row and conns=100 benches run 40–60 ops a second; `-benchtime 1s`
  is enough. `B/op` and `allocs/op` are deterministic to the byte and the
  best signal for a change in the render path.
- Extra metrics: `B/patch` (the reply's bytes; `B/open`/`B/close` for the
  modal), `renders/op` (Render calls an op executed, the page's own render
  function included, from `live.Renders`), `gcs/op` (GC cycles per op).

## Numbers

Apple M1 Pro, Go 1.27.3, `GOMAXPROCS=4`, median of 5 runs; "spread" is the
range of the middle three runs over the median. The machine was shared with
other agents' test suites (load 4–8 on 10 cores); every row was re-run until
its spread was under 10%.

| bench | µs/op | spread | B/op | allocs/op | wire | renders/op | gcs/op |
|---|---:|---:|---:|---:|---|---:|---:|
| Table/rows=10/http | 155.1 | 6.4% | 222 KiB | 1,657 | 17.6 KB html | | |
| Table/rows=10/join | 250.2 | 2.8% | 211 KiB | 2,675 | 10.0 KB join | | |
| Table/rows=10/event | 191.1 | 6.5% | 146 KiB | 2,431 | 149 B/patch | 12 | 0.06 |
| Table/rows=10/event-plain | 39.0 | 4.4% | 25 KiB | 414 | 149 B/patch | 3 | 0.01 |
| Table/rows=100/http | 1,599 | 1.1% | 2.3 MiB | 15,757 | 175 KB html | | |
| Table/rows=100/join | 3,629 | 8.3% | 2.2 MiB | 25,529 | 88 KB join | | |
| Table/rows=100/event | 1,939 | 5.5% | 1.4 MiB | 23,360 | 149 B/patch | 102 | 0.75 |
| Table/rows=100/event-plain | 231.0 | 0.3% | 147 KiB | 2,158 | 149 B/patch | 3 | 0.07 |
| Table/rows=1000/http | 17,889 | 5.1% | 24.2 MiB | 164,944 | 1.75 MB html | | |
| Table/rows=1000/join | 27,993 | 1.1% | 22.8 MiB | 262,918 | 879 KB join | | |
| Table/rows=1000/event | 19,803 | 1.7% | 13.7 MiB | 239,999 | 150 B/patch | 1002 | 3.5 |
| Table/rows=1000/event-plain | 2,442 | 0.5% | 1.3 MiB | 21,844 | 150 B/patch | 3 | 0.40 |
| Wide/tiles=50/own | 20.3 | 0.1% | 15 KiB | 193 | 64 B/patch | 2 | 0.01 |
| Wide/tiles=50/props | 104.6 | 0.4% | 70 KiB | 1,093 | 63 B/patch | 3 | 0.03 |
| Wide/tiles=50/slice | 307.3 | 1.1% | 273 KiB | 6,071 | 63 B/patch | 52 | 0.11 |
| Wide/tiles=500/own | 152.5 | 1.7% | 101 KiB | 667 | 63 B/patch | 2 | 0.06 |
| Wide/tiles=500/props | 1,058 | 0.3% | 643 KiB | 10,672 | 63 B/patch | 3 | 0.39 |
| Wide/tiles=500/slice | 3,404 | 2.0% | 2.6 MiB | 60,622 | 63 B/patch | 502 | 1.57 |
| Modal/http | 93.6 | 7.8% | 87 KiB | 1,363 | 4.6 KB html | | |
| Modal/join | 142.4 | 2.9% | 138 KiB | 1,642 | 4.6 KB join | | |
| Modal/open-close | 153.4 | 2.2% | 140 KiB | 1,706 | 3,181 B/open, 66 B/close | 2 | 0.08 |
| Broadcast/conns=10 | 2,516 | 0.4% | 1.4 MiB | 21,525 | 149 B/patch × 10 | 30 | 0.35 |
| Broadcast/conns=100 | 26,994 | 1.5% | 14.3 MiB | 215,202 | 149 B/patch × 100 | 300 | 0.44 |

Cold path against the socket: a v2 join costs 1.5–1.7× the HTTP render of the
same page (27.9 ms against 17.9 ms at 1000 rows), and a page's socket
re-render after an event costs about what its HTTP render does when the rows
can't be skipped (19.8 ms against 17.9 ms), a seventh of it when they can
(2.4 ms).

GC share, `GOMAXPROCS=1`, GOGC=100 against GOGC=off, mean of 3:

| bench | GOGC=100 | GOGC=off | GC's share |
|---|---:|---:|---:|
| Table/rows=1000/event | 24.5 ms | 16.7 ms | 32% |
| Table/rows=1000/event-plain | 2.74 ms | 2.09 ms | 24% |
| Broadcast/conns=100 | 33.2 ms | 24.5 ms | 26% |

## Profiles

CPU profiles at `GOMAXPROCS=1`, `-benchtime 10s`; allocation profiles at the
default rate, `-benchtime 2s`. Percentages are of the op's own work
(`-focus` on `Session.HandleJSON` or `Session.InfoBatch`,
`-relative_percentages`). On darwin the unfocused profile also charges ~35–45%
to `runtime.startTheWorldWithSema → kevent` and ~15–20% to `runtime.madvise`
(GC cycles and heap regrowth — the profiler's darwin attribution of them is
unreliable, so the GOGC table above, not these lines, is the measure of GC's
cost).

```sh
GOMAXPROCS=1 /tmp/live.test -test.run '^$' -test.bench 'BenchmarkRenderTable/rows=1000/event$' \
    -test.benchtime 10s -test.cpuprofile table.cpu
go tool pprof -top -cum -focus='Session\).HandleJSON' -relative_percentages /tmp/live.test table.cpu
```

### Table/rows=1000/event (kit rows) — top 10 cumulative

```
      flat  flat%   sum%        cum   cum%
         0     0%     0%      1.80s   100%  live.(*Session).HandleJSON
         0     0%     0%      1.71s 95.00%  live.(*Session).patch
         0     0%     0%      1.65s 91.67%  live.(*Session).renderAll
         0     0%  0.56%      1.63s 90.56%  live.(*scope).place          (each row: placed, then rendered)
     0.01s  0.56%  0.56%      1.63s 90.56%  ui.BuildTree
     0.01s  0.56%  0.56%      0.59s 32.78%  runtime.mallocgc
         0     0%  0.56%      0.32s 17.78%  ui.(*tmpl).frame
         0     0%  0.56%      0.31s 17.22%  runtime.gcAssistAlloc
         0     0%  0.56%      0.22s 12.22%  live.rooted / ui.RootAttrs
         0     0%  0.56%      0.21s 11.67%  benchpages.(*UserRow).Render
         0     0%  0.56%      0.16s  8.89%  ui.(*HTMLRenderer).attr
         0     0%  0.56%      0.09s  5.00%  ui.ResolveContext   ← ActionMenu.View.func1 (ui.Ctx)
```

Allocations (alloc_space, flat): `ui.(*tmpl).frame` 32.2%,
`ui.(*treeBuilder).frame` 21.0%, `treeBuilder.endDyn` 6.2%,
`UserRow.Render` 5.8%, `tmpl.render` 4.8%; `ui.(*byteBuf).WriteString`, the
only buffer write in the list, 2.2%.

### Broadcast/conns=100 — top 10 cumulative

```
      flat  flat%   sum%        cum   cum%
     0.02s  0.26%  0.26%      7.68s   100%  live.(*Session).InfoBatch
         0     0%  0.26%      6.69s 87.11%  live.(*Session).patch
         0     0%  0.26%      6.33s 82.42%  live.(*Session).renderAll
     0.02s  0.26%  0.65%      6.18s 80.47%  live.(*scope).place
         0     0%  0.26%      6.18s 80.47%  ui.BuildTree
     0.14s  1.82%  3.26%      2.27s 29.56%  runtime.mallocgc
     0.15s  1.95%  5.34%      1.06s 13.80%  live.sameProps
     0.07s  0.91%  6.25%      1.00s 13.02%  live.copyProps
     0.01s  0.13%           0.82s 10.68%  live.(*Session).beginURL   (urlParams 9.6%, tableParams 7.2%, maps.Clone 5.5%)
                            0.76s  9.90%  benchpages.(*FeedPage).Render
                            0.57s  7.42%  runtime.concatstrings      (place keys)
                            0.54s  7.03%  live.(*Session).order
                            0.53s  6.90%  live.(*instance).trackMethods
                            0.52s  6.77%  ui.IsDeferred              (sync.Map per field, from sameProps)
                            0.30s  3.91%  ui.FillAutoIDs
```

`Table/rows=1000/event-plain` profiles the same way: `place`'s own work —
`copyProps` 15.2%, `sameProps` 13.2% (`IsDeferred` 6.1% of it), place-key
string concatenation 9.6%, `FillAutoIDs` 5.1%, `trackMethods` 4.9% — is ~46%
of the event; `beginURL` 6.5%, `order` 5.5%, the page's own Render 10.3%.
Allocations: `place` 22.0% flat (all of it the place-key strings:
`place := sc.keys + n.Site + "#"`, `+= "k" + keyText(n.Key)`,
`at := parent.at + "/" + place`), `UsersPage.Render` 21.3% (the row props
structs and their method props), `treeBuilder.frame` 13.4%,
`order.func1` 10.7% (the instance slice, rebuilt by every walk).

The tree store and the wire diff are small everywhere: `treeStore.treeVs` +
`listVs` 2–4%, `treeUpdates`/`enc.*` ≤ 3.3%, `Marshal` 0%. No `maphash`,
`sig`, `treeHash` or `staticsKey` frame has a single sample in any of the
three profiles.

## Where the time goes, hypothesis by hypothesis

### (a) Unchanged component renders dominate event handling on wide pages — CONFIRMED, for two reasons that aren't the skip logic failing in general

When the skip works (`Wide/props`, `Table/event-plain`) an event executes 3
renders whatever the width — the skip logic is sound. Two realistic shapes
defeat it:

1. **A row using `ui.ActionMenu` renders on every render of its page.**
   ActionMenu reads its menu id from the context (`ui.Ctx`), `ResolveContext`
   calls the host's `ContextRead`, which sets `instance.contextual`, and
   `visit` never keeps a contextual instance. `Table/rows=1000/event` executes
   1002 renders for a one-row change: 19.8 ms against 2.4 ms for the same
   markup with plain buttons — 88% of the event is the 999 unchanged rows
   rendering again, and 10× the bytes and allocations. At 100 rows: 1.94 ms
   against 0.23 ms. `DataTable`, `formelement`, `formcomp` and `formref` use
   `ui.Ctx` the same way.
2. **A slice (or map, pointer, interface) prop is never "the same"**
   (`sameValue` is true for those only when both are nil): `Wide/slice`
   executes 502 renders at 500 tiles, ~3× the `props` variant.

Even with every unchanged component kept, *placing* them is most of a wide
page's event: `Wide/tiles=500/props` costs 6.9× `own` (1.06 ms against
0.15 ms) with the same 3 renders, and `place`'s own reflection and string work
is ~46% of `Table/rows=1000/event-plain`.

TODO(a1): ActionMenu's `Ctx` (`actionmenu_html.go:86`) makes its row
contextual — profile: `ui.ResolveContext ← ActionMenu.View.func1`, 1002
renders/op against 3.
TODO(a2): `place`'s per-child cost with the render skipped — profile:
`copyProps 15.2%, sameProps 13.2%, concatstrings 9.6%, FillAutoIDs 5.1%,
trackMethods 4.9%` of `Table/rows=1000/event-plain`.

### (b) maphash content signatures cost materially where versioned states already know the answer — REFUTED

Zero samples in `maphash`, `live.sig`, `treeHash`, `listHash`, `dynHash` or
`ui.staticsKey` across ~20 s of profiled events and broadcasts; the whole tree
store (`treeVs`, `listVs`, `letGoKids`) is 2–4%, and the wire diff ≤ 3.3%.
Hashing runs only over the frames a render built, and the renders that run are
few once (a) is fixed; the dirty marks already keep kept instances out of it.
No TODO.

### (c) Buffer/string allocations in the render path are a significant GC load — PARTLY CONFIRMED: GC is significant, buffers aren't its source

GC is 24–32% of the CPU of an event or a broadcast (the GOGC table), and the
1000-row kit event runs 3.5 GC cycles per op. But render buffers are not what
it collects: `byteBuf.WriteString` is 2.2% of the bytes, `strings.Builder`
nowhere. The bytes are tree frames (`tmpl.frame` + `treeBuilder.frame` +
`endDyn` ≈ 59% on the kit table), the props structs a parent's render builds
for every child (21%), and one kind of string: the place keys `place` concatenates for
every child on every render, kept or not (22% of `event-plain`'s bytes), with
`order()`'s instance slice (11%).

TODO(c): place keys rebuilt per child per render — profile: `place` 247 MB
flat of 1,122 MB on `event-plain`, `session.go:1311/1313/1320`.
TODO(c2): `order()` rebuilt by each walk (`beginURL` twice, `treeUpdates`,
`InfoBatch`) — profile: `order.func1` 10.7% of the bytes, `order` 5.5–7% of
the CPU.

### (d) Broadcast fan-out renders identical output K times — CONFIRMED

`Broadcast/conns=K` executes exactly 3 renders per connection (300 at K=100),
every connection's patch is the same 149 bytes, and the cost is linear:
0.25 ms per connection at K=10, 0.27 ms at K=100 — the cost of one
`event-plain`-style render of the 100-row page each time. The tree store makes
the K sessions share their trees' *memory*, not the render: 87% of each
delivery (`patch → renderAll`) is work every other connection repeats
identically; the per-connection part that must stay (the diff against what
that browser holds, the JSON) is ≤ 3%.

TODO(d): render a broadcast once per distinct page state — profile:
`InfoBatch → patch → renderAll` 82.4% of each of the K deliveries, 300
renders/op for one Broadcast.

### (e) What the hypotheses missed

- **`beginURL` on pages with no URL state**: every event and broadcast walks
  the instances three times (`urlParams`, `tableParams`, the snapshot loop)
  and clones each instance's table keys (`maps.Clone`), for a page with no
  `@ui.URL` field and no table — 10.7% of a broadcast, 6.5% of
  `event-plain`. TODO(e1), profile: `beginURL 10.68% (urlParams 9.64%,
  tableParams 7.16%, maps.clone 5.47%)`.
- **`sameProps` asks `ui.IsDeferred` per field** through a `sync.Map`, and
  `IsLiveType`/`IsFormField`/`IsAutoID` per field, on every placement: the
  per-type answers never change. Covered by TODO(a2).
- **A server-owned `ui.Modal` sends its whole markup on every open**: 3.2 KB
  per open for a five-field form, though the dialog's frame and the form's
  statics don't change between users. The teleport renders as one opaque
  string, and the form's `_vk_idem` token is new on every render, so not even
  the connection's long-markup reference catches a reopen. TODO(e2), wire:
  `B/open 3181` against `B/close 66`.
- **The v2 join costs 1.5–1.7× the HTTP render of the same page**, which
  every live page pays twice on load (render, then join). Not profiled in
  depth here; the join bench is the harness for it.

## Recommendation: what to build first

1. **Make a context read that its own render satisfies not mark the instance
   contextual** — or give ActionMenu (and DataTable, the form elements) the
   menu id without `ui.Ctx`. One change, the largest win measured: a kit data
   table's one-row event 19.8 ms → ~2.4 ms at 1000 rows and 1.9 ms → ~0.23 ms
   at 100 (8×), allocations 11× fewer, GC cycles per event 3.5 → 0.4. Every
   page with an actions menu per row gets it.
2. **A per-type placement plan in `place`**: cache which fields are props,
   funcs, deferred, auto ids; compare-then-copy only on change; build place
   keys once per instance (or intern them). Expect ~40% off every event and
   broadcast on a wide page once (1) is in (`event-plain` 2.4 ms → ~1.4 ms;
   `Wide/tiles=500/props` 1.06 ms → ~0.6 ms), and a quarter of the bytes.
3. **Shared broadcast renders** (TODO d): the largest multiplier — up to K×
   on the CPU of a broadcast to K identical pages — but it needs a reliable
   "same page state" fingerprint across sessions and touches the web side;
   build it after (1) and (2) have made each render cheap, and measure with
   `Broadcast/conns=K`.
4. `beginURL`'s walks (−7–10%, small and local) and the modal's reopen bytes
   (wire only) after that.
