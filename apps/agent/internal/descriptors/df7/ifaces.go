package df7

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/vpp"
)

// NoIndex is VPP's "no interface / all" index (~0).
const NoIndex = ^uint32(0)

// local0 is never anybody's.
const local0 = "local0"

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

// Iface is what DF-7 needs from sw_interface_details.
type Iface struct {
	Index uint32
	Name  string
	Tag   string
}

// Interfaces is one sw_interface_dump snapshot with owner attribution. Take one per
// Create/Retrieve and resolve every interface reference from it.
type Interfaces struct {
	owner   string
	claim   bool
	byName  map[string]Iface
	byIndex map[uint32]Iface
}

// DumpInterfaces dumps every interface of VPP.
func DumpInterfaces(ctx context.Context, c vpp.Client, owner string, o Options) (*Interfaces, error) {
	stream, err := interfaces.NewServiceClient(c).SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{SwIfIndex: interface_types.InterfaceIndex(NoIndex)})
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	details, err := Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("sw_interface_dump: %w", err)
	}
	s := &Interfaces{owner: owner, claim: o.ClaimUntagged, byName: map[string]Iface{}, byIndex: map[uint32]Iface{}}
	for _, d := range details {
		i := Iface{
			Index: uint32(d.SwIfIndex),
			Name:  strings.TrimRight(d.InterfaceName, "\x00"),
			Tag:   strings.TrimRight(d.Tag, "\x00"),
		}
		s.byName[i.Name] = i
		s.byIndex[i.Index] = i
	}
	return s, nil
}

// Lookup returns the interface called name.
func (s *Interfaces) Lookup(name string) (Iface, bool) {
	i, ok := s.byName[name]
	return i, ok
}

// Index resolves an interface name (ErrNoSuchInterface when absent).
func (s *Interfaces) Index(name string) (uint32, error) {
	i, ok := s.byName[name]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrNoSuchInterface, name)
	}
	return i.Index, nil
}

// OwnedIndex resolves an interface name for an object this owner is about to put on it: the
// interface must exist and must not carry another owner's tag (ErrForeignInterface); an
// untagged interface is accepted only with ClaimUntagged, because Retrieve could otherwise
// never report the object back.
func (s *Interfaces) OwnedIndex(name string) (uint32, error) {
	i, ok := s.byName[name]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrNoSuchInterface, name)
	}
	if !s.owned(i) {
		return 0, fmt.Errorf("%w: %q is tagged %q (owner %q, claim-untagged %v)", ErrForeignInterface, name, i.Tag, s.owner, s.claim)
	}
	return i.Index, nil
}

// Name returns the name of sw_if_index idx.
func (s *Interfaces) Name(idx uint32) (string, bool) {
	i, ok := s.byIndex[idx]
	return i.Name, ok
}

// NameOrIndex returns the name of idx, or "#<idx>" for an unknown index so a diff shows the
// problem instead of silently matching.
func (s *Interfaces) NameOrIndex(idx uint32) string {
	if n, ok := s.Name(idx); ok {
		return n
	}
	return fmt.Sprintf("#%d", idx)
}

func (s *Interfaces) owned(i Iface) bool {
	if i.Name == local0 {
		return false
	}
	if _, ok := vpp.ParseOwnerTag(i.Tag, s.owner); ok {
		return true
	}
	return s.claim && i.Tag == ""
}

// Owned reports whether objects on sw_if_index idx belong to this owner.
func (s *Interfaces) Owned(idx uint32) bool {
	i, ok := s.byIndex[idx]
	return ok && s.owned(i)
}

// OwnedName returns the interface name when idx is owned.
func (s *Interfaces) OwnedName(idx uint32) (string, bool) {
	if !s.Owned(idx) {
		return "", false
	}
	return s.Name(idx)
}

// OwnedIndices returns every owned sw_if_index, ascending.
func (s *Interfaces) OwnedIndices() []uint32 {
	var out []uint32
	for idx, i := range s.byIndex {
		if s.owned(i) {
			out = append(out, idx)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a] < out[b] })
	return out
}
