package policy

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"ngfw/agent/internal/renderers/frr"
)

// Path is a location in the configuration document: the segments of its RFC 6901 pointer (review M2: a render error
// reports the field, not /routing). Record keys are segments of their own, so a key with dots (an IPv4 neighbour) stays
// one segment.
type Path []string

// P builds a Path.
func P(segs ...string) Path { return Path(segs) }

// At returns p extended by segs (p is not modified).
func (p Path) At(segs ...string) Path { return append(slices.Clone(p), segs...) }

// Index returns p extended by an array index.
func (p Path) Index(i int) Path { return p.At(fmt.Sprint(i)) }

// String is the dotted form used in messages ("routing.bgp.neighbors.10.0.0.1.description").
func (p Path) String() string { return strings.Join(p, ".") }

// Pointer is the RFC 6901 pointer ("/routing/bgp/neighbors/10.0.0.1/description").
func (p Path) Pointer() string {
	var b strings.Builder
	for _, s := range p {
		b.WriteByte('/')
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(s))
	}
	return b.String()
}

// FieldError is an input error at one field of the document. It wraps frr.ErrInput and the cause.
type FieldError struct {
	Path Path
	Err  error
}

func (e *FieldError) Error() string   { return fmt.Sprintf("%v: %s: %v", frr.ErrInput, e.Path, e.Err) }
func (e *FieldError) Unwrap() []error { return []error{frr.ErrInput, e.Err} }

// Errf is a FieldError at p with a formatted cause.
func Errf(p Path, format string, a ...any) error {
	return &FieldError{Path: p, Err: fmt.Errorf(format, a...)}
}

// Wrap is a FieldError at p around err (err unchanged when it already is a FieldError).
func Wrap(p Path, err error) error {
	var fe *FieldError
	if errors.As(err, &fe) {
		return err
	}
	return &FieldError{Path: p, Err: err}
}
