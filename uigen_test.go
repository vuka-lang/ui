package ui_test

import (
	"context"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui"
	"github.com/vuka-lang/ui/uigen"
)

// TestStateAPI: a State's API is Get and Set; what the compiler lowers to
// is package uigen's.
func TestStateAPI(t *testing.T) {
	var names []string
	m := reflect.TypeFor[ui.State[bool]]()
	for i := 0; i < m.NumMethod(); i++ {
		names = append(names, m.Method(i).Name)
	}
	if !slices.Equal(names, []string{"Get", "Set"}) {
		t.Errorf("ui.State's methods: %v, want [Get Set]", names)
	}
}

// TestChainOrder: writes chain in the order an on… attribute writes them,
// whatever their kind.
func TestChainOrder(t *testing.T) {
	open := uigen.State("o", false)
	tab := uigen.State("t", "a")
	n := uigen.State("n", 1)
	keys := regexp.MustCompile(`"s":"(\w+):\w+"`)
	got := keys.ReplaceAllString(encode(t, uigen.Inc(uigen.Toggle(ui.JS.Set(tab, "b"), open), n).Push(func() {})), `"s":"$1"`)
	want := `[["state_set",{"s":"t","i":"a","v":"b"}],["state_toggle",{"s":"o","i":false}],["state_inc",{"s":"n","i":1,"by":1}],["push",{}]]`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// fields keeps its states in fields, declared by their type alone.
type fields struct {
	ui.Live
	open   ui.State[bool]
	tab    ui.State[string]
	Shared ui.State[int] // a parent may pass one
	pushed string
}

func (f *fields) Mount(ctx context.Context) error {
	f.PushJS(f.tab.Set("env")) // a write before the first render
	return nil
}

func (f *fields) Render() ui.Node {
	hidden := uigen.Not(f.open)
	return ui.HTML(`<p hidden={hidden} title={f.tab}></p>`, hidden, f.tab)
}

// TestFieldStates: a live component's zero ui.State fields get states of
// their own before it mounts, one per field; a state its parent set stays.
func TestFieldStates(t *testing.T) {
	shared := uigen.State("shared", 3)
	c := &fields{Shared: shared}
	if err := ui.MountLive(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Shared, shared) {
		t.Errorf("the parent's state was replaced")
	}
	out, _ := page(t, c.Render())
	if !strings.Contains(out, "hidden") || strings.Count(out, `&#34;s&#34;:&#34;c0:f`) != 2 {
		t.Errorf("the fields' states:\n%s", out)
	}
	again := &fields{}
	uigen.States(again)
	before := again.open
	uigen.States(again)
	if !reflect.DeepEqual(again.open, before) {
		t.Errorf("States replaced a state it had given")
	}
}

// static is a struct component: its View gives its fields states first.
type static struct {
	on ui.State[bool]
}

func (s static) View() ui.Node {
	uigen.States(&s)
	return ui.HTML(`<b data-on={s.on}></b>`, s.on)
}

func TestStaticStates(t *testing.T) {
	out, _ := page(t, static{}.View())
	if !strings.Contains(out, `&#34;s&#34;:&#34;c0:f`) {
		t.Errorf("a static component's state:\n%s", out)
	}
}

// TestAssign: uigen.Assign is tracked state: a set marks it changed.
func TestAssign(t *testing.T) {
	var a uigen.Assign[[]int]
	a.Set([]int{1})
	a.Update(func(v []int) []int { return append(v, 2) })
	if got := a.Get(); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("Get: %v", got)
	}
	if !*a.AssignDirty() {
		t.Error("a set didn't mark it changed")
	}
}
