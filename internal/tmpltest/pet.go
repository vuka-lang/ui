// Package tmpltest holds the components ui's template tests render: each
// written in a .vuka.html file, compiled with ui as their target.
package tmpltest

import (
	"errors"

	"github.com/vuka-lang/ui"
)

type Pet struct {
	Name string
	Age  int
}

// Avatar is a function component that can fail: an empty url fails the
// render.
func Avatar(url string) (ui.Node, error) {
	if url == "" {
		return nil, errors.New("no avatar")
	}
	return ui.El("img", []ui.Attr{{Name: "src", Value: url}}), nil
}

type DocPet struct {
	ID   int
	Name string
}

type DocPost struct{ Title, URL string }
