package ui_test

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/forms"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

// user is a record a form edits: a model with more columns than the form
// lets a visitor set.
type user struct {
	ID      int64
	Name    string `validate:"required,max=20"`
	Email   string `validate:"required,email"`
	Age     *int
	Admin   bool
	Tags    []string
	Country string
}

type profile struct {
	ui.FormState
	Name  string `validate:"required"`
	Email string `validate:"required,email"`
	Age   int    `validate:"omitempty,min=18"`
	Tags  []string
}

func TestLoadApply(t *testing.T) {
	age := 30
	u := user{ID: 7, Name: "Pa", Email: "pa@example.com", Age: &age, Admin: true, Tags: []string{"a"}}
	var f profile
	f.Load(&u)
	if _, err := ui.String(context.Background(), ui.Form{For: ui.For(&f)}); err != nil {
		t.Fatal(err)
	}
	if f.Name != "Pa" || f.Email != "pa@example.com" || f.Age != 30 || !slices.Equal(f.Tags, []string{"a"}) {
		t.Fatalf("Load before the first render: %+v", f)
	}
	if f.Dirty() || f.Changed(&f.Name) {
		t.Errorf("a loaded form is dirty")
	}
	u.Tags[0] = "b"
	if f.Tags[0] != "a" {
		t.Errorf("the form shares the record's slice")
	}
	f.Name = "Paul"
	if !f.Dirty() || !f.Changed(&f.Name) || f.Changed(&f.Email) {
		t.Errorf("Dirty %v, Changed %v %v", f.Dirty(), f.Changed(&f.Name), f.Changed(&f.Email))
	}
	var out user
	out.ID, out.Admin = 9, false
	set := f.Apply(&out)
	if out.Name != "Paul" || out.Email != "pa@example.com" || out.Age == nil || *out.Age != 30 || out.ID != 9 || out.Admin {
		t.Errorf("Apply: %+v", out)
	}
	if !slices.Equal(set, []string{"Name", "Email", "Age", "Tags"}) {
		t.Errorf("Apply set %v", set)
	}
	f.Age = 0
	f.Apply(&out)
	if out.Age != nil {
		t.Errorf("an empty field sets a pointer to nil: %v", *out.Age)
	}
	f.Reset()
	if f.Name != "Pa" || f.Age != 30 || f.Dirty() {
		t.Errorf("Reset: %+v", f)
	}
	html, _ := ui.String(context.Background(), ui.Form{For: ui.For(&f)})
	if !strings.Contains(html, `data-vk-gen="2"`) {
		t.Errorf("Load and Reset bump the generation:\n%s", html)
	}
}

// editPage loads its record in Mount, before its form renders.
type editPage struct {
	ui.Live
	Form  profile
	saved *[]user
	user  user
}

func (p *editPage) LiveSchema(l *ui.LiveSchema) { l.Mount(p.load) }

func (p *editPage) load(ctx context.Context) error {
	p.user = user{ID: 3, Name: "Ana", Email: "ana@example.com", Admin: true}
	p.Form.Load(p.user)
	return nil
}

func (p *editPage) Save(ctx context.Context, f profile) error {
	f.Apply(&p.user)
	*p.saved = append(*p.saved, p.user)
	return nil
}

func (p *editPage) Render() ui.Node {
	return ui.Fragment(ui.El("h1", nil, ui.Text("Editing "+p.Form.Name)), ui.Form{For: ui.For(&p.Form), OnSubmit: p.Save, ConfirmLeave: true})
}

