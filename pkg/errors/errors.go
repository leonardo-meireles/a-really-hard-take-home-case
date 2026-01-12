// Package errors provides error wrapping utilities for context-aware error messages.
package errors

import "fmt"

// Wrap wraps an error with additional context information.
// If err is nil, it returns nil without wrapping.
func Wrap(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}
