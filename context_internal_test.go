package ui

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// A renderLocal key is set only inside a render: by a withContext node,
// for the nodes it renders. A live host keeps the last render of a
// component reading one on that promise, so a key's value must never
// reach a context a page, a request or a session starts from.
func TestRenderLocalKeysSetInRenderOnly(t *testing.T) {
	fset := token.NewFileSet()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = f
	}
	local := map[string]bool{}
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "renderLocal" {
				continue
			}
			if id, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
				local[id.Name] = true
			}
		}
	}
	if !local["actionMenuKey"] {
		t.Fatalf("found renderLocal keys %v, want actionMenuKey among them", local)
	}
	if _, ok := any(actionMenuKey{}).(renderLocal); !ok {
		t.Fatal("actionMenuKey is not renderLocal")
	}
	sets := 0
	for _, f := range files {
		var stack []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			stack = append(stack, n)
			call, ok := n.(*ast.CallExpr)
			if !ok || !isSel(call.Fun, "context", "WithValue") || len(call.Args) != 3 {
				return true
			}
			lit, ok := call.Args[1].(*ast.CompositeLit)
			if !ok {
				return true
			}
			id, ok := lit.Type.(*ast.Ident)
			if !ok || !local[id.Name] {
				return true
			}
			sets++
			if !inWithContext(stack) {
				t.Errorf("%s: %s is set outside a withContext node", fset.Position(call.Pos()), id.Name)
			}
			return true
		})
	}
	if sets == 0 {
		t.Fatal("no renderLocal key is set anywhere")
	}
}

func isSel(e ast.Expr, pkg, name string) bool {
	s, ok := e.(*ast.SelectorExpr)
	if !ok || s.Sel.Name != name {
		return false
	}
	id, ok := s.X.(*ast.Ident)
	return ok && id.Name == pkg
}

// inWithContext reports whether the innermost function literal on the
// stack is the first argument of a withContext call.
func inWithContext(stack []ast.Node) bool {
	for i := len(stack) - 1; i > 0; i-- {
		lit, ok := stack[i].(*ast.FuncLit)
		if !ok {
			continue
		}
		call, ok := stack[i-1].(*ast.CallExpr)
		if !ok || len(call.Args) == 0 || call.Args[0] != lit {
			return false
		}
		id, ok := call.Fun.(*ast.Ident)
		return ok && id.Name == "withContext"
	}
	return false
}

type plainKey struct{}

type countingHost struct {
	LiveHost
	ctx, local int
}

func (h *countingHost) ContextRead()      { h.ctx++ }
func (h *countingHost) LocalContextRead() { h.local++ }

// A Ctx build reading a renderLocal key, present or not, is a local read;
// reading anything else of the context — another key, its cancellation —
// is a context read, as every Ctx build was before.
func TestResolveContextClassifiesReads(t *testing.T) {
	for _, tc := range []struct {
		name       string
		build      func(ctx context.Context)
		ctx, local int
	}{
		{"none", func(context.Context) {}, 0, 0},
		{"local absent", func(ctx context.Context) { ctx.Value(actionMenuKey{}) }, 0, 1},
		{"local derived", func(ctx context.Context) {
			context.WithValue(ctx, plainKey{}, 1).Value(actionMenuKey{})
		}, 0, 1},
		{"other key", func(ctx context.Context) { ctx.Value(plainKey{}) }, 1, 0},
		{"host", func(ctx context.Context) { liveHost(ctx) }, 1, 0},
		{"err", func(ctx context.Context) { _ = ctx.Err() }, 1, 0},
		{"done", func(ctx context.Context) { ctx.Done() }, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &countingHost{}
			ctx := WithLiveHost(context.Background(), h)
			ResolveContext(ctx, Ctx(func(ctx context.Context) Node { tc.build(ctx); return nil }))
			if h.ctx != tc.ctx || h.local != tc.local {
				t.Fatalf("context reads %d, local %d; want %d, %d", h.ctx, h.local, tc.ctx, tc.local)
			}
		})
	}
}
