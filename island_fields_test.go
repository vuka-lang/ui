package ui

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/vuka-lang/vuka"
)

type levelsPage struct {
	Q       string   `url:"q" bind:"q"`
	Page    int      `json:"page" bind:"page,debounce=0"`
	Sort    string   `json:"sort" bind:",debounce=1s"`
	Levels  []string `json:"levels"`
	Error   string   // a prop with no handle: live.error is live's own
	OnPick  func(string)
	changed int
	seen    string
}

func (p *levelsPage) Mount() {}
func (p *levelsPage) Changed(ctx context.Context) error {
	p.changed++
	p.seen = p.Q
	if p.Q == "boom" {
		return errors.New("boom")
	}
	return nil
}
func (p *levelsPage) Delete(id int64) error { return nil }

func init() { Island("islands/Levels.vue")(vuka.TypeOf[levelsPage]("app.levelsPage")) }

func TestIslandFields(t *testing.T) {
	fields, err := IslandFields(reflect.TypeFor[*levelsPage]())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range fields {
		got = append(got, f.Handle+"="+f.Wire+":"+map[bool]string{true: "bind " + f.Debounce.String(), false: "read"}[f.Bind])
	}
	want := []string{"q=Q:bind 300ms", "page=page:bind 0s", "sort=sort:bind 1s", "levels=levels:read", "error=Error:read"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("fields\n got  %v\n want %v", got, want)
	}
	if err := CheckIsland(reflect.TypeFor[*levelsPage]()); err != nil {
		t.Error(err)
	}
	for _, m := range IslandMethods(reflect.TypeFor[*levelsPage]()) {
		if m.Name == "changed" || m.Name == "mount" {
			t.Errorf("%s is callable", m.Name)
		}
	}
}

func TestCheckIsland(t *testing.T) {
	type badBind struct {
		Q string `bind:"query"`
	}
	type badOpt struct {
		Q string `bind:"q,wait=1"`
	}
	type bindSecret struct {
		q string `bind:"q"`
	}
	type bindReserved struct {
		Pending bool `bind:""`
	}
	type twoQ struct {
		Q  string
		Q2 string `json:"q"`
	}
	for _, c := range []struct {
		t    reflect.Type
		want string
	}{
		{reflect.TypeFor[badBind](), `bind:"query" names the field live.q`},
		{reflect.TypeFor[badOpt](), `unknown option "wait"`},
		{reflect.TypeFor[bindSecret](), "bind: is for a prop"},
		{reflect.TypeFor[bindReserved](), "live.pending is live's own"},
		{reflect.TypeFor[twoQ](), "fields Q and Q2 are both live.q"},
		{reflect.TypeFor[methodFieldClash](), "the method Rows and the field List are both live.rows"},
		{reflect.TypeFor[badChanged](), "Changed is Changed(), Changed(ctx"},
	} {
		err := CheckIsland(c.t)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: got %v, want %q", c.t, err, c.want)
		}
	}
}

type methodFieldClash struct {
	List []string `json:"rows"`
}

func (p *methodFieldClash) Rows() {}

type badChanged struct{ Q string }

func (p *badChanged) Changed(q string) {}

func TestSetBound(t *testing.T) {
	p := &levelsPage{}
	x := reflect.ValueOf(p)
	if err := SetBound(context.Background(), x, json.RawMessage(`{"Q":"gr","page":2}`)); err != nil {
		t.Fatal(err)
	}
	if p.Q != "gr" || p.Page != 2 || p.changed != 1 || p.seen != "gr" {
		t.Errorf("after set: %+v", p)
	}
	for _, c := range []struct {
		data   string
		status int
		field  string
	}{
		{`{"levels":["x"]}`, 403, "levels"},
		{`{"Q":"x","Error":"y"}`, 403, "Error"},
		{`{"nope":1}`, 403, "nope"},
		{`{"page":"two"}`, 422, "page"},
		{`[1]`, 400, "set"},
	} {
		err := SetBound(context.Background(), x, json.RawMessage(c.data))
		e := ErrorOf(err)
		if e.Status != c.status || e.Fields[c.field] == "" {
			t.Errorf("%s: %+v", c.data, e)
		}
	}
	if p.Q != "gr" || p.changed != 1 {
		t.Errorf("a refused set changed the page: %+v", p)
	}
	if err := SetBound(context.Background(), x, json.RawMessage(`{"Q":"boom"}`)); err == nil || err.Error() != "boom" {
		t.Errorf("Changed's error: %v", err)
	}
}

func TestIslandPageFields(t *testing.T) {
	s := renderIsland(t, IslandPage(&levelsPage{Q: "a"}, CallAt("/levels", "sig")))
	for _, want := range []string{
		`data-vk-fields="{&#34;error&#34;:{&#34;w&#34;:&#34;Error&#34;},&#34;levels&#34;:{&#34;w&#34;:&#34;levels&#34;},&#34;page&#34;:{&#34;d&#34;:0,&#34;w&#34;:&#34;page&#34;},&#34;q&#34;:{&#34;d&#34;:300,&#34;w&#34;:&#34;Q&#34;},&#34;sort&#34;:{&#34;d&#34;:1000,&#34;w&#34;:&#34;sort&#34;}}"`,
		`data-vk-methods="[&#34;delete&#34;]"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("lacks %s:\n%s", want, s)
		}
	}
}
