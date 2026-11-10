package racun

import (
	"errors"
	"strings"
)

// Errors a call to CIS can end with. A refusal is told apart from a server
// fault, from an answer that can't be trusted, and from a call that got no
// readable answer. All but a refusal say whether the request reached CIS,
// the only case where CIS may have recorded it.
//
// A receipt is issued to the customer whatever the call ends with; without
// a JIR it is sent again later with LateDelivery set.
var (
	// ErrCIS is a refusal: a signed answer with errors, whose Code is the
	// first error's code (such as "s005"), or a SOAP client fault, whose
	// Code is the fault code. CIS recorded nothing.
	ErrCIS = newError("cis")
	// ErrServer is a SOAP fault CIS raised on its own side (fault code
	// Server or Server.*). CIS got the request, and a fault does not prove
	// it recorded nothing, so RequestWritten is true.
	ErrServer = newError("server")
	// ErrConnection is a call that got no readable answer: the connection
	// failed, timed out or dropped, CIS answered with a redirect, or with
	// an HTTP error and no answer message, or the answer can't be read.
	// When RequestWritten is false nothing reached CIS. When it is true
	// CIS may have recorded the receipt.
	ErrConnection = newError("connection")
	// ErrSignature is an answer whose signature does not verify, is
	// missing, or comes from a certificate other than the CIS one that the
	// CA bundle trusts (fiskalcis, or fiskalcistest in sandbox). CIS
	// answered, so RequestWritten is true, but nothing in the answer can be
	// relied on, including whether CIS recorded the receipt.
	ErrSignature = newError("signature")
)

// Error is a structured error from a call to CIS.
type Error struct {
	key            string
	code           string
	message        string
	requestWritten bool
	cause          error
}

func newError(key string) *Error {
	return &Error{key: key}
}

// Error produces a human readable error message.
func (e *Error) Error() string {
	out := []string{e.key}
	if e.code != "" {
		out = append(out, e.code)
	}
	if e.message != "" {
		out = append(out, e.message)
	}
	return strings.Join(out, ": ")
}

// Key returns the key for the error.
func (e *Error) Key() string {
	return e.key
}

// Code returns the code CIS or the HTTP layer gave, if any.
func (e *Error) Code() string {
	return e.code
}

// Message returns the human message for the error.
func (e *Error) Message() string {
	return e.message
}

// RequestWritten reports whether the request reached CIS, so that CIS may
// have recorded it.
func (e *Error) RequestWritten() bool {
	return e.requestWritten
}

// WithCode duplicates and adds the code to the error.
func (e *Error) WithCode(code string) *Error {
	e = e.clone()
	e.code = code
	return e
}

// WithMessage duplicates and adds the message to the error.
func (e *Error) WithMessage(msg string) *Error {
	e = e.clone()
	e.message = msg
	return e
}

// WithRequestWritten duplicates and sets whether the request was written.
func (e *Error) WithRequestWritten(written bool) *Error {
	e = e.clone()
	e.requestWritten = written
	return e
}

// WithCause duplicates and adds the underlying error.
func (e *Error) WithCause(err error) *Error {
	e = e.clone()
	e.cause = err
	return e
}

// Unwrap returns the underlying error, if any.
func (e *Error) Unwrap() error {
	return e.cause
}

func (e *Error) clone() *Error {
	ne := new(Error)
	*ne = *e
	return ne
}

// Is checks whether the target error has the same key.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.key == t.key
}

// CodeOf returns the code of a CIS refusal, such as "s005", or the fault
// code of a SOAP client fault. It returns "" for any other error.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) && e.key == ErrCIS.key {
		return e.code
	}
	return ""
}
