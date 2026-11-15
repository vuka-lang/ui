package ui

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"reflect"
)

// ShowOnce is a value shown once and kept on screen until the page is
// left: a secret or a token an action made, shown the one time it exists,
// a flash message, a one-time download link. The server renders it while
// it has the value; the browser keeps what it was given after the server
// no longer has it.
//
//	<ui.ShowOnce id="token" key={p.tokenKey}>
//		<code>{p.token}</code>
//	</ui.ShowOnce>
//
// or in Go, ui.ShowOnce{ID: "token", Key: p.tokenKey, Children: code}. It
// renders one element, always, whose id is ShowOnceID(ID), so it is the
// same place in every render of the page. Key names the value it shows:
// with a Key it holds Children; with none (nil, "" or any zero value) it
// renders empty and Children isn't rendered.
//
// In the browser, a render with no Key, or with the Key it shows, leaves
// the element as it is. The live connection's first render (whether it
// adopted the session the page's request mounted or mounted afresh, where
// a value read from storage that a read empties is gone), an event's
// render, a render the server pushes, a reconnection's and the page
// loaded again in place all keep what is shown: the page needn't hold the
// value to keep it on screen, and the browser never asks for it again. A
// render with another Key replaces it: a second value made on the same
// page takes the first one's place. Give each value its own Key
// (ShowOnceKey makes one) rather than a count: a page loaded again counts
// from the start, and a count the browser already shows wouldn't replace
// what it shows.
//
// The value goes when the page is left: a navigation to another page,
// back or forward included, takes it off even where that page has a
// ShowOnce of the same ID, and the snapshot that back and forward show of
// the page holds the element empty. A navigation within the page (a patch
// of its URL) keeps it.
type ShowOnce struct {
	// ID names the place on the page; two on a page need two.
	ID string
	// Key names the value shown: its id, a ShowOnceKey, the value itself
	// when that may be written into the page. The zero value shows nothing.
	Key any
	// Class is the element's classes.
	Class    string
	Children Node
}

// ShowOnceKey is a fresh Key for a ShowOnce: each call's differs from any
// other's, on this page or on the page loaded again.
func ShowOnceKey() string { return rand.Text() }

// ShowOnceID is the id of the element a ShowOnce of id renders, for a JS
// command or a test to target it.
func ShowOnceID(id string) string { return "vk-once-" + id }

var errShowOnceID = errors.New("ui: ShowOnce needs an ID")

// key is the Key as the element's data-vk-once: "" for none.
func (o ShowOnce) key() string {
	if o.Key == nil || reflect.ValueOf(o.Key).IsZero() {
		return ""
	}
	return toString(o.Key)
}

func (o ShowOnce) View() Node {
	if o.ID == "" {
		return TryNode{Err: errShowOnceID}
	}
	k := o.key()
	attrs := []Attr{{Name: "id", Value: ShowOnceID(o.ID)}, {Name: "data-vk-once", Value: k}}
	if o.Class != "" {
		attrs = append(attrs, Attr{Name: "class", Value: o.Class})
	}
	if k == "" {
		return El("div", attrs)
	}
	return El("div", attrs, o.Children)
}

func (o ShowOnce) Render(ctx context.Context, w io.Writer) error { return o.View().Render(ctx, w) }
