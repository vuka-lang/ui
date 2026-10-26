package live

import (
	"errors"
	"fmt"
)

// PublicError is an error whose message is meant for the person using the
// page: a transport that hides errors' text from the browser (see
// Session.SetErrorText) still shows PublicMessage. A form field that
// doesn't parse is one; an error a web framework answers with a status and
// a message can be one too.
type PublicError interface {
	error
	PublicMessage() string
}

// PublicMessage is the message of the first PublicError in err's chain,
// and whether there is one.
func PublicMessage(err error) (string, bool) {
	var p PublicError
	if errors.As(err, &p) {
		return p.PublicMessage(), true
	}
	return "", false
}

// publicErr is an error of the session's own the browser may read: an
// event's payload that doesn't parse. Its PublicMessage is short, without
// what the full text says of Go values and types (a JSON decoder's
// message); the full text is Error's, for the log and for development.
type publicErr struct {
	error
	short string // "": the full text is public
}

func (e publicErr) PublicMessage() string {
	if e.short == "" {
		return e.Error()
	}
	return e.short
}
func (e publicErr) Unwrap() error { return e.error }

// publicText is err's text for the browser: its PublicMessage, else (an
// error of this package's own whose text is safe) its text.
func publicText(err error) string {
	if m, ok := PublicMessage(err); ok {
		return m
	}
	return err.Error()
}

// NoHandlerError is the error of an event whose target names no handler:
// the browser holds a handler id of a render the session no longer keeps.
// Its PublicMessage is the short form (live: no handler "c3:75"); Error
// says whose handler it was and why it's gone, naming the component's Go
// type, for the server's log and for development.
type NoHandlerError struct {
	Target    string // the event's target, "c3:75"
	Component string // the component's type ("views.Table[…]"), "page" for the page's handlers, "" when no component has the id
	reason    string
}

func (e *NoHandlerError) Error() string {
	return fmt.Sprintf("live: no handler %q: %s", e.Target, e.reason)
}

func (e *NoHandlerError) PublicMessage() string {
	return fmt.Sprintf("live: no handler %q", e.Target)
}

// SetErrorText sets how the session turns an error into the text a
// message to the browser carries: what a handler, Mount or Update
// returned, what it panicked with, and a render's or the session's own
// failure. Without it the text is err.Error(), which may say more than
// the browser should see (a query, a path, a panic's value, a component's
// Go type): a transport serving pages to the public sets it, showing
// PublicMessage (or a generic message) and logging the rest, and the full
// text in development. The session's own errors keep their detail out of
// PublicMessage: a NoHandlerError's is `live: no handler "c3:75"`, a
// payload's that doesn't parse names no Go type.
func (s *Session) SetErrorText(text func(err error) string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errText = text
}

// ErrorText is err's text for the browser, as SetErrorText says.
func (s *Session) ErrorText(err error) string {
	s.mu.Lock()
	text := s.errText
	s.mu.Unlock()
	return s.text(text, err)
}

// errorText is ErrorText for a caller holding the session's lock.
func (s *Session) errorText(err error) string { return s.text(s.errText, err) }

func (s *Session) text(text func(error) string, err error) string {
	if err == nil {
		return ""
	}
	if text == nil {
		return err.Error()
	}
	return text(err)
}
