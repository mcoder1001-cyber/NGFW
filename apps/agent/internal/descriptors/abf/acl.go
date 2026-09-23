// Package abf implements the descriptors of VPP's abf plugin (ACL-based forwarding): the
// policy (ACL → path list) and its attachment to interfaces. Messages come from
// apps/agent/binapi/abf (and binapi/acl for resolving ACL names) only.
package abf

import (
	"context"
	"fmt"

	"ngfw/agent/binapi/acl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/vpp"
)

// ACLs is one acl_dump snapshot restricted to this owner's ACLs. DF-4 stamps every ACL it
// creates with acl_add_replace.tag = vpp.OwnerTag(owner, <name>); the policy references the
// ACL by that name (key acl/<name>) and this snapshot turns it into the acl_index.
type ACLs struct {
	byName  map[string]uint32
	byIndex map[uint32]string
}

// DumpACLs dumps every ACL and keeps those tagged by owner.
func DumpACLs(ctx context.Context, c vpp.Client, owner string) (*ACLs, error) {
	stream, err := acl.NewServiceClient(c).ACLDump(ctx, &acl.ACLDump{ACLIndex: ^uint32(0)})
	if err != nil {
		return nil, fmt.Errorf("acl_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("acl_dump: %w", err)
	}
	s := &ACLs{byName: map[string]uint32{}, byIndex: map[uint32]string{}}
	for _, d := range details {
		name, owned := vpp.ParseOwnerTag(d.Tag, owner)
		if !owned {
			continue
		}
		s.byName[name] = d.ACLIndex
		s.byIndex[d.ACLIndex] = name
	}
	return s, nil
}

// Index resolves an ACL name.
func (s *ACLs) Index(name string) (uint32, error) {
	idx, ok := s.byName[name]
	if !ok {
		return 0, fmt.Errorf("acl %q: not found among this owner's ACLs", name)
	}
	return idx, nil
}

// Name returns the name of an owned ACL index.
func (s *ACLs) Name(idx uint32) (string, bool) {
	n, ok := s.byIndex[idx]
	return n, ok
}
