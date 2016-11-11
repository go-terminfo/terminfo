package terminfo

import (
	"errors"
	"fmt"
)

var (
	ErrNoTerm                      = errors.New("no TERM set")
	ErrNotImplemented              = errors.New("not implemented")
	ErrTruncatedParametrizedString = errors.New("truncated parametrized string")
	ErrBadParametrizedString       = errors.New("bad parametrized string")
	ErrMissingArgs                 = errors.New("missing args")
)

type ErrBadThing struct {
	Filename string
	Thing    string
	Err      error
}

func (e ErrBadThing) Error() string {
	return fmt.Sprintf("bad %s in terminfo file %q: %v", e.Thing, e.Filename, e.Err)
}
