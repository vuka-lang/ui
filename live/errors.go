package live

import "errors"

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

// publicErr is an error of the session's own whose text the browser may
// see: an event's payload that doesn't parse.
type publicErr struct{ error }

func (e publicErr) PublicMessage() string { return e.Error() }
func (e publicErr) Unwrap() error         { return e.error }

// SetErrorText sets how the session turns an error into the text a
// message to the browser carries: what a handler, Mount or Update
// returned, what it panicked with, and a render's or the session's own
// failure. Without it the text is err.Error(), which may say more than
// the browser should see (a query, a path, a panic's value): a transport
// serving pages to the public sets it, showing PublicMessage (or a
// generic message) and logging the rest.
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
