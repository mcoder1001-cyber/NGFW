// Package abf implements the descriptors of VPP's abf plugin (ACL-based forwarding): the
// policy (ACL → path list) and its attachment to interfaces. Messages come from
// apps/agent/binapi/abf (and binapi/acl for mapping ACL indices back to names) only.
package abf

import (
	"context"
	"fmt"
	"sort"

	aclapi "ngfw/agent/binapi/acl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/vpp"
)

// dupSeparator is DF-4's separator for non-canonical duplicates ("<name>#<index>", D-066).
const dupSeparator = "#"

// ACLs maps this owner's acl_index values to the names DF-4 reports for them (D-066): the
// ACL tag is vpp.OwnerTag(owner, <name>); when several ACLs carry one tag, the lowest index
// is canonical ("<name>") and every other one is "<name>#<index>", which DF-4 deletes. A
// policy on a non-canonical ACL therefore diffs against desired and is recreated on the
// canonical one. Create resolves names with DF-4's acl.LookupIndex (same rule).
type ACLs struct{ byIndex map[uint32]string }

// DumpACLs dumps every ACL and keeps those tagged by owner.
func DumpACLs(ctx context.Context, c vpp.Client, owner string) (*ACLs, error) {
	stream, err := aclapi.NewServiceClient(c).ACLDump(ctx, &aclapi.ACLDump{ACLIndex: ^uint32(0)})
	if err != nil {
		return nil, fmt.Errorf("acl_dump: %w", err)
	}
	details, err := df2.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("acl_dump: %w", err)
	}
	type owned struct {
		index uint32
		name  string
	}
	var list []owned
	for _, d := range details {
		if name, ok := vpp.ParseOwnerTag(d.Tag, owner); ok {
			list = append(list, owned{d.ACLIndex, name})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].index < list[j].index })
	s := &ACLs{byIndex: map[uint32]string{}}
	seen := map[string]bool{}
	for _, o := range list {
		if seen[o.name] {
			s.byIndex[o.index] = fmt.Sprintf("%s%s%d", o.name, dupSeparator, o.index)
			continue
		}
		seen[o.name] = true
		s.byIndex[o.index] = o.name
	}
	return s, nil
}

// Name returns the reported name of an owned ACL index.
func (s *ACLs) Name(idx uint32) (string, bool) {
	n, ok := s.byIndex[idx]
	return n, ok
}
