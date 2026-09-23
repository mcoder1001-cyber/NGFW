package df6

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/interface_types"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// NoInterface is VPP's "no interface" sw_if_index (~0).
const NoInterface = ^uint32(0)

// Interfaces is one sw_interface_dump snapshot seen by one owner. Interface references in DF-6
// models are LOGICAL names (D-065/D-069): our tag id for interfaces an agent descriptor created,
// VPP's name for untagged (physical / pre-existing) ones. Resolution goes through DF-1's one
// resolver (iface.Table.IndexByName), which refuses interfaces tagged by another owner
// (iface.ErrForeignInterface); Retrieve reports logical names (iface.Table.Logical).
type Interfaces struct {
	t *iface.Table
}

// DumpInterfaces dumps every interface of VPP for owner.
func DumpInterfaces(ctx context.Context, c vpp.Client, owner string) (*Interfaces, error) {
	t, err := iface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	return &Interfaces{t: t}, nil
}

// Table returns DF-1's table behind the snapshot.
func (s *Interfaces) Table() *iface.Table { return s.t }

// Index resolves a logical interface name. Errors wrap ErrNoSuchInterface and DF-1's
// iface.ErrNotFound / iface.ErrForeignInterface.
func (s *Interfaces) Index(name string) (interface_types.InterfaceIndex, error) {
	idx, err := s.t.IndexByName(name)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrNoSuchInterface, err)
	}
	return interface_types.InterfaceIndex(idx), nil
}

// OptionalIndex resolves an optional interface reference: "" is NoInterface.
func (s *Interfaces) OptionalIndex(name string) (interface_types.InterfaceIndex, error) {
	if name == "" {
		return interface_types.InterfaceIndex(NoInterface), nil
	}
	return s.Index(name)
}

// Name returns VPP's name of sw_if_index idx.
func (s *Interfaces) Name(idx uint32) (string, bool) {
	if _, ok := s.t.Details(idx); !ok {
		return "", false
	}
	return s.t.VPPName(idx), true
}

// NameOrEmpty returns the logical name of idx for this owner, "" for NoInterface, unknown
// indexes and interfaces of other owners.
func (s *Interfaces) NameOrEmpty(idx uint32) string {
	if idx == NoInterface {
		return ""
	}
	n, _ := s.t.Logical(idx)
	return n
}

// OwnedID returns the object id from the owner tag of idx, ok when tagged by this owner.
func (s *Interfaces) OwnedID(idx uint32) (string, bool) { return s.t.OwnedID(idx) }

// Owned reports whether the interface carries this owner's tag.
func (s *Interfaces) Owned(idx uint32) bool {
	_, ok := s.t.OwnedID(idx)
	return ok
}

// IndexByTag returns the interface tagged "<owner>:<id>" (the object's own interface).
func (s *Interfaces) IndexByTag(id string) (uint32, bool) {
	for _, idx := range s.t.Indexes() {
		if got, ok := s.t.OwnedID(idx); ok && got == id {
			return idx, true
		}
	}
	return 0, false
}

// Owns reports whether per-interface objects of holder on idx are ours: our tagged interface,
// or an untagged interface holder has claimed (D-071 claim rule).
func (s *Interfaces) Owns(idx uint32, holder string) bool { return s.t.Owns(idx, holder) }

// ClaimIfUntagged records holder's claim on an untagged idx.
func (s *Interfaces) ClaimIfUntagged(idx uint32, holder string) error {
	return s.t.ClaimIfUntagged(idx, holder)
}

// Untagged reports whether idx is an untagged (physical / pre-existing) interface.
func (s *Interfaces) Untagged(idx uint32) bool { return s.t.Untagged(idx) }
