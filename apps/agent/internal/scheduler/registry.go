package scheduler

import (
	"errors"
	"fmt"
	"sync"
)

// Registry errors returned by MapRegistry.Add (and carried by the panic of Register).
var (
	ErrNilDescriptor         = errors.New("scheduler: nil descriptor")
	ErrInvalidDescriptorName = errors.New("scheduler: invalid descriptor name")
	ErrDuplicateDescriptor   = errors.New("scheduler: duplicate descriptor name")
)

// MapRegistry is the Registry implementation: a map from descriptor name to Descriptor that
// also remembers registration order (the deterministic tie-breaker for plan ordering). It is
// safe for concurrent use.
type MapRegistry struct {
	mu     sync.RWMutex
	byName map[string]Descriptor
	order  []string
}

var _ Registry = (*MapRegistry)(nil)

// NewRegistry returns an empty registry.
func NewRegistry() *MapRegistry {
	return &MapRegistry{byName: make(map[string]Descriptor)}
}

// Add registers d, returning ErrNilDescriptor, ErrInvalidDescriptorName or
// ErrDuplicateDescriptor (wrapped with the offending name) instead of panicking.
func (r *MapRegistry) Add(d Descriptor) error {
	if d == nil {
		return ErrNilDescriptor
	}
	name := d.Name()
	if !ValidName(name) {
		return fmt.Errorf("%w: %q", ErrInvalidDescriptorName, name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("%w: %q", ErrDuplicateDescriptor, name)
	}
	r.byName[name] = d
	r.order = append(r.order, name)
	return nil
}

// Register implements Registry. It panics with the error Add would return.
func (r *MapRegistry) Register(d Descriptor) {
	if err := r.Add(d); err != nil {
		panic(err)
	}
}

// Get returns the descriptor registered under name.
func (r *MapRegistry) Get(name string) (Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byName[name]
	return d, ok
}

// ForKey returns the descriptor that owns k (looked up by k.Descriptor()).
func (r *MapRegistry) ForKey(k Key) (Descriptor, bool) {
	return r.Get(k.Descriptor())
}

// Descriptors returns all descriptors in registration order.
func (r *MapRegistry) Descriptors() []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Descriptor, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name])
	}
	return out
}

// Names returns all descriptor names in registration order.
func (r *MapRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Len returns the number of registered descriptors.
func (r *MapRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.order)
}

// ValidName reports whether name is a legal descriptor name: one or more dot-separated
// segments, each starting with a lower-case letter, continuing with lower-case letters,
// digits or "-" and not ending in "-"; at most 64 bytes. "/" is never allowed because it separates the name from
// the object id in a Key.
func ValidName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	startOfSegment, prev := true, byte(0)
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
			startOfSegment = false
		case (c >= '0' && c <= '9') || c == '-':
			if startOfSegment {
				return false
			}
		case c == '.':
			if startOfSegment || prev == '-' {
				return false // empty segment, leading dot or segment ending in "-"
			}
			startOfSegment = true
		default:
			return false
		}
		prev = c
	}
	return !startOfSegment && prev != '-' // no trailing dot or hyphen
}
