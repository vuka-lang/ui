package ui_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/vuka-lang/ui/v2"
	"github.com/vuka-lang/ui/v2/live"
	"github.com/vuka-lang/ui/v2/live/livetest"
)

type avatarForm struct {
	ui.FormState
	Name   string  `validate:"required"`
	Avatar ui.File `accept:"image/*" max:"1KB" validate:"required"`
}

type avatarPage struct {
	ui.Live
	Form  avatarForm
	saved *[]byte
	name  *string
}

func (p *avatarPage) Save(_ context.Context, f avatarForm) error {
	r, err := f.Avatar.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	*p.saved, err = io.ReadAll(r)
	*p.name = f.Avatar.Name()
	return err
}

func (p *avatarPage) Render() ui.Node { return ui.Form{For: ui.For(&p.Form), OnSubmit: p.Save} }

type fileHost struct{ store ui.FileStore }

func (fileHost) FormToken(context.Context) string { return "" }
func (fileHost) FormKey() []byte                   { return []byte("a test key, the same everywhere") }
func (fileHost) FormStore() ui.FormStore           { return ui.NewFormStore() }
func (h fileHost) FileStore() ui.FileStore         { return h.store }

var (
	png       = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 200)...)
	ticketRef = regexp.MustCompile(`name="_vk_upload\.avatar" value="([^"]+)"`)
)

func avatarSession(t *testing.T) (context.Context, *live.Session, *livetest.Client, *[]byte, *string) {
	t.Helper()
	ctx := ui.WithFormHost(context.Background(), fileHost{ui.DirFileStore(t.TempDir())})
	var saved []byte
	var name string
	s := live.NewSession(ctx, func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "avatar", Props: &avatarPage{saved: &saved, name: &name}, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, s, c, &saved, &name
}

func upload(t *testing.T, ctx context.Context, html, name, ctype string, body []byte) (ui.Uploaded, error) {
	t.Helper()
	m := ticketRef.FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no upload ticket in\n%s", html)
	}
	return ui.Upload(ctx, unescape(m[1]), name, ctype, bytes.NewReader(body))
}

