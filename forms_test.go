package ui_test

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/forms"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

type signup struct {
	ui.FormState
	Name     string `label:"Full name" validate:"required,max=10" autocomplete:"name"`
	Email    string `validate:"required,email" help:"We'll send a confirmation link"`
	Password string `input:"new-password" validate:"required,min=4"`
	Postcode string `input:"numeric" validate:"required,len=5"`
	Age      int    `validate:"omitempty,min=18"`
	Plan     string `input:"radio" validate:"required"`
	Agree    bool   `label:"I accept the terms" validate:"required" msg:"required=Accept the terms to continue"`
	Note     string `input:"textarea"`
	Secret   string `form:"-"`
}

func (f *signup) PlanChoices() []ui.Option {
	return []ui.Option{{Value: "free", Label: "Free"}, {Value: "pro", Label: "Pro"}}
}

var emailChecks int

func (f *signup) CleanEmail(ctx context.Context) error {
	if forms.Live(ctx) {
		return nil
	}
	emailChecks++
	if f.Email == "taken@example.com" {
		return forms.Errorf("That email address is already registered")
	}
	return nil
}

type joinPage struct {
	ui.Live
	Form signup
	got  *joined
}

// joined is what a session's instance of a joinPage saw: the page the
// session renders is a copy of the one the test made.
type joined struct {
	saved []signup
	fail  func(j *joinPage) error
	last  *joinPage
}

func (j *joinPage) Save(ctx context.Context, f signup) error {
	j.got.last = j
	if j.got.fail != nil {
		return j.got.fail(j)
	}
	j.got.saved = append(j.got.saved, f)
	return nil
}

func (j *joinPage) Render() ui.Node { return ui.Form{For: ui.For(&j.Form), OnSubmit: j.Save} }

var (
	submitTarget = regexp.MustCompile(`data-vk-on-submit="([^"]+)"`)
	inputTarget  = regexp.MustCompile(`data-vk-on-input="([^"]+)"`)
	hiddenValue  = func(html, name string) string {
		m := regexp.MustCompile(`name="` + name + `" value="([^"]*)"`).FindStringSubmatch(html)
		if m == nil {
			return ""
		}
		return m[1]
	}
)

func joinForm(t *testing.T, p *joinPage) *livetest.Client {
	t.Helper()
	if p.got == nil {
		p.got = &joined{}
	}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return &ui.ComponentNode{Site: "join", Props: p, Routed: true} })
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// formValues are the values the browser sends for the page's form: the
// hidden fields as rendered, and fields.
func formValues(html string, fields map[string]string) url.Values {
	v := url.Values{}
	for _, h := range []string{ui.FormIDField, ui.FormIdemField, ui.FormFieldsField, ui.FormUsedField} {
		v.Set(h, unescape(hiddenValue(html, h)))
	}
	for k, x := range fields {
		v.Set(k, x)
	}
	return v
}

func unescape(s string) string {
	return strings.NewReplacer("&amp;", "&", "&#34;", `"`, "&#39;", "'").Replace(s)
}

