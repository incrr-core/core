package main

import (
	"strings"
)

type (

	// ErrParseUint64 initiates the ParseUint64 error
	ErrParseUint64 interface {
		error
	}

	// ErrStr the constant error type
	errStr string

	errErr error

	// ErrInternalService initiates the HTTP Internal Service Error behavior
	ErrInternalService struct{ errErr }

	// ErrBadRequest initiates the HTTP Bad Request Error behavior
	ErrBadRequest struct{ errErr }
)

// Error satisfies the error interface
func (e errStr) Error() string { return string(e) }

func (e ErrBadRequest) Error() string {
	s := e.errErr.Error()
	s = strings.TrimSuffix(s, "& <nil>")
	return s
}

const errNumLessThan errStr = "the number was not incremented"
const errMaxIncrementRange errStr = "exhausted max number increments"
const errSkipNilValue errStr = "nil Skip value"
