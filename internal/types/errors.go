package types

import (
	"errors"
	"fmt"
)

var (
	ErrDataNotFound     = errors.New("data not found")
	ErrInvalidFormat    = errors.New("invalid format")
	ErrNetworkError     = errors.New("network error")
	ErrPermissionDenied = errors.New("permission denied")
	ErrInvalidConfig    = errors.New("invalid configuration")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error in field %s: %s", e.Field, e.Message)
}

type LoaderError struct {
	Path string
	Err  error
}

func (e LoaderError) Error() string {
	return fmt.Sprintf("failed to load from %s: %v", e.Path, e.Err)
}

func (e LoaderError) Unwrap() error {
	return e.Err
}

type ParseError struct {
	Line int
	Err  error
}

func (e ParseError) Error() string {
	return fmt.Sprintf("parse error at line %d: %v", e.Line, e.Err)
}

func (e ParseError) Unwrap() error {
	return e.Err
}
