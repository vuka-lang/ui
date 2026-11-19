package ui

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

type sharedKey struct{}

type sharedCtx struct {
	name   string
	strict bool
}

// WithSharedFragment marks ctx as the context of the shared fragment name,
// which renders once for every page that shows it (package live, Shared), so
// holds nothing of a viewer. With strict, ViewerRead fails the render.
func WithSharedFragment(ctx context.Context, name string, strict bool) context.Context {
	return context.WithValue(ctx, sharedKey{}, sharedCtx{name, strict})
}

// SharedFragment is the name of the shared fragment ctx renders, if it does.
func SharedFragment(ctx context.Context) (string, bool) {
	c, ok := ctx.Value(sharedKey{}).(sharedCtx)
	return c.name, ok
}

// ViewerRead is called by what reads something of the viewer from a context
// (the request, the session, the identity). In a strict shared fragment it
// fails the render naming what was read; anywhere else it does nothing.
func ViewerRead(ctx context.Context, what string) {
	if ctx == nil {
		return
	}
	if c, ok := ctx.Value(sharedKey{}).(sharedCtx); ok && c.strict {
		Must(struct{}{}, fmt.Errorf("ui: the shared fragment %q reads %s, which differs per viewer; a shared fragment renders once for every page", c.name, what))
	}
}

// SharedComponent is what Broadcast takes as a type: a component that
// embeds SharedLive.
type SharedComponent interface {
	Stateful
	vukaShared()
}

// SharedName is the stable name of a shared component type, the key a
// broadcast to it travels under between replicas: its package path and name.
func SharedName(typ reflect.Type) string {
	return typ.Elem().PkgPath() + "." + typ.Elem().Name()
}

var sharedTypes sync.Map // SharedName → reflect.Type

// RegisterShared, called from an init, tells the runtime that c's type is a
// shared component, so a broadcast that arrives from another replica can
// mount and update it before a page here has shown it. A broadcast sent from
// here (Broadcast) and a page that shows the component register it too.
func RegisterShared(c Stateful) {
	if !IsShared(c) {
		panic(fmt.Sprintf("ui: RegisterShared: %T does not embed ui.SharedLive", c))
	}
	typ := reflect.TypeOf(c)
	sharedTypes.Store(SharedName(typ), typ)
}

// SharedByName is the shared component type SharedName names, if it has been
// registered, broadcast to or shown here.
func SharedByName(name string) (reflect.Type, bool) {
	t, ok := sharedTypes.Load(name)
	if !ok {
		return nil, false
	}
	return t.(reflect.Type), true
}

// SharedBroadcaster delivers a message to a shared component's one instance:
// the app serving a request or a live event, which ctx carries
// (WithSharedBroadcaster).
type SharedBroadcaster interface {
	BroadcastShared(ctx context.Context, typ reflect.Type, msg any) error
}

type broadcasterKey struct{}

// WithSharedBroadcaster is ctx carrying b, for Broadcast.
func WithSharedBroadcaster(ctx context.Context, b SharedBroadcaster) context.Context {
	return context.WithValue(ctx, broadcasterKey{}, b)
}

// Broadcast delivers msg to the Update method taking its type of the one
// instance of the shared component T, mounting it first when no page or
// broadcast has yet. It renders once, and every page showing <T /> gets the
// change as one frame; across replicas it goes through the app's relay.
// T is a pointer to a struct embedding SharedLive, which the compiler checks.
//
//	ui.Broadcast[*PriceBoard](ctx, Price{"TZS", 2510})
//
// ctx is a request's or a live event's.
func Broadcast[T SharedComponent](ctx context.Context, msg any) error {
	typ := reflect.TypeFor[T]()
	b, _ := ctx.Value(broadcasterKey{}).(SharedBroadcaster)
	if b == nil {
		return fmt.Errorf("ui: Broadcast[%s] needs the context of a request or of a live event", typ.Elem().Name())
	}
	sharedTypes.Store(SharedName(typ), typ)
	return b.BroadcastShared(ctx, typ, msg)
}
