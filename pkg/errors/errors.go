package errors

import (
	"errors"
	"fmt"
)

// Invalid is a behavioural error type indicating an invalid value or
// configuration. Use this to indicate to users that they need to change some
// configuration in order to proceed.
//
// Examples of such errors are unknown config map names or empty keys.
type Invalid struct {
	Err error
}

func (i *Invalid) Error() string {
	return fmt.Sprintf("%v", i.Err)
}

func (i *Invalid) Invalid() bool {
	return true
}

func (i *Invalid) Unwrap() error {
	return i.Err
}

// NewInvalid returns an Invalid error with err inside. If err is nil, nil is
// returned.
func NewInvalid(err error) error {
	if err == nil {
		return nil
	}
	return &Invalid{
		Err: err,
	}
}

// IsInvalid returns whether err is an Invalid error.
func IsInvalid(err error) bool {
	var invalidErr interface {
		Invalid() bool
	}
	return errors.As(err, &invalidErr) && invalidErr.Invalid()
}

// Temporary is a behavioural error type indicating an temporary error
// condition. Use this to indicate to users that an error occoured but the
// controller will keep trying to recover.
//
// Examples of such errors are unknown config map names or unreachable hosts.
type Temporary struct {
	Err error
}

func (i *Temporary) Error() string {
	return fmt.Sprintf("%v", i.Err)
}

func (i *Temporary) Temporary() bool {
	return true
}

func (i *Temporary) Unwrap() error {
	return i.Err
}

// NewTemporary returns an Temporary error with err inside. If err is nil, nil is
// returned.
func NewTemporary(err error) error {
	if err == nil {
		return nil
	}
	return &Temporary{
		Err: err,
	}
}

// IsTemporary returns whether err is an Temporary error.
func IsTemporary(err error) bool {
	var temporaryError interface {
		Temporary() bool
	}
	return errors.As(err, &temporaryError) && temporaryError.Temporary()
}
