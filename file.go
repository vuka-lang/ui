package ui

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vuka-lang/ui/v2/forms"
)

// File is a form field holding a file the visitor sends:
//
//	type Profile struct {
//		ui.FormState
//		Avatar ui.File `accept:"image/*" max:"5MB" validate:"required"`
//	}
//
// The browser sends the file as soon as the visitor chooses it, over HTTP
// with a progress bar, its size enforced while it streams; the form then
// carries a signed reference to it instead of the file, so validating
// never sends it again and a failed submit keeps it ("photo.jpg ✓
// Change"). Without script the form posts as multipart and the first post
// stores the file. The submit handler reads it with Open.
//
// accept: lists what the field takes, as the HTML attribute does
// ("image/*", ".pdf", "application/pdf"), matched against the file's name,
// its declared type and the type its first bytes show; it guides the
// visitor and isn't a security check. max: is its largest size ("5MB",
// "500KB"), DefaultFileMax when absent. Files are kept in the app's
// FileStore for FileTTL.
type File struct {
	ref   string // the signed reference the form carries
	id    string // the store's
	name  string
	size  int64
	ctype string
	store FileStore
}

// Name is the file's name as the visitor's device gave it: shown back to
// them, never a path to write to.
func (f File) Name() string { return f.name }

// Size is the file's size in bytes.
func (f File) Size() int64 { return f.size }

// ContentType is the file's type, as its first bytes show it, else as the
// browser declared it.
func (f File) ContentType() string { return f.ctype }

// Empty reports whether the field holds no file.
func (f File) Empty() bool { return f.ref == "" }

// ErrNoFile is what Open answers for a field holding no file.
var ErrNoFile = errors.New("ui: the field holds no file")

// Open reads the file.
func (f File) Open() (io.ReadCloser, error) {
	if f.ref == "" || f.store == nil {
		return nil, ErrNoFile
	}
	return f.store.Open(f.id)
}

// String is the file's name.
func (f File) String() string { return f.name }

var fileType = reflect.TypeFor[File]()

// DefaultFileMax is the largest file a File field without a max: tag
// takes.
const DefaultFileMax = 10 << 20

// FileTTL is how long a file sent to a form is kept, and how long a page
// may send one: long enough for a visitor to finish the form.
const FileTTL = 24 * time.Hour

// A FileStore keeps the files sent to forms until their forms are
// submitted. The default keeps them in a directory of this machine: an app
// with several replicas gives them a store they share (WithFileStore in
// package web), else a submit reaching another replica than its upload did
// asks the visitor to choose the file again.
type FileStore interface {
	// Put stores what r reads and names it.
	Put(r io.Reader) (id string, err error)
	// Open reads the file named id; an error when it is no longer kept.
	Open(id string) (io.ReadCloser, error)
	// Delete removes the file named id.
	Delete(id string) error
}

// A FileHost is a FormHost that names the app's FileStore.
type FileHost interface {
	FileStore() FileStore
}

// DirFileStore is a FileStore keeping files in dir, removing those older
// than FileTTL as it stores more.
func DirFileStore(dir string) FileStore { return &dirStore{dir: dir} }

type dirStore struct {
	dir   string
	mu    sync.Mutex
	swept time.Time
}

