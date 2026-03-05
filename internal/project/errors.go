package project

import (
	"errors"
	"fmt"
)

// TODO deprecate
type BuildError struct {
	Errors []error
}

func (e *BuildError) Error() string {
	if len(e.Errors) == 0 {
		return "no build errors"
	}
	return fmt.Sprintf("%d build errors", len(e.Errors))
}

func (e *BuildError) Unwrap() error {
	return errors.Join(e.Errors...)
}
