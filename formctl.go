package ui

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vuka-lang/ui/v2/forms"
)

// Config is how forms render, app-wide (Configure) or for the pages
// rendered with a context (WithConfig).
type Config struct {
	// MarkRequired marks required fields' labels "(required)" instead of
	// optional fields' "(optional)", the default.
	MarkRequired bool
	// RequiredAttr gives required fields' controls the required attribute.
	// Off by default: forms are novalidate, the requirement is in the label,
	// and the attribute treats whitespace as a value where the binder trims
	// it.
	RequiredAttr bool
	// BrowserChecks has a struct form's controls carry the rules the
	// browser can check exactly as the server does (required, len, min,
	// max, email, oneof), with their messages, so a submit sent without a
	// live connection shows its errors at once instead of after a round
	// trip; the server still checks every submit. Off by default: the
	// messages then ride in the page's markup.
	BrowserChecks bool
	// Messages is the app's message catalogue: its own wording of the
	// default messages, its message keys' texts, its labels' translations
	// (forms.Messages, forms.Translations). A request's own catalogue
	// (forms.WithCatalogue) comes before it, the default English wording
	// after.
	Messages forms.Catalogue
}

// messageCtx is ctx resolving messages with its config's catalogue too.
func messageCtx(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return forms.WithBase(ctx, configOf(ctx).Messages)
}

// tr is text in ctx's language (forms.Translate): a label, a hint, the
// form components' own words by key ("form.problem").
func tr(ctx context.Context, text string) string {
	if text == "" {
		return ""
	}
	return forms.Translate(messageCtx(ctx), text)
}

var (
	configMu  sync.RWMutex
	appConfig Config
)

// Configure sets how forms render in every page without a Config of its
// own (WithConfig).
func Configure(c Config) {
	configMu.Lock()
	appConfig = c
	configMu.Unlock()
}

type configKey struct{}

// WithConfig is ctx rendering forms as c says.
func WithConfig(ctx context.Context, c Config) context.Context {
	return context.WithValue(ctx, configKey{}, c)
}

func configOf(ctx context.Context) Config {
	if ctx != nil {
		if c, ok := ctx.Value(configKey{}).(Config); ok {
			return c
		}
	}
	configMu.RLock()
	defer configMu.RUnlock()
	return appConfig
}

// A FormHost is what serves a page's forms: package web puts one in the
// context of every request and live page (WithFormHost).
type FormHost interface {
	// FormToken is the CSRF token a form carries in its _vk_csrf field;
	// "" for none.
	FormToken(ctx context.Context) string
	// FormKey signs the list of fields a form rendered (_vk_fields), so a
	// plain POST can't claim others: the same on every replica.
	FormKey() []byte
	// FormStore keeps submits' outcomes by idempotency key; nil for the
	// default, in this process.
	FormStore() FormStore
}

type formHostKey struct{}

// WithFormHost is ctx whose forms h serves.
func WithFormHost(ctx context.Context, h FormHost) context.Context {
	return context.WithValue(ctx, formHostKey{}, h)
}

func formHostOf(ctx context.Context) FormHost {
	if ctx == nil {
		return nil
	}
	h, _ := ctx.Value(formHostKey{}).(FormHost)
	return h
}

// A FormStore keeps what came of each submit, by its idempotency key: a
// form carries a key per render, so a submit sent twice — the back button,
// a retry after a lost reply — gets the first one's outcome instead of
// running again.
type FormStore interface {
	// Begin claims key for a submit about to run its handler; ok is false
	// when a submit with the key began before, out being what it came to
	// (out.Done false while it still runs).
	Begin(key string) (out FormOutcome, ok bool)
	// Finish records the outcome of the submit that claimed key.
	Finish(key string, out FormOutcome)
	// Abandon frees key: the submit didn't pass its checks (a 422), so the
	// visitor may correct the form and send it again.
	Abandon(key string)
}

// FormOutcome is what a submit came to: Done once its handler returned
// without an error, Redirect where it sent the visitor.
type FormOutcome struct {
	Done     bool
	Redirect string
}

