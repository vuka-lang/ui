package live_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/forms"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

type task struct {
	ID   int
	Name string
}

// RenameTask is the form of a task's row.
type RenameTask struct {
	ui.FormState
	Name string `validate:"required,max=12"`
}

// CleanName is a Clean method's check, run on the submit as any form's.
func (f *RenameTask) CleanName(ctx context.Context) error {
	if f.Name == "admin" {
		return forms.Errorf("Pick a name that isn't reserved")
	}
	return nil
}

// TaskRows is a stream of tasks, each row holding a form that renames it.
type TaskRows struct {
	ui.Live
	Form    func(t task) ui.Form // the row's form; the default when nil
	Renamed *[]string
	tasks   ui.Stream[task]
}

func (l *TaskRows) LiveSchema(s *ui.LiveSchema) { s.Mount(l.mount) }

func (l *TaskRows) mount(context.Context) error {
	l.tasks.Reset(task{1, "one"}, task{2, "two"})
	return nil
}

func (l *TaskRows) Rename(ctx context.Context, id int, f RenameTask) error {
	if f.Name == "taken" {
		return f.Fail("That name is taken")
	}
	*l.Renamed = append(*l.Renamed, f.Name)
	l.tasks.Insert(task{id, f.Name})
	return nil
}

func (l *TaskRows) Other(ctx context.Context, id int, f otherForm) error { return nil }

type otherForm struct {
	ui.FormState
	Note string
}

func (l *TaskRows) Render() ui.Node {
	var rows []ui.Node
	for _, t := range l.tasks.Rows() {
		form := ui.Form{For: &RenameTask{Name: t.Name}, OnSubmit: ui.JS.Push(l.Rename, t.ID)}
		if l.Form != nil {
			form = l.Form(t)
		}
		rows = append(rows, ui.HTML(`<li key={id}><span>{name}</span>{form}</li>`, t.ID, t.Name, form))
	}
	b := l.tasks.Block(rows)
	return ui.HTML(`<main><ul {attrs...}>{rows}</ul></main>`, b.Attrs(), b)
}

func submitRow(t *testing.T, s *live.Session, id int, name string) (live.ServerMessage, map[string]ui.FormReport) {
	t.Helper()
	key := strconv.Itoa(id)
	form := url.Values{ui.FormIDField: {"rename-task-" + key}, ui.FormRowField: {key}, "name": {name}}
	m := s.Handle(live.ClientMessage{Type: live.EventMsg, Ref: 9, Event: ui.Event{Target: "c1:Rename", Type: "submit", Args: []json.RawMessage{json.RawMessage(key)}, Form: form}})
	if m.Type != live.PatchMsg || m.Error != "" {
		t.Fatalf("the submit answered %+v", m)
	}
	var res map[string]ui.FormReport
	if err := json.Unmarshal(m.Result, &res); err != nil {
		t.Fatalf("result %s: %v", m.Result, err)
	}
	return m, res
}

// A <ui.Form> in a stream row submits as a Push of its owner's method:
// the form's checks run on the values it sends, and failing them — or the
// method answering a form error — the reply's result is the form's report,
// addressed to that row's form, and nothing renders; a success inserts
// the row again and the report clears the form's errors.
func TestStreamRowForm(t *testing.T) {
	l := &TaskRows{Renamed: new([]string)}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("tasks", nil, l) })
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	h := c.HTML()
	for _, want := range []string{`id="rename-task-1"`, `data-vk-form="rename-task-1"`, `id="rename-task-2-name"`, `data-vk-row-form`,
		`name="_vk_row" value="1"`, `&#34;h&#34;:&#34;c1:Rename&#34;,&#34;args&#34;:[1],&#34;form&#34;:true`} {
		if !strings.Contains(h, want) {
			t.Fatalf("the rows' forms lack %s:\n%s", want, h)
		}
	}
	for _, not := range []string{"data-vk-on-submit", "data-vk-on-input", "_vk_idem"} {
		if strings.Contains(h, not) {
			t.Fatalf("a row's form has %s, which the session would keep for it:\n%s", not, h)
		}
	}

	m, res := submitRow(t, s, 1, "")
	r := res["report"]
	if len(m.Trees) != 0 {
		t.Fatalf("a failed row form renders: %+v", m.Trees)
	}
	if r.Form != "rename-task-1" || len(r.Errors) != 1 || r.Errors[0].ID != "rename-task-1-name" || r.Errors[0].Key != "validate.required" || r.Summary == nil || r.Summary.Items[0].Href != "#rename-task-1-name" {
		t.Fatalf("report %+v", r)
	}
	_, res = submitRow(t, s, 2, "a name far too long")
	if r := res["report"]; r.Form != "rename-task-2" || len(r.Errors) != 1 || r.Errors[0].Key != "validate.max.string" {
		t.Fatalf("report %+v", r)
	}
	_, res = submitRow(t, s, 1, "admin") // a Clean method's error
	if r := res["report"]; len(r.Errors) != 1 || r.Errors[0].Text != "Pick a name that isn't reserved" || r.Summary == nil || len(r.Summary.Items) != 1 {
		t.Fatalf("report %+v", r)
	}
	_, res = submitRow(t, s, 1, "taken") // the method's
	if r := res["report"]; len(r.Errors) != 0 || r.Summary == nil || r.Summary.Items[0].Text != "That name is taken" {
		t.Fatalf("report %+v", r)
	}
	if len(*l.Renamed) != 0 {
		t.Fatalf("the method ran for a form failing its checks: %v", *l.Renamed)
	}

	m, res = submitRow(t, s, 1, "uno")
	if r := res["report"]; r.Form != "rename-task-1" || len(r.Errors) != 0 || r.Summary != nil {
		t.Fatalf("a success's report %+v", r)
	}
	if len(m.Trees) == 0 || len(*l.Renamed) != 1 || (*l.Renamed)[0] != "uno" {
		t.Fatalf("the success renders %+v, renamed %v", m.Trees, *l.Renamed)
	}
	if h := s.HTML(); strings.Contains(h, "data-vk-row") {
		t.Fatalf("the session keeps the row: %s", h)
	}
}

// A row's form submits as a push of a method taking it (E0362), and checks
// only as it is submitted: onChange would keep a handler for the row (E0363).
func TestStreamRowFormErrors(t *testing.T) {
	for want, form := range map[string]func(l *TaskRows, t task) ui.Form{
		"[E0362]: the <ui.Form> of RenameTask in a stream row has no onSubmit": func(l *TaskRows, t task) ui.Form {
			return ui.Form{For: &RenameTask{}}
		},
		"[E0362]: the onSubmit of the <ui.Form> of RenameTask in a stream row pushes Other, which doesn't take the form": func(l *TaskRows, t task) ui.Form {
			return ui.Form{For: &RenameTask{}, OnSubmit: ui.JS.Push(l.Other, t.ID)}
		},
		"[E0363]": func(l *TaskRows, t task) ui.Form {
			return ui.Form{For: &RenameTask{}, OnSubmit: ui.JS.Push(l.Rename, t.ID), OnChange: ui.JS.Push(l.Rename, t.ID)}
		},
		"[E0358]": func(l *TaskRows, t task) ui.Form {
			return ui.Form{For: &RenameTask{}, OnSubmit: l.Rename}
		},
	} {
		l := &TaskRows{}
		l.Form = func(t task) ui.Form { return form(l, t) }
		s := live.NewSession(context.Background(), func(context.Context) ui.Node { return ui.Component("tasks", nil, l) })
		if _, err := livetest.Join(s, 2); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
}
