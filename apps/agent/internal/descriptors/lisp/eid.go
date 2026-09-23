// Package lisp holds the LISP control-plane and LISP-GPE descriptors (DF-6, WBS D6.8, the
// minimal set: enable, locator sets and locators, local EIDs, map resolvers/servers, remote
// mappings, adjacencies, EID-table maps, PITR, GPE enable and GPE forwarding entries).
//
// LISP objects carry no owner tag: an object is ours only through the claim written by our
// own Create (D-071, df6.KeyedDescriptor); the global switches are the globals owner's.
package lisp

import (
	"context"
	"fmt"
	"strings"

	"ngfw/agent/binapi/ethernet_types"
	lispapi "ngfw/agent/binapi/lisp"
	"ngfw/agent/binapi/lisp_types"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Plugin names the VPP plugin in ErrPluginNotLoaded.
const Plugin = "lisp"

// MaxNameLen is the usable size of VPP's string[64] locator-set names.
const MaxNameLen = 63

// canonEID canonicalises an EID string: a masked IP prefix or a lower-case MAC.
func canonEID(s string) (string, error) {
	if strings.Contains(s, "/") {
		return df6.CanonicalPrefix(s)
	}
	if m, err := df6.CanonicalMAC(s); err == nil {
		return m, nil
	}
	return "", fmt.Errorf("%w: eid %q is neither an IP prefix nor a MAC", df6.ErrBadValue, s)
}

// isMAC reports whether a canonical EID is a MAC.
func isMAC(eid string) bool { return !strings.Contains(eid, "/") }

// eidOf converts a canonical EID string to the API type.
func eidOf(s string) (lisp_types.Eid, error) {
	if s == "" {
		return lisp_types.Eid{}, nil
	}
	if isMAC(s) {
		m, err := df6.ParseMAC(s)
		if err != nil {
			return lisp_types.Eid{}, err
		}
		return lisp_types.Eid{Type: lisp_types.EID_TYPE_API_MAC, Address: lisp_types.EidAddressUnionMac(m)}, nil
	}
	p, err := df6.PrefixOf(s)
	if err != nil {
		return lisp_types.Eid{}, err
	}
	return lisp_types.Eid{Type: lisp_types.EID_TYPE_API_PREFIX, Address: lisp_types.EidAddressUnionPrefix(p)}, nil
}

// eidString decodes an API EID ("" for NSH or unknown types).
func eidString(e lisp_types.Eid) string {
	switch e.Type {
	case lisp_types.EID_TYPE_API_PREFIX:
		return df6.PrefixString(e.Address.GetPrefix())
	case lisp_types.EID_TYPE_API_MAC:
		m := e.Address.GetMac()
		if m == (ethernet_types.MacAddress{}) {
			return ""
		}
		return df6.MACString(m)
	}
	return ""
}

func checkName(n string) error {
	if n == "" || len(n) > MaxNameLen {
		return fmt.Errorf("%w: locator set name must be 1–%d bytes", df6.ErrBadValue, MaxNameLen)
	}
	return nil
}

func u8(v uint32, what string) (uint8, error) {
	if v > 255 {
		return 0, fmt.Errorf("%w: %s %d > 255", df6.ErrBadValue, what, v)
	}
	return uint8(v), nil
}

// Keys of objects other LISP objects depend on.

// LocatorSetKey is the key of locator set name.
func LocatorSetKey(name string) scheduler.Key { return scheduler.Join(LocatorSetName, name) }

// EnableKey / GpeEnableKey are the keys of the global switches.
var (
	EnableKey    = scheduler.Join(EnableName, df6.SingletonID)
	GpeEnableKey = scheduler.Join(GpeEnableName, df6.SingletonID)
)

// locatorSets returns the local locator sets: index → name.
func locatorSets(ctx context.Context, c vpp.Client) (map[uint32]string, error) {
	stream, err := lispapi.NewServiceClient(c).LispLocatorSetDump(ctx, &lispapi.LispLocatorSetDump{Filter: lispapi.LISP_LOCATOR_SET_FILTER_API_LOCAL})
	if err != nil {
		return nil, fmt.Errorf("lisp_locator_set_dump: %w", err)
	}
	recs, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("lisp_locator_set_dump: %w", err)
	}
	out := make(map[uint32]string, len(recs))
	for _, r := range recs {
		out[r.LsIndex] = r.LsName
	}
	return out, nil
}

// locatorsOf dumps the locators of a locator set by index.
func locatorsOf(ctx context.Context, c vpp.Client, index uint32) ([]*lispapi.LispLocatorDetails, error) {
	stream, err := lispapi.NewServiceClient(c).LispLocatorDump(ctx, &lispapi.LispLocatorDump{LsIndex: index, IsIndexSet: 1})
	if err != nil {
		return nil, fmt.Errorf("lisp_locator_dump: %w", err)
	}
	recs, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("lisp_locator_dump: %w", err)
	}
	return recs, nil
}

// eidTable dumps the local or remote EID table.
func eidTable(ctx context.Context, c vpp.Client, filter lispapi.LispLocatorSetFilter) ([]*lispapi.LispEidTableDetails, error) {
	stream, err := lispapi.NewServiceClient(c).LispEidTableDump(ctx, &lispapi.LispEidTableDump{Filter: filter})
	if err != nil {
		return nil, fmt.Errorf("lisp_eid_table_dump: %w", err)
	}
	recs, err := df6.Collect(stream.Recv)
	if err != nil {
		return nil, fmt.Errorf("lisp_eid_table_dump: %w", err)
	}
	return recs, nil
}

// status reads show_lisp_status.
func status(ctx context.Context, c vpp.Client) (lispOn, gpeOn bool, err error) {
	rep, err := lispapi.NewServiceClient(c).ShowLispStatus(ctx, &lispapi.ShowLispStatus{})
	if err != nil {
		return false, false, fmt.Errorf("show_lisp_status: %w", err)
	}
	return rep.IsLispEnabled, rep.IsGpeEnabled, nil
}

// Status reports whether LISP and LISP-GPE are enabled (tests read it before touching the
// globals on a shared VPP).
func Status(ctx context.Context, c vpp.Client) (lispOn, gpeOn bool, err error) {
	return status(ctx, c)
}
