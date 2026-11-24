package errors

import (
	oe "errors"
	"fmt"
	"runtime"
)

func New(text string) error {
	_, file, line, _ := runtime.Caller(1)
	return fmt.Errorf("%s at %s:%d", text, file, line)
}

func Wrap(err error) error {
	_, file, line, _ := runtime.Caller(1)
	return fmt.Errorf("at %s:%d\n%w", file, line, err)
}

func Is(err, target error) bool {
	return oe.Is(err, target)
}