func (d *dirStore) Put(r io.Reader) (string, error) {
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return "", err
	}
	d.sweep()
	var b [16]byte
	rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	f, err := os.OpenFile(filepath.Join(d.dir, id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return id, nil
}

func (d *dirStore) path(id string) (string, error) {
	if len(id) != 32 || strings.Trim(id, "0123456789abcdef") != "" {
		return "", fmt.Errorf("ui: no file %q", id)
	}
	return filepath.Join(d.dir, id), nil
}

func (d *dirStore) Open(id string) (io.ReadCloser, error) {
	p, err := d.path(id)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (d *dirStore) Delete(id string) error {
	p, err := d.path(id)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// sweep removes the files older than FileTTL, at most every ten minutes.
func (d *dirStore) sweep() {
	d.mu.Lock()
	if time.Since(d.swept) < 10*time.Minute {
		d.mu.Unlock()
		return
	}
	d.swept = time.Now()
	d.mu.Unlock()
	entries, _ := os.ReadDir(d.dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > FileTTL {
			os.Remove(filepath.Join(d.dir, e.Name()))
		}
	}
}

var defaultFileStore = DirFileStore(filepath.Join(os.TempDir(), "vuka-uploads"))

func fileStoreOf(ctx context.Context) FileStore {
	if h, ok := formHostOf(ctx).(FileHost); ok {
		if s := h.FileStore(); s != nil {
			return s
		}
	}
	return defaultFileStore
}

// The fields a File field adds to its form, each its path after a dot:
const (
	FileTicketField = "_vk_upload"   // the signed ticket the upload of its file presents
	FileField       = "_vk_file"     // the file itself, posted without script
	FileErrorField  = "_vk_file_err" // why the upload failed ("max", "accept", "failed"), set by the runtime
)

// fileSpec reads what a File field's tags say: its largest size, what it
// accepts.
func (f *formField) fileSpec(sf reflect.StructField) error {
	f.input, f.fileMax, f.accept = "file", DefaultFileMax, strings.TrimSpace(sf.Tag.Get("accept"))
	if m := sf.Tag.Get("max"); m != "" {
		n, err := parseSize(m)
		if err != nil {
			return fmt.Errorf("max:%q: %w", m, err)
		}
		f.fileMax = n
	}
	return nil
}

// parseSize reads a size: 5MB, 500KB, 1.5GB, 2048 (bytes).
func parseSize(s string) (int64, error) {
	t := strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	unit := int64(1)
	for _, u := range []struct {
		suffix string
		n      int64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}} {
		if strings.HasSuffix(t, u.suffix) {
			t, unit = strings.TrimSuffix(t, u.suffix), u.n
			break
		}
	}
	n, err := strconv.ParseFloat(t, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("not a size like 5MB")
	}
	return int64(n * float64(unit)), nil
}

// sizeText is n bytes as a person reads it: 5 MB, 500 KB.
func sizeText(n int64) string {
	for _, u := range []struct {
		name string
		n    int64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}} {
		if n >= u.n {
			return strings.TrimSuffix(strconv.FormatFloat(float64(n)/float64(u.n), 'f', 1, 64), ".0") + " " + u.name
		}
	}
	return strconv.FormatInt(n, 10) + " bytes"
}

// acceptText is an accept: list as a message says it: "an image", "a PDF
// or DOCX file".
func acceptText(accept string) string {
	var kinds, codes []string
	for _, a := range strings.Split(accept, ",") {
		a = strings.TrimSpace(strings.ToLower(a))
		switch {
		case a == "":
		case strings.HasSuffix(a, "/*"):
			switch a {
			case "image/*":
				kinds = append(kinds, "an image")
			case "video/*":
				kinds = append(kinds, "a video")
			case "audio/*":
				kinds = append(kinds, "an audio file")
			default:
				kinds = append(kinds, "a "+strings.TrimSuffix(a, "/*")+" file")
			}
		case strings.HasPrefix(a, "."):
			codes = append(codes, strings.ToUpper(a[1:]))
		default:
			_, sub, _ := strings.Cut(a, "/")
			codes = append(codes, strings.ToUpper(sub))
		}
	}
	if len(codes) > 0 {
		kinds = append(kinds, "a "+orList(codes)+" file")
	}
	return orList(kinds)
}

func orList(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
}

// accepts reports whether a file of name and types (declared, sniffed)
// is one accept lists; an empty list takes any.
func accepts(accept, name string, types ...string) bool {
	if strings.TrimSpace(accept) == "" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	for _, a := range strings.Split(accept, ",") {
		a = strings.TrimSpace(strings.ToLower(a))
		for _, t := range types {
			t, _, _ = mime.ParseMediaType(t)
			switch {
			case a == "":
			case strings.HasPrefix(a, "."):
			case strings.HasSuffix(a, "/*") && strings.HasPrefix(t, strings.TrimSuffix(a, "*")):
				return true
			case a == t:
				return true
			}
		}
		if strings.HasPrefix(a, ".") && a == ext {
			return true
		}
	}
	return false
}

// seal signs payload's parts for purpose: the parts base64url-encoded, a
// dot, and the signature.
func seal(key []byte, purpose string, parts ...string) string {
	body := base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, "\x00")))
	return body + "." + sealMAC(key, purpose, body)
}

