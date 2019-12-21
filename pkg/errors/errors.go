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
	return errors.As(err, &invalidErr)
}