func send(t *testing.T, c *livetest.Client, re *regexp.Regexp, typ string, form url.Values) live.ServerMessage {
	t.Helper()
	m := re.FindStringSubmatch(c.HTML())
	if m == nil {
		t.Fatalf("no %s handler in %s", typ, c.HTML())
	}
	msg, err := c.Event(ui.Event{Target: m[1], Type: typ, Form: form})
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestFormRendersFields(t *testing.T) {
	c := joinForm(t, &joinPage{})
	html := c.HTML()
	for _, want := range []string{
		`novalidate`,
		`data-vk-form="signup"`,
		`<label class="vk-field__label" for="signup-name">Full name</label>`,
		`id="signup-email-hint">We&#39;ll send a confirmation link</div>`,
		`name="email" id="signup-email" type="email" value="" autocomplete="email" aria-describedby="signup-email-hint"`,
		`name="password" id="signup-password" type="password" autocomplete="new-password"`,
		`aria-controls="signup-password" aria-pressed="false"`,
		`name="postcode" id="signup-postcode" type="text" value="" inputmode="numeric"`,
		`for="signup-age">Age<span class="vk-field__marker"> (optional)</span>`,
		`<legend class="vk-fieldset__legend">Plan</legend>`,
		`id="signup-plan" value="free" name="plan" type="radio"`,
		`id="signup-plan-2" value="pro"`,
		`type="checkbox" value="true"`,
		`<textarea class="vk-textarea" rows="3" name="note" id="signup-note"`,
		`Save</span></button>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s\nin %s", want, html)
		}
	}
	if strings.Contains(html, "secret") || strings.Contains(html, "aria-invalid") || strings.Contains(html, "vk-summary") {
		t.Errorf("a left-out field, an invalid mark or a summary before any use:\n%s", html)
	}
}

func TestFormTiming(t *testing.T) {
	c := joinForm(t, &joinPage{})
	// typing in a field never flagged: nothing shows
	send(t, c, inputTarget, "input", formValues(c.HTML(), map[string]string{"email": "pa@"}))
	if strings.Contains(c.HTML(), "vk-field__error") {
		t.Fatalf("an error shows while typing:\n%s", c.HTML())
	}
	// leaving it non-empty: its error shows, not the empty required ones
	send(t, c, inputTarget, "focusout", formValues(c.HTML(), map[string]string{"email": "pa@", ui.FormUsedField: "email"}))
	html := c.HTML()
	if !strings.Contains(html, `id="signup-email-error"><span class="vk-sr-only">Error:</span> Enter an email address in the correct format, like name@example.com</p>`) {
		t.Fatalf("no email error after blur:\n%s", html)
	}
	if !strings.Contains(html, `aria-invalid="true" aria-describedby="signup-email-hint signup-email-error"`) && !strings.Contains(html, `aria-describedby="signup-email-hint signup-email-error" aria-invalid="true"`) {
		t.Errorf("the control isn't linked to its error:\n%s", html)
	}
	if strings.Contains(html, "Enter full name") || strings.Contains(html, "vk-summary") {
		t.Errorf("an empty required field flagged, or a summary, before submit:\n%s", html)
	}
	// letters in a digits field show at once; a fixed length at its length
	send(t, c, inputTarget, "input", formValues(c.HTML(), map[string]string{"email": "pa@", "postcode": "12a"}))
	if !strings.Contains(c.HTML(), "Postcode must only include numbers") {
		t.Errorf("no error for letters in a numeric field:\n%s", c.HTML())
	}
	send(t, c, inputTarget, "input", formValues(c.HTML(), map[string]string{"email": "pa@", "postcode": "123456"}))
	if !strings.Contains(c.HTML(), "Postcode must be 5 characters") {
		t.Errorf("no error at a fixed length's length:\n%s", c.HTML())
	}
	// fixing a shown error clears it as soon as it's fixed
	send(t, c, inputTarget, "input", formValues(c.HTML(), map[string]string{"email": "pa@example.com", "postcode": "12345"}))
	if strings.Contains(c.HTML(), "vk-field__error") {
		t.Errorf("a fixed field still shows its error:\n%s", c.HTML())
	}
}

func TestFormSubmit(t *testing.T) {
	emailChecks = 0
	p := &joinPage{}
	c := joinForm(t, p)
	m := send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"email": "taken@example.com", "age": "x"}))
	if m.Error != "" {
		t.Fatalf("a failed submit's reply has an error: %q", m.Error)
	}
	html := c.HTML()
	for _, want := range []string{
		`<h2 class="vk-summary__title">There is a problem</h2>`,
		`<a href="#signup-name">Enter full name</a>`,
		`<a href="#signup-password">Enter password</a>`,
		`<a href="#signup-age">Enter a number</a>`,
		`<a href="#signup-plan">Select plan</a>`,
		`<a href="#signup-agree">Accept the terms to continue</a>`,
		`value="x"`, // what was typed stays
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s\nin %s", want, html)
		}
	}
	if len(p.got.saved) != 0 {
		t.Fatalf("Save ran with errors")
	}
	if emailChecks != 1 {
		t.Errorf("CleanEmail ran %d times on a submit", emailChecks)
	}
	ok := map[string]string{"name": "  Pa  ", "email": "taken@example.com", "password": " pass ", "postcode": "12345", "plan": "pro", "agree": "true", "age": ""}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), ok))
	if !strings.Contains(c.HTML(), "That email address is already registered") || len(p.got.saved) != 0 {
		t.Fatalf("CleanEmail's error doesn't show, or Save ran:\n%s", c.HTML())
	}
	// a live change of another field keeps CleanEmail's error, the email unchanged
	ok["note"] = "hi"
	send(t, c, inputTarget, "input", formValues(c.HTML(), ok))
	if !strings.Contains(c.HTML(), "That email address is already registered") {
		t.Fatalf("CleanEmail's error went with another field's change:\n%s", c.HTML())
	}
	ok["email"] = "pa@example.com"
	send(t, c, submitTarget, "submit", formValues(c.HTML(), ok))
	if len(p.got.saved) != 1 {
		t.Fatalf("Save didn't run:\n%s", c.HTML())
	}
	got := p.got.saved[0]
	if got.Name != "Pa" || got.Password != " pass " || got.Plan != "pro" || !got.Agree || got.Note != "hi" {
		t.Errorf("saved %+v: strings trimmed, passwords raw", got)
	}
	if strings.Contains(c.HTML(), "vk-summary") || strings.Contains(c.HTML(), "vk-field__error") {
		t.Errorf("errors after a successful submit:\n%s", c.HTML())
	}
	// the same submit again (a retry) doesn't run Save again
	idem := hiddenValue(c.HTML(), ui.FormIdemField)
	send(t, c, submitTarget, "submit", formValues(c.HTML(), ok))
	send(t, c, submitTarget, "submit", func() url.Values { v := formValues(c.HTML(), ok); v.Set(ui.FormIdemField, idem); return v }())
	if len(p.got.saved) != 2 {
		t.Errorf("a new render's submit runs once, a repeated key doesn't: %d saves", len(p.got.saved))
	}
}

func TestFormFailAndUnchecked(t *testing.T) {
	p := &joinPage{got: &joined{fail: func(*joinPage) error { return errors.New("boom") }}}
	c := joinForm(t, p)
	ok := map[string]string{"name": "Pa", "email": "pa@example.com", "password": "pass", "postcode": "12345", "plan": "pro", "agree": "true"}
	m := send(t, c, submitTarget, "submit", formValues(c.HTML(), ok))
	if m.Error == "" {
		t.Errorf("Save's internal error isn't the reply's")
	}
	p.got.fail = func(j *joinPage) error { return j.Form.Fail("We couldn't create your account") }
	send(t, c, submitTarget, "submit", formValues(c.HTML(), ok))
	if !strings.Contains(c.HTML(), "<li>We couldn&#39;t create your account</li>") {
		t.Errorf("Fail's error isn't in the summary:\n%s", c.HTML())
	}
	// a rendered checkbox left out of the values is false
	p.got.fail = nil
	delete(ok, "agree")
	send(t, c, submitTarget, "submit", formValues(c.HTML(), ok))
	if p.got.last.Form.Agree || !strings.Contains(c.HTML(), "Accept the terms to continue") {
		t.Errorf("an unticked rendered checkbox isn't false: %v", p.got.last.Form.Agree)
	}
}

func TestFormPlainSubmit(t *testing.T) {
	p := &joinPage{got: &joined{}}
	s := live.NewSession(context.Background(), func(context.Context) ui.Node { return &ui.ComponentNode{Site: "join", Props: p, Routed: true} })
	html, err := s.Render()
	if err != nil {
		t.Fatal(err)
	}
	pt, err := s.Submit("signup", formValues(html, map[string]string{"name": "Pa"}))
	if err != nil {
		t.Fatal(err)
	}
	if !ui.QuietError(pt.Err) || !strings.Contains(s.HTML(), "There is a problem") {
		t.Fatalf("a failed plain submit: %v\n%s", pt.Err, s.HTML())
	}
	if _, err := s.Submit("other", nil); err == nil {
		t.Errorf("no error submitting a form the page doesn't have")
	}
}

type pathForm struct {
	ui.FormState
	Address struct {
		City string `validate:"required"`
	}
	FirstName string
	UserID    string
}

func TestFormPaths(t *testing.T) {
	var f pathForm
	html, err := ui.String(context.Background(), ui.Form{For: ui.For(&f)})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`name="address.city" id="path-form-address-city"`, `name="firstName" id="path-form-first-name"`, `for="path-form-user-id">User ID`, `autocomplete="given-name"`, `<legend class="vk-fieldset__legend">Address</legend>`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s\nin %s", want, html)
		}
	}
	if f.ErrorOf(&f.Address.City) != "" || f.Touched(&f.FirstName) || f.Changed(&f.FirstName) {
		t.Errorf("a fresh form has state")
	}
	f.SetError(&f.Address.City, "Enter a city in Tanzania")
	if f.ErrorOf(&f.Address.City) != "Enter a city in Tanzania" {
		t.Errorf("SetError: %q", f.ErrorOf(&f.Address.City))
	}
	f.FirstName = "x"
	if !f.Changed(&f.FirstName) {
		t.Errorf("Changed")
	}
}

func TestMessages(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		e     forms.Error
		label string
		want  string
	}{
		{forms.Error{Rule: "required"}, "Full name", "Enter full name"},
		{forms.Error{Rule: "required", Params: map[string]string{"kind": "email"}}, "Email", "Enter your email address"},
		{forms.Error{Rule: "max", Params: map[string]string{"kind": "string", "n": "80"}}, "Name", "Name must be 80 characters or less"},
		{forms.Error{Rule: "email"}, "Email", "Enter an email address in the correct format, like name@example.com"},
		{forms.Error{Rule: "number"}, "Age", "Enter a number"},
		{forms.Error{Key: "custom"}, "Age", "custom"},
		{forms.Error{Rule: "required"}, "URL", "Enter URL"},
	} {
		if got := forms.Text(ctx, c.e, c.label); got != c.want {
			t.Errorf("%+v: %q, want %q", c.e, got, c.want)
		}
	}
	ctx = forms.WithCatalogue(ctx, forms.Messages{"validate.required": "{Label} can't be empty"})
	if got := forms.Text(ctx, forms.Error{Rule: "required"}, "Name"); got != "Name can't be empty" {
		t.Errorf("a catalogue's message: %q", got)
	}
}