func sealMAC(key []byte, purpose, body string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(purpose + "\x00" + body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:18])
}

// unseal is a sealed value's n parts; ok is false when it isn't one
// signed for purpose, or has expired (its last part, a Unix time).
func unseal(key []byte, purpose, sealed string, n int) ([]string, bool) {
	body, sig, ok := strings.Cut(sealed, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(sealMAC(key, purpose, body))) {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return nil, false
	}
	parts := strings.Split(string(raw), "\x00")
	if len(parts) != n {
		return nil, false
	}
	exp, err := strconv.ParseInt(parts[n-1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return nil, false
	}
	return parts, true
}

func expiry() string { return strconv.FormatInt(time.Now().Add(FileTTL).Unix(), 10) }

// ticket is what the upload of a File field's file presents: the form and
// field it is for, its largest size and what it accepts.
func (b boundField) ticket(ctx context.Context) string {
	return seal(formKeyOf(ctx), "vk-upload", b.st.identity(), b.f.path, strconv.FormatInt(b.f.fileMax, 10), b.f.accept, expiry())
}

// An Uploaded is a file Upload stored: the reference its field takes, its
// name and size.
type Uploaded struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// An UploadError is why Upload refused a file: Rule is "max" (too large),
// "accept" (not what the field takes) or "failed" (the ticket isn't valid,
// or storing failed); Status the HTTP status saying so.
type UploadError struct {
	Rule   string
	Status int
	Err    error
}

func (e *UploadError) Error() string {
	if e.Err != nil {
		return "ui: upload: " + e.Rule + ": " + e.Err.Error()
	}
	return "ui: upload: " + e.Rule
}

func (e *UploadError) Unwrap() error { return e.Err }

// Upload stores a file sent for a File field, what body reads, presented
// with the ticket the field rendered (its _vk_upload field), the name and
// type the browser gave it. It reads no more than the field's largest size
// and one byte: a larger file is refused as it streams. Package web
// answers uploads with it.
func Upload(ctx context.Context, ticket, name, contentType string, body io.Reader) (Uploaded, error) {
	parts, ok := unseal(formKeyOf(ctx), "vk-upload", ticket, 5)
	if !ok {
		return Uploaded{}, &UploadError{Rule: "failed", Status: http.StatusBadRequest, Err: errors.New("the ticket isn't valid")}
	}
	identity, path, accept := parts[0], parts[1], parts[3]
	max, _ := strconv.ParseInt(parts[2], 10, 64)
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	head := make([]byte, 512)
	n, err := io.ReadFull(body, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return Uploaded{}, &UploadError{Rule: "failed", Status: http.StatusBadRequest, Err: err}
	}
	head = head[:n]
	sniffed := http.DetectContentType(head)
	if !accepts(accept, name, contentType, sniffed) {
		return Uploaded{}, &UploadError{Rule: "accept", Status: http.StatusUnsupportedMediaType}
	}
	ctype := sniffed
	if strings.HasPrefix(ctype, "application/octet-stream") && contentType != "" {
		ctype = contentType
	}
	counted := &countReader{r: io.LimitReader(io.MultiReader(bytes.NewReader(head), body), max+1)}
	store := fileStoreOf(ctx)
	id, err := store.Put(counted)
	if err != nil {
		return Uploaded{}, &UploadError{Rule: "failed", Status: http.StatusInternalServerError, Err: err}
	}
	if counted.n > max {
		store.Delete(id)
		return Uploaded{}, &UploadError{Rule: "max", Status: http.StatusRequestEntityTooLarge}
	}
	size := strconv.FormatInt(counted.n, 10)
	return Uploaded{Ref: seal(formKeyOf(ctx), "vk-file", identity, path, id, name, size, ctype, expiry()), Name: name, Size: counted.n}, nil
}

type countReader struct {
	r io.Reader
	n int64
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// bindFile sets a File field from what the form sent: its reference, or
// why its upload failed.
func (st *formState) bindFile(ctx context.Context, v reflect.Value, f *formField, vs []string, vals map[string][]string) {
	v.SetZero()
	if errs := vals[FileErrorField+"."+f.path]; len(errs) > 0 && errs[len(errs)-1] != "" {
		st.fileErr(f, errs[len(errs)-1])
		return
	}
	ref := ""
	if len(vs) > 0 {
		ref = strings.TrimSpace(vs[len(vs)-1])
	}
	if ref == "" {
		return
	}
	parts, ok := unseal(formKeyOf(ctx), "vk-file", ref, 7)
	if !ok || parts[0] != st.identity() || parts[1] != f.path {
		st.fileErr(f, "gone")
		return
	}
	store := fileStoreOf(ctx)
	r, err := store.Open(parts[2])
	if err != nil {
		st.fileErr(f, "gone")
		return
	}
	r.Close()
	size, _ := strconv.ParseInt(parts[4], 10, 64)
	v.Set(reflect.ValueOf(File{ref: ref, id: parts[2], name: parts[3], size: size, ctype: parts[5], store: store}))
}

// fileErr records why a File field holds no file: kind is "max",
// "accept", "gone" or "failed".
func (st *formState) fileErr(f *formField, kind string) {
	switch kind {
	case "max", "accept", "gone":
	default:
		kind = "failed"
	}
	if st.parsed == nil {
		st.parsed = map[string]forms.Error{}
	}
	e := forms.Error{Path: f.path, Rule: "file", Params: map[string]string{"kind": kind, "size": sizeText(f.fileMax), "types": acceptText(f.accept)}}
	if k := f.msgs["file"]; k != "" {
		e.Key = k
	}
	st.parsed[f.path] = e
}

// fileControl is a File field's control: the reference the form carries
// (data-vk-lazy: the runtime sends it on change), the file it holds
// ("photo.jpg ✓ Change", hidden while it holds none, for the runtime to
// fill), and the file input the runtime sends a chosen file from — posted
// as multipart without script.
func (b boundField) fileControl(ctx context.Context, opts Field, invalid bool) Node {
	f, st := b.f, b.st
	var held File
	if v, err := b.value().FieldByIndexErr(f.index); err == nil {
		held, _ = v.Interface().(File)
	}
	id := st.controlID(f.path)
	hidden := func(name string, v any, extra ...Attr) Node {
		return El("input", append([]Attr{{"type", "hidden"}, {"name", name}, {"value", v}}, extra...))
	}
	cls := "vk-file__input"
	if invalid {
		cls += " vk-file__input--invalid"
	}
	if !held.Empty() {
		cls += " vk-file__input--held"
	}
	input := []Attr{
		{"type", "file"},
		{"class", cls},
		{"id", id},
		{"name", FileField + "." + f.path},
		{"accept", nonEmpty(f.accept)},
		{"aria-describedby", lazyValue{func(context.Context) any { return st.describedBy(f) }}},
		{"aria-invalid", lazyValue{func(context.Context) any {
			if _, ok := st.visible(f.path); ok {
				return "true"
			}
			return nil
		}}},
	}
	if opts.Disabled {
		input = append(input, Attr{"disabled", true})
	}
	input = append(input, opts.Attrs...)
	shown := El("p", []Attr{{"class", "vk-file__held"}, {"hidden", held.Empty()}},
		El("span", []Attr{{"class", "vk-file__name"}}, Text(held.Name())),
		El("span", []Attr{{"class", "vk-file__ok"}, {"aria-hidden", "true"}}, Text(" ✓ ")),
		El("label", []Attr{{"class", "vk-file__change"}, {"for", id}}, Text(tr(ctx, "form.file.change")),
			El("span", []Attr{{"class", "vk-sr-only"}}, Text(" "+tr(ctx, cmpOr(opts.Label, f.label))))))
	return El("div", []Attr{{"class", "vk-file"}, {"data-vk-file", f.path}},
		hidden(f.path, lazyValue{func(context.Context) any {
			st.renderField(f.path, "")
			return held.ref
		}}, Attr{"data-vk-lazy", true}),
		hidden(FileTicketField+"."+f.path, lazyValue{func(ctx context.Context) any { return b.ticket(ctx) }}),
		hidden(FileErrorField+"."+f.path, ""),
		shown,
		El("input", input),
		El("progress", []Attr{{"class", "vk-file__progress"}, {"max", "100"}, {"hidden", true}}),
	)
}
