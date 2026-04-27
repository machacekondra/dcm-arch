package validation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dcm-io/dcm/pkg/apis/meta"
)

// Result accumulates validation errors. All errors are collected before
// returning, so the caller sees every problem at once.
type Result struct {
	errors []string
}

// AddError records a validation error with a field path and message.
func (r *Result) AddError(field, msg string) {
	r.errors = append(r.errors, fmt.Sprintf("%s: %s", field, msg))
}

// AddErrorf records a formatted validation error.
func (r *Result) AddErrorf(field, format string, args ...any) {
	r.AddError(field, fmt.Sprintf(format, args...))
}

// OK returns true if no validation errors were recorded.
func (r *Result) OK() bool {
	return len(r.errors) == 0
}

// Errors returns all validation errors as a single combined error,
// or nil if validation passed.
func (r *Result) Error() error {
	if r.OK() {
		return nil
	}
	return fmt.Errorf("validation failed:\n  - %s", strings.Join(r.errors, "\n  - "))
}

// Messages returns the raw error messages.
func (r *Result) Messages() []string {
	return r.errors
}

// dnsNameRegex matches valid DNS-style names: lowercase alphanumeric, hyphens, dots.
var dnsNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-\.]*[a-z0-9])?$`)

func validateObjectMeta(m meta.ObjectMeta, r *Result) {
	if m.Name == "" {
		r.AddError("metadata.name", "is required")
		return
	}
	if len(m.Name) > 253 {
		r.AddError("metadata.name", "must be 253 characters or fewer")
	}
	if !dnsNameRegex.MatchString(m.Name) {
		r.AddErrorf("metadata.name", "%q is not a valid DNS name (lowercase alphanumeric, hyphens, dots)", m.Name)
	}
}
