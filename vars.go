package ui

import "context"

// Template variables — what an app's context processors give every
// template (package web's ContextProcessors) — are tracked by a live
// session the way Assigns are: each read is recorded against the instance
// whose render made it, with the version of the value it read, and an
// instance whose state and props didn't change renders again only when a
// variable it read has another version now. Package web reads the
// variables and keeps their versions; ui only records and compares.

// VarSource is what keeps a render's template variables: package web's
// store of its context processors' results. A live session asks it, before
// keeping an instance's last render, for the current version of each
// variable that render read.
type VarSource interface {
	// VarVersion is the version of the variable name as a read with ctx
	// would find it now: the same version for the same value. It may run
	// the variable's processor again when what that processor read
	// changed.
	VarVersion(ctx context.Context, name string) uint64
}

type varSourceKey struct{}

// WithVarSource is ctx whose renders read their template variables from
// src.
func WithVarSource(ctx context.Context, src VarSource) context.Context {
	return context.WithValue(ctx, varSourceKey{}, src)
}

// VarSourceOf is the VarSource of ctx, nil when it has none.
func VarSourceOf(ctx context.Context) VarSource {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(varSourceKey{}).(VarSource)
	return s
}

// ReadVar records that the render of ctx read the template variable name
// at version ver: a live session renders the instance making the read
// again when the variable's version moves. It does nothing outside a live
// render.
func ReadVar(ctx context.Context, name string, ver uint64) {
	if h, ok := liveHost(ctx).(interface{ VarRead(string, uint64) }); ok {
		h.VarRead(name, ver)
	}
}

// ReadContext records that the render of ctx read something of its
// context no version follows (a value overridden for part of the page): a
// live session never keeps that instance's last render on its state alone.
func ReadContext(ctx context.Context) {
	if h, ok := liveHost(ctx).(interface{ ContextRead() }); ok {
		h.ContextRead()
	}
}

// PageHeader is what the page being rendered says of itself: its title and
// breadcrumbs, as PageTitle and PageCrumbs read them.
type PageHeader struct {
	Title  string
	Crumbs []Crumb
}

// PageVars is PageContext's result: the template variable page.
type PageVars struct {
	Page PageHeader
}

// PageContext is a context processor (package web's ContextProcessors)
// giving templates the variable page: the page's Title and Crumbs, which a
// layout's header shows. It reads them as PageTitle and PageCrumbs do.
func PageContext(ctx context.Context) (PageVars, error) {
	return PageVars{Page: PageHeader{Title: PageTitle(ctx), Crumbs: PageCrumbs(ctx)}}, nil
}
