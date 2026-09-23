package acl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.fd.io/govpp/api"

	dfiface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/vpp"
)

// ErrNoInterface is returned (wrapped) when a desired binding names an interface VPP does not have.
var ErrNoInterface = errors.New("acl: no such interface")

// allInterfaces is the sw_if_index wildcard of the dump requests (~0).
const allInterfaces = ^uint32(0)

// ifaceInfo is what the acl descriptors need from sw_interface_details.
type ifaceInfo struct {
	Index uint32
	Name  string // logical name (D-069): our tag id, an untagged interface's VPP name; VPP name otherwise
	Tag   string
}

// ifaceTable maps logical interface names to indexes and back for one Retrieve/Create call. It is
// built on DF-1's interface table so that every descriptor package resolves interface names the
// same way (D-069: "interface/<name>" carries the logical name).
type ifaceTable struct {
	t       *dfiface.Table
	byIndex map[uint32]ifaceInfo
	byName  map[string]ifaceInfo
}

// dumpInterfaces runs sw_interface_dump (all interfaces) once. Bindings are keyed by logical
// interface name (stable across restarts) while the plugin speaks sw_if_index, so every binding
// descriptor needs this table.
func dumpInterfaces(ctx context.Context, c vpp.Client, owner string) (*ifaceTable, error) {
	t, err := dfiface.Dump(ctx, c, owner)
	if err != nil {
		return nil, err
	}
	out := &ifaceTable{t: t, byIndex: map[uint32]ifaceInfo{}, byName: map[string]ifaceInfo{}}
	for _, idx := range t.Indexes() {
		d, _ := t.Details(idx)
		name, ok := t.Logical(idx)
		if !ok {
			name = t.VPPName(idx) // another owner's interface / local0: never resolvable, named for display
		}
		info := ifaceInfo{Index: idx, Name: name, Tag: strings.TrimRight(d.Tag, "\x00")}
		out.byIndex[idx] = info
		if ok {
			out.byName[name] = info
		}
	}
	return out, nil
}

// index resolves a logical interface name with DF-1's resolver (dfiface.Table.IndexByName); the
// error wraps ErrNoInterface, or ErrForeignInterface for an interface tagged by another owner.
func (t *ifaceTable) index(name string) (uint32, error) {
	idx, err := t.t.IndexByName(name)
	switch {
	case err == nil:
		return idx, nil
	case errors.Is(err, dfiface.ErrForeignInterface):
		return 0, fmt.Errorf("%w: %w", ErrForeignInterface, err)
	default:
		return 0, fmt.Errorf("%w: %q: %w", ErrNoInterface, name, err)
	}
}

// indexShared is index for ACL bindings: an ACL list on an interface is shared between owners
// (acl_interface_set_acl_list keeps other owners' ACLs, review finding 6 of DF-4), so another
// owner's interface may be named by its VPP name; ours and untagged ones resolve by logical name.
func (t *ifaceTable) indexShared(name string) (uint32, error) {
	idx, err := t.index(name)
	if errors.Is(err, ErrForeignInterface) {
		for _, i := range t.t.Indexes() {
			if t.t.VPPName(i) == name {
				return i, nil
			}
		}
	}
	return idx, err
}

// drain reads a generated dump stream until io.EOF (the control_ping_reply). On any other error
// the stream is closed and the error returned.
func drain[T any](stream api.Stream, recv func() (T, error)) ([]T, error) {
	var out []T
	for {
		d, err := recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			_ = stream.Close()
			return nil, err
		}
		out = append(out, d)
	}
}
