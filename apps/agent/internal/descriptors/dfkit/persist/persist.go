// Package persist is the product agent's guard against ownership records that do not survive an
// agent restart (TD-11b, review 3.2; D-075, D-076, D-080). A claim store records which untagged
// objects are ours (D-071); a boot store records which non-idempotent adds were applied on this VPP
// instance (D-076). Every descriptor family defaults to an in-memory store so that unit tests need no
// state dir — but an in-memory store in the product agent forgets every claim on an agent restart,
// and the agent then neither reports nor deletes its own objects on untagged interfaces.
//
// The protocol is structural, so that no descriptor package has to import another:
//
//   - a store that survives an agent restart has the method Persistent() bool (returning true):
//     subsystems.IfaceClaims / KeyedClaims / PairClaims, dfkit.FileBootStore, df2.FileClaimStore,
//     df6.FileClaimStore;
//   - a descriptor that records ownership has the method CheckPersistent() error, which returns
//     Require(...) over every store it records in (natcommon.Descriptor, the DF-1 descriptors,
//     df6's keyed and bypass descriptors; df2 and dfkit consumers call df2.Options.CheckPersistent,
//     dfkit.CheckClaims and dfkit.CheckBoot from theirs);
//   - a descriptor that records no ownership in any store — its ownership is what VPP itself
//     carries: an owner tag, an owner-prefixed name — declares it with the method
//     RecordsNoOwnership() (core loopback/VRF, af_packet);
//   - subsystems.Register runs Declared and Check over every descriptor it registered and refuses
//     to start the agent when a descriptor declares neither (TD-11b fix round 1, review M1: the
//     guard is complete, a family that forgets the declaration fails its row's CI) or records in a
//     volatile store.
package persist

import (
	"errors"
	"fmt"
	"reflect"

	"ngfw/agent/internal/scheduler"
)

// Store is implemented by claim and boot stores that survive an agent restart.
type Store interface{ Persistent() bool }

// Checker is implemented by descriptors that record ownership claims or applied-once records.
type Checker interface{ CheckPersistent() error }

// NoOwnership is implemented by descriptors that record no ownership in any claim, boot or owner
// store (their ownership is the owner tag or name VPP itself carries).
type NoOwnership interface{ RecordsNoOwnership() }

// ErrUndeclared means a descriptor declares neither CheckPersistent nor RecordsNoOwnership: the
// guard cannot tell whether it records ownership, so the product agent refuses it.
var ErrUndeclared = errors.New("descriptor declares neither CheckPersistent (it records ownership) nor RecordsNoOwnership")

// ErrConflictingDeclaration means a descriptor declares both.
var ErrConflictingDeclaration = errors.New("descriptor declares both CheckPersistent and RecordsNoOwnership")

// Declared returns nil when d, or a descriptor d wraps (see Check), declares how it records
// ownership — exactly one of Checker and NoOwnership on the same value — and ErrUndeclared or
// ErrConflictingDeclaration otherwise.
func Declared(d any) error {
	for depth := 0; d != nil && depth < 16; depth++ {
		_, checks := d.(Checker)
		_, none := d.(NoOwnership)
		switch {
		case checks && none:
			return fmt.Errorf("%w: %T", ErrConflictingDeclaration, d)
		case checks || none:
			return nil
		}
		d = unwrap(d)
	}
	return ErrUndeclared
}

// ErrVolatile means a descriptor records ownership in a store that does not survive an agent
// restart (an in-memory default).
var ErrVolatile = errors.New("ownership store does not survive an agent restart (in memory)")

// Is reports whether s survives an agent restart: it implements Store and says so. nil does not.
func Is(s any) bool {
	p, ok := s.(Store)
	return ok && !isNil(s) && p.Persistent()
}

// Require returns nil when every store survives an agent restart, otherwise ErrVolatile naming what
// (e.g. "natcommon nat44-ed.static-mapping: claims") and the type of the first store that does not.
func Require(what string, stores ...any) error {
	for _, s := range stores {
		if !Is(s) {
			return fmt.Errorf("%w: %s: %T", ErrVolatile, what, s)
		}
	}
	return nil
}

// Check runs CheckPersistent of d and of every descriptor d wraps (a wrapper's Unwrap() method, or
// an embedded scheduler.Descriptor field — the usual decorator shape), and joins their errors.
func Check(d any) error {
	var errs []error
	for depth := 0; d != nil && depth < 16; depth++ {
		if c, ok := d.(Checker); ok {
			if err := c.CheckPersistent(); err != nil {
				errs = append(errs, err)
			}
		}
		d = unwrap(d)
	}
	return errors.Join(errs...)
}

var descriptorType = reflect.TypeFor[scheduler.Descriptor]()

// unwrap returns the descriptor d decorates, or nil.
func unwrap(d any) any {
	if u, ok := d.(interface{ Unwrap() scheduler.Descriptor }); ok {
		if inner := u.Unwrap(); !isNil(inner) {
			return inner
		}
		return nil
	}
	v := reflect.ValueOf(d)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if !f.Anonymous || f.Type != descriptorType || !f.IsExported() {
			continue
		}
		if inner := v.Field(i); !inner.IsNil() {
			return inner.Interface()
		}
	}
	return nil
}

func isNil(x any) bool {
	if x == nil {
		return true
	}
	switch v := reflect.ValueOf(x); v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return v.IsNil()
	}
	return false
}