// FormKeyTTL is how long a FormStore made with NewFormStore keeps a key.
const FormKeyTTL = 24 * time.Hour

// NewFormStore is a FormStore in memory, keeping each key for FormKeyTTL.
// An app with several replicas keeps keys where they all see them.
func NewFormStore() FormStore { return &memFormStore{keys: map[string]memOutcome{}} }

type memFormStore struct {
	mu   sync.Mutex
	keys map[string]memOutcome
	ops  int
}

type memOutcome struct {
	out     FormOutcome
	expires time.Time
}

func (m *memFormStore) Begin(key string) (FormOutcome, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if m.ops++; m.ops%256 == 0 {
		for k, o := range m.keys {
			if now.After(o.expires) {
				delete(m.keys, k)
			}
		}
	}
	if o, ok := m.keys[key]; ok && now.Before(o.expires) {
		return o.out, false
	}
	m.keys[key] = memOutcome{expires: now.Add(FormKeyTTL)}
	return FormOutcome{}, true
}

func (m *memFormStore) Finish(key string, out FormOutcome) {
	m.mu.Lock()
	m.keys[key] = memOutcome{out: out, expires: time.Now().Add(FormKeyTTL)}
	m.mu.Unlock()
}

func (m *memFormStore) Abandon(key string) {
	m.mu.Lock()
	delete(m.keys, key)
	m.mu.Unlock()
}

var defaultFormStore = NewFormStore()

func formStoreOf(ctx context.Context) FormStore {
	if h := formHostOf(ctx); h != nil {
		if s := h.FormStore(); s != nil {
			return s
		}
	}
	return defaultFormStore
}

var processFormKey = func() []byte {
	k := make([]byte, 32)
	rand.Read(k)
	return k
}()

func formKeyOf(ctx context.Context) []byte {
	if h := formHostOf(ctx); h != nil {
		if k := h.FormKey(); len(k) > 0 {
			return k
		}
	}
	return processFormKey
}

// signFields is a form's rendered-field list as its _vk_fields carries it:
// the list, a dot, and its signature for the form's identity.
func signFields(key []byte, identity, list string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("vk-form-fields\x00" + identity + "\x00" + list))
	return list + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}

// openFields is the rendered fields a signed list names, with their
// modifiers; ok is false when the signature isn't the form's.
func openFields(key []byte, identity, signed string) (map[string]string, bool) {
	i := strings.LastIndexByte(signed, '.')
	if i < 0 || !hmac.Equal([]byte(signFields(key, identity, signed[:i])), []byte(signed)) {
		return nil, false
	}
	out := map[string]string{}
	for _, item := range strings.Split(signed[:i], ",") {
		if item == "" {
			continue
		}
		path, mods, _ := strings.Cut(item, ":")
		out[path] = strings.ReplaceAll(mods, "+", ",")
	}
	return out, true
}

