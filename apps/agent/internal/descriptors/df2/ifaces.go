package df2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp"
)

// NoInterface is VPP's "no interface" sw_if_index (~0).
const NoInterface = ^uint32(0)

func isEOF(err error) bool { return errors.Is(err, io.EOF) }

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

// Name returns the name of sw_if_index idx.
func (s *Interfaces) Name(idx uint32) (string, bool) {
	d, ok := s.byIndex[idx]
	if !ok {
		return "", false
	}
	return d.InterfaceName, true
}

// Owned reports whether the interface carries this owner's tag (vpp.OwnerTag). Objects that
// hang off an interface and have no tag of their own (neighbours, RA config, uRPF, …) are
// attributed to the owner of their interface.
func (s *Interfaces) Owned(idx uint32) bool {
	d, ok := s.byIndex[idx]
	if !ok {
		return false
	}
	_, owned := vpp.ParseOwnerTag(d.Tag, s.owner)
	return owned
}

// OwnedName returns the interface name when idx is an interface of this owner.
func (s *Interfaces) OwnedName(idx uint32) (string, bool) {
	if !s.Owned(idx) {
		return "", false
	}
	return s.Name(idx)
}

// OwnedIndices returns the sw_if_index of every interface tagged by this owner, ascending.
func (s *Interfaces) OwnedIndices() []uint32 {
	var out []uint32
	for idx := range s.byIndex {
		if s.Owned(idx) {
			out = append(out, idx)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
