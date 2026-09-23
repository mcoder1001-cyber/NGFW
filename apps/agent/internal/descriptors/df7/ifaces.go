package df7

import (
	"context"
	"errors"
	"fmt"
	"io"

	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// NoIndex is VPP's "no interface / all" index (~0).
const NoIndex = ^uint32(0)

// Collect drains a generated dump stream: recv is the stream's Recv, which ends with io.EOF
// after the control_ping_reply.
func Collect[T any](recv func() (T, error)) ([]T, error) {
	var out []T
	for {
		d, err := recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
}

// Interfaces is one sw_interface_dump snapshot seen through DF-1's logical-name resolver
// (D-065, D-069): interfaces are named by their LOGICAL name (the owner-tag id of interfaces
// this owner created, VPP's name for untagged/physical ones); interfaces tagged by another
// owner cannot be resolved (iface.ErrForeignInterface) and are never reported. Take one per
// Create/Delete/Retrieve.
//
// Ownership of a per-interface object (D-071 claim rule): on our tagged interface it is ours; on
// an untagged interface only if this owner's ClaimStore (iface.Claims — DF-1's store, which P05
// persists) holds a claim for the object's key; on another owner's interface never.
type Interfaces struct {
	owner string
	t     *iface.Table
}

// DumpInterfaces dumps every interface of VPP.
func DumpInterfaces(ctx context.Context, c vpp.Client, owner string, _ Options) (*Interfaces, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	return &Interfaces{owner: owner, t: t}, nil
}

// Resolve resolves a logical interface name for a reference (a SPAN destination, a next hop, a
// tracked interface): ErrNoSuchInterface when absent, ErrForeignInterface for another owner's.
func (s *Interfaces) Resolve(name string) (uint32, error) { return s.t.IndexByName(name) }

// Attach resolves the interface an object of this owner is put on and, when the interface is
// untagged, records the claim of holder (the object's key) so Retrieve reports the object.
func (s *Interfaces) Attach(name, holder string) (uint32, error) {
	idx, err := s.t.IndexByName(name)
	if err != nil {
		return 0, err
	}
	if err := s.t.ClaimIfUntagged(idx, holder); err != nil {
		return 0, fmt.Errorf("claim %q for %s: %w", name, holder, err)
	}
	return idx, nil
}

// Logical returns the logical name of idx (false for other owners' interfaces, local0 and
// unknown indexes).
func (s *Interfaces) Logical(idx uint32) (string, bool) { return s.t.Logical(idx) }

// Name returns the logical name of idx, or "#<idx>" so a diff shows an unresolvable reference
// instead of silently matching.
func (s *Interfaces) Name(idx uint32) string {
	if n, ok := s.t.Logical(idx); ok {
		return n
	}
	return fmt.Sprintf("#%d", idx)
}

// Owned returns the logical name of idx when an object whose key holder(name) computes is this
// owner's: our tagged interface, or an untagged one with a claim for that key.
func (s *Interfaces) Owned(idx uint32, holder func(name string) string) (string, bool) {
	name, ok := s.t.Logical(idx)
	if !ok || !s.t.Owns(idx, holder(name)) {
		return "", false
	}
	return name, true
}

// OwnedTagged returns the logical name of idx when it carries this owner's tag (objects that
// are owned through their interface without a per-object claim, e.g. MPLS tunnels we create).
func (s *Interfaces) OwnedTagged(idx uint32) (string, bool) { return s.t.OwnedID(idx) }

// Release drops holder's claim on the untagged interface name (no-op otherwise).
func Release(owner, name, holder string) error { return iface.ReleaseRef(owner, name, holder) }

// Reresolve is what every Delete that addresses VPP by sw_if_index does first (D-071: indexes
// are reused after a VPP restart): resolve the object's interface by logical name again and
// check the object is still ours. found is false when the interface is gone (nothing left to
// delete); an interface that resolves but is not ours fails.
func (s *Interfaces) Reresolve(name, holder string) (idx uint32, found bool, err error) {
	idx, err = s.t.IndexByName(name)
	switch {
	case errors.Is(err, iface.ErrNotFound):
		return 0, false, nil
	case err != nil:
		return 0, false, err
	}
	if !s.t.Owns(idx, holder) {
		return 0, false, fmt.Errorf("%w: %q no longer carries %s", ErrForeignInterface, name, holder)
	}
	return idx, true, nil
}
