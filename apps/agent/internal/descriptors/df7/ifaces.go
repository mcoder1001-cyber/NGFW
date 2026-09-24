package df7

import (
	"context"
	"errors"
	"fmt"
	"io"

	"ngfw/agent/internal/descriptors/dfkit"
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
// (D-065, D-069) and dfkit's claim rules (D-071, D-080): interfaces are named by their LOGICAL
// name; interfaces tagged by another owner cannot be resolved (ErrForeignInterface) and are never
// reported; an object on an untagged interface is ours only with a claim of the object's key
// that was recorded after our own successful add, on the running VPP instance (boot identity)
// and for that sw_if_index — never adopted.
type Interfaces struct {
	owner string
	k     *dfkit.Ifaces
}

// DumpInterfaces dumps every interface of VPP.
func DumpInterfaces(ctx context.Context, c vpp.Client, owner string, _ Options) (*Interfaces, error) {
	k, err := dfkit.DumpInterfaces(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	return &Interfaces{owner: owner, k: k}, nil
}

// Resolve resolves a logical interface name for a reference (a SPAN destination, a next hop, a
// tracked interface): ErrNoSuchInterface when absent, ErrForeignInterface for another owner's.
func (s *Interfaces) Resolve(name string) (uint32, error) { return s.k.Resolve(name) }

// Logical returns the logical name of idx (false for other owners' interfaces, local0 and
// unknown indexes).
func (s *Interfaces) Logical(idx uint32) (string, bool) { return s.k.Logical(idx) }

// Name returns the logical name of idx, or "#<idx>" so a diff shows an unresolvable reference
// instead of silently matching.
func (s *Interfaces) Name(idx uint32) string {
	if n, ok := s.k.Logical(idx); ok {
		return n
	}
	return fmt.Sprintf("#%d", idx)
}

// Owned returns the logical name of idx when an object whose key holder(name) computes is this
// owner's: our tagged interface, or an untagged one with this instance's claim for that key.
func (s *Interfaces) Owned(idx uint32, holder func(name string) string) (string, bool) {
	name, ok := s.k.Logical(idx)
	if !ok {
		return "", false
	}
	return s.k.Reportable(idx, holder(name))
}

// OwnedTagged returns the logical name of idx when it carries this owner's tag.
func (s *Interfaces) OwnedTagged(idx uint32) (string, bool) { return s.k.T.OwnedID(idx) }

// Target resolves the interface an object (holder = its key) is about to be put on, WITHOUT
// claiming it: the caller claims (Target.Claim) only after VPP accepted the add (review M1).
func Target(ctx context.Context, c vpp.Client, owner, name, holder string) (dfkit.Target, error) {
	return dfkit.ResolveTarget(ctx, c, name, owner, holder)
}

// Reresolve is what every Delete / Update that addresses VPP by sw_if_index does first (D-071:
// indexes are reused after a VPP restart): resolve the object's interface by logical name again
// and check the object is ours on this VPP instance. found is false when the interface is gone
// or when an untagged interface carries no claim of ours on this instance (after a VPP restart
// or an interface re-creation the object cannot be ours — nothing to touch).
func Reresolve(ctx context.Context, c vpp.Client, owner, name, holder string) (dfkit.Target, bool, error) {
	tg, err := dfkit.ResolveTarget(ctx, c, name, owner, holder)
	switch {
	case errors.Is(err, iface.ErrNotFound):
		return dfkit.Target{}, false, nil
	case err != nil:
		return dfkit.Target{}, false, err
	}
	return tg, tg.Claimed(), nil
}