func TestLoadInMount(t *testing.T) {
	var saved []user
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "edit", Props: &editPage{saved: &saved}, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	html := c.HTML()
	for _, want := range []string{`<h1>Editing Ana</h1>`, `value="ana@example.com"`, `data-vk-confirm-leave="You have changes`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
	if strings.Contains(html, `data-vk-dirty=""`) || regexp.MustCompile(`data-vk-dirty[ >]`).MatchString(html) {
		t.Errorf("a loaded form is dirty:\n%s", html)
	}
	send(t, c, inputTarget, "input", formValues(c.HTML(), map[string]string{"name": "Anna", "email": "ana@example.com"}))
	if !regexp.MustCompile(`data-vk-dirty[ >]`).MatchString(c.HTML()) {
		t.Errorf("a changed form isn't dirty:\n%s", c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Anna", "email": "ana@example.com"}))
	if len(saved) != 1 || saved[0].Name != "Anna" || saved[0].ID != 3 || !saved[0].Admin {
		t.Fatalf("saved %+v", saved)
	}
	html = c.HTML()
	if regexp.MustCompile(`data-vk-dirty[ >]`).MatchString(html) {
		t.Errorf("a saved form is dirty:\n%s", html)
	}
	if !regexp.MustCompile(`data-vk-saved="\d+" data-vk-on-vksaved="c1:\d+"`).MatchString(html) {
		t.Errorf("a saved form asks again once Saved is over:\n%s", html)
	}
}

type account struct {
	ID       int64
	Name     string `validate:"required,max=20" label:"Full name"`
	Email    string `validate:"required,email"`
	Role     string
	Password string
}

func (a *account) RoleChoices() []ui.Option {
	return []ui.Option{{Value: "user", Label: "User"}, {Value: "admin", Label: "Admin"}}
}

type accountPage struct {
	ui.Live
	Form  *ui.ModelForm[account]
	saved *[]account
}

func (p *accountPage) LiveSchema(l *ui.LiveSchema) { l.Mount(p.load) }

func (p *accountPage) load(context.Context) error {
	p.Form = ui.FormOf[account]("Name", "Email")
	p.Form.Load(account{ID: 1, Name: "Pa", Email: "pa@example.com", Role: "user", Password: "x"})
	return nil
}

func (p *accountPage) Save(ctx context.Context, f *ui.ModelForm[account]) error {
	rec := account{ID: 1, Role: "user", Password: "x"}
	*p.saved = append(*p.saved, *f.Apply(&rec))
	return nil
}

func (p *accountPage) Render() ui.Node { return ui.Form{For: ui.For(&p.Form), OnSubmit: p.Save} }

func TestFormOf(t *testing.T) {
	var saved []account
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "acct", Props: &accountPage{saved: &saved}, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	html := c.HTML()
	for _, want := range []string{`data-vk-form="account"`, `for="account-name">Full name`, `name="email" id="account-email"`, `value="pa@example.com"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
	if strings.Contains(html, `name="role"`) || strings.Contains(html, `name="password"`) || strings.Contains(html, `name="iD"`) {
		t.Errorf("a field FormOf doesn't name renders:\n%s", html)
	}
	send(t, c, submitTarget, "submit", formValues(html, map[string]string{"name": "", "email": "pa@example.com", "role": "admin"}))
	if len(saved) != 0 || !strings.Contains(c.HTML(), `<a href="#account-name">Enter full name</a>`) {
		t.Fatalf("the model's rules don't run:\n%s", c.HTML())
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Paul", "email": "paul@example.com", "role": "admin", "password": "y"}))
	if len(saved) != 1 {
		t.Fatalf("no save:\n%s", c.HTML())
	}
	if got := saved[0]; got.Name != "Paul" || got.Email != "paul@example.com" || got.Role != "user" || got.Password != "x" || got.ID != 1 {
		t.Errorf("a submit set a field FormOf doesn't name: %+v", got)
	}

	f := ui.FormOf[account]("Name", "Emial")
	_, err = ui.String(context.Background(), ui.Form{For: ui.For(&f)})
	if err == nil || !strings.Contains(err.Error(), "account has no field Emial; did you mean Email?") {
		t.Errorf("an unknown field: %v", err)
	}
}

type place struct {
	ui.FormState
	Country string `validate:"required"`
	City    string `validate:"required" depends:"Country"`
	Note    string
}

var cityCalls int

func (f *place) CountryChoices() []ui.Option {
	return []ui.Option{{Value: "tz", Label: "Tanzania"}, {Value: "ke", Label: "Kenya"}}
}

func (f *place) CityChoices(ctx context.Context) ([]ui.Option, error) {
	cityCalls++
	switch f.Country {
	case "tz":
		return []ui.Option{{Value: "dar", Label: "Dar es Salaam"}, {Value: "aru", Label: "Arusha"}}, nil
	case "ke":
		return []ui.Option{{Value: "nbo", Label: "Nairobi"}}, nil
	}
	return nil, nil
}

type placePage struct {
	ui.Live
	Form  place
	saves *int
}

func (p *placePage) Save(context.Context, place) error { *p.saves++; return nil }

func (p *placePage) Render() ui.Node { return ui.Form{For: ui.For(&p.Form), OnSubmit: p.Save} }

func TestDependentChoices(t *testing.T) {
	saves := 0
	page := &placePage{saves: &saves}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "place", Props: page, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.HTML(), `<noscript><button type="submit" class="vk-btn vk-btn--secondary vk-field__update" name="_vk_intent" value="update">Update</button></noscript>`) {
		t.Errorf("no Update button after the field City depends on:\n%s", c.HTML())
	}
	if strings.Count(c.HTML(), "_vk_intent") != 1 {
		t.Errorf("an Update button after a field nothing depends on:\n%s", c.HTML())
	}
	send(t, c, inputTarget, "change", formValues(c.HTML(), map[string]string{"country": "tz"}))
	if !strings.Contains(c.HTML(), `value="aru">Arusha`) {
		t.Fatalf("City's choices don't follow Country:\n%s", c.HTML())
	}
	cityCalls = 0
	send(t, c, inputTarget, "change", formValues(c.HTML(), map[string]string{"country": "tz", "city": "aru"}))
	if cityCalls != 1 {
		t.Errorf("CityChoices ran %d times in one event", cityCalls)
	}
	send(t, c, inputTarget, "change", formValues(c.HTML(), map[string]string{"country": "ke", "city": "aru", ui.FormUsedField: "city"}))
	html := c.HTML()
	if strings.Contains(html, `value="aru" selected`) || !strings.Contains(html, `value="nbo">Nairobi`) {
		t.Errorf("a City no longer among the choices stays:\n%s", html)
	}
	if strings.Contains(html, `vk-field__error`) {
		t.Errorf("an error shows before submit:\n%s", html)
	}
	// a stray value submitted is an error, Save not run
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"country": "ke", "city": "aru"}))
	if saves != 0 || !strings.Contains(c.HTML(), `<a href="#place-city">Select city from the list</a>`) {
		t.Errorf("a value not among the choices passes (%d saves):\n%s", saves, c.HTML())
	}
	// the Update intent renders again and saves nothing
	html = c.HTML()
	v := formValues(html, map[string]string{"country": "tz", "city": "nbo"})
	v.Set(ui.FormIntentField, "update")
	m := send(t, c, submitTarget, "submit", v)
	if m.Error != "" || saves != 0 {
		t.Fatalf("an intent: %q, %d saves", m.Error, saves)
	}
	pl := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "place", Props: &placePage{saves: &saves}, Routed: true}
	})
	if _, err := pl.Render(); err != nil {
		t.Fatal(err)
	}
	p, err := pl.Submit("place", v)
	if err != nil || !errors.Is(p.Err, ui.ErrFormIntent) || !ui.QuietError(p.Err) {
		t.Fatalf("a plain intent: %v %v", err, p.Err)
	}
	if !strings.Contains(pl.HTML(), `value="aru">Arusha`) || strings.Contains(pl.HTML(), "vk-summary") {
		t.Errorf("a plain intent's render:\n%s", pl.HTML())
	}
}

func TestPlurals(t *testing.T) {
	ctx := context.Background()
	max := func(n string) forms.Error {
		return forms.Error{Rule: "max", Params: map[string]string{"kind": "string", "n": n}}
	}
	if got := forms.Text(ctx, max("1"), "Code"); got != "Code must be 1 character or less" {
		t.Errorf("one: %q", got)
	}
	if got := forms.Text(ctx, max("80"), "Name"); got != "Name must be 80 characters or less" {
		t.Errorf("other: %q", got)
	}
	ru := forms.Translations{"ru": forms.Messages{"validate.max.string": "{Label}: не больше {n, plural, one {# символа} few {# символов} many {# символов} other {# символа}}", "Name": "Имя"}}
	rctx := forms.WithLocale(forms.WithCatalogue(ctx, ru), "ru-RU", "en")
	for n, want := range map[string]string{"1": "Имя: не больше 1 символа", "3": "Имя: не больше 3 символов", "11": "Имя: не больше 11 символов", "21": "Имя: не больше 21 символа", "1.5": "Имя: не больше 1.5 символа"} {
		if got := forms.Text(rctx, max(n), "Name"); got != want {
			t.Errorf("ru %s: %q, want %q", n, got, want)
		}
	}
	for _, c := range []struct {
		lang string
		n    float64
		frac int
		want string
	}{{"fr", 0, 0, "one"}, {"fr", 2, 0, "other"}, {"pl", 22, 0, "few"}, {"pl", 25, 0, "many"}, {"ja", 1, 0, "other"}, {"en", 1, 1, "other"}, {"sw", 1, 0, "one"}} {
		if got := forms.PluralCategory(c.lang, c.n, c.frac); got != c.want {
			t.Errorf("%s %v: %s, want %s", c.lang, c.n, got, c.want)
		}
	}
	if got := forms.Text(ctx, forms.Error{Key: "{n, plural, =0 {none} one {one item} other {# items}}", Params: map[string]string{"n": "0"}}, ""); got != "none" {
		t.Errorf("=0: %q", got)
	}
	if got := forms.ParseAcceptLanguage("en;q=0.5, sw-TZ, fr;q=0, *;q=0.1, de;q=0.8"); !slices.Equal(got, []string{"sw-TZ", "de", "en"}) {
		t.Errorf("Accept-Language: %v", got)
	}
}

func TestTranslations(t *testing.T) {
	sw := forms.Translations{"sw": forms.Messages{
		"validate.required":   "Weka {label}",
		"form.problem":        "Kuna tatizo",
		"form.optional":       "(si lazima)",
		"form.error":          "Kosa:",
		"Full name":           "Jina kamili",
		"We'll send a link":   "Tutakutumia kiungo",
		"signup.email.taken":  "Barua pepe hii imesajiliwa",
		"validate.email":      "Weka barua pepe sahihi",
		"validate.max.string": "{Label} isizidi herufi {n}",
		"validate.min.number": "{Label} iwe {n} au zaidi",
	}}
	ui.Configure(ui.Config{Messages: sw, BrowserChecks: true})
	defer ui.Configure(ui.Config{})
	type swForm struct {
		ui.FormState
		Name string `label:"Full name" validate:"required" help:"We'll send a link"`
		Bio  string
	}
	var f swForm
	ctx := forms.WithLocale(context.Background(), "sw")
	html, err := ui.String(ctx, ui.Form{For: ui.For(&f)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{">Jina kamili</label>", ">Tutakutumia kiungo</div>", "Bio<span class=\"vk-field__marker\"> (si lazima)</span>", `data-vk-words="{&#34;error&#34;:&#34;Kosa:&#34;,&#34;problem&#34;:&#34;Kuna tatizo&#34;}"`, "Weka jina kamili"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
	en, _ := ui.String(forms.WithLocale(context.Background(), "fr", "en"), ui.Form{For: ui.For(&swForm{})})
	if !strings.Contains(en, ">Full name") || !strings.Contains(en, "Enter full name") {
		t.Errorf("a language the catalogue hasn't falls back to English:\n%s", en)
	}
	// a request's own catalogue comes before the app's
	own := forms.WithCatalogue(ctx, forms.Messages{"validate.required": "Jaza {label}"})
	if got := forms.Text(forms.WithBase(own, sw), forms.Error{Rule: "required"}, "Full name"); got != "Jaza jina kamili" {
		t.Errorf("a request's catalogue: %q", got)
	}
}

type shapes struct {
	ui.FormState
	Pin   string  `input:"numeric" validate:"required,len=4"`
	Name  string  `validate:"required,max=40"`
	Qty   int     `input:"number" validate:"min=1,max=9"`
	Price float64 `validate:"min=0.5"`
}

func TestConstraintAttrs(t *testing.T) {
	var f shapes
	html, err := ui.String(context.Background(), ui.Form{For: ui.For(&f)})
	if err != nil {
		t.Fatal(err)
	}
	pin := regexp.MustCompile(`<input[^>]*name="pin"[^>]*>`).FindString(html)
	name := regexp.MustCompile(`<input[^>]*name="name"[^>]*>`).FindString(html)
	qty := regexp.MustCompile(`<input[^>]*name="qty"[^>]*>`).FindString(html)
	if !strings.Contains(pin, `minlength="4" maxlength="4"`) || strings.Contains(pin, " required") {
		t.Errorf("a numeric field's lengths: %s", pin)
	}
	if strings.Contains(name, "maxlength") || strings.Contains(html, "pattern=") {
		t.Errorf("a text field's length written: %s", name)
	}
	if !strings.Contains(qty, `type="number"`) || !strings.Contains(qty, `min="1" max="9"`) {
		t.Errorf("a number input's bounds: %s", qty)
	}
	if strings.Contains(html, "data-vk-check") {
		t.Errorf("browser checks without Config.BrowserChecks")
	}
	checked, _ := ui.String(ui.WithConfig(context.Background(), ui.Config{BrowserChecks: true}), ui.Form{For: ui.For(&shapes{})})
	pin = regexp.MustCompile(`<input[^>]*name="pin"[^>]*>`).FindString(checked)
	if !strings.Contains(pin, `data-vk-check="{&#34;d&#34;:1,&#34;k&#34;:&#34;s&#34;,&#34;r&#34;:[[&#34;required&#34;,null,&#34;Enter pin&#34;,&#34;validate.required&#34;],[&#34;len&#34;,4,&#34;Pin must be 4 characters&#34;,&#34;validate.len.string&#34;]],&#34;t&#34;:1}"`) {
		t.Errorf("the browser's checks: %s", pin)
	}
	req, _ := ui.String(ui.WithConfig(context.Background(), ui.Config{RequiredAttr: true}), ui.Form{For: ui.For(&shapes{})})
	if !regexp.MustCompile(`name="name"[^>]* required`).MatchString(req) {
		t.Errorf("Config.RequiredAttr writes required:\n%s", req)
	}
}

type tickForm struct {
	ui.FormState
	Email string `validate:"required,email"`
}

type tickPage struct {
	ui.Live
	Form tickForm
}

func (p *tickPage) Render() ui.Node {
	return ui.Form{For: ui.For(&p.Form), Children: ui.Field{For: ui.For(&p.Form.Email, &p.Form), OK: true}}
}

func TestFieldOK(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "tick", Props: &tickPage{}, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.HTML(), "vk-field__ok") {
		t.Fatalf("a tick on a field not used")
	}
	send(t, c, inputTarget, "focusout", formValues(c.HTML(), map[string]string{"email": "pa@", ui.FormUsedField: "email"}))
	if strings.Contains(c.HTML(), "vk-field__ok") {
		t.Fatalf("a tick on a field in error")
	}
	send(t, c, inputTarget, "input", formValues(c.HTML(), map[string]string{"email": "pa@example.com"}))
	if !strings.Contains(c.HTML(), `vk-field vk-field--ok`) || !strings.Contains(c.HTML(), `<span class="vk-field__ok" aria-hidden="true">✓</span>`) {
		t.Errorf("no tick on a valid used field:\n%s", c.HTML())
	}
}

// signupBox is a stateful component holding a form, embedded in a page
// that re-renders around it.
type signupBox struct {
	ui.Live
	Title string
	Form  tickForm
}

func (b *signupBox) Render() ui.Node {
	return ui.Fragment(ui.El("h2", nil, ui.Text(b.Title)), ui.Form{For: ui.For(&b.Form)})
}

type boxPage struct {
	ui.Live
	n int
}

func (p *boxPage) Bump(context.Context) error { p.n++; return nil }

func (p *boxPage) Render() ui.Node {
	return ui.Fragment(ui.El("button", []ui.Attr{{Name: "onClick", Value: ui.On(p.Bump)}}, ui.Text("bump")),
		&ui.ComponentNode{Site: "box", Props: &signupBox{Title: "Join " + strings.Repeat("!", p.n)}})
}

func TestEmbeddedForm(t *testing.T) {
	s := live.NewSession(context.Background(), func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "boxes", Props: &boxPage{}, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	send(t, c, inputTarget, "focusout", formValues(c.HTML(), map[string]string{"email": "pa@", ui.FormUsedField: "email"}))
	if !strings.Contains(c.HTML(), `value="pa@"`) {
		t.Fatalf("the component's form didn't take the value:\n%s", c.HTML())
	}
	click := regexp.MustCompile(`data-vk-on-click="([^"]+)"`).FindStringSubmatch(c.HTML())
	if _, err := c.Event(ui.Event{Target: click[1], Type: "click"}); err != nil {
		t.Fatal(err)
	}
	html := c.HTML()
	if !strings.Contains(html, "<h2>Join !</h2>") {
		t.Fatalf("the parent's render didn't reach the component's props:\n%s", html)
	}
	if !strings.Contains(html, `value="pa@"`) || !strings.Contains(html, `data-vk-error="validate.email"`) {
		t.Errorf("the parent's render reset the component's form:\n%s", html)
	}
}

type planForm struct {
	ui.FormState
	Plan  string   `validate:"required"`
	Extra []string `validate:"max=1"`
}

func (f *planForm) PlanChoices() []ui.Option {
	return []ui.Option{{Value: "free", Label: "Free"}, {Value: "pro", Label: "Pro"}}
}

func (f *planForm) ExtraChoices() []ui.Option {
	return []ui.Option{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}
}

func TestChoicesComponent(t *testing.T) {
	f := planForm{Plan: "pro", Extra: []string{"b"}}
	html, err := ui.String(context.Background(), ui.Form{For: ui.For(&f), Children: ui.Fragment(
		ui.Choices{For: ui.For(&f.Plan, &f)}, ui.Choices{For: ui.For(&f.Extra, &f)})})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`id="plan-form-plan" value="free" name="plan" type="radio">`,
		`id="plan-form-plan-2" value="pro" name="plan" type="radio" checked>`,
		`id="plan-form-extra-2" value="b" name="extra" type="checkbox" checked>`,
		`<label class="vk-check__label" for="plan-form-plan-2">Pro</label>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
}