func TestFileFieldRenders(t *testing.T) {
	_, _, c, _, _ := avatarSession(t)
	html := c.HTML()
	for _, want := range []string{
		`enctype="multipart/form-data"`,
		`type="file"`,
		`name="_vk_file.avatar"`,
		`accept="image/*"`,
		`name="_vk_file_err.avatar"`,
		`<progress class="vk-file__progress"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in\n%s", want, html)
		}
	}
	if !regexp.MustCompile(`<label class="vk-field__label" for="([^"]+)">Avatar.*id="([^"]+)"[^>]*name="_vk_file\.avatar"`).MatchString(html) {
		t.Errorf("the label isn't for the file input:\n%s", html)
	}
}

func TestUploadRefusesAsItStreams(t *testing.T) {
	ctx, _, c, _, _ := avatarSession(t)
	if _, err := upload(t, ctx, c.HTML(), "big.png", "image/png", append(png, make([]byte, 2000)...)); !isUpload(err, "max", http.StatusRequestEntityTooLarge) {
		t.Errorf("a file over max: %v", err)
	}
	if _, err := upload(t, ctx, c.HTML(), "notes.txt", "text/plain", []byte("hello")); !isUpload(err, "accept", http.StatusUnsupportedMediaType) {
		t.Errorf("a file accept: doesn't list: %v", err)
	}
	if _, err := ui.Upload(ctx, "forged.ticket", "a.png", "image/png", bytes.NewReader(png)); !isUpload(err, "failed", http.StatusBadRequest) {
		t.Errorf("a forged ticket: %v", err)
	}
	up, err := upload(t, ctx, c.HTML(), `C:\Users\pa\photo.png`, "", png)
	if err != nil || up.Name != "photo.png" || up.Size != int64(len(png)) || up.Ref == "" {
		t.Fatalf("an upload: %+v %v", up, err)
	}
}

func isUpload(err error, rule string, status int) bool {
	var ue *ui.UploadError
	return errors.As(err, &ue) && ue.Rule == rule && ue.Status == status
}

func TestFileSubmit(t *testing.T) {
	ctx, _, c, saved, name := avatarSession(t)
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Pa"}))
	if !strings.Contains(c.HTML(), "Select avatar") || *saved != nil {
		t.Fatalf("no file: the required message, nothing saved:\n%s", c.HTML())
	}
	up, err := upload(t, ctx, c.HTML(), "photo.png", "image/png", png)
	if err != nil {
		t.Fatal(err)
	}
	// a failed submit keeps the file: no re-upload
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"avatar": up.Ref}))
	html := c.HTML()
	if *saved != nil || !strings.Contains(html, `<span class="vk-file__name">photo.png</span>`) || !strings.Contains(html, ">Change<") || !strings.Contains(html, `value="`+up.Ref+`"`) {
		t.Fatalf("a failed submit doesn't show the file it keeps:\n%s", html)
	}
	if strings.Contains(html, "Select avatar") {
		t.Errorf("the kept file is still missing:\n%s", html)
	}
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Pa", "avatar": unescape(hiddenValue(html, "avatar"))}))
	if !bytes.Equal(*saved, png) || *name != "photo.png" {
		t.Fatalf("Save read %d bytes, name %q:\n%s", len(*saved), *name, c.HTML())
	}
}

func TestFileErrors(t *testing.T) {
	ctx, _, c, _, _ := avatarSession(t)
	up, err := upload(t, ctx, c.HTML(), "photo.png", "image/png", png)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		fields map[string]string
		want   string
	}{
		{"too large", map[string]string{"_vk_file_err.avatar": "max"}, "Avatar must be smaller than 1 KB"},
		{"not accepted", map[string]string{"_vk_file_err.avatar": "accept"}, "Avatar must be an image"},
		{"failed", map[string]string{"_vk_file_err.avatar": "network"}, "Avatar could not be sent: select it again"},
		{"forged", map[string]string{"avatar": up.Ref[:len(up.Ref)-2] + "xx"}, "Select avatar again"},
	} {
		send(t, c, submitTarget, "submit", formValues(c.HTML(), mergeFields(tc.fields, "name", "Pa")))
		if !strings.Contains(c.HTML(), tc.want) {
			t.Errorf("%s: no %q in\n%s", tc.name, tc.want, c.HTML())
		}
	}
}

func TestFileNoLongerKept(t *testing.T) {
	store := &forgetful{FileStore: ui.DirFileStore(t.TempDir())}
	ctx := ui.WithFormHost(context.Background(), fileHost{store})
	var saved []byte
	var name string
	s := live.NewSession(ctx, func(context.Context) ui.Node {
		return &ui.ComponentNode{Site: "avatar", Props: &avatarPage{saved: &saved, name: &name}, Routed: true}
	})
	c, err := livetest.Join(s, 2)
	if err != nil {
		t.Fatal(err)
	}
	up, err := upload(t, ctx, c.HTML(), "photo.png", "image/png", png)
	if err != nil {
		t.Fatal(err)
	}
	store.forget = true
	send(t, c, submitTarget, "submit", formValues(c.HTML(), map[string]string{"name": "Pa", "avatar": up.Ref}))
	if saved != nil || !strings.Contains(c.HTML(), "the file sent before is no longer kept") {
		t.Fatalf("a file the store lost:\n%s", c.HTML())
	}
}

type forgetful struct {
	ui.FileStore
	forget bool
}

func (f *forgetful) Open(id string) (io.ReadCloser, error) {
	if f.forget {
		return nil, errors.New("gone")
	}
	return f.FileStore.Open(id)
}

func mergeFields(m map[string]string, kv ...string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}