// fieldList is a rendered-field set as a list: "email,bio:raw,age:number+lazy".
func fieldList(set map[string]string, ft *formType) string {
	var b strings.Builder
	for _, f := range ft.fields {
		mods, ok := set[f.path]
		if !ok {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(f.path)
		if mods != "" {
			b.WriteString(":" + strings.ReplaceAll(mods, ",", "+"))
		}
	}
	return b.String()
}

func newIdemKey() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ErrFormInvalid is what a form's submit answers when the form didn't pass
// its checks: its errors show in the form, so a live page shows no other
// error, and a plain POST answers 422 with the form.
var ErrFormInvalid = errors.New("ui: the form has errors")

// ErrFormIntent is what a form's submit answers when a button with an
// intent sent it ("update": the form renders again, its choices following
// its new values) rather than a submit of the form: a plain POST answers
// 200 with the form, Save not called.
var ErrFormIntent = errors.New("ui: the form was sent for an intent, not submitted")

// ErrFormChecked is what a form's submit answers when it was sent for its
// checks only (its _vk_check field: a form on a page that isn't live,
// checked over HTTP as the visitor leaves a field): the checks ran live,
// as a change's do, and the submit handler didn't.
var ErrFormChecked = errors.New("ui: the form was sent for its checks, not submitted")

// QuietError reports whether a handler's error shows only in the page (a
// form's ErrFormInvalid, ErrFormIntent, ErrFormChecked), not as an error
// message of its own.
func QuietError(err error) bool {
	return errors.Is(err, ErrFormInvalid) || errors.Is(err, ErrFormIntent) || errors.Is(err, ErrFormChecked)
}

// RedirectOf is where err, a handler's error, sends the visitor: a value
// with a RedirectLocation method, as web.Redirect has.
func RedirectOf(err error) (string, bool) {
	var r interface{ RedirectLocation() string }
	if errors.As(err, &r) {
		return r.RedirectLocation(), true
	}
	return "", false
}

// A FormRegistrar is a live host that keeps the submit handlers of the forms
// a render has, by their identity, for a plain POST of one (package live).
type FormRegistrar interface {
	Form(identity string, submit any)
}

// formCtl is a rendered form's controller: its submit and change handlers.
type formCtl struct {
	form     reflect.Value // *T, T embedding FormState
	st       *formState
	onSubmit any // the page's handler, run once the form passes its checks
	onChange any
	validate string // "", "submit" or "off"
}

// usedFrom adds the fields the runtime marked used to the form's.
func (st *formState) usedFrom(vals url.Values) {
	for _, list := range vals[FormUsedField] {
		for _, p := range strings.Split(list, ",") {
			if p = strings.TrimSpace(p); p != "" && st.ft.byPath[p] != nil {
				if st.used == nil {
					st.used = map[string]bool{}
				}
				st.used[p] = true
			}
		}
	}
}

// renderedFrom is the fields the form rendered: what its signed
// _vk_fields says, else what its last render had.
func (st *formState) renderedFrom(ctx context.Context, vals url.Values) map[string]string {
	if s := vals.Get(FormFieldsField); s != "" {
		if set, ok := openFields(formKeyOf(ctx), st.identity(), s); ok {
			return set
		}
	}
	return st.rendered
}

// change answers a change of a field (input, change, focusout): the form
// takes the values, every check runs live, and the fields the visitor used
// show their errors.
func (c *formCtl) change(ctx context.Context, ev Event) error {
	st := c.st
	if seq, err := strconv.ParseInt(ev.Form.Get(FormSeqField), 10, 64); err == nil {
		if seq <= st.seq {
			return nil // older than what the form took since: a submit's reply stands
		}
		st.seq = seq
	}
	st.usedFrom(ev.Form)
	st.bind(c.form.Elem(), ev.Form, st.renderedFrom(ctx, ev.Form))
	if c.validate == "" {
		if err := st.check(ctx, c.form.Elem(), true); err != nil {
			return err
		}
	}
	if c.onChange != nil {
		return CallHandler(ctx, c.onChange, c.payload(ev))
	}
	return nil
}

// submit answers the form's submit: it takes the values, runs every check,
// and calls the page's handler only when all pass. A failed check, or the
// handler's form error (Fail, a forms.Error, a public error), shows in the
// form, and the submit answers ErrFormInvalid. A submit with an
// idempotency key used before gets that submit's outcome instead.
func (c *formCtl) submit(ctx context.Context, ev Event) error {
	st := c.st
	if seq, err := strconv.ParseInt(ev.Form.Get(FormSeqField), 10, 64); err == nil && seq > st.seq {
		st.seq = seq
	}
	if check := ev.Form.Get(FormCheckField); check != "" {
		// a check over HTTP: what a change does on a live page, with no
		// state kept between requests — the request says whether the form
		// was submitted
		st.usedFrom(ev.Form)
		st.submitted = st.submitted || check == "submitted"
		st.bind(c.form.Elem(), ev.Form, st.renderedFrom(ctx, ev.Form))
		if c.validate == "" {
			if err := st.check(ctx, c.form.Elem(), true); err != nil {
				return err
			}
		}
		return ErrFormChecked
	}
	if ev.Form.Get(FormIntentField) == "update" {
		// the no-script Update button: the form takes its values, its
		// choices follow them, nothing is saved
		st.usedFrom(ev.Form)
		st.bind(c.form.Elem(), ev.Form, st.renderedFrom(ctx, ev.Form))
		if c.validate == "" {
			if err := st.check(ctx, c.form.Elem(), true); err != nil {
				return err
			}
		}
		return ErrFormIntent
	}
	key := ev.Form.Get(FormIdemField)
	store := formStoreOf(ctx)
	if key != "" {
		if out, ok := store.Begin(key); !ok {
			switch {
			case out.Redirect != "":
				return formRedirect(out.Redirect)
			case out.Done:
				return nil
			}
			return ErrFormInvalid // the first is still running: this one is dropped
		}
	}
	abandon := func() {
		if key != "" {
			store.Abandon(key)
		}
	}
	form := c.form.Elem()
	st.usedFrom(ev.Form)
	st.bind(form, ev.Form, st.renderedFrom(ctx, ev.Form))
	st.submitted, st.failed = true, nil
	if c.validate != "off" {
		if err := st.check(ctx, form, false); err != nil {
			abandon()
			return err
		}
	}
	if len(st.all()) > 0 && c.validate != "off" {
		abandon()
		return ErrFormInvalid
	}
	st.submitting = true
	var herr error
	if c.onSubmit != nil {
		herr = CallHandler(ctx, c.onSubmit, c.payload(ev))
	}
	st.submitting = false
	if herr == nil {
		st.succeeded(form)
		if key != "" {
			store.Finish(key, FormOutcome{Done: true})
		}
		return nil
	}
	if loc, ok := RedirectOf(herr); ok {
		st.succeeded(form)
		if key != "" {
			store.Finish(key, FormOutcome{Done: true, Redirect: loc})
		}
		return herr
	}
	if errs, ok := asFormErrors(herr, ""); ok {
		abandon()
		for _, e := range errs {
			if f := st.ft.byPath[e.Path]; f != nil && e.Path != "" {
				st.pin(e.Path, &pinnedErrors{errs: []forms.Error{e}, value: valueOf(form, f), set: true})
				st.markVisible(e.Path)
				continue
			}
			if !slices.ContainsFunc(st.failed, func(x forms.Error) bool { return reflect.DeepEqual(x, e) }) {
				st.failed = append(st.failed, e) // Fail added it already: once
			}
		}
		return ErrFormInvalid
	}
	var pub interface{ PublicMessage() string }
	if errors.As(herr, &pub) {
		abandon()
		st.failed = append(st.failed, forms.Error{Key: pub.PublicMessage()})
		return ErrFormInvalid
	}
	abandon()
	return herr
}

// savedOver answers the browser asking again once a successful submit's
// Saved is over: the form renders, Saved false.
func (c *formCtl) savedOver(context.Context) error { return nil }

// payload binds the page's handler's payload: the form (T or *T, a copy
// sharing the form's state), the event, or its values.
func (c *formCtl) payload(ev Event) func(t reflect.Type) (reflect.Value, error) {
	ft := c.form.Type().Elem()
	return func(t reflect.Type) (reflect.Value, error) {
		switch {
		case t == ft:
			return c.form.Elem(), nil
		case t.Kind() == reflect.Pointer && t.Elem() == ft:
			cp := reflect.New(ft)
			cp.Elem().Set(c.form.Elem())
			return cp, nil
		case t == eventType:
			return reflect.ValueOf(ev), nil
		case reflect.TypeFor[url.Values]().ConvertibleTo(t) && t.Kind() == reflect.Map:
			return reflect.ValueOf(ev.Form).Convert(t), nil
		}
		return reflect.Value{}, fmt.Errorf("ui: a form's handler takes the form (%s or *%[1]s), a ui.Event or url.Values; this one takes %s", ft.Name(), t)
	}
}

// formRedirect is a redirect a replayed submit answers with.
type formRedirect string

func (r formRedirect) Error() string            { return "redirect to " + string(r) }
func (r formRedirect) RedirectLocation() string { return string(r) }

// current is the form a <ui.Form> renders, for what renders inside it.
type current struct {
	b boundField
}

type currentKey struct{}

func currentForm(ctx context.Context) *current {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(currentKey{}).(*current)
	return c
}
