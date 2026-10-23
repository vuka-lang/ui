package live_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/uigen"
)

// fakeVars is a VarSource of fixed values, each with its version.
type fakeVars struct {
	vals map[string]string
	vers map[string]uint64
	asks int
}

func (f *fakeVars) VarVersion(_ context.Context, name string) uint64 {
	f.asks++
	return f.vers[name]
}

func (f *fakeVars) set(name, v string) { f.vals[name] = v; f.vers[name]++ }

// varShow reads one template variable in its markup, as compiled code
// reading web.Var does.
type varShow struct {
	ui.Live
	Name    string
	renders *int `vuka:"-"`
	n       uigen.Assign[int]
	vars    *fakeVars `vuka:"-"`
}

func (v *varShow) Render() ui.Node {
	*v.renders++
	return ui.El("p", nil, ui.NodeFunc(func(ctx context.Context, w io.Writer) error {
		ui.ReadVar(ctx, v.Name, v.vars.vers[v.Name])
		_, err := io.WriteString(w, v.Name+"="+v.vars.vals[v.Name])
		return err
	}))
}

// TestVarReadsTracked: an instance whose state and props didn't change
// renders again when a template variable it read has another version, and
// only that one.
func TestVarReadsTracked(t *testing.T) {
	for _, trees := range []bool{false, true} {
		t.Run(map[bool]string{false: "html", true: "trees"}[trees], func(t *testing.T) {
			defer func(v bool) { live.Verify = v }(live.Verify)
			live.Verify = false
			vars := &fakeVars{vals: map[string]string{"user": "ada", "request": "/a"}, vers: map[string]uint64{"user": 1, "request": 1}}
			ctx := ui.WithVarSource(context.Background(), vars)
			var users, paths int
			noop := func() {}
			s := live.NewSession(ctx, func(context.Context) ui.Node {
				return ui.El("main", nil,
					ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(noop)}}, ui.Text("go")),
					ui.Component("user", nil, &varShow{Name: "user", renders: &users, vars: vars}),
					ui.Component("path", nil, &varShow{Name: "request", renders: &paths, vars: vars}))
			})
			defer s.Close()
			var err error
			if trees {
				_, err = s.RenderTrees()
			} else {
				_, err = s.Render()
			}
			if err != nil {
				t.Fatal(err)
			}
			if users != 1 || paths != 1 {
				t.Fatalf("first render: %d, %d", users, paths)
			}
			if _, err := s.Event(ui.Event{Target: "c0:0"}); err != nil {
				t.Fatal(err)
			}
			if users != 1 || paths != 1 {
				t.Fatalf("nothing moved, yet renders: %d, %d", users, paths)
			}
			vars.set("user", "grace")
			p, err := s.Event(ui.Event{Target: "c0:0"})
			if err != nil {
				t.Fatal(err)
			}
			if users != 2 || paths != 1 {
				t.Fatalf("user moved: renders %d, %d; want 2, 1", users, paths)
			}
			if !strings.Contains(s.HTML(), "user=grace") || !strings.Contains(s.HTML(), "request=/a") {
				t.Fatalf("html: %s", s.HTML())
			}
			if trees && len(p.Trees) == 0 || !trees && len(p.Updates) == 0 {
				t.Fatalf("no update sent: %+v", p)
			}
		})
	}
}

// TestVarReadsVerified: verifying sessions agree with the tracking — a
// variable that moved renders its readers, so the kept render never
// differs.
func TestVarReadsVerified(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = true
	vars := &fakeVars{vals: map[string]string{"user": "ada"}, vers: map[string]uint64{"user": 1}}
	var n int
	noop := func() {}
	s := live.NewSession(ui.WithVarSource(context.Background(), vars), func(context.Context) ui.Node {
		return ui.El("main", nil, ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(noop)}}),
			ui.Component("user", nil, &varShow{Name: "user", renders: &n, vars: vars}))
	})
	defer s.Close()
	if _, err := s.RenderTrees(); err != nil {
		t.Fatal(err)
	}
	vars.set("user", "grace")
	if _, err := s.Event(ui.Event{Target: "c0:0"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.HTML(), "user=grace") {
		t.Fatal(s.HTML())
	}
}

// TestVarReadsWithoutSource: a render that read variables, in a session
// with no VarSource to ask, is never kept on its state alone.
func TestVarReadsWithoutSource(t *testing.T) {
	defer func(v bool) { live.Verify = v }(live.Verify)
	live.Verify = false
	vars := &fakeVars{vals: map[string]string{"user": "ada"}, vers: map[string]uint64{"user": 1}}
	var n int
	noop := func() {}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return ui.El("main", nil, ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(noop)}}),
			ui.Component("user", nil, &varShow{Name: "user", renders: &n, vars: vars}))
	})
	defer s.Close()
	if _, err := s.Render(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Event(ui.Event{Target: "c0:0"}); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("renders %d, want 2", n)
	}
}
