package df6

import (
	"context"
	"fmt"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// NoInterface is VPP's "no interface" sw_if_index (~0).
const NoInterface = ^uint32(0)

// Interfaces is one sw_interface_dump snapshot: name ↔ sw_if_index and owner tags. Take one
// per Create/Retrieve and resolve every interface reference from it.
type Interfaces struct {
	owner   string
	byName  map[string]*interfaces.SwInterfaceDetails
	byIndex map[uint32]*interfaces.SwInterfaceDetails
}

// DumpInterfaces dumps every interface of VPP.
func DumpInterfaces(ctx context.Context, c vpp.Client, owner string) (*Interfaces, error) {
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(NoInterface)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	details, err := Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	s := &Interfaces{owner: owner, byName: map[string]*interfaces.SwInterfaceDetails{}, byIndex: map[uint32]*interfaces.SwInterfaceDetails{}}
	for _, d := range details {
		s.byName[d.InterfaceName] = d
		s.byIndex[uint32(d.SwIfIndex)] = d
	}
	return s, nil
}

// Index resolves an interface name; ErrNoSuchInterface when VPP has none of that name.
func (s *Interfaces) Index(name string) (interface_types.InterfaceIndex, error) {
	d, ok := s.byName[name]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrNoSuchInterface, name)
	}
	return d.SwIfIndex, nil
}

// OptionalIndex resolves an optional interface reference: "" is NoInterface.
func (s *Interfaces) OptionalIndex(name string) (interface_types.InterfaceIndex, error) {
	if name == "" {
		return interface_types.InterfaceIndex(NoInterface), nil
	}
	return s.Index(name)
}

// Name returns the name of sw_if_index idx.
func (s *Interfaces) Name(idx uint32) (string, bool) {
	d, ok := s.byIndex[idx]
	if !ok {
		return "", false
	}
	return d.InterfaceName, true
}

// NameOrEmpty returns the name of idx, or "" for NoInterface / unknown.
func (s *Interfaces) NameOrEmpty(idx uint32) string {
	if idx == NoInterface {
		return ""
	}
	n, _ := s.Name(idx)
	return n
}

// Details returns the dump record of idx.
func (s *Interfaces) Details(idx uint32) (*interfaces.SwInterfaceDetails, bool) {
	d, ok := s.byIndex[idx]
	return d, ok
}

// OwnedID returns the object id from the owner tag of idx (vpp.ParseOwnerTag), ok when the
// interface is tagged by this owner.
func (s *Interfaces) OwnedID(idx uint32) (string, bool) {
	d, ok := s.byIndex[idx]
	if !ok {
		return "", false
	}
	return vpp.ParseOwnerTag(d.Tag, s.owner)
}

// Owned reports whether the interface carries this owner's tag.
func (s *Interfaces) Owned(idx uint32) bool {
	_, ok := s.OwnedID(idx)
	return ok
}

// OwnedName returns the interface name when idx is an interface of this owner.
func (s *Interfaces) OwnedName(idx uint32) (string, bool) {
	if !s.Owned(idx) {
		return "", false
	}
	return s.Name(idx)
}

// TagInterface stamps sw_if_index idx with the owner tag of key (vpp.OwnerTag(owner,
// key.ID())). Every tunnel/session interface a DF-6 descriptor creates is tagged this way and
// Retrieve keeps only tagged interfaces.
func TagInterface(ctx context.Context, c vpp.Client, owner string, key scheduler.Key, idx interface_types.InterfaceIndex) error {
	tag, err := vpp.OwnerTag(owner, key.ID())
	if err != nil {
		return err
	}
	if _, err := interfaces.NewServiceClient(c).SwInterfaceTagAddDel(ctx, &interfaces.SwInterfaceTagAddDel{IsAdd: true, SwIfIndex: idx, Tag: tag}); err != nil {
		return fmt.Errorf("sw_interface_tag_add_del: %w", err)
	}
	return nil
}

// TagOrRollback tags idx and, when tagging fails, calls rollback (the descriptor's delete of
// the just-created object) so no untagged object leaks on the shared VPP.
func TagOrRollback(ctx context.Context, c vpp.Client, owner string, key scheduler.Key, idx interface_types.InterfaceIndex, rollback func() error) error {
	if err := TagInterface(ctx, c, owner, key, idx); err != nil {
		if rerr := rollback(); rerr != nil {
			return fmt.Errorf("%w (rollback failed: %v)", err, rerr)
		}
		return err
	}
	return nil
}
